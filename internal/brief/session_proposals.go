package brief

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/progress"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// SessionGuidance keeps the planner proposal separate from its observed signal
// provenance. It contains hashes and stable codes only; it never carries a
// prompt, tool output, path, or repository text.
type SessionGuidance struct {
	Proposal contract.GuidanceProposal
	Evidence []contract.EvidenceRef
}

// ScopeFingerprint returns a stable, opaque binding for one worktree revision
// and optional agent session. It is safe to persist with SessionSignals.
func ScopeFingerprint(scope contract.Scope) string {
	scope = contract.NormalizeScope(scope)
	identity := strings.Join([]string{scope.RepoRoot, scope.WorktreeID, scope.SessionID, scope.WorktreeRevision}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])[:24]
}

// ProposeSessionGuidance turns already-observed, scope-matched session signals
// into small planner candidates. It does not read files, render advice, persist
// state, or infer missing events. The registry must belong to the same Scope.
func ProposeSessionGuidance(scope contract.Scope, signals SessionSignals, observability contract.Observability, phase contract.TaskPhase, now time.Time, registry *contract.EvidenceRegistry[retrieve.Evidence]) []SessionGuidance {
	scope = contract.NormalizeScope(scope)
	if registry == nil || !scope.Equal(registry.Scope()) || scope.SessionID == "" || scope.WorktreeRevision == "" {
		return nil
	}
	if signals.ScopeFingerprint != ScopeFingerprint(scope) || signals.Version != scope.WorktreeRevision || now.Unix() >= signals.Expires {
		return nil
	}
	if phase == "" {
		phase = contract.PhaseUnknown
	}

	var out []SessionGuidance
	if observability.ToolEvents == contract.ObservationSeen {
		epoch := currentEpoch(signals.Trace)
		progressReport := progress.Analyze(epoch)
		reason := ""
		if progressReport.Labels[progress.RepeatedError] > 0 {
			reason = "session.observed_repeated_error"
		} else if progressReport.Redundant > 0 {
			reason = "session.observed_repeated_navigation"
		}
		if reason != "" {
			proposal, refs, ok := addSessionProposal(scope, registry, reason, phase, contract.ObservationToolEvents, traceFingerprint(epoch))
			if ok {
				out = append(out, SessionGuidance{Proposal: proposal, Evidence: refs})
			}
		}
	}
	if signals.Compactions > 0 && phase == contract.PhaseResume && observability.PreCompact == contract.ObservationSeen {
		resumePhase := contract.PhaseResume
		key := fmt.Sprintf("compaction:%d", signals.Compactions)
		proposal, refs, ok := addSessionProposal(scope, registry, "session.observed_precompact", resumePhase, contract.ObservationPreCompact, key)
		if ok {
			out = append(out, SessionGuidance{Proposal: proposal, Evidence: refs})
		}
	}
	return out
}

func currentEpoch(events []progress.Event) []progress.Event {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == "edit" {
			start = i
			break
		}
	}
	return events[start:]
}

func traceFingerprint(events []progress.Event) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString(event.Kind)
		b.WriteByte(0)
		b.WriteString(event.Key)
		b.WriteByte(0)
		b.WriteString(event.Err)
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:24]
}

func addSessionProposal(scope contract.Scope, registry *contract.EvidenceRegistry[retrieve.Evidence], reason string, phase contract.TaskPhase, required contract.ObservationEvent, signalKey string) (contract.GuidanceProposal, []contract.EvidenceRef, bool) {
	identity := ScopeFingerprint(scope) + "\x00" + reason + "\x00" + signalKey
	sum := sha256.Sum256([]byte(identity))
	key := "session:" + hex.EncodeToString(sum[:])[:24]
	ref := contract.EvidenceRef{
		Key:          key,
		Origin:       "session_observation",
		Confidence:   contract.ConfidenceCandidate,
		Freshness:    contract.FreshnessCurrent,
		CostEstimate: 1,
	}
	canonical, _, err := registry.Add(ref, retrieve.Evidence{
		ID:     key,
		Hit:    store.SymbolHit{Kind: "session_signal", Name: reason},
		Source: "session_observation",
	})
	if err != nil {
		return contract.GuidanceProposal{}, nil, false
	}
	proposal := contract.GuidanceProposal{
		Kind:                "SESSION_GUIDANCE",
		EvidenceKeys:        []string{canonical.Key},
		Priority:            35,
		TokenEstimate:       sessionTokenEstimate(reason),
		DeliveryReason:      reason,
		Phase:               phase,
		RequiresObservation: required,
	}
	return proposal, []contract.EvidenceRef{canonical}, true
}

func sessionTokenEstimate(reason string) int {
	text := "Session observation was recorded."
	switch reason {
	case "session.observed_repeated_error":
		text = "A repeated test or tool error was observed in this session; recheck the relevant source before retrying."
	case "session.observed_repeated_navigation":
		text = "Repeated navigation was observed in this session; prioritize the relevant source evidence."
	case "session.observed_precompact":
		text = "The session compacted; re-establish evidence validated for the current repository snapshot."
	}
	rendered := fmt.Sprintf("%s\n- %s [observed] (source: %s)\n", ctxbuild.Session, text, reason)
	return max(1, ctxbuild.EstimateTokens(rendered))
}
