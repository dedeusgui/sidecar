package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/store"
)

// contractRepo indexes a small repository and returns its index and root.
func contractRepo(t *testing.T, files map[string]string) (*index.Index, string) {
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

// definition returns the indexed definition with the given kind and name.
func definition(t *testing.T, ix *index.Index, kind, name string) store.SymbolHit {
	t.Helper()
	all, err := ix.Store().AllSymbols()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range all {
		if h.Kind == kind && h.Name == name {
			return h
		}
	}
	t.Fatalf("no %s %s indexed", kind, name)
	return store.SymbolHit{}
}

// findingsFor runs the contract check for one definition.
func findingsFor(t *testing.T, ix *index.Index, root string, h store.SymbolHit) []Finding {
	t.Helper()
	fs, err := ContractEvidence(ix.Store(), root, []store.SymbolHit{h})
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func conflictsOf(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Status == ConflictCandidate {
			out = append(out, f)
		}
	}
	return out
}

func hasFinding(fs []Finding, status ContractStatus, source, path string, line int) bool {
	for _, f := range fs {
		if f.Status == status && f.Source == source && f.Path == path && f.Line == line {
			return true
		}
	}
	return false
}

// constantRepo is a constant with a literal value and a doc, test and docstring that can agree or disagree.
func constantRepo(docValue, testValue string) map[string]string {
	files := map[string]string{
		"queue/limits.go":      "package queue\n\n// MaxSlots limits how many trucks may wait at the same time.\nconst MaxSlots = 3\n\n// Gate admits a truck while a slot is free.\nfunc Gate(n int) bool { return n < MaxSlots }\n",
		"queue/limits_test.go": "package queue\n\nimport \"testing\"\n\nfunc TestMaxSlotsIsThree(t *testing.T) {\n\tassertEqual(t, " + testValue + ", MaxSlots)\n}\n",
		"docs/operacao.md":     "# Operação\n\n## Limites\n\n`MaxSlots` = " + docValue + " caminhões na fila.\n",
	}
	return files
}

func TestAlignedCodeDocAndTestAreNotConflicts(t *testing.T) {
	ix, root := contractRepo(t, constantRepo("3", "3"))
	fs := findingsFor(t, ix, root, definition(t, ix, "constant", "MaxSlots"))
	if len(conflictsOf(fs)) != 0 {
		t.Fatalf("aligned sources reported a conflict: %+v", fs)
	}
	if !hasFinding(fs, Documented, "doc", "docs/operacao.md", 5) {
		t.Errorf("doc agreeing with the value is not DOCUMENTED: %+v", fs)
	}
	if !hasFinding(fs, TestExpected, "test", "queue/limits_test.go", 6) {
		t.Errorf("test asserting the value is not TEST_EXPECTED: %+v", fs)
	}
}

func TestDocValueDivergenceIsConflictWithBothSides(t *testing.T) {
	ix, root := contractRepo(t, constantRepo("4", "3"))
	fs := findingsFor(t, ix, root, definition(t, ix, "constant", "MaxSlots"))
	cs := conflictsOf(fs)
	if len(cs) != 1 {
		t.Fatalf("conflicts = %d, want 1: %+v", len(cs), fs)
	}
	if cs[0].Path != "docs/operacao.md" || cs[0].Line != 5 || cs[0].Claim != "4" {
		t.Errorf("conflict located at %s:%d claiming %q, want docs/operacao.md:5 claiming 4", cs[0].Path, cs[0].Line, cs[0].Claim)
	}

	note := conflictNote(cs[0])
	for _, want := range []string{conflictHead, "constant MaxSlots = 3 (queue/limits.go:4)", "doc: docs/operacao.md:5", "diz 4"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q lacks %q", note, want)
		}
	}
}

