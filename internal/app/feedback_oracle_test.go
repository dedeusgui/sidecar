package app

import (
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/store"
)

// indexedFeedbackProject indexes the fixture project used by the feedback tests.
func indexedFeedbackProject(t *testing.T) string {
	t.Helper()
	setupHome(t)
	root := makeProject(t)
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	return root
}

// The agent's completion claim is not an oracle. Without an oracle report the record is unknown,
// carries no provenance, is weak, and its weight is the one an unknown oracle gives.
func TestCompletionClaimIsNotAnOracle(t *testing.T) {
	root := indexedFeedbackProject(t)
	recs, err := RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, TaskCompleted: true, Confidence: "high",
		HelpfulEvidenceIDs: []string{"release/handler.go#Release"},
	})
	if err != nil || len(recs) != 1 {
		t.Fatalf("RecordFeedback = %d records (%v), want 1", len(recs), err)
	}
	r := recs[0]
	if !r.AgentCompleted || r.OracleStatus != store.OracleUnknown || r.OraclePassed != 0 ||
		r.Provenance != store.ProvenanceNone || r.Tier != store.TierWeak {
		t.Fatalf("completion claim was treated as an oracle: %+v", r)
	}
	if want := store.ComputeReconciledWeight(1, "high", r.TraceCorroborated, 0, false); r.ReconciledWeight != want {
		t.Fatalf("weight = %v, want the unknown-oracle weight %v", r.ReconciledWeight, want)
	}
}

// Who ran the oracle and whether the file was edited decide the tier: a self-reported pass is weak
// without an edit, medium with one, and strong only when an independent run reported it.
func TestOracleProvenanceAndPatchDecideTier(t *testing.T) {
	root := indexedFeedbackProject(t)
	passed := true
	const evidence = "release/handler.go#Release"

	recs, err := RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, OraclePassed: &passed, Confidence: "high", HelpfulEvidenceIDs: []string{evidence},
	})
	if err != nil || len(recs) != 1 {
		t.Fatalf("self-report: %d records (%v)", len(recs), err)
	}
	if r := recs[0]; r.Tier != store.TierWeak || r.Provenance != store.ProvenanceAgentReport || r.OracleStatus != store.OraclePassed {
		t.Fatalf("self-reported pass without edit = tier %q, provenance %q, status %q; want weak, agent_report, passed",
			r.Tier, r.Provenance, r.OracleStatus)
	}

	s := brief.NewSignals(worktreeVersion(root), time.Now())
	s.Edits[hashID(root, "release/handler.go")] = 1
	if err := brief.SaveSignals(paths.ObserverState(root, string(adapter.Claude), "edit-session"), s); err != nil {
		t.Fatal(err)
	}
	recs, err = RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, SessionID: "edit-session", OraclePassed: &passed, Confidence: "high", HelpfulEvidenceIDs: []string{evidence},
	})
	if err != nil || len(recs) != 1 {
		t.Fatalf("edited report: %d records (%v)", len(recs), err)
	}
	if r := recs[0]; r.Tier != store.TierMedium || r.EvidenceUsed != store.EvidenceConfirmed || r.TraceCorroborated != store.TraceEdited {
		t.Fatalf("edited file = tier %q, used %q, trace %d; want medium, confirmed, %d",
			r.Tier, r.EvidenceUsed, r.TraceCorroborated, store.TraceEdited)
	}

	recs, err = RecordFeedback(root, store.AgentFeedbackReport{
		SchemaVersion: 1, OracleStatus: "failed", OracleProvenance: "harness", Confidence: "high",
		MisleadingEvidenceIDs: []string{evidence},
	})
	if err != nil || len(recs) != 1 {
		t.Fatalf("harness report: %d records (%v)", len(recs), err)
	}
	if r := recs[0]; r.Tier != store.TierStrong || r.Provenance != store.ProvenanceHarness || r.OraclePassed != -1 {
		t.Fatalf("independent failed run = tier %q, provenance %q, outcome %d; want strong, harness, -1",
			r.Tier, r.Provenance, r.OraclePassed)
	}
}

// Contradictory or unknown oracle values are refused before anything is written.
func TestNormalizeOracleRejectsContradictions(t *testing.T) {
	yes, no := true, false
	bad := []store.AgentFeedbackReport{
		{OracleStatus: "passed", OraclePassed: &no},
		{OracleStatus: "unknown", OraclePassed: &yes},
		{OracleStatus: "maybe"},
		{OracleStatus: "passed", OracleProvenance: "bogus"},
	}
	for _, r := range bad {
		r.SchemaVersion = 1
		if err := normalizeReport(&r); err == nil {
			t.Fatalf("normalizeReport accepted %+v", r)
		}
	}
	derived := store.AgentFeedbackReport{SchemaVersion: 1, OraclePassed: &no}
	if err := normalizeReport(&derived); err != nil || derived.OracleStatus != store.OracleFailed || derived.OracleProvenance != store.ProvenanceAgentReport {
		t.Fatalf("derived oracle = %q, %q (%v); want failed, agent_report", derived.OracleStatus, derived.OracleProvenance, err)
	}
	none := store.AgentFeedbackReport{SchemaVersion: 1, TaskCompleted: true}
	if err := normalizeReport(&none); err != nil || none.OracleStatus != store.OracleUnknown || none.OracleProvenance != store.ProvenanceNone {
		t.Fatalf("report without oracle = %q, %q (%v); want unknown, none", none.OracleStatus, none.OracleProvenance, err)
	}
}
