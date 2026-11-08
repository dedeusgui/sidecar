package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/impact"
)

func TestChangeCheckNoteIsBoundedAndKeepsConflictsFirst(t *testing.T) {
	long := strings.Repeat("x", 120)
	rep := impact.Report{Changed: []string{"a.go"}, Findings: []impact.Finding{
		{Kind: "contract", Status: impact.Conflict, Text: "docs/x.md states MaxRetries = 5 but the code disagrees"},
		{Kind: "consumer", Status: impact.Confirmed, Text: long}, {Kind: "consumer", Status: impact.Candidate, Text: long},
		{Kind: "test", Status: impact.Confirmed, Text: "test file t_test.go references X and was not changed"},
	}}
	text, _ := changeCheckNote(rep)
	if n := len([]rune(text)); n > 296 {
		t.Fatalf("note has %d runes", n)
	}
	if !strings.Contains(text, "[CONFLICT]") {
		t.Fatalf("the conflict must survive trimming: %s", text)
	}
	if note, _ := changeCheckNote(impact.Report{Findings: []impact.Finding{{Kind: "doc", Status: impact.Confirmed, Text: "d"}}}); note != "" {
		t.Fatalf("nothing the agent is likely to miss must give no note: %q", note)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestHookImpactNoteAppearsOnlyWithFeatureAndUncommittedEdits(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/shop\n\ngo 1.22\n")
	write("billing/charge.go", "package billing\n\n// ChargeInvoice charges once.\nfunc ChargeInvoice(id string) error { return nil }\n")
	write("api/quote.go", "package api\n\nimport \"example.com/shop/billing\"\n\n// Quote charges the invoice.\nfunc Quote(id string) error { return billing.ChargeInvoice(id) }\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	t.Setenv("SIDECAR_ENGINE", "v2")
	var spawned []string
	prompt := "como o ChargeInvoice trata erros?"

	t.Setenv("SIDECAR_FEATURES", "impact")
	out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "i1"), &spawned)
	if strings.Contains(out, "Change check") {
		t.Fatalf("a clean worktree has no change to check: %q", out)
	}
	write("billing/charge.go", "package billing\n\n// ChargeInvoice charges once.\nfunc ChargeInvoice(id string, cents int) error { return nil }\n")
	out, log := runHook(t, adapter.Claude, payload(t, root, prompt, "i2"), &spawned)
	if !strings.Contains(out, "Change check") || !strings.Contains(out, "api/quote.go") {
		t.Fatalf("an unchanged consumer of the edited file must be flagged: %q\\nlog: %s", out, log)
	}
	t.Setenv("SIDECAR_FEATURES", "")
	out, _ = runHook(t, adapter.Claude, payload(t, root, prompt, "i3"), &spawned)
	if strings.Contains(out, "Change check") {
		t.Fatalf("impact is opt-in: %q", out)
	}
}
