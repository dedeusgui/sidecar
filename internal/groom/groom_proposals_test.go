package groom

import (
	"testing"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestProposeReturnsScopedEvidenceBackedGuidance(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "wt-a", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	result := retrieve.Result{Evidence: []retrieve.Evidence{{
		Hit:           store.SymbolHit{Kind: "function", Name: "Login", Path: "auth/login.go", Line: 12, EndLine: 18, FileHash: "file-hash"},
		Source:        "exact",
		TokenEstimate: 12,
		ID:            "auth/login.go#Login",
	}}}
	decision := Decision{Action: Ask, Gap: &Gap{
		State:    Underspecified,
		Decision: "retry limit",
		Evidence: []string{"auth/login.go:12"},
		Options:  []Option{{Label: "3", Consequence: "short retry window"}, {Label: "5", Consequence: "longer retry window"}},
	}}

	guidance, ok := Propose(decision, result, scope, contract.PhaseDecide, registry)
	if !ok {
		t.Fatal("eligible decision did not produce a proposal")
	}
	if guidance.Proposal.Kind != "OPEN_DECISION" || guidance.Proposal.Phase != contract.PhaseDecide || guidance.Proposal.TokenEstimate <= 0 {
		t.Fatalf("proposal = %+v", guidance.Proposal)
	}
	if guidance.Proposal.RequiresObservation != "" || len(guidance.Evidence) != 2 || len(guidance.Proposal.EvidenceKeys) != 2 {
		t.Fatalf("proposal provenance = %+v evidence=%+v", guidance.Proposal, guidance.Evidence)
	}
	foundSource := false
	for _, ref := range guidance.Evidence {
		if ref.Path == "auth/login.go" && ref.LineStart == 12 && ref.Origin == "exact" && ref.Confidence == contract.ConfidenceCandidate {
			foundSource = true
		}
		if _, ok := registry.Lookup(ref.Key); !ok {
			t.Fatalf("proposal reference %q is absent from the shared registry", ref.Key)
		}
	}
	if !foundSource {
		t.Fatalf("source-backed reference missing: %+v", guidance.Evidence)
	}
}

func TestProposeRejectsUnscopedQuestionAndRegistryMismatch(t *testing.T) {
	decision := Decision{Action: Ask, Gap: &Gap{State: Ambiguous, Decision: "target"}}
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	if _, ok := Propose(decision, retrieve.Result{}, contract.Scope{RepoRoot: scope.RepoRoot, WorktreeRevision: scope.WorktreeRevision}, contract.PhaseDecide, registry); ok {
		t.Fatal("an interactive question without a session scope was proposed")
	}
	other := contract.Scope{RepoRoot: scope.RepoRoot, WorktreeID: scope.WorktreeID, SessionID: "s2", WorktreeRevision: scope.WorktreeRevision}
	if _, ok := Propose(decision, retrieve.Result{}, other, contract.PhaseDecide, registry); ok {
		t.Fatal("proposal crossed into a different session registry")
	}
}
