package app

import (
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/progress"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestObserverProposalsRequireKnownCoverageAndMatchingScope(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	now := time.Now()
	scope := contract.Scope{RepoRoot: root, WorktreeID: "git-worktree-1", SessionID: "s1", WorktreeRevision: "head-1"}
	storedScope := observerScope(root, scope.SessionID, scope.WorktreeRevision)
	signals := brief.NewSignals(scope.WorktreeRevision, now)
	signals.ScopeFingerprint = brief.ScopeFingerprint(storedScope)
	signals.Trace = []progress.Event{{Kind: "search", Key: "query-hash"}, {Kind: "search", Key: "query-hash"}}
	statePath := paths.ObserverState(root, string(adapter.Claude), scope.SessionID)
	if err := brief.SaveSignals(statePath, signals); err != nil {
		t.Fatal(err)
	}

	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	if got := ProposeObserverGuidance(scope, adapter.Claude, contract.Observability{}, contract.PhaseExplore, now, registry); len(got) != 0 {
		t.Fatalf("unknown coverage produced an intervention: %+v", got)
	}
	obs := contract.Observability{ToolEvents: ObservationStateFromCoverage("observed")}
	got := ProposeObserverGuidance(scope, adapter.Claude, obs, contract.PhaseExplore, now, registry)
	if len(got) != 1 || got[0].Proposal.RequiresObservation != contract.ObservationToolEvents {
		t.Fatalf("observed event proposal = %+v", got)
	}
	if ObservationStateFromCoverage("future-value") != contract.ObservationUnsupported {
		t.Fatal("unknown coverage must remain unobserved")
	}

	otherScope := scope
	otherScope.SessionID = "s2"
	otherRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](otherScope)
	if other := ProposeObserverGuidance(otherScope, adapter.Claude, obs, contract.PhaseExplore, now, otherRegistry); len(other) != 0 {
		t.Fatalf("observer state crossed session scope: %+v", other)
	}
}
