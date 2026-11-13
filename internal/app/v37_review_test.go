package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/core"
	"github.com/dedeusgui/sidecar/internal/explore/service"
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

// V3.6 keeps whole snippets and skips those that do not fit the remaining budget. V3.7 must still
// offer the head of an oversized snippet that V3.6 skipped while budget remains, instead of only
// compacting when V3.6 produced nothing at all.
func TestV37CompactsOversizedSnippetSkippedByV36WhileBudgetRemains(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	scope := contract.NormalizeScope(contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"})
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)

	const path = "app/process.go"
	small := retrieve.Evidence{ID: "small", Hit: store.SymbolHit{Path: path, Kind: "function", Name: "Small", Line: 1, EndLine: 6}, Section: ctxbuild.Code}
	big := retrieve.Evidence{ID: "big", Hit: store.SymbolHit{Path: path, Kind: "function", Name: "Big", Line: 10, EndLine: 80}, Section: ctxbuild.Code}
	for _, ev := range []retrieve.Evidence{small, big} {
		meta := contract.EvidenceRef{Key: ev.Key(), Path: path, Fingerprint: ev.Fingerprint(), Freshness: contract.FreshnessCurrent,
			Confidence: contract.ConfidenceCandidate, LineStart: ev.Hit.Line, LineEnd: ev.Hit.EndLine}
		if _, _, err := registry.Add(meta, ev); err != nil {
			t.Fatal(err)
		}
	}

	expansion := service.ExpansionResult{Explore: core.Result{Files: []core.File{{
		Path: path, Role: "candidate", Snippets: []core.Snippet{
			{Start: 1, End: 6, Text: numberedLines(1, 6)},
			{Start: 10, End: 80, Text: numberedLines(10, 80)},
		},
	}}}}

	fullBig := ctxbuild.Item{Section: ctxbuild.Code, Key: big.Key(), Provenance: fmt.Sprintf("%s:10-80", path), Priority: 115,
		Body: strings.Split(numberedLines(10, 80), "\n")}
	smallOnly := ctxbuild.Item{Section: ctxbuild.Code, Key: small.Key(), Provenance: fmt.Sprintf("%s:1-6", path), Priority: 115,
		Body: strings.Split(numberedLines(1, 6), "\n")}
	// Enough for the small snippet and the 24-line head of the big one, but not for the whole big snippet.
	budget := ctxbuild.SectionFrameTokens(ctxbuild.Code, 1) + ctxbuild.PackItemTokens(smallOnly) +
		ctxbuild.PackItemTokens(fullBig)/2
	if ctxbuild.SectionFrameTokens(ctxbuild.Code, 1)+ctxbuild.PackItemTokens(smallOnly)+ctxbuild.PackItemTokens(fullBig) <= budget {
		t.Fatal("fixture must make the full big snippet unaffordable")
	}

	items, _ := v37ExpansionItems(expansion, registry, budget)
	var haveSmall, haveBigHead bool
	for _, it := range items {
		switch it.Provenance {
		case fmt.Sprintf("%s:1-6", path):
			haveSmall = true
		case fmt.Sprintf("%s:10-80", path):
			haveBigHead = true
			if len(it.Body) != v37MaxSnippetLines {
				t.Fatalf("oversized snippet must be compacted to its %d-line head, got %d lines", v37MaxSnippetLines, len(it.Body))
			}
		}
	}
	if !haveSmall {
		t.Fatalf("the snippet V3.6 keeps must still be delivered, got=%+v", items)
	}
	if !haveBigHead {
		t.Fatalf("the oversized snippet V3.6 skipped must be offered in compacted form while budget remains, got=%+v", items)
	}
	used := ctxbuild.SectionFrameTokens(ctxbuild.Code, 1)
	for _, it := range items {
		used += ctxbuild.PackItemTokens(it)
	}
	if used > budget {
		t.Fatalf("delivered items use %d tokens, over the %d budget", used, budget)
	}
}

// numberedLines renders explore snippet text: "<line>\t<source>" per line, as the hook receives it.
func numberedLines(start, end int) string {
	var b strings.Builder
	for i := start; i <= end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d\tvalue_%d := compute(%d)", i, i, i)
	}
	return b.String()
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
