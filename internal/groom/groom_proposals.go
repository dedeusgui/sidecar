package groom

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Guidance keeps the open decision payload available to the later renderer
// while the planner receives only the typed proposal and its evidence links.
type Guidance struct {
	Proposal contract.GuidanceProposal
	Evidence []contract.EvidenceRef
	Decision Decision
}

// Propose converts a decided open question or safe assumption into a scoped
// planner proposal. It never renders text or changes pending-answer state.
// Source evidence is registered in the caller's shared registry; a hashed
// analysis reference preserves provenance when the gap came from request
// signals without a matching repository location.
func Propose(decision Decision, result retrieve.Result, scope contract.Scope, phase contract.TaskPhase, registry *contract.EvidenceRegistry[retrieve.Evidence]) (Guidance, bool) {
	scope = contract.NormalizeScope(scope)
	if decision.Gap == nil || (decision.Action != Ask && decision.Action != Assume) || decision.Gap.ResolvedByRepo != "" {
		return Guidance{}, false
	}
	if decision.Action == Ask && scope.SessionID == "" {
		return Guidance{}, false
	}
	if registry == nil || !scope.Equal(registry.Scope()) {
		return Guidance{}, false
	}
	if phase == "" {
		phase = contract.PhaseDecide
	}

	gap := decision.Gap
	selected := evidenceForGap(*gap, result)
	refs := make([]contract.EvidenceRef, 0, len(selected)+1)
	keys := make([]string, 0, len(selected)+1)
	for _, ev := range selected {
		ref := contract.EvidenceRef{
			Key:          ev.Key(),
			Fingerprint:  ev.Hit.FileHash,
			Path:         ev.Hit.Path,
			LineStart:    ev.Hit.Line,
			LineEnd:      ev.Hit.EndLine,
			Origin:       nonEmpty(ev.Source, "groom_repository_evidence"),
			Confidence:   contract.ConfidenceCandidate,
			Freshness:    contract.FreshnessUnknown,
			CostEstimate: max(1, ev.TokenEstimate),
		}
		canonical, _, err := registry.Add(ref, ev)
		if err != nil {
			continue
		}
		refs = append(refs, canonical)
		keys = append(keys, canonical.Key)
	}

	syntheticKey := "groom-analysis:" + digest(scopeIdentity(scope)+"\x00"+string(gap.State)+"\x00"+gap.Decision)
	synthetic := contract.EvidenceRef{
		Key:          syntheticKey,
		Origin:       "groom_analysis",
		Confidence:   contract.ConfidenceCandidate,
		Freshness:    contract.FreshnessCurrent,
		CostEstimate: 1,
	}
	canonical, _, err := registry.Add(synthetic, retrieve.Evidence{
		ID:     syntheticKey,
		Hit:    store.SymbolHit{Kind: "guidance_signal", Name: "open_decision"},
		Source: "groom_analysis",
	})
	if err != nil {
		return Guidance{}, false
	}
	refs = append(refs, canonical)
	keys = append(keys, canonical.Key)
	sort.Strings(keys)

	verb := "assume"
	if decision.Action == Ask {
		verb = "ask"
	}
	proposal := contract.GuidanceProposal{
		Kind:           "OPEN_DECISION",
		EvidenceKeys:   keys,
		Priority:       70,
		TokenEstimate:  estimateGuidance(*gap),
		DeliveryReason: "groom." + verb + "." + string(gap.State),
		Phase:          phase,
	}
	return Guidance{Proposal: proposal, Evidence: refs, Decision: decision}, true
}

func evidenceForGap(gap Gap, result retrieve.Result) []retrieve.Evidence {
	var out []retrieve.Evidence
	seen := map[string]bool{}
	for _, ev := range result.Evidence {
		if ev.Note != "" || ev.Hit.Path == "" {
			continue
		}
		location := fmt.Sprintf("%s:%d", ev.Hit.Path, ev.Hit.Line)
		matched := false
		for _, cited := range gap.Evidence {
			if strings.Contains(cited, location) {
				matched = true
				break
			}
		}
		if matched && !seen[ev.Key()] {
			seen[ev.Key()] = true
			out = append(out, ev)
		}
	}
	return out
}

func estimateGuidance(gap Gap) int {
	runes := utf8.RuneCountInString(gap.Decision)
	for _, option := range gap.Options {
		runes += utf8.RuneCountInString(option.Label) + utf8.RuneCountInString(option.Consequence)
	}
	return max(1, 24+(runes+3)/4)
}

func scopeIdentity(scope contract.Scope) string {
	scope = contract.NormalizeScope(scope)
	return strings.Join([]string{scope.RepoRoot, scope.WorktreeID, scope.SessionID, scope.WorktreeRevision}, "\x00")
}

// ScopeFingerprint returns an opaque key for one worktree/revision/session.
func ScopeFingerprint(scope contract.Scope) string {
	return digest(scopeIdentity(scope))
}

// NewScopedState creates fresh grooming counters bound to one contract scope.
func NewScopedState(scope contract.Scope) SessionState {
	return SessionState{ScopeFingerprint: ScopeFingerprint(scope)}
}

// LoadScopedState ignores unscoped, corrupt, or mismatched state rather than
// carrying a pending question or quota into another worktree/session.
func LoadScopedState(path string, scope contract.Scope) SessionState {
	fresh := NewScopedState(scope)
	if path == "" {
		return fresh
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fresh
	}
	var state SessionState
	if json.Unmarshal(data, &state) != nil || state.ScopeFingerprint != fresh.ScopeFingerprint {
		return fresh
	}
	return state
}

// SaveScopedState persists only a state carrying the exact target scope
// fingerprint. Writes are atomic and stay in the caller-selected cache path.
func SaveScopedState(path string, scope contract.Scope, state SessionState) error {
	if path == "" {
		return errors.New("empty scoped grooming state path")
	}
	if state.ScopeFingerprint != ScopeFingerprint(scope) {
		return errors.New("grooming state scope mismatch")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
