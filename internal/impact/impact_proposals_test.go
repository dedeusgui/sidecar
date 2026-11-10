package impact

import (
	"testing"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestProposeRequiresRelevantUncommittedChangeAndPreservesProvenance(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	report := Report{
		Changed: []string{"billing/charge.go"},
		Findings: []Finding{
			{Kind: "contract", Status: Conflict, Path: "docs/billing.md", Line: 7, Symbol: "Charge", Text: "documentation disagrees with code"},
			{Kind: "consumer", Status: Candidate, Path: "api/quote.go", Symbol: "Quote", Text: "possible consumer"},
			{Kind: "test", Status: Confirmed, Path: "billing/charge_test.go", Line: 22, Symbol: "Charge", Text: "test file references Charge and was not changed"},
		},
	}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	if _, ok := Propose(report, scope, contract.PhaseVerify, false, true, registry); ok {
		t.Fatal("committed/no-diff report produced impact guidance")
	}
	if _, ok := Propose(report, scope, contract.PhaseVerify, true, false, registry); ok {
		t.Fatal("unrelated report produced impact guidance")
	}

	guidance, ok := Propose(report, scope, contract.PhaseVerify, true, true, registry)
	if !ok {
		t.Fatal("eligible report did not produce a proposal")
	}
	if guidance.Proposal.Kind != "CHANGE_IMPACT" || guidance.Proposal.Phase != contract.PhaseVerify || guidance.Proposal.TokenEstimate <= 0 {
		t.Fatalf("proposal = %+v", guidance.Proposal)
	}
	if len(guidance.Findings) != 3 || len(guidance.Proposal.EvidenceKeys) != 3 {
		t.Fatalf("guidance lost findings or evidence links: %+v", guidance)
	}
	for _, ref := range guidance.Evidence {
		if _, ok := registry.Lookup(ref.Key); !ok {
			t.Fatalf("evidence reference %q was not added to the shared registry", ref.Key)
		}
		if ref.Confidence == contract.ConfidenceVerified && !ref.Proof.Valid() {
			t.Fatalf("verified reference lacks resolver provenance: %+v", ref)
		}
	}
}

func TestProposeRequiresMatchingRegistryScope(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: scope.RepoRoot, SessionID: "s2", WorktreeRevision: scope.WorktreeRevision})
	report := Report{Changed: []string{"a.go"}, Findings: []Finding{{Kind: "gap", Status: Unknown, Text: "no test found"}}}
	if _, ok := Propose(report, scope, contract.PhaseVerify, true, true, registry); ok {
		t.Fatal("proposal crossed into a different session registry")
	}
}
