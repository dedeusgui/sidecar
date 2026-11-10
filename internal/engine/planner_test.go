package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func plannerFixture(t *testing.T) (PlanInput, string) {
	t.Helper()
	scope := contract.Scope{
		RepoRoot:         "/repo",
		WorktreeID:       "worktree-a",
		SessionID:        "session-a",
		WorktreeRevision: "rev-1",
	}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	ref, inserted, err := registry.Add(contract.EvidenceRef{
		Path: "internal/router.go", Fingerprint: "sha256:route", LineStart: 10, LineEnd: 15,
		Origin: "index", Confidence: contract.ConfidenceVerified,
		Proof:     contract.VerificationProof{Kind: contract.VerificationIndex, Reference: "symbol:1:" + strings.Repeat("a", 64)},
		Freshness: contract.FreshnessCurrent, CostEstimate: 20,
	}, retrieve.Evidence{ID: "evidence-1", Verdict: "VERIFIED"})
	if err != nil || !inserted {
		t.Fatalf("registry Add() = inserted %v, err %v", inserted, err)
	}
	input := PlanInput{
		Task: contract.TaskContext[TaskSignals]{
			Scope:         scope,
			Phase:         contract.PhaseEdit,
			Observability: contract.Observability{ToolEvents: contract.ObservationUnsupported},
			Evidence:      []contract.EvidenceRef{ref},
			Guidance: []contract.GuidanceProposal{
				{Kind: "VERIFY_HINT", EvidenceKeys: []string{ref.Key}, Priority: 4, TokenEstimate: 12, DeliveryReason: "changed_contract", Phase: contract.PhaseVerify},
				{Kind: "BRIEF", EvidenceKeys: []string{ref.Key}, Priority: 5, TokenEstimate: 8, DeliveryReason: "edit_context", Phase: contract.PhaseEdit},
				{Kind: "LOOP_HINT", EvidenceKeys: []string{ref.Key}, Priority: 9, TokenEstimate: 3, DeliveryReason: "observed_loop", RequiresObservation: contract.ObservationToolEvents},
			},
		},
		Decision: Decision{
			Action: ActionBrief, Budget: 60, MaxItems: 4,
			Included: []Choice{{ID: "evidence-1"}}, RetrievalRan: true,
		},
		Evidence:     registry,
		BudgetTokens: 45,
	}
	return input, ref.Key
}

