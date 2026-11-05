package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func sampleResult() retrieve.Result {
	return retrieve.Result{
		Evidence: []retrieve.Evidence{
			{
				Hit: store.SymbolHit{ID: 1, Path: "queue/ticket.go", Name: "MaxActiveTickets", Kind: "constant",
					Value: "1", Line: 2, EndLine: 2, FileHash: "aaa111"},
				Verdict: "VERIFIED", Section: "EXISTING CONTRACTS",
			},
			{
				Hit: store.SymbolHit{ID: 2, Path: "release/handler.go", Name: "Release", Kind: "method",
					Signature: "func (h *Handler) Release(id string) error", Line: 9, EndLine: 12, FileHash: "bbb222"},
				Verdict: "INFERRED", Section: "RELEVANT CODE",
				Relations: []string{"calls:Enqueue(same_file)"}, Dirty: true,
			},
		},
		Trace: retrieve.Trace{Budget: 800},
	}
}

func TestRenderContainsLocationsVerdictsAndRelations(t *testing.T) {
	out := Render(sampleResult(), nil)
	for _, want := range []string{
		"queue/ticket.go:2-2", "MaxActiveTickets", "= 1", "[VERIFIED]",
		"release/handler.go:9-12", "[INFERRED]", "calls:Enqueue(same_file)",
		"EXISTING CONTRACTS", "RELEVANT CODE", "GIT EVIDENCE", "uncommitted changes in release/handler.go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderIsEmptyWithoutEvidence(t *testing.T) {
	if out := Render(retrieve.Result{}, nil); out != "" {
		t.Fatalf("no evidence rendered %q", out)
	}
}

func TestSessionSuppressesRepeatedEvidenceUntilItChanges(t *testing.T) {
	state := filepath.Join(t.TempDir(), "session.json")
	res := sampleResult()
	first := RenderForSession(res, state)
	if first == "" {
		t.Fatal("first prompt got no context")
	}
	if again := RenderForSession(res, state); again != "" {
		t.Fatalf("unchanged evidence re-injected in the same session:\n%s", again)
	}
	res.Evidence[0].Hit.FileHash = "changed"
	changed := RenderForSession(res, state)
	if !strings.Contains(changed, "MaxActiveTickets") || strings.Contains(changed, "release/handler.go:9-12") {
		t.Fatalf("only the changed item should be re-injected:\n%s", changed)
	}
}

func TestSessionReinjectsAfterTTL(t *testing.T) {
	state := filepath.Join(t.TempDir(), "session.json")
	res := sampleResult()
	RenderForSession(res, state)
	for i := 0; i < TTLTurns; i++ {
		RenderForSession(retrieve.Result{}, state) // turns without evidence still advance the session
	}
	if out := RenderForSession(res, state); out == "" {
		t.Fatal("evidence was never re-injected after the TTL, context may be lost after compaction")
	}
}

func TestCorruptSessionStateFailsOpen(t *testing.T) {
	state := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(state, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := RenderForSession(sampleResult(), state); out == "" {
		t.Fatal("corrupt session state must not suppress context")
	}
}

func TestEmptySessionPathDisablesDeduplication(t *testing.T) {
	res := sampleResult()
	if RenderForSession(res, "") == "" || RenderForSession(res, "") == "" {
		t.Fatal("without a session id every prompt should receive context")
	}
}
