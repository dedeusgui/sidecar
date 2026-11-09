package ctxbuild

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
)

// GuidanceProposal is rendered only when the OutputPlan allocated a matching
// section and every referenced evidence key is present and current in the
// scoped registry.
type GuidanceProposal struct {
	Proposal contract.GuidanceProposal
	Items    []Item
}

// PackResult is the single rendered output of a global packing pass.
type PackResult struct {
	Text            string
	Items           []Item
	EstimatedTokens int
	NotDelivered    []contract.PlanReason
}

type packSection struct {
	budget   int
	maxUnits int
	groups   []map[string]struct{}
}

type packCandidate struct {
	item     Item
	unit     string
	sequence int
}

// Pack maps the plan's selected evidence keys and grounded guidance proposals
// into one sanitized context block. The mapper adapts caller-owned evidence
// values to one or more rendered items; the registry remains the shared source
// of identity, freshness and worktree scope.
func Pack[T any](
	plan contract.OutputPlan,
	evidence *contract.EvidenceRegistry[T],
	mapEvidence func(contract.EvidenceEntry[T]) []Item,
	proposals []GuidanceProposal,
	seen Seen,
) PackResult {
	result := PackResult{NotDelivered: []contract.PlanReason{}}
	if plan.BudgetTokens <= 0 {
		result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "budget_zero"})
		return result
	}
	if plan.MaxItems <= 0 {
		result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "item_budget_zero"})
		return result
	}
	if evidence == nil {
		result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "evidence_registry_missing"})
		return result
	}
	if !contract.NormalizeScope(plan.Scope).Equal(evidence.Scope()) {
		result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "evidence_scope_mismatch"})
		return result
	}

	sections := make(map[string]*packSection)
	var candidates []packCandidate
	sequence := 0
	for _, allocation := range plan.Sections {
		if !knownPackSection(allocation.Section) {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "section_unsupported", Kind: allocation.Section})
			continue
		}
		if allocation.BudgetTokens <= 0 || allocation.MaxItems <= 0 {
			continue
		}
		section := sections[allocation.Section]
		if section == nil {
			section = &packSection{}
			sections[allocation.Section] = section
		}
		section.budget += allocation.BudgetTokens
		section.maxUnits += allocation.MaxItems
		group := make(map[string]struct{}, len(allocation.EvidenceKeys))
		for _, key := range allocation.EvidenceKeys {
			group[key] = struct{}{}
			entry, ok := evidence.Lookup(key)
			if !ok || entry.Ref.Freshness == contract.FreshnessStale || mapEvidence == nil {
				continue
			}
			for _, item := range mapEvidence(entry) {
				if item.Section != "" && item.Section != allocation.Section {
					continue
				}
				item.Section = allocation.Section
				if item.Key == "" {
					item.Key = entry.Ref.Key
				}
				if item.Fingerprint == "" {
					item.Fingerprint = entry.Ref.Fingerprint
				}
				if item.Key == "" {
					continue
				}
				candidates = append(candidates, packCandidate{
					item: item, unit: "evidence:" + allocation.Section + ":" + key, sequence: sequence,
				})
				sequence++
			}
		}
		section.groups = append(section.groups, group)
	}

	orderedProposals := append([]GuidanceProposal(nil), proposals...)
	sort.SliceStable(orderedProposals, func(i, j int) bool {
		left, right := orderedProposals[i].Proposal, orderedProposals[j].Proposal
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		leftKeys, rightKeys := strings.Join(sortedKeys(left.EvidenceKeys), "\x00"), strings.Join(sortedKeys(right.EvidenceKeys), "\x00")
		if leftKeys != rightKeys {
			return leftKeys < rightKeys
		}
		return left.DeliveryReason < right.DeliveryReason
	})
	for _, proposal := range orderedProposals {
		metadata := proposal.Proposal
		section := sections[metadata.Kind]
		if section == nil || metadata.TokenEstimate <= 0 || len(metadata.EvidenceKeys) == 0 {
			continue
		}
		valid := false
		keys := sortedKeys(metadata.EvidenceKeys)
		for _, group := range section.groups {
			if len(group) != len(keys) {
				continue
			}
			allAllocated := true
			for _, key := range keys {
				if _, allocated := group[key]; !allocated {
					allAllocated = false
					break
				}
			}
			if allAllocated {
				valid = true
				break
			}
		}
		if valid {
			for _, key := range keys {
				entry, ok := evidence.Lookup(key)
				if !ok || entry.Ref.Freshness == contract.FreshnessStale {
					valid = false
					break
				}
			}
		}
		if !valid {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "proposal_evidence_unavailable", Kind: metadata.Kind})
			continue
		}
		unit := "guidance:" + metadata.Kind + ":" + strings.Join(keys, "\x00") + ":" + metadata.DeliveryReason
		for index, item := range proposal.Items {
			if item.Section != "" && item.Section != metadata.Kind {
				continue
			}
			item.Section = metadata.Kind
			if item.Key == "" {
				item.Key = unit + ":" + fmt.Sprint(index)
			}
			if item.Fingerprint == "" {
				sum := sha256.Sum256([]byte(item.Text + "\x00" + item.Qualifier + "\x00" + item.Provenance))
				item.Fingerprint = fmt.Sprintf("sha256:%x", sum[:])
			}
			if item.Source == "" {
				item.Source = "guidance"
			}
			candidates = append(candidates, packCandidate{item: item, unit: unit, sequence: sequence})
			sequence++
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftPriority, rightPriority := packingPriority(left.item), packingPriority(right.item)
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		if sectionRank(left.item.Section) != sectionRank(right.item.Section) {
			return sectionRank(left.item.Section) < sectionRank(right.item.Section)
		}
		if left.item.Key != right.item.Key {
			return left.item.Key < right.item.Key
		}
		if left.item.Fingerprint != right.item.Fingerprint {
			return left.item.Fingerprint < right.item.Fingerprint
		}
		if left.item.Source != right.item.Source {
			return left.item.Source < right.item.Source
		}
		if left.item.Text != right.item.Text {
			return left.item.Text < right.item.Text
		}
		return left.sequence < right.sequence
	})

	base := openMarker + "\n" + preamble + "\n" + closeMarker
	if EstimateTokens(base) > plan.BudgetTokens {
		result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "frame_over_budget"})
		return result
	}

	selected := make(map[string][]Item)
	selectedIdentities := make(map[string]struct{})
	selectedUnits := make(map[string]struct{})
	sectionUnits := make(map[string]map[string]struct{})
	for _, candidate := range candidates {
		item := sanitizePackItem(candidate.item)
		if item.Text == "" || seen != nil && seen(item.Key, item.Fingerprint) {
			continue
		}
		identity := item.Key + "\x00" + item.Fingerprint
		if _, duplicate := selectedIdentities[identity]; duplicate {
			continue
		}
		section := sections[item.Section]
		if section == nil || section.budget <= 0 || section.maxUnits <= 0 {
			continue
		}
		if sectionUnits[item.Section] == nil {
			sectionUnits[item.Section] = make(map[string]struct{})
		}
		_, unitAlreadySelected := selectedUnits[candidate.unit]
		if !unitAlreadySelected && len(selectedUnits) >= plan.MaxItems {
			continue
		}
		if _, inSection := sectionUnits[item.Section][candidate.unit]; !inSection && len(sectionUnits[item.Section]) >= section.maxUnits {
			continue
		}

		trial := cloneSelected(selected)
		trial[item.Section] = append(trial[item.Section], item)
		sectionText := renderPackSection(item.Section, trial[item.Section])
		if EstimateTokens(sectionText) > section.budget {
			continue
		}
		block := renderPack(trial)
		if EstimateTokens(block) > plan.BudgetTokens {
			continue
		}
		selected = trial
		selectedIdentities[identity] = struct{}{}
		if !unitAlreadySelected {
			selectedUnits[candidate.unit] = struct{}{}
			sectionUnits[item.Section][candidate.unit] = struct{}{}
		}
		result.Items = append(result.Items, item)
	}

	if len(result.Items) == 0 {
		result.Items = nil
		return result
	}
	result.Text = renderPack(selected)
	result.EstimatedTokens = EstimateTokens(result.Text)
	result.Items = result.Items[:0]
	for _, section := range canonicalPackSections {
		result.Items = append(result.Items, selected[section]...)
	}
	return result
}

