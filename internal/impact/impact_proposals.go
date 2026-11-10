package impact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

const maxImpactProofSourceBytes = 8 << 20

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
		freshness := contract.FreshnessUnknown
		proof := contract.VerificationProof{}
		var value retrieve.Evidence
		switch finding.Status {
		case Confirmed, Conflict:
			if sourceRef, sourceEvidence, current := currentImpactProof(finding, scope, registry); current {
				confidence = contract.ConfidenceVerified
				freshness = contract.FreshnessCurrent
				proof = sourceRef.Proof
				value = sourceEvidence
			}
		case Unknown:
			confidence = contract.ConfidenceUnknown
		}
		if value.Hit.Path == "" {
			value = retrieve.Evidence{
				ID: key,
				Hit: store.SymbolHit{Kind: "impact_" + finding.Kind, Name: finding.Symbol,
					Path: finding.Path, Line: finding.Line},
				Verdict: string(finding.Status), Source: "impact_index",
			}
		}
		ref := contract.EvidenceRef{
			Key:          key,
			Path:         finding.Path,
			LineStart:    finding.Line,
			LineEnd:      finding.Line,
			Fingerprint:  value.Hit.FileHash,
			Origin:       "impact_index",
			Confidence:   confidence,
			Freshness:    freshness,
			CostEstimate: max(1, estimateFinding(finding)),
			Proof:        proof,
		}
		value.ID = key
		canonical, _, err := registry.Add(ref, value)
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

func currentImpactProof(finding Finding, scope contract.Scope, registry *contract.EvidenceRegistry[retrieve.Evidence]) (contract.EvidenceRef, retrieve.Evidence, bool) {
	if finding.Path == "" || scope.RepoRoot == "" {
		return contract.EvidenceRef{}, retrieve.Evidence{}, false
	}
	rel := filepath.Clean(filepath.FromSlash(finding.Path))
	if !filepath.IsLocal(rel) {
		return contract.EvidenceRef{}, retrieve.Evidence{}, false
	}
	abs := filepath.Join(scope.RepoRoot, rel)
	if !repo.Contained(scope.RepoRoot, abs) {
		return contract.EvidenceRef{}, retrieve.Evidence{}, false
	}
	source, reason := repo.ReadSource(abs, maxImpactProofSourceBytes)
	if reason != "" {
		return contract.EvidenceRef{}, retrieve.Evidence{}, false
	}
	digest := sha256.Sum256(source)
	currentHash := hex.EncodeToString(digest[:])
	for _, entry := range registry.Snapshot() {
		hit := entry.Value.Hit
		if filepath.Clean(filepath.FromSlash(hit.Path)) != rel || hit.ID <= 0 || hit.FileHash != currentHash ||
			entry.Ref.Freshness != contract.FreshnessCurrent || entry.Ref.Confidence != contract.ConfidenceVerified || !entry.Ref.Proof.Valid() {
			continue
		}
		if finding.Line > 0 && hit.Line != finding.Line || finding.Symbol != "" && hit.Name != finding.Symbol {
			continue
		}
		if !strings.HasSuffix(entry.Ref.Proof.Reference, fmt.Sprintf(":%d:%s", hit.ID, currentHash)) {
			continue
		}
		return entry.Ref, entry.Value, true
	}
	return contract.EvidenceRef{}, retrieve.Evidence{}, false
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
