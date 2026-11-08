package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/store"
)

// dispatchSource is one of two identical functions, so both match the prompt equally and the
// order between them comes only from feedback.
func dispatchSource(pkg string) string {
	return "package " + pkg + "\n\n// Dispatch sends the ticket to the queue.\nfunc Dispatch(ticket string) string { return ticket }\n"
}

// dispatchFixture indexes two copies of Dispatch in two files of a fresh repository.
func dispatchFixture(t *testing.T) (string, *index.Index) {
	t.Helper()
	root := t.TempDir()
	for _, pkg := range []string{"alpha", "beta"} {
		p := filepath.Join(root, pkg, "queue.go")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(dispatchSource(pkg)), 0o644); err != nil {
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
	return root, ix
}

// dispatchPaths lists the file of each Dispatch hit in ranked order.
func dispatchPaths(t *testing.T, st *store.Store, root string, prompt string, noFeedback bool) []string {
	t.Helper()
	res, err := Search(context.Background(), st, prompt, Options{EngineVersion: "v2", Root: root, Ablate: Ablation{NoFeedback: noFeedback}})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ev := range res.Evidence {
		if ev.Hit.Name == "Dispatch" {
			out = append(out, ev.Hit.Path)
		}
	}
	return out
}

// recordDispatchFeedback stores feedback that the tests treat as strong, so it applies to the ranking.
func recordDispatchFeedback(t *testing.T, st *store.Store, root, path string, polarity int, weight float64) {
	t.Helper()
	recordDispatchFeedbackAs(t, st, root, path, polarity, weight, store.TierStrong)
}

// recordDispatchFeedbackAs stores one feedback record of the given tier on Dispatch in path.
func recordDispatchFeedbackAs(t *testing.T, st *store.Store, root, path string, polarity int, weight float64, tier string) {
	t.Helper()
	files, err := st.FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertFeedback(store.FeedbackRecord{
		Worktree: root, EvidenceID: path + "#Dispatch", FilePath: path, Symbol: "Dispatch",
		FileHash: files[path].Hash, Polarity: polarity, ReconciledWeight: weight, Tier: tier, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

const dispatchPrompt = "dispatch the ticket to the queue"

// Misleading feedback on one copy and helpful feedback on the other reorders them on the next query.
func TestFeedbackPromotesHelpfulAndDemotesMisleading(t *testing.T) {
	root, ix := dispatchFixture(t)
	st := ix.Store()
	base := dispatchPaths(t, st, root, dispatchPrompt, true)
	if len(base) != 2 {
		t.Fatalf("want both Dispatch copies retrieved, got %v", base)
	}
	misleading, helpful := base[0], base[1]
	recordDispatchFeedback(t, st, root, misleading, -1, store.ComputeReconciledWeight(-1, "high", store.TraceEdited, -1, true))
	recordDispatchFeedback(t, st, root, helpful, 1, store.ComputeReconciledWeight(1, "high", store.TraceEdited, 1, false))

	if on := dispatchPaths(t, st, root, dispatchPrompt, false); !slices.Equal(on, []string{helpful, misleading}) {
		t.Fatalf("feedback did not reorder the copies: got %v, want [%s %s]", on, helpful, misleading)
	}
}

// Editing the misleading copy and re-indexing it makes the penalty stale: the ranking is again
// the one computed without feedback.
func TestFeedbackInvalidatedByReindexRestoresRanking(t *testing.T) {
	root, ix := dispatchFixture(t)
	st := ix.Store()
	base := dispatchPaths(t, st, root, dispatchPrompt, true)
	misleading := base[0]
	recordDispatchFeedback(t, st, root, misleading, -1, store.ComputeReconciledWeight(-1, "high", store.TraceEdited, -1, true))
	if on := dispatchPaths(t, st, root, dispatchPrompt, false); on[0] == misleading {
		t.Fatalf("penalty had no effect before the change: %v", on)
	}

	p := filepath.Join(root, filepath.FromSlash(misleading))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(data, []byte("\n// reviewed after the change.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListFeedback(root)
	if err != nil || len(list) != 1 || list[0].Status != store.FeedbackStale {
		t.Fatalf("feedback after re-index = %+v (%v), want one stale record", list, err)
	}
	if got, want := dispatchPaths(t, st, root, dispatchPrompt, false), dispatchPaths(t, st, root, dispatchPrompt, true); !slices.Equal(got, want) {
		t.Fatalf("ranking after re-index = %v, want the feedback-free ranking %v", got, want)
	}
}

// Weak feedback (a self-report with no edit in the trace) is stored but never reorders the copies,
// however strongly the agent worded it.
func TestWeakFeedbackDoesNotReorderRanking(t *testing.T) {
	root, ix := dispatchFixture(t)
	st := ix.Store()
	base := dispatchPaths(t, st, root, dispatchPrompt, true)
	recordDispatchFeedbackAs(t, st, root, base[0], -1, store.ComputeReconciledWeight(-1, "high", store.TraceNotObserved, 0, true), store.TierWeak)
	recordDispatchFeedbackAs(t, st, root, base[1], 1, store.ComputeReconciledWeight(1, "high", store.TraceNotObserved, 1, false), store.TierWeak)
	if on := dispatchPaths(t, st, root, dispatchPrompt, false); !slices.Equal(on, base) {
		t.Fatalf("weak feedback changed the ranking: got %v, want the feedback-free order %v", on, base)
	}
}