func TestTestAssertionDivergenceIsConflict(t *testing.T) {
	ix, root := contractRepo(t, constantRepo("3", "5"))
	fs := findingsFor(t, ix, root, definition(t, ix, "constant", "MaxSlots"))
	cs := conflictsOf(fs)
	if len(cs) != 1 || cs[0].Source != "test" || cs[0].Path != "queue/limits_test.go" || cs[0].Line != 6 {
		t.Fatalf("want one test conflict at queue/limits_test.go:6, got %+v", fs)
	}
	if note := conflictNote(cs[0]); !strings.Contains(note, "teste: queue/limits_test.go:6 diz 5") {
		t.Errorf("note %q does not name the test side", note)
	}
}

func TestDocstringDivergenceIsConflict(t *testing.T) {
	files := constantRepo("3", "3")
	files["queue/gate.go"] = "package queue\n\n// Admit rejects a truck once MaxSlots = 4 is reached.\nfunc Admit(n int) bool { return n < 4 }\n"
	ix, root := contractRepo(t, files)
	fs := findingsFor(t, ix, root, definition(t, ix, "constant", "MaxSlots"))
	cs := conflictsOf(fs)
	if len(cs) != 1 || cs[0].Source != "docstring" || cs[0].Path != "queue/gate.go" || cs[0].Claim != "4" {
		t.Fatalf("want one docstring conflict in queue/gate.go claiming 4, got %+v", fs)
	}
}

func TestRouteMethodDivergenceIsConflict(t *testing.T) {
	ix, root := contractRepo(t, map[string]string{
		"api/routes.ts": "router.get('/api/trucks', listTrucks);\n",
		"docs/api.md":   "# API\n\nUse `POST /api/trucks` para cadastrar caminhões.\n",
	})
	fs := findingsFor(t, ix, root, definition(t, ix, "route", "GET /api/trucks"))
	cs := conflictsOf(fs)
	if len(cs) != 1 || cs[0].Path != "docs/api.md" || cs[0].Line != 3 || cs[0].Claim != "POST /api/trucks" {
		t.Fatalf("want a route conflict at docs/api.md:3 claiming POST /api/trucks, got %+v", fs)
	}
}

// Ambiguous text is not a conflict: the number may belong to another definition, or the
// wording is a range or a comparison. Such a mention is at most UNVERIFIED.
func TestAmbiguousMentionsDoNotConflict(t *testing.T) {
	ix, root := contractRepo(t, map[string]string{
		"queue/limits.go": "package queue\n\n// MaxSlots limits how many trucks may wait at the same time.\nconst MaxSlots = 3\n",
		"docs/range.md":   "# R\n\n`MaxSlots` pode ser 3 ou 4 caminhões.\n",
		"docs/bound.md":   "# B\n\n`MaxSlots` < 4 caminhões.\n",
		"docs/pair.md":    "# P\n\n`MaxSlots` e `MinSlots` = 4 caminhões.\n",
		"docs/daily.md":   "# D\n\nLimite de 4 por dia; `MaxSlots` = 3.\n",
	})
	fs := findingsFor(t, ix, root, definition(t, ix, "constant", "MaxSlots"))
	if cs := conflictsOf(fs); len(cs) != 0 {
		t.Fatalf("ambiguous mentions produced false conflicts: %+v", cs)
	}
	for _, f := range fs {
		if f.Source == "doc" && f.Status != Unverified {
			t.Errorf("ambiguous doc line classified %s, want UNVERIFIED: %+v", f.Status, f)
		}
	}
}

// A conflict reaches the context block as a concern, and ablating the contract layer removes it.
func TestConflictReachesConcernsOnlyWithContractLayer(t *testing.T) {
	ix, root := contractRepo(t, constantRepo("4", "3"))
	count := func(o Options) (concerns int, note string) {
		res, err := Search(context.Background(), ix.Store(), "MaxSlots", o)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.Evidence {
			if e.Section == ctxbuild.Concerns {
				concerns++
				note = e.Note
			}
		}
		return concerns, note
	}
	n, note := count(Options{EngineVersion: "v2", Root: root})
	if n != 1 || !strings.HasPrefix(note, conflictHead) {
		t.Fatalf("v2 concerns = %d, note %q; want one CONFLICT_CANDIDATE note", n, note)
	}
	if n, _ := count(Options{EngineVersion: "v2", Root: root, Ablate: Ablation{NoContract: true}}); n != 0 {
		t.Errorf("NoContract still produced %d concerns", n)
	}
	if n, _ := count(Options{Root: root}); n != 0 {
		t.Errorf("v1 produced %d concerns; the v1 pipeline must not change", n)
	}
}

