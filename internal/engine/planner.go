package engine

import (
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// PlanInput joins the existing V3 decision with a scoped task snapshot and
// registry. BudgetTokens is an additional cap; the effective cap never
// exceeds Decision.Budget.
type PlanInput struct {
	Task         contract.TaskContext[TaskSignals]
	Decision     Decision
	Evidence     *contract.EvidenceRegistry[retrieve.Evidence]
	BudgetTokens int
}

// Planner creates a deterministic dry-run allocation. Its zero value is
// ready to use; it does not retrieve, persist or render evidence.
type Planner struct{}

// Plan converts an existing V3 decision into phase/session-scoped section
// allocations under one token and item budget.
func (Planner) Plan(input PlanInput) contract.OutputPlan {
	task := input.Task
	scope := contract.NormalizeScope(task.Scope)
	phase := task.Phase
	switch phase {
	case contract.PhaseExplore, contract.PhaseDecide, contract.PhaseEdit, contract.PhaseVerify, contract.PhaseResume, contract.PhaseUnknown:
	default:
		phase = contract.PhaseUnknown
	}
	capability := task.ExploreCapability
	if capability != contract.CapabilityAvailable && capability != contract.CapabilityUnsupported {
		capability = contract.CapabilityUnverified
	}
	plan := contract.OutputPlan{
		Scope:             scope,
		Action:            string(input.Decision.Action),
		Phase:             phase,
		BudgetTokens:      minNonNegative(input.BudgetTokens, input.Decision.Budget),
		MaxItems:          max(0, input.Decision.MaxItems),
		Sections:          []contract.SectionAllocation{},
		NotDelivered:      []contract.PlanReason{},
		ExploreCapability: capability,
	}
	if !knownAction(input.Decision.Action) {
		plan.BudgetTokens = 0
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "decision_unknown"})
		return plan
	}
	if input.Decision.Action == ActionAbstain {
		plan.BudgetTokens = 0
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "decision_abstain"})
		return plan
	}
	if plan.BudgetTokens == 0 {
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "budget_zero"})
		return plan
	}
	if plan.MaxItems == 0 {
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "item_budget_zero"})
		return plan
	}
	if input.Evidence == nil || !scope.Equal(input.Evidence.Scope()) {
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "evidence_scope_mismatch"})
		return plan
	}

	registry := make(map[string]contract.EvidenceEntry[retrieve.Evidence])
	registryByID := make(map[string][]string)
	for _, entry := range input.Evidence.Snapshot() {
		registry[entry.Ref.Key] = entry
		if entry.Value.ID != "" {
			registryByID[entry.Value.ID] = append(registryByID[entry.Value.ID], entry.Ref.Key)
		}
	}
	for id := range registryByID {
		sort.Strings(registryByID[id])
	}
	taskEvidence := make(map[string]contract.EvidenceRef)
	for _, ref := range task.Evidence {
		if ref.Key == "" {
			continue
		}
		if _, exists := taskEvidence[ref.Key]; !exists {
			taskEvidence[ref.Key] = ref
		}
	}
	if len(taskEvidence) == 0 {
		plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "no_evidence"})
		return plan
	}

	remainingTokens, remainingItems := plan.BudgetTokens, plan.MaxItems
	selected := make([]string, 0, min(len(taskEvidence), remainingItems))
	selectedCost := 0
	keys := make([]string, 0, len(taskEvidence))
	for key := range taskEvidence {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, inRegistry := registry[key]; !inRegistry {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "evidence_missing", Key: key})
		}
	}
	considered := make(map[string]struct{})
	for _, choice := range input.Decision.Included {
		if !input.Decision.RetrievalRan {
			break
		}
		for _, key := range registryByID[choice.ID] {
			if _, done := considered[key]; done {
				continue
			}
			considered[key] = struct{}{}
			if _, inTask := taskEvidence[key]; !inTask {
				continue
			}
			ref := registry[key]
			if ref.Ref.Freshness == contract.FreshnessStale {
				plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "evidence_stale", Key: key})
				continue
			}
			if ref.Ref.CostEstimate <= 0 {
				plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "evidence_cost_unknown", Key: key})
				continue
			}
			if ref.Ref.CostEstimate > remainingTokens {
				plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "evidence_over_budget", Key: key})
				continue
			}
			if remainingItems == 0 {
				plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "item_budget_full", Key: key})
				continue
			}
			selected = append(selected, key)
			selectedCost += ref.Ref.CostEstimate
			remainingTokens -= ref.Ref.CostEstimate
			remainingItems--
		}
	}
	if len(selected) > 0 {
		plan.Sections = append(plan.Sections, contract.SectionAllocation{
			Section:      contract.SectionEvidence,
			BudgetTokens: selectedCost,
			MaxItems:     len(selected),
			EvidenceKeys: selected,
		})
	}

	proposals := append([]contract.GuidanceProposal(nil), task.Guidance...)
	sort.SliceStable(proposals, func(i, j int) bool {
		if proposals[i].Priority != proposals[j].Priority {
			return proposals[i].Priority > proposals[j].Priority
		}
		if proposals[i].Kind != proposals[j].Kind {
			return proposals[i].Kind < proposals[j].Kind
		}
		if proposals[i].DeliveryReason != proposals[j].DeliveryReason {
			return proposals[i].DeliveryReason < proposals[j].DeliveryReason
		}
		return strings.Join(sortedCopy(proposals[i].EvidenceKeys), "\x00") < strings.Join(sortedCopy(proposals[j].EvidenceKeys), "\x00")
	})
	for _, proposal := range proposals {
		if strings.TrimSpace(proposal.Kind) == "" || proposal.TokenEstimate <= 0 {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "proposal_invalid", Kind: proposal.Kind})
			continue
		}
		if proposal.Phase != "" && proposal.Phase != phase {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "phase_mismatch", Kind: proposal.Kind})
			continue
		}
		if !observationSatisfied(task.Observability, proposal.RequiresObservation) {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "observation_unverified", Kind: proposal.Kind})
			continue
		}
		proposalKeys := sortedCopy(proposal.EvidenceKeys)
		if len(proposalKeys) == 0 {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "proposal_without_evidence", Kind: proposal.Kind})
			continue
		}
		allAvailable := true
		for _, key := range proposalKeys {
			if _, inTask := taskEvidence[key]; !inTask {
				allAvailable = false
				break
			}
			if entry, inRegistry := registry[key]; !inRegistry || entry.Ref.Freshness == contract.FreshnessStale {
				allAvailable = false
				break
			}
		}
		if !allAvailable {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "proposal_evidence_unavailable", Kind: proposal.Kind})
			continue
		}
		if len(proposalKeys) != len(uniqueStrings(proposalKeys)) {
			proposalKeys = uniqueStrings(proposalKeys)
		}
		if proposal.TokenEstimate > remainingTokens {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "proposal_over_budget", Kind: proposal.Kind})
			continue
		}
		if remainingItems == 0 {
			plan.NotDelivered = append(plan.NotDelivered, contract.PlanReason{Code: "item_budget_full", Kind: proposal.Kind})
			continue
		}
		plan.Sections = append(plan.Sections, contract.SectionAllocation{
			Section:      proposal.Kind,
			BudgetTokens: proposal.TokenEstimate,
			MaxItems:     1,
			EvidenceKeys: proposalKeys,
		})
		remainingTokens -= proposal.TokenEstimate
		remainingItems--
	}
	return plan
}

func minNonNegative(a, b int) int {
	if a < 0 || b < 0 {
		return 0
	}
	return min(a, b)
}

func knownAction(action Action) bool {
	switch action {
	case ActionAbstain, ActionLean, ActionBrief, ActionExploreAvailable, ActionClarify, ActionVerify:
		return true
	default:
		return false
	}
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	unique := values[:1]
	for _, value := range values[1:] {
		if value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

func observationSatisfied(observability contract.Observability, required contract.ObservationEvent) bool {
	switch required {
	case "":
		return true
	case contract.ObservationToolEvents:
		return observability.ToolEvents == contract.ObservationSeen
	case contract.ObservationPreCompact:
		return observability.PreCompact == contract.ObservationSeen
	default:
		return false
	}
}
