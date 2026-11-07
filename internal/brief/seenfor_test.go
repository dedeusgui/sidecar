package brief

import (
	"path/filepath"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// SeenFor must agree with what RenderSession suppresses on the next prompt, or the packer and
// the renderer would disagree about what the session already holds.
func TestSeenForMatchesWhatRenderSessionSuppresses(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	ev := retrieve.Evidence{
		Hit:     store.SymbolHit{Kind: "function", Name: "Release", Path: "a.go", Line: 1, EndLine: 2, FileHash: "h1"},
		Verdict: "VERIFIED", Section: ctxbuild.Code,
	}
	res := retrieve.Result{Evidence: []retrieve.Evidence{ev}, Trace: retrieve.Trace{Budget: 800}}

	if SeenFor(statePath)(ev.Key(), ev.Fingerprint()) {
		t.Fatal("nothing was delivered yet, but SeenFor reports the item as seen")
	}
	if text, _ := RenderSession(res, statePath); text == "" {
		t.Fatal("first render sent nothing")
	}
	seen := SeenFor(statePath)
	if !seen(ev.Key(), ev.Fingerprint()) {
		t.Error("delivered item with the same fingerprint is not reported as seen")
	}
	if seen(ev.Key(), "changed") {
		t.Error("a changed item is reported as seen; it must be sent again")
	}
	if SeenFor("") != nil {
		t.Error("an empty state path must mean no session memory")
	}
}
