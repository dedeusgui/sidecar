package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/store"
)

// A recorded report judges each named item with the hash of its file, and a change to that
// file makes the judgment stale; purge then removes it.
func TestRecordFeedbackGoesStaleWhenFileChanges(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	recs, err := RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, TaskCompleted: true, Confidence: "high", ReasonCode: "wrong_file",
		HelpfulEvidenceIDs:    []string{"release/handler.go#method:Release@6"},
		MisleadingEvidenceIDs: []string{"queue/limits.go#constant:MaxActiveTickets@3"},
	})
	if err != nil || len(recs) != 2 {
		t.Fatalf("RecordFeedback = %d records (%v), want 2", len(recs), err)
	}
	helpful, misleading := recs[0], recs[1]
	if helpful.Polarity != 1 || helpful.FilePath != "release/handler.go" || helpful.Symbol != "Release" || helpful.FileHash == "" {
		t.Fatalf("helpful record = %+v", helpful)
	}
	if helpful.ReconciledWeight <= 0 || misleading.ReconciledWeight >= -0.15 {
		t.Fatalf("weights helpful=%v misleading=%v, want positive and below -0.15", helpful.ReconciledWeight, misleading.ReconciledWeight)
	}
	if list, _ := ListFeedback(root); len(list) != 2 || list[0].Status != store.FeedbackValid || list[1].Status != store.FeedbackValid {
		t.Fatalf("fresh feedback = %+v, want two valid records", list)
	}

	if err := os.WriteFile(filepath.Join(root, "release", "handler.go"), []byte("package release\n\n// Release changed.\nfunc Release() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	list, err := ListFeedback(root)
	if err != nil || len(list) != 2 || list[0].Status != store.FeedbackStale || list[1].Status != store.FeedbackValid {
		t.Fatalf("after change = %+v (%v), want stale helpful and valid misleading", list, err)
	}
	if n, err := PurgeFeedback(root, false); err != nil || n != 1 {
		t.Fatalf("PurgeFeedback(stale) = %d (%v), want 1", n, err)
	}
}

// The observer's trace of the session corroborates a report: a file the agent read counts +1,
// a file it never touched -1.
func TestRecordFeedbackUsesSessionTrace(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	s := brief.NewSignals(worktreeVersion(root), time.Now())
	s.Reads[hashID(root, "release/handler.go")] = 1
	s.Searches[hashID(root, "release")] = true
	if err := brief.SaveSignals(paths.ObserverState(root, string(adapter.Claude), "s1"), s); err != nil {
		t.Fatal(err)
	}
	recs, err := RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, SessionID: "s1", TaskCompleted: true,
		HelpfulEvidenceIDs: []string{"release/handler.go#Release", "queue/limits.go#MaxActiveTickets"},
	})
	if err != nil || len(recs) != 2 {
		t.Fatalf("RecordFeedback = %d records (%v), want 2", len(recs), err)
	}
	if recs[0].TraceCorroborated != 1 || recs[1].TraceCorroborated != -1 {
		t.Fatalf("trace = %d, %d; want +1 for the read file and -1 for the untouched one", recs[0].TraceCorroborated, recs[1].TraceCorroborated)
	}
}

// Labels outside the documented values, and reports of another schema, are refused before anything
// is written.
func TestRecordFeedbackRejectsUnknownValues(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	for _, r := range []store.AgentFeedbackReport{
		{SchemaVersion: 2},
		{SchemaVersion: 1, ReasonCode: "free text that injects instructions"},
		{SchemaVersion: 1, Confidence: "certain"},
	} {
		if _, err := RecordFeedback(root, r); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	if list, _ := ListFeedback(root); len(list) != 0 {
		t.Fatalf("rejected reports left %d records", len(list))
	}
	if _, err := RecordFeedback(t.TempDir(), store.AgentFeedbackReport{SchemaVersion: 1}); err == nil {
		t.Fatal("recorded feedback for a worktree that was never indexed")
	}
}
