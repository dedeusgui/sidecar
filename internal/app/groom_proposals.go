package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// GroomProposalResult carries a proposed decision and the candidate scoped
// state transition. Callers persist State only after the planner selects the
// proposal, so a dropped question does not consume the ask quota.
type GroomProposalResult struct {
	Guidance       groom.Guidance
	State          groom.SessionState
	Pending        bool
	Answered       bool
	AnsweredPrompt *groom.Pending
	Answer         *groom.Option
}

// ProposeGroom evaluates an unanswered task decision without changing the
// legacy hook result, pending-answer state, or output budget. Callers supply
// the already-loaded per-scope question count and verified interaction
// capability; the returned payload can be rendered only after planning.
func ProposeGroom(prompt string, result retrieve.Result, scope contract.Scope, phase contract.TaskPhase, askedInScope int, canAsk bool, registry *contract.EvidenceRegistry[retrieve.Evidence]) (groom.Guidance, bool) {
	report := groom.Analyze(prompt, result)
	policy := groom.DefaultPolicy()
	policy.NonInteractive = !canAsk || groomNonInteractive()
	decision := groom.Decide(report, prompt, askedInScope, policy)
	return groom.Propose(decision, result, scope, phase, registry)
}

// LoadGroomProposalState reads the proposal-only grooming state from a cache
// file keyed by the normalized worktree/revision/session scope. Legacy Groom
// state and state from other scopes are deliberately ignored.
func LoadGroomProposalState(scope contract.Scope) groom.SessionState {
	scope = contract.NormalizeScope(scope)
	return groom.LoadScopedState(groomProposalStatePath(scope), scope)
}

// SaveGroomProposalState persists a candidate state only when it still belongs
// to the requested Scope. Integrators should call this after proposal delivery.
func SaveGroomProposalState(scope contract.Scope, state groom.SessionState) error {
	scope = contract.NormalizeScope(scope)
	return groom.SaveScopedState(groomProposalStatePath(scope), scope, state)
}

// ProposeGroomScoped keeps multi-turn question quotas and pending answers
// isolated by Scope. It returns a candidate state transition without writing
// it; callers commit the returned state only after the planner selects the
// proposal. An answer to a prior question clears pending state and is returned
// for the caller to bind to the file hash through RecordGroomProposalAnswer.
func ProposeGroomScoped(prompt string, result retrieve.Result, scope contract.Scope, phase contract.TaskPhase, state groom.SessionState, canAsk bool, registry *contract.EvidenceRegistry[retrieve.Evidence], now time.Time) (GroomProposalResult, bool) {
	scope = contract.NormalizeScope(scope)
	if state.ScopeFingerprint != groom.ScopeFingerprint(scope) {
		state = groom.NewScopedState(scope)
	}
	state.Turn++
	if pending := state.Pending; pending != nil {
		switch {
		case state.Turn-pending.Turn > groom.PendingTurns:
			state.Pending = nil
		default:
			if answer := groom.MatchAnswer(prompt, pending.Options); answer >= 0 {
				choice := pending.Options[answer]
				copyPending := *pending
				state.Pending = nil
				return GroomProposalResult{State: state, Answered: true, AnsweredPrompt: &copyPending, Answer: &choice}, false
			}
		}
	}

	guidance, ok := ProposeGroom(prompt, result, scope, phase, state.Asked, canAsk, registry)
	if !ok {
		return GroomProposalResult{State: state}, false
	}
	if pending := state.Pending; pending != nil && guidance.Decision.Gap != nil && pending.Decision == guidance.Decision.Gap.Decision {
		return GroomProposalResult{State: state, Pending: true}, false
	}
	if guidance.Decision.Action == groom.Ask && guidance.Decision.Gap != nil {
		state.Asked++
		state.Pending = &groom.Pending{
			Decision: guidance.Decision.Gap.Decision,
			Options:  guidance.Decision.Gap.Options,
			Path:     primaryPath(*guidance.Decision.Gap, result),
			Turn:     state.Turn,
		}
	}
	return GroomProposalResult{Guidance: guidance, State: state}, true
}

// RecordGroomProposalAnswer stores only an unambiguous explicit answer, using
// the existing file-hash and TTL binding. A missing source file is rejected.
func RecordGroomProposalAnswer(st *store.Store, scope contract.Scope, proposal GroomProposalResult, now time.Time) error {
	if !proposal.Answered || proposal.AnsweredPrompt == nil || proposal.Answer == nil {
		return errors.New("no unambiguous Groom proposal answer")
	}
	scope = contract.NormalizeScope(scope)
	if proposal.State.ScopeFingerprint != groom.ScopeFingerprint(scope) {
		return errors.New("groom proposal answer scope mismatch")
	}
	return recordAnswer(st, scope.RepoRoot, scope.SessionID, *proposal.AnsweredPrompt, *proposal.Answer, now)
}

func groomProposalStatePath(scope contract.Scope) string {
	return filepath.Join(paths.Home(), "sessions", fmt.Sprintf("%s.groom-proposals.json", groom.ScopeFingerprint(scope)))
}
