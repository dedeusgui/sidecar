package impact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
)

func fixture(t *testing.T, files map[string]string) (*index.Index, string) {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	return ix, root
}

var repoFiles = map[string]string{
	"go.mod":                 "module example.com/shop\n\ngo 1.22\n",
	"billing/charge.go":      "package billing\n\n// MaxRetries is how many times a charge is retried.\nconst MaxRetries = 3\n\n// ChargeInvoice charges once.\nfunc ChargeInvoice(id string) error { return nil }\n\n// Refund returns money.\nfunc Refund(id string) error { return nil }\n",
	"billing/charge_test.go": "package billing\n\nimport \"testing\"\n\nfunc TestChargeInvoice(t *testing.T) { _ = ChargeInvoice(\"x\") }\n",
	"api/quote.go":           "package api\n\nimport \"example.com/shop/billing\"\n\n// Quote charges the invoice.\nfunc Quote(id string) error { return billing.ChargeInvoice(id) }\n",
	"docs/billing.md":        "# Billing\n\nA cobrança tenta `MaxRetries` = 5 vezes.\n",
}

func byKind(r Report, kind string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Kind == kind {
			out = append(out, f)
		}
	}
	return out
}

func TestAnalyzeFlagsUnchangedConsumerTestDocAndConflict(t *testing.T) {
	ix, root := fixture(t, repoFiles)
	rep, err := Analyze(ix.Store(), root, []string{"billing/charge.go"})
	if err != nil {
		t.Fatal(err)
	}
	var consumer bool
	for _, f := range append(byKind(rep, "consumer"), byKind(rep, "importer")...) {
		if f.Path == "api/quote.go" {
			consumer = true
		}
	}
	if !consumer {
		t.Fatalf("api/quote.go calls and imports the changed file but was not flagged: %+v", rep.Findings)
	}
	tests := byKind(rep, "test")
	if len(tests) == 0 || tests[0].Path != "billing/charge_test.go" || tests[0].Status != Confirmed || !strings.Contains(tests[0].Text, "was not changed") {
		t.Fatalf("test finding: %+v", tests)
	}
	conf := byKind(rep, "contract")
	if len(conf) != 1 || conf[0].Status != Conflict || conf[0].Path != "docs/billing.md" {
		t.Fatalf("a doc that states MaxRetries = 5 against the code's 3 must be a CONFLICT: %+v", conf)
	}
	if v := byKind(rep, "verify"); len(v) != 1 || !strings.Contains(v[0].Text, "go test ./billing") {
		t.Fatalf("verify: %+v", v)
	}
	for _, p := range []string{"billing/charge.go", "api/quote.go", "billing/charge_test.go", "docs/billing.md"} {
		found := false
		for _, s := range rep.Surface {
			found = found || s == p
		}
		if !found {
			t.Errorf("surface misses %s: %v", p, rep.Surface)
		}
	}
}

func TestAnalyzeDoesNotFlagWhatTheChangeAlreadyCovers(t *testing.T) {
	ix, root := fixture(t, repoFiles)
	rep, err := Analyze(ix.Store(), root, []string{"billing/charge.go", "api/quote.go", "billing/charge_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if (f.Kind == "consumer" || f.Kind == "importer") && f.Path == "api/quote.go" {
			t.Fatalf("a consumer already in the change must not be flagged: %+v", f)
		}
		if f.Kind == "test" && strings.Contains(f.Text, "was not changed") {
			t.Fatalf("a changed test must not be reported as unchanged: %+v", f)
		}
	}
}

func TestAnalyzeStatesTestGapForExportedSymbolWithoutReferencingTest(t *testing.T) {
	ix, root := fixture(t, repoFiles)
	rep, _ := Analyze(ix.Store(), root, []string{"billing/charge.go"})
	var refundGap bool
	for _, f := range byKind(rep, "gap") {
		refundGap = refundGap || (f.Symbol == "Refund" && f.Status == Unknown)
	}
	if !refundGap {
		t.Fatalf("Refund has no referencing test; the gap must be explicit: %+v", rep.Findings)
	}
}

func TestAnalyzeIsBoundedAndSkipsNonCodeAndUnindexed(t *testing.T) {
	ix, root := fixture(t, repoFiles)
	rep, err := Analyze(ix.Store(), root, []string{"docs/billing.md", "missing/ghost.go", "./billing/../billing/charge.go"})
	if err != nil {
		t.Fatal(err)
	}
	skipped := strings.Join(rep.Skipped, ";")
	if !strings.Contains(skipped, "docs/billing.md (not code)") || !strings.Contains(skipped, "missing/ghost.go (no symbols indexed)") {
		t.Fatalf("skipped: %v", rep.Skipped)
	}
	if len(rep.Findings) == 0 {
		t.Fatal("the cleaned path billing/charge.go must still be analysed")
	}
	if len(rep.Findings) > maxFindings {
		t.Fatalf("unbounded findings: %d", len(rep.Findings))
	}
}

func TestAnalyzeDeterministic(t *testing.T) {
	ix, root := fixture(t, repoFiles)
	a, _ := Analyze(ix.Store(), root, []string{"billing/charge.go"})
	b, _ := Analyze(ix.Store(), root, []string{"billing/charge.go"})
	if len(a.Findings) != len(b.Findings) {
		t.Fatal("different finding counts")
	}
	for i := range a.Findings {
		if a.Findings[i] != b.Findings[i] {
			t.Fatalf("finding %d differs", i)
		}
	}
}
