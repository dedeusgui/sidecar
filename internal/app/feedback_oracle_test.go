package app

import (
	"testing"

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