func TestPlannerIsDeterministicAndPhaseAware(t *testing.T) {
	input, key := plannerFixture(t)
	planner := Planner{}
	first := planner.Plan(input)
	second := planner.Plan(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same plan input produced different output:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if first.Phase != contract.PhaseEdit || first.Scope.SessionID != "session-a" {
		t.Fatalf("plan lost task phase/scope: %+v", first)
	}
	if len(first.Sections) != 2 {
		t.Fatalf("sections = %+v, want evidence and phase-eligible BRIEF only", first.Sections)
	}
	if first.Sections[0].Section != contract.SectionEvidence || !reflect.DeepEqual(first.Sections[0].EvidenceKeys, []string{key}) {
		t.Fatalf("evidence allocation = %+v", first.Sections[0])
	}
	if first.Sections[1].Section != "BRIEF" {
		t.Fatalf("second allocation = %+v, want BRIEF", first.Sections[1])
	}
	if first.Sections[0].BudgetTokens+first.Sections[1].BudgetTokens > first.BudgetTokens {
		t.Fatalf("section budgets exceed global budget: %+v", first)
	}
	if !hasPlanReason(first, "phase_mismatch", "VERIFY_HINT") || !hasPlanReason(first, "observation_unverified", "LOOP_HINT") {
		t.Fatalf("missing phase/observation omission reasons: %+v", first.NotDelivered)
	}
}

func TestPlannerZeroBudgetReturnsValidEmptyPlan(t *testing.T) {
	input, _ := plannerFixture(t)
	input.BudgetTokens = 0
	plan := (Planner{}).Plan(input)
	if plan.BudgetTokens != 0 || len(plan.Sections) != 0 || plan.Sections == nil {
		t.Fatalf("zero-budget plan is not a valid empty allocation: %+v", plan)
	}
	if !hasPlanReason(plan, "budget_zero", "") {
		t.Fatalf("zero-budget plan has no reason: %+v", plan.NotDelivered)
	}
}

func TestPlannerPreservesV3EvidenceOrderWhenBudgetIsTight(t *testing.T) {
	input, preferredKey := plannerFixture(t)
	otherRef, inserted, err := input.Evidence.Add(contract.EvidenceRef{
		Path: "a.go", Fingerprint: "sha256:other", LineStart: 1, LineEnd: 2,
		Confidence: contract.ConfidenceCandidate, Freshness: contract.FreshnessCurrent, CostEstimate: 3,
	}, retrieve.Evidence{ID: "evidence-0", Verdict: "INFERRED"})
	if err != nil || !inserted {
		t.Fatalf("second registry Add() = inserted %v, err %v", inserted, err)
	}
	input.Task.Evidence = append(input.Task.Evidence, otherRef)
	input.Decision.Included = []Choice{{ID: "evidence-1"}, {ID: "evidence-0"}}
	input.Decision.MaxItems = 1

	plan := (Planner{}).Plan(input)
	if len(plan.Sections) != 1 || !reflect.DeepEqual(plan.Sections[0].EvidenceKeys, []string{preferredKey}) {
		t.Fatalf("planner did not preserve V3 ranking order: %+v", plan.Sections)
	}
}

func TestPlannerRejectsEvidenceFromAnotherScope(t *testing.T) {
	input, _ := plannerFixture(t)
	for _, test := range []struct {
		name   string
		mutate func(*contract.Scope)
	}{
		{name: "different worktree", mutate: func(scope *contract.Scope) { scope.WorktreeID = "worktree-b" }},
		{name: "different session", mutate: func(scope *contract.Scope) { scope.SessionID = "session-b" }},
		{name: "different revision", mutate: func(scope *contract.Scope) { scope.WorktreeRevision = "rev-2" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := input
			task := copy.Task
			test.mutate(&task.Scope)
			copy.Task = task
			plan := (Planner{}).Plan(copy)
			if len(plan.Sections) != 0 || !hasPlanReason(plan, "evidence_scope_mismatch", "") {
				t.Fatalf("plan mixed scopes: %+v", plan)
			}
		})
	}
}

func TestPlannerClampsGlobalBudgetAndAbstains(t *testing.T) {
	input, _ := plannerFixture(t)
	input.BudgetTokens = 22 // The selected evidence costs 20; the 8-token brief no longer fits.
	plan := (Planner{}).Plan(input)
	if len(plan.Sections) != 1 || plan.Sections[0].BudgetTokens != 20 {
		t.Fatalf("over-budget proposal was allocated: %+v", plan.Sections)
	}
	if plan.BudgetTokens != 22 || plan.Sections[0].BudgetTokens > plan.BudgetTokens {
		t.Fatalf("global budget was not respected: %+v", plan)
	}
	if !hasPlanReason(plan, "proposal_over_budget", "BRIEF") {
		t.Fatalf("missing over-budget reason: %+v", plan.NotDelivered)
	}
	input.Decision.Action = ActionAbstain
	plan = (Planner{}).Plan(input)
	if len(plan.Sections) != 0 || plan.BudgetTokens != 0 {
		t.Fatalf("abstaining decision allocated output: %+v", plan)
	}
}

func TestPlannerFailsClosedOnUnknownDecisionAndPhase(t *testing.T) {
	input, _ := plannerFixture(t)
	input.Decision.Action = Action("unexpected")
	plan := (Planner{}).Plan(input)
	if len(plan.Sections) != 0 || plan.BudgetTokens != 0 || !hasPlanReason(plan, "decision_unknown", "") {
		t.Fatalf("unknown decision did not fail closed: %+v", plan)
	}

	input.Decision.Action = ActionBrief
	input.Task.Phase = contract.TaskPhase("unexpected")
	plan = (Planner{}).Plan(input)
	if plan.Phase != contract.PhaseUnknown {
		t.Fatalf("unknown phase = %q, want %q", plan.Phase, contract.PhaseUnknown)
	}
}

func hasPlanReason(plan contract.OutputPlan, code, kind string) bool {
	for _, reason := range plan.NotDelivered {
		if reason.Code == code && (kind == "" || reason.Kind == kind) {
			return true
		}
	}
	return false
}
