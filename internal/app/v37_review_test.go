package app

import (
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// A contract choice suppresses the TaskBrief test orientation only when the contract itself names a
// test file. A contract about a non-test file (an API doc, a schema) must not hide the test pointer.
func TestV37ContractOnNonTestFileDoesNotSuppressTestOrientation(t *testing.T) {
	testItem := ctxbuild.Item{Section: ctxbuild.TaskBrief, Key: "brief|test|router_test.go", Fingerprint: "router_test.go",
		Text: "test file references the router", Role: "test"}
	proposals := func() []ctxbuild.GuidanceProposal {
		return []ctxbuild.GuidanceProposal{{
			Proposal: contract.GuidanceProposal{Kind: ctxbuild.TaskBrief, EvidenceKeys: []string{"k1"}},
			Items:    []ctxbuild.Item{testItem},
		}}
	}

	nonTest := []engine.Choice{{ID: "api-doc", Source: "contract", Path: "docs/api.md"}}
	got, _ := v37PrepareProposals(proposals(), nonTest, map[string]string{}, nil)
	if len(got) != 1 || len(got[0].Items) != 1 {
		t.Fatalf("a contract on docs/api.md must not suppress the test orientation, got=%+v", got)
	}

	withTest := []engine.Choice{{ID: "router-test", Source: "contract", Path: "router/router_test.go"}}
	got, _ = v37PrepareProposals(proposals(), withTest, map[string]string{}, nil)
	if len(got) != 0 {
		t.Fatalf("a contract that names a test file keeps suppressing the duplicate test orientation, got=%+v", got)
	}
}

// V3.7 renders only the most actionable relation annotations per item: test pointers and call edges
// before containment. The fingerprint and the stored relations are untouched, so session dedupe and
// later stages see the same evidence.
func TestV37RenderShowsMostActionableRelationsOnly(t *testing.T) {
	ev := retrieve.Evidence{
		ID:        "place-order",
		Hit:       store.SymbolHit{Path: "orders/order_service.go", Kind: "method", Name: "PlaceOrder", Line: 12, EndLine: 15},
		Section:   ctxbuild.Code,
		Relations: []string{"contains:Length", "cochange:orders/order_service_test.go(3)", "tested_by:orders/order_service_test.go"},
	}
	item := v37EvidenceItem(contract.EvidenceEntry[retrieve.Evidence]{Value: ev})
	if !strings.Contains(item.Text, "tested_by:orders/order_service_test.go") {
		t.Fatalf("the test pointer must survive the relation cap, got %q", item.Text)
	}
	if strings.Contains(item.Text, "contains:Length") {
		t.Fatalf("containment must be the first relation dropped, got %q", item.Text)
	}
	rendered := 0
	for _, prefix := range []string{"tested_by:", "called_by:", "calls:", "member_of:", "cochange:", "contains:"} {
		rendered += strings.Count(item.Text, prefix)
	}
	if rendered > v37MaxRelations {
		t.Fatalf("rendered %d relations, want at most v37MaxRelations=%d: %q", rendered, v37MaxRelations, item.Text)
	}
	if item.Fingerprint != ev.Fingerprint() {
		t.Fatal("fingerprint must not depend on rendered relations")
	}
	if len(ev.Relations) != 3 {
		t.Fatal("the caller's evidence relations must not be mutated")
	}
}
