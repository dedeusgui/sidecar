package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestProposeGroomKeepsInteractivePolicyAndReturnsProposal(t *testing.T) {
	interactive(t)
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	guidance, ok := ProposeGroom("adicione um limite de tentativas no login", retrieve.Result{}, scope, contract.PhaseDecide, 0, true, registry)
	if !ok || guidance.Decision.Action != groom.Ask {
		t.Fatalf("interactive gap = %+v, ok=%v", guidance, ok)
	}
	if guidance.Proposal.Kind != "OPEN_DECISION" || guidance.Proposal.DeliveryReason != "groom.ask.underspecified" || len(guidance.Proposal.EvidenceKeys) == 0 {
		t.Fatalf("proposal = %+v", guidance.Proposal)
	}
	if len(guidance.Evidence) != 1 || guidance.Evidence[0].Origin != "groom_analysis" {
		t.Fatalf("prompt-derived provenance = %+v", guidance.Evidence)
	}
}

func TestProposeGroomDoesNotAskWithoutInteractionOrSession(t *testing.T) {
	interactive(t)
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	guidance, ok := ProposeGroom("adicione um limite de tentativas no login", retrieve.Result{}, scope, contract.PhaseDecide, 0, false, registry)
	if !ok || guidance.Decision.Action != groom.Assume {
		t.Fatalf("noninteractive gap = %+v, ok=%v", guidance, ok)
	}
	if guidance.Proposal.DeliveryReason != "groom.assume.underspecified" {
		t.Fatalf("noninteractive decision reason = %q", guidance.Proposal.DeliveryReason)
	}

	noSession := contract.Scope{RepoRoot: scope.RepoRoot, WorktreeID: scope.WorktreeID, WorktreeRevision: scope.WorktreeRevision}
	noSessionRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](noSession)
	if _, ok := ProposeGroom("adicione um limite de tentativas no login", retrieve.Result{}, noSession, contract.PhaseDecide, 0, true, noSessionRegistry); ok {
		t.Fatal("interactive question was proposed without a session identity")
	}
}

func TestGroomProposalStateAndPendingQuestionAreScoped(t *testing.T) {
	setupHome(t)
	interactive(t)
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "wt-a", SessionID: "s1", WorktreeRevision: "head-1"}
	state := LoadGroomProposalState(scope)
	if state.ScopeFingerprint != groom.ScopeFingerprint(scope) || state.Asked != 0 {
		t.Fatalf("fresh scoped state = %+v", state)
	}
	state.Asked = 1
	if err := SaveGroomProposalState(scope, state); err != nil {
		t.Fatal(err)
	}

	otherSession := scope
	otherSession.SessionID = "s2"
	if got := LoadGroomProposalState(otherSession); got.Asked != 0 {
		t.Fatalf("other session inherited Groom quota: %+v", got)
	}
	otherWorktree := scope
	otherWorktree.WorktreeID = "wt-b"
	if got := LoadGroomProposalState(otherWorktree); got.Asked != 0 {
		t.Fatalf("other worktree inherited Groom quota: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(paths.Home(), "sessions")); err != nil {
		t.Fatalf("scoped state was not stored in the Sidecar cache: %v", err)
	}

	state = LoadGroomProposalState(scope)
	state.Asked = 0
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	prompt := "adicione um limite de tentativas no login"
	proposal, ok := ProposeGroomScoped(prompt, retrieve.Result{}, scope, contract.PhaseDecide, state, true, registry, time.Now())
	if !ok || proposal.Guidance.Decision.Action != groom.Ask || proposal.State.Pending == nil || proposal.State.Asked != 1 {
		t.Fatalf("first scoped question = %+v, ok=%v", proposal, ok)
	}

	registry = contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	pending, ok := ProposeGroomScoped(prompt, retrieve.Result{}, scope, contract.PhaseDecide, proposal.State, true, registry, time.Now())
	if ok || !pending.Pending || pending.State.Asked != 1 {
		t.Fatalf("pending question repeated or quota changed: %+v, ok=%v", pending, ok)
	}
	answer, ok := ProposeGroomScoped("A", retrieve.Result{}, scope, contract.PhaseDecide, pending.State, true, registry, time.Now())
	if ok || !answer.Answered || answer.Answer == nil || answer.State.Pending != nil {
		t.Fatalf("unambiguous answer was not consumed: %+v, ok=%v", answer, ok)
	}
}