func knownPackSection(section string) bool {
	_, ok := sectionRanks[section]
	return ok
}

var sectionRanks = map[string]int{
	TaskBrief:         0,
	Code:              1,
	V35Relations:      2,
	Relations:         2,
	ContractsAndTests: 3,
	Contracts:         3,
	OpenDecision:      4,
	TaskCheck:         4,
	ChangeImpact:      5,
	Concerns:          5,
	Verification:      6,
	Gaps:              7,
	Git:               8,
	Memory:            8,
	Session:           8,
	Explore:           8,
}

func sectionRank(section string) int {
	if rank, ok := sectionRanks[section]; ok {
		return rank
	}
	return len(sectionRanks)
}

func packingPriority(item Item) int {
	if item.Priority != 0 {
		return item.Priority
	}
	switch item.Section {
	case OpenDecision, TaskCheck:
		return 120
	case Code:
		return 100
	case Gaps:
		return 95
	case ContractsAndTests, Contracts:
		return 90
	case TaskBrief:
		return 80
	case Verification:
		return 75
	case ChangeImpact, Concerns:
		return 65
	case V35Relations, Relations:
		return 60
	case Explore:
		return 50
	case Memory, Session:
		return 40
	case Git:
		return 20
	default:
		return 0
	}
}

func sanitizePackItem(item Item) Item {
	limit := limitFor(item.Section)
	qualifier := sanitizeTo(item.Qualifier, 100)
	provenance := sanitizeTo(item.Provenance, 120)
	var suffix strings.Builder
	if qualifier != "" {
		suffix.WriteString(" [")
		suffix.WriteString(qualifier)
		suffix.WriteByte(']')
	}
	if provenance != "" {
		suffix.WriteString(" (source: ")
		suffix.WriteString(provenance)
		suffix.WriteByte(')')
	}
	suffixText := suffix.String()
	bodyLimit := limit - utf8.RuneCountInString(suffixText)
	if suffixText != "" {
		bodyLimit--
	}
	if bodyLimit < 0 {
		bodyLimit = 0
	}
	if bodyLimit > 0 {
		// sanitizeTo adds one ellipsis rune when it truncates, so reserve it here
		// to keep the final item within the documented per-section limit.
		item.Text = sanitizeTo(item.Text, bodyLimit-1)
	} else {
		item.Text = ""
	}
	item.Text = strings.TrimSpace(item.Text + suffixText)
	return item
}

func renderPackSection(section string, items []Item) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(section)
	b.WriteByte('\n')
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func renderPack(sections map[string][]Item) string {
	var lines []string
	for _, section := range canonicalPackSections {
		if content := renderPackSection(section, sections[section]); content != "" {
			lines = append(lines, strings.TrimSuffix(content, "\n"))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(openMarker)
	b.WriteByte('\n')
	b.WriteString(preamble)
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(closeMarker)
	return b.String()
}

var canonicalPackSections = []string{
	TaskBrief, Code, V35Relations, Relations, ContractsAndTests, Contracts,
	OpenDecision, TaskCheck, ChangeImpact, Concerns, Verification, Gaps,
	Git, Memory, Session, Explore,
}

func cloneSelected(in map[string][]Item) map[string][]Item {
	out := make(map[string][]Item, len(in)+1)
	for section, items := range in {
		out[section] = append([]Item(nil), items...)
	}
	return out
}

func sortedKeys(keys []string) []string {
	out := append([]string(nil), keys...)
	sort.Strings(out)
	return out
}