// signatureRepo documents a Go function whose call in the doc is given by call.
func signatureRepo(call string) map[string]string {
	return map[string]string{
		"svc/orders.go": "package svc\n\n// ProcessOrder handles one order.\nfunc ProcessOrder(ctx context.Context, id string, retry int) error {\n\treturn nil\n}\n",
		"docs/api.md":   "# API\n\n## Processamento\n\nUse `" + call + "` para processar.\n",
	}
}

func TestDocumentedCallWithMatchingArityIsDocumented(t *testing.T) {
	ix, root := contractRepo(t, signatureRepo("ProcessOrder(ctx, id, retry)"))
	fs := findingsFor(t, ix, root, definition(t, ix, "function", "ProcessOrder"))
	if len(conflictsOf(fs)) != 0 {
		t.Fatalf("matching arity reported a conflict: %+v", fs)
	}
	if !hasFinding(fs, Documented, "doc", "docs/api.md", 5) {
		t.Errorf("matching call is not DOCUMENTED: %+v", fs)
	}
}

func TestDocumentedCallWithDifferentArityIsConflictWithBothSides(t *testing.T) {
	ix, root := contractRepo(t, signatureRepo("ProcessOrder(ctx, id)"))
	fs := findingsFor(t, ix, root, definition(t, ix, "function", "ProcessOrder"))
	cs := conflictsOf(fs)
	if len(cs) != 1 || cs[0].Path != "docs/api.md" || cs[0].Line != 5 || cs[0].Claim != "ProcessOrder(ctx, id)" {
		t.Fatalf("want one arity conflict at docs/api.md:5 claiming ProcessOrder(ctx, id), got %+v", fs)
	}
	note := conflictNote(cs[0])
	for _, want := range []string{"function ProcessOrder (svc/orders.go:4)", "doc: docs/api.md:5", "diz ProcessOrder(ctx, id)"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q lacks %q", note, want)
		}
	}
}

func TestVariadicSignatureIsNotCompared(t *testing.T) {
	ix, root := contractRepo(t, map[string]string{
		"svc/log.go":  "package svc\n\nfunc Log(ctx context.Context, args ...string) {}\n",
		"docs/api.md": "# API\n\n## Log\n\nUse `Log(ctx)` ou `Log(ctx, a, b)`.\n",
	})
	fs := findingsFor(t, ix, root, definition(t, ix, "function", "Log"))
	if len(conflictsOf(fs)) != 0 {
		t.Fatalf("variadic signature was compared: %+v", fs)
	}
}

// C writes an empty parameter list as (void); a documented call with no arguments matches it.
// C files are not indexed, so the helper is exercised with the symbol the C grammar would give.
func TestCVoidParameterListMatchesNoArgumentCall(t *testing.T) {
	sym := store.SymbolHit{Kind: "function", Name: "start", Signature: "int start(void) {", Path: "app/main.c", Line: 1}
	if st, claim := arityMention("Chame `start()` para iniciar.", sym, false); st != Documented || claim != "start()" {
		t.Fatalf("call with no arguments vs (void) = %s %q, want DOCUMENTED start()", st, claim)
	}
	if st, _ := arityMention("Chame `start(1)` para iniciar.", sym, false); st != ConflictCandidate {
		t.Fatalf("call with one argument vs (void) = %s, want CONFLICT_CANDIDATE", st)
	}
}
