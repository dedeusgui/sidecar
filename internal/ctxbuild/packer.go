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
	maxItems int
	groups   []map[string]struct{}
}

type packCandidate struct {
	item     Item
	sequence int
}

// PackOptions selects packer refinements. The zero value is the frozen V3.5 behaviour.
type PackOptions struct {
	// Ranked breaks priority ties by arrival order (the plan's evidence order, then each
	// proposal's item order) instead of by item key, so retrieval rank decides both what
	// survives a tight budget and the order the agent reads.
	Ranked bool
}

// Pack is PackWith under the V3.5 options.
func Pack[T any](
	plan contract.OutputPlan,
	evidence *contract.EvidenceRegistry[T],
	mapEvidence func(contract.EvidenceEntry[T]) []Item,
	proposals []GuidanceProposal,
	seen Seen,
) PackResult {
	return PackWith(PackOptions{}, plan, evidence, mapEvidence, proposals, seen)
}

// PackItemTokens is what Pack charges for one item: its sanitized text with the qualifier and
// provenance suffix, plus its body lines. Planners must budget with it, not with a metadata
// estimate, or an allocated item is dropped at packing time.
func PackItemTokens(item Item) int {
	return EstimateTokens(renderPackItem(sanitizePackItem(item)))
}

// PackWith maps the plan's selected evidence keys and grounded guidance proposals
// into one sanitized context block. The mapper adapts caller-owned evidence
// values to one or more rendered items; the registry remains the shared source
// of identity, freshness and worktree scope.
func PackWith[T any](
	options PackOptions,
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
		sectionName := canonicalPackSection(allocation.Section)
		if !knownPackSection(sectionName) {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "section_unsupported", Kind: allocation.Section})
			continue
		}
		if allocation.BudgetTokens <= 0 || allocation.MaxItems <= 0 {
			continue
		}
		section := sections[sectionName]
		if section == nil {
			section = &packSection{}
			sections[sectionName] = section
		}
		section.budget += allocation.BudgetTokens
		section.maxItems += allocation.MaxItems
		group := make(map[string]struct{}, len(allocation.EvidenceKeys))
		for _, key := range allocation.EvidenceKeys {
			group[key] = struct{}{}
			entry, ok := evidence.Lookup(key)
			if !ok || entry.Ref.Freshness == contract.FreshnessStale || mapEvidence == nil {
				continue
			}
			for _, item := range mapEvidence(entry) {
				itemSection := canonicalPackSection(item.Section)
				if item.Section != "" && itemSection != sectionName {
					continue
				}
				item.Section = sectionName
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
					item: item, sequence: sequence,
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
		sectionName := canonicalPackSection(metadata.Kind)
		section := sections[sectionName]
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
		unit := "guidance:" + sectionName + ":" + strings.Join(keys, "\x00") + ":" + metadata.DeliveryReason
		for index, item := range proposal.Items {
			if item.Section != "" && canonicalPackSection(item.Section) != sectionName {
				continue
			}
			item.Section = sectionName
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
			candidates = append(candidates, packCandidate{item: item, sequence: sequence})
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
		if options.Ranked {
			return left.sequence < right.sequence
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
	sectionItemCounts := make(map[string]int)
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
		if section == nil || section.budget <= 0 || section.maxItems <= 0 {
			continue
		}
		// An allocated item that does not fit is reported, never dropped silently.
		if len(result.Items) >= plan.MaxItems || sectionItemCounts[item.Section] >= section.maxItems {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "pack_item_limit", Kind: item.Section, Key: item.Key})
			continue
		}

		trial := cloneSelected(selected)
		trial[item.Section] = append(trial[item.Section], item)
		sectionText := renderPackSection(item.Section, trial[item.Section])
		if EstimateTokens(sectionText) > section.budget {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "pack_section_over_budget", Kind: item.Section, Key: item.Key})
			continue
		}
		block := renderPack(trial)
		if EstimateTokens(block) > plan.BudgetTokens {
			result.NotDelivered = append(result.NotDelivered, contract.PlanReason{Code: "pack_over_budget", Kind: item.Section, Key: item.Key})
			continue
		}
		selected = trial
		selectedIdentities[identity] = struct{}{}
		sectionItemCounts[item.Section]++
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

func canonicalPackSection(section string) string {
	switch section {
	case "CHANGE_IMPACT":
		return ChangeImpact
	case "MEMORY_DECISION":
		return Memory
	case "SESSION_GUIDANCE":
		return Session
	default:
		return section
	}
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
	if len(item.Body) > 0 {
		body := make([]string, 0, len(item.Body))
		for _, line := range item.Body {
			if line = sanitizeBodyLine(line); line != "" {
				body = append(body, line)
			}
		}
		item.Body = body
	}
	return item
}

// maxBodyLineRunes caps one rendered source line; minified or generated lines are cut, not dropped.
const maxBodyLineRunes = 160

// sanitizeBodyLine renders one "N<tab>source" line as "N| source". Indentation survives (a tab
// becomes two spaces); control characters and forged block markers do not. Every body line is
// emitted behind a fixed indent, so repository text can never start a line of the block.
func sanitizeBodyLine(line string) string {
	number, source, numbered := strings.Cut(line, "\t")
	if !numbered {
		number, source = "", line
	}
	source = strings.TrimRight(source, " \t\r\n")
	indent := 0
indentLoop:
	for _, r := range source {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 2
		default:
			break indentLoop
		}
	}
	text := sanitizeTo(strings.TrimLeft(source, " \t"), maxBodyLineRunes)
	if text == "" {
		return ""
	}
	return sanitizeTo(number, 8) + "| " + strings.Repeat(" ", min(indent, 24)) + text
}

func renderPackItem(item Item) string {
	if len(item.Body) == 0 {
		return item.Text
	}
	return item.Text + "\n    " + strings.Join(item.Body, "\n    ")
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
		b.WriteString(renderPackItem(item))
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
