package brief

import (
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/progress"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestSessionGuidanceRequiresObservedScopedSignals(t *testing.T) {
	now := time.Now()
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "wt-1", SessionID: "s1", WorktreeRevision: "head-1"}
	signals := NewSignals(scope.WorktreeRevision, now)
	signals.ScopeFingerprint = ScopeFingerprint(scope)
	signals.Trace = []progress.Event{{Kind: "read", Key: "path-hash"}, {Kind: "read", Key: "path-hash"}}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)

	unknown := ProposeSessionGuidance(scope, signals, contract.Observability{}, contract.PhaseEdit, now, registry)
	if len(unknown) != 0 {
		t.Fatalf("unknown observer coverage became guidance: %+v", unknown)
	}
	observed := contract.Observability{ToolEvents: contract.ObservationSeen}
	guidance := ProposeSessionGuidance(scope, signals, observed, contract.PhaseEdit, now, registry)
	if len(guidance) != 1 {
		t.Fatalf("observed repeated read produced %d proposals: %+v", len(guidance), guidance)
	}
	proposal := guidance[0].Proposal
	if proposal.RequiresObservation != contract.ObservationToolEvents || proposal.DeliveryReason != "session.observed_repeated_navigation" {
		t.Fatalf("proposal does not retain observation provenance: %+v", proposal)
	}
	if len(guidance[0].Evidence) != 1 || len(proposal.EvidenceKeys) != 1 {
		t.Fatalf("proposal evidence = %+v", guidance[0].Evidence)
	}
	if _, ok := registry.Lookup(proposal.EvidenceKeys[0]); !ok {
		t.Fatal("session evidence reference was not registered")
	}

	otherScope := scope
	otherScope.SessionID = "s2"
	otherRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](otherScope)
	if got := ProposeSessionGuidance(otherScope, signals, observed, contract.PhaseEdit, now, otherRegistry); len(got) != 0 {
		t.Fatalf("signals crossed session scope: %+v", got)
	}
}

func TestSessionCompactionProposalIsResumeOnlyAndRequiresPreCompact(t *testing.T) {
	now := time.Now()
	scope := contract.Scope{RepoRoot: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	signals := NewSignals(scope.WorktreeRevision, now)
	signals.ScopeFingerprint = ScopeFingerprint(scope)
	signals.Compactions = 1
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	obs := contract.Observability{PreCompact: contract.ObservationSeen}
	if got := ProposeSessionGuidance(scope, signals, obs, contract.PhaseEdit, now, registry); len(got) != 0 {
		t.Fatalf("compaction leaked into a non-resume phase: %+v", got)
	}
	got := ProposeSessionGuidance(scope, signals, obs, contract.PhaseResume, now, registry)
	if len(got) != 1 || got[0].Proposal.RequiresObservation != contract.ObservationPreCompact {
		t.Fatalf("resume proposal = %+v", got)
	}
}
