package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// MemoryGuidance pairs a validated explicit decision with its planner
// proposal. Decision summaries remain caller-owned payload; they are not
// represented as verified repository facts.
type MemoryGuidance struct {
	Proposal contract.GuidanceProposal
	Evidence []contract.EvidenceRef
	Decision store.DecisionRecord
}

// ProposeMemory returns relevant, explicit decisions after rechecking their
// file hash, TTL, symbol existence, worktree, and optional session scope. It
// does not append text or allocate budget. Empty SessionID decisions are
// user-managed worktree decisions; non-empty ones are visible only to their
// matching session.
func ProposeMemory(st *store.Store, scope contract.Scope, evidence []contract.EvidenceRef, named []string, now time.Time, registry *contract.EvidenceRegistry[retrieve.Evidence]) ([]MemoryGuidance, error) {
	scope = contract.NormalizeScope(scope)
	if st == nil {
		return nil, errors.New("nil decision store")
	}
	if scope.RepoRoot == "" || registry == nil || !scope.Equal(registry.Scope()) {
		return nil, errors.New("memory proposal scope does not match evidence registry")
	}
	list, err := st.RefreshDecisions(scope.RepoRoot, now, func(rel string) (string, bool) { return diskHash(scope.RepoRoot, rel) })
	if err != nil {
		return nil, err
	}
	byName := make(map[string]bool, len(named))
	for _, name := range named {
		name = strings.TrimSpace(name)
		if name != "" {
			byName[name] = true
		}
	}

	var proposals []MemoryGuidance
	for i := len(list) - 1; i >= 0 && len(proposals) < maxDecisionsInContext; i-- {
		decision := list[i]
		if decision.Status != store.DecisionValid || (decision.Origin != "manual" && decision.Origin != "user_answer") {
			continue
		}
		if decision.SessionID != "" && decision.SessionID != scope.SessionID {
			continue
		}
		if !memoryDecisionRelevant(decision, evidence, byName) {
			continue
		}
		guidance, ok := memoryDecisionProposal(decision, registry)
		if ok {
			proposals = append(proposals, guidance)
		}
	}
	return proposals, nil
}

func memoryDecisionRelevant(decision store.DecisionRecord, evidence []contract.EvidenceRef, named map[string]bool) bool {
	if named[decision.SymbolOrPath] || (decision.Symbol != "" && named[decision.Symbol]) {
		return true
	}
	for _, ref := range evidence {
		if ref.Path == decision.FilePath {
			return true
		}
	}
	return false
}

func memoryDecisionProposal(decision store.DecisionRecord, registry *contract.EvidenceRegistry[retrieve.Evidence]) (MemoryGuidance, bool) {
	key := fmt.Sprintf("memory-decision:%d:%s", decision.ID, decision.FileHash)
	ref := contract.EvidenceRef{
		Key:          key,
		Path:         decision.FilePath,
		Origin:       "explicit_decision_memory:" + decision.Origin,
		Confidence:   contract.ConfidenceCandidate,
		Freshness:    contract.FreshnessCurrent,
		CostEstimate: 1,
	}
	canonical, _, err := registry.Add(ref, retrieve.Evidence{
		ID:      key,
		Hit:     store.SymbolHit{Kind: "decision_memory", Name: decision.SymbolOrPath, Path: decision.FilePath, FileHash: decision.FileHash},
		Verdict: "EXPLICIT_USER_DECISION",
		Source:  "decision_memory",
	})
	if err != nil {
		return MemoryGuidance{}, false
	}
	text := fmt.Sprintf("[DECISION] %s (hash=%s): %s", decision.SymbolOrPath, shortHash(decision.FileHash), decision.Summary)
	proposal := contract.GuidanceProposal{
		Kind:           "MEMORY_DECISION",
		EvidenceKeys:   []string{canonical.Key},
		Priority:       90,
		TokenEstimate:  max(1, ctxbuild.EstimateTokens(text)),
		DeliveryReason: "memory.explicit_decision_valid_hash_ttl",
	}
	return MemoryGuidance{Proposal: proposal, Evidence: []contract.EvidenceRef{canonical}, Decision: decision}, true
}

func shortHash(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
