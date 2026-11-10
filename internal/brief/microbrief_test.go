package brief

import (
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestMicroBriefProposalIsCompactAndSharesSourceIdentity(t *testing.T) {
	entry := ev("function", "ChargeInvoice", "billing/charge.go", 10, "exact", "VERIFIED", "tested_by:billing/charge_test.go")
	proposal, ok := MicroBriefProposal(retrieve.Result{Evidence: []retrieve.Evidence{
		entry,
		ev("function", "InvoiceConsumer", "billing/consumer.go", 4, "structural", "INFERRED", "calls:ChargeInvoice(import)"),
	}})
	if !ok {
		t.Fatal("expected a compact brief proposal")
	}
	if len(proposal) < 2 || len(proposal) > 4 {
		t.Fatalf("microbrief has %d items, want 2–4: %+v", len(proposal), proposal)
	}
	var foundEntry, foundTest bool
	for _, item := range proposal {
		if item.Section != ctxbuild.TaskBrief || item.Source != "brief" || item.Role == "" {
			t.Errorf("proposal item missing metadata: %+v", item)
		}
		foundEntry = foundEntry || (item.Key == entry.Key() && item.Fingerprint == entry.Fingerprint() && strings.Contains(item.Text, "entrypoint"))
		foundTest = foundTest || (strings.Contains(item.Key, "brief|test|") && strings.Contains(item.Text, "charge_test.go"))
	}
	if !foundEntry || !foundTest {
		t.Fatalf("expected entry and test orientation with stable source IDs: %+v", proposal)
	}
}

func TestMicroBriefProposalAbstainsWhenEvidenceIsThin(t *testing.T) {
	proposal, ok := MicroBriefProposal(retrieve.Result{Evidence: []retrieve.Evidence{
		ev("function", "MaybeRun", "a/run.go", 1, "lexical", "INFERRED"),
	}})
	if ok || len(proposal) != 0 {
		t.Fatalf("thin evidence should not produce a microbrief: ok=%v items=%+v", ok, proposal)
	}
}

func TestMicroBriefProposalPacksBesideCode(t *testing.T) {
	entry := ev("function", "ChargeInvoice", "billing/charge.go", 10, "exact", "VERIFIED", "tested_by:billing/charge_test.go")
	briefItems, ok := MicroBriefProposal(retrieve.Result{Evidence: []retrieve.Evidence{entry}})
	if !ok {
		t.Fatal("expected a microbrief proposal")
	}
	scope := contract.Scope{RepoRoot: "/repo", WorktreeID: "worktree", WorktreeRevision: "rev1"}
	registry := contract.NewEvidenceRegistry[ctxbuild.Item](scope)
	ref, _, err := registry.Add(contract.EvidenceRef{
		Key: "charge-invoice", Path: entry.Hit.Path, Fingerprint: entry.Fingerprint(),
		LineStart: entry.Hit.Line, LineEnd: entry.Hit.EndLine,
		Confidence: contract.ConfidenceVerified, Freshness: contract.FreshnessCurrent,
		CostEstimate: 50, Proof: contract.VerificationProof{Kind: contract.VerificationIndex, Reference: "symbol:charge"},
	}, ctxbuild.Item{
		Section: ctxbuild.Code, Key: entry.Key(), Fingerprint: entry.Fingerprint(),
		Text: "func ChargeInvoice(ctx Context) error { return charge(ctx) }", Source: "code", Role: "snippet",
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := contract.OutputPlan{Scope: scope, BudgetTokens: 180, MaxItems: 2, Sections: []contract.SectionAllocation{
		{Section: ctxbuild.TaskBrief, BudgetTokens: 110, MaxItems: 1, EvidenceKeys: []string{ref.Key}},
		{Section: ctxbuild.Code, BudgetTokens: 110, MaxItems: 1, EvidenceKeys: []string{ref.Key}},
	}}
	packed := ctxbuild.Pack(plan, registry, func(item contract.EvidenceEntry[ctxbuild.Item]) []ctxbuild.Item { return []ctxbuild.Item{item.Value} }, []ctxbuild.GuidanceProposal{{
		Proposal: contract.GuidanceProposal{Kind: ctxbuild.TaskBrief, EvidenceKeys: []string{ref.Key}, TokenEstimate: 80, DeliveryReason: "microbrief"},
		Items:    briefItems,
	}}, nil)
	if packed.Text == "" || packed.EstimatedTokens > plan.BudgetTokens {
		t.Fatalf("hybrid output missing or over budget: %+v", packed)
	}
	for _, want := range []string{ctxbuild.TaskBrief, ctxbuild.Code, "func ChargeInvoice(ctx Context)", "charge_test.go"} {
		if !strings.Contains(packed.Text, want) {
			t.Errorf("hybrid output missing %q:\n%s", want, packed.Text)
		}
	}
	if strings.Contains(packed.Text, "[CONFIRMED] entrypoint") {
		t.Fatalf("duplicated entrypoint summary should yield to code:\n%s", packed.Text)
	}
}
