package impact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Guidance keeps the impact findings as provider payload while exposing a
// budgeted, provenance-linked candidate to the planner. Findings are
// recommendations and evidence; they do not assert that verification ran.
type Guidance struct {
	Proposal contract.GuidanceProposal
	Evidence []contract.EvidenceRef
	Findings []Finding
}

// Propose groups the bounded, task-relevant findings from an actual
// uncommitted diff into one planner proposal. It does not analyze the
// repository, render text, execute tests, or allocate extra output budget.
func Propose(report Report, scope contract.Scope, phase contract.TaskPhase, actualUncommitted, taskRelevant bool, registry *contract.EvidenceRegistry[retrieve.Evidence]) (Guidance, bool) {
	scope = contract.NormalizeScope(scope)
	if !actualUncommitted || !taskRelevant || len(report.Changed) == 0 || len(report.Findings) == 0 {
		return Guidance{}, false
	}
	if registry == nil || !scope.Equal(registry.Scope()) {
		return Guidance{}, false
	}
	if phase == "" || phase == contract.PhaseUnknown {
		phase = contract.PhaseVerify
	}

	findings := selectedFindings(report.Findings)
	refs := make([]contract.EvidenceRef, 0, len(findings))
	keys := make([]string, 0, len(findings))
	for _, finding := range findings {
		identity := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", finding.Kind, finding.Path, finding.Line, finding.Symbol, finding.Status)
		sum := sha256.Sum256([]byte(scopeIdentity(scope) + "\x00" + identity))
		key := "impact:" + hex.EncodeToString(sum[:])[:24]
		confidence := contract.ConfidenceCandidate
		proof := contract.VerificationProof{}
		switch finding.Status {
		case Confirmed, Conflict:
			confidence = contract.ConfidenceVerified
			proof = contract.VerificationProof{Kind: contract.VerificationIndex, Reference: key}
		case Unknown:
			confidence = contract.ConfidenceUnknown
		}
		ref := contract.EvidenceRef{
			Key:          key,
			Path:         finding.Path,
			LineStart:    finding.Line,
			LineEnd:      finding.Line,
			Origin:       "impact_index",
			Confidence:   confidence,
			Freshness:    contract.FreshnessUnknown,
			CostEstimate: max(1, estimateFinding(finding)),
			Proof:        proof,
		}
		canonical, _, err := registry.Add(ref, retrieve.Evidence{
			ID:      key,
			Hit:     store.SymbolHit{Kind: "impact_" + finding.Kind, Name: finding.Symbol, Path: finding.Path, Line: finding.Line},
			Verdict: string(finding.Status),
			Source:  "impact_index",
		})
		if err != nil {
			continue
		}
		refs = append(refs, canonical)
		keys = append(keys, canonical.Key)
	}
	if len(refs) == 0 {
		return Guidance{}, false
	}
	sort.Strings(keys)

	tokens := 24
	for _, finding := range findings {
		tokens += (utf8.RuneCountInString(finding.Text) + 3) / 4
	}
	tokens = min(tokens, 96)
	proposal := contract.GuidanceProposal{
		Kind:           "CHANGE_IMPACT",
		EvidenceKeys:   keys,
		Priority:       45,
		TokenEstimate:  tokens,
		DeliveryReason: "impact.uncommitted_relevant_change",
		Phase:          phase,
	}
	return Guidance{Proposal: proposal, Evidence: refs, Findings: findings}, true
}

func selectedFindings(findings []Finding) []Finding {
	var out []Finding
	counts := map[string]int{}
	for _, finding := range findings {
		switch {
		case finding.Kind == "contract" && finding.Status == Conflict:
			out = append(out, finding)
		case (finding.Kind == "consumer" || finding.Kind == "importer") && counts["consumer"] < 3:
			counts["consumer"]++
			out = append(out, finding)
		case finding.Kind == "test" && strings.Contains(finding.Text, "was not changed") && counts["test"] < 2:
			counts["test"]++
			out = append(out, finding)
		case finding.Kind == "gap" && counts["gap"] < 1:
			counts["gap"]++
			out = append(out, finding)
		}
	}
	return out
}

func estimateFinding(finding Finding) int {
	return max(1, (utf8.RuneCountInString(finding.Text)+3)/4)
}

func scopeIdentity(scope contract.Scope) string {
	scope = contract.NormalizeScope(scope)
	return strings.Join([]string{scope.RepoRoot, scope.WorktreeID, scope.SessionID, scope.WorktreeRevision}, "\x00")
}
