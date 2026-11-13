package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestUnifiedIncludesVersion37(t *testing.T) {
	for _, v := range []string{Version35, Version36, Version37} {
		if !Unified(v) {
			t.Errorf("Unified(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "v1", "v2", Version} {
		if Unified(v) {
			t.Errorf("Unified(%q) = true, want false", v)
		}
	}
}

func TestV37PreferImplementationPreservesMatchedTier(t *testing.T) {
	ev := []retrieve.Evidence{
		{ID: "noise", Hit: store.SymbolHit{Path: "pkg/noise.go", Kind: "function", Name: "GetRecords"}, Matched: []string{"record"}},
		{ID: "doc", Hit: store.SymbolHit{Path: "docs/weighing.md", Kind: "section", Name: "Weighing Records"}, Matched: []string{"weighing", "record"}},
		{ID: "test", Hit: store.SymbolHit{Path: "pkg/weighing_test.go", Kind: "function", Name: "TestWeighing"}, Matched: []string{"weighing", "record"}},
		{ID: "impl", Hit: store.SymbolHit{Path: "pkg/weighing.go", Kind: "function", Name: "SaveWeighing"}, Matched: []string{"weighing", "record"}},
	}
	preferImplementationEvidenceV37(ev)
	got := []string{ev[0].ID, ev[1].ID, ev[2].ID, ev[3].ID}
	want := []string{"impl", "test", "doc", "noise"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferImplementationEvidenceV37 order = %v, want %v", got, want)
	}
}

func TestV37HasCodeAnchorTop8SectionAndTranslationRules(t *testing.T) {
	// 1) Top-8 window: a matching symbol at index 8 (9th item) must NOT anchor.
	var evs []retrieve.Evidence
	for i := 0; i < 8; i++ {
		evs = append(evs, retrieve.Evidence{Hit: store.SymbolHit{Kind: "file", Name: "f.go"}})
	}
	evs = append(evs, retrieve.Evidence{Hit: store.SymbolHit{Kind: "function", Name: "ConfirmWeight"}, Matched: []string{"confirm"}})
	sig := TaskSignals{PromptTerms: 2}
	res := retrieve.Result{Evidence: evs, Trace: retrieve.Trace{Terms: []string{"confirm"}}}
	if hasCodeAnchorV37(sig, res) {
		t.Fatal("hasCodeAnchorV37 should inspect only top 8 evidence items")
	}

	// 2) Section hit allowed only with >= 2 matched terms and PresentRatio >= 0.4.
	sectionRes := retrieve.Result{
		Evidence: []retrieve.Evidence{{
			Hit:     store.SymbolHit{Kind: "section", Name: "Database Migrations"},
			Matched: []string{"database", "migration"},
		}},
		Trace: retrieve.Trace{Terms: []string{"database", "migration"}},
	}
	if !hasCodeAnchorV37(TaskSignals{PresentRatio: 0.5}, sectionRes) {
		t.Fatal("section hit with >=2 matched terms and PresentRatio>=0.4 should anchor")
	}
	if hasCodeAnchorV37(TaskSignals{PresentRatio: 0.3}, sectionRes) {
		t.Fatal("section hit with PresentRatio<0.4 must not anchor")
	}
	sectionRes.Evidence[0].Matched = []string{"database"}
	if hasCodeAnchorV37(TaskSignals{PresentRatio: 0.5}, sectionRes) {
		t.Fatal("section hit with <2 matched terms must not anchor")
	}

	// 3) Translation-only ubiquitous polysemous 1-term match must NOT anchor without cue/software/literal.
	polyRes := retrieve.Result{
		Evidence: []retrieve.Evidence{{
			Hit:         store.SymbolHit{Kind: "function", Name: "CreateOrder"},
			Matched:     []string{"pedido→order"},
			MinSymbolDF: 1,
		}},
		Trace: retrieve.Trace{
			Terms:        []string{"pedido"},
			Translations: map[string][]string{"pedido": {"order"}},
			TotalSymbols: 100,
		},
	}
	if hasCodeAnchorV37(TaskSignals{PromptTerms: 3}, polyRes) {
		t.Fatal("1-term translation onto ubiquitous word 'order' without cue/software must not anchor")
	}

	// 4) Rare non-ubiquitous 1-term translation match anchors.
	rareRes := retrieve.Result{
		Evidence: []retrieve.Evidence{{
			Hit:         store.SymbolHit{Kind: "function", Name: "ConfirmWeight"},
			Matched:     []string{"peso→weight"},
			MinSymbolDF: 1,
		}},
		Trace: retrieve.Trace{
			Terms:        []string{"peso"},
			Translations: map[string][]string{"peso": {"weight"}},
			TotalSymbols: 100,
		},
	}
	if !hasCodeAnchorV37(TaskSignals{PromptTerms: 3}, rareRes) {
		t.Fatal("rare discriminative translated symbol DF should anchor")
	}

	// 5) Translation with >= 2 matched terms anchors even for polysemous words.
	twoTermTrans := retrieve.Result{
		Evidence: []retrieve.Evidence{{
			Hit:         store.SymbolHit{Kind: "function", Name: "CreateOrder"},
			Matched:     []string{"criar→create", "pedido→order"},
			MinSymbolDF: 10,
		}},
		Trace: retrieve.Trace{
			Terms:        []string{"criar", "pedido"},
			Translations: map[string][]string{"criar": {"create"}, "pedido": {"order"}},
			TotalSymbols: 100,
		},
	}
	if !hasCodeAnchorV37(TaskSignals{PromptTerms: 3}, twoTermTrans) {
		t.Fatal(">=2 matched terms on translated symbol should anchor")
	}
}

func TestV37RelevanceGateRules(t *testing.T) {
	// 1) HasAbsentAnchor with weak/1-term abstains.
	rel, _ := relevanceGateV37(TaskSignals{BestSupport: 1, PresentRatio: 0.5, SoftwareTerms: []string{"route"}}, retrieve.Result{
		Evidence: []retrieve.Evidence{{Hit: store.SymbolHit{Kind: "function", Name: "Route"}, Matched: []string{"route"}}},
		Trace:    retrieve.Trace{HasAbsentAnchor: true, TotalSymbols: 100},
	})
	if rel != Irrelevant {
		t.Fatalf("HasAbsentAnchor with BestSupport=1 must be Irrelevant, got %s", rel)
	}

	// 2) Weak 1-word generic match must NOT fall through to Ambiguous.
	rel, _ = relevanceGateV37(TaskSignals{BestSupport: 1, PresentRatio: 0.5, PromptTerms: 2}, retrieve.Result{
		Evidence: []retrieve.Evidence{{Hit: store.SymbolHit{Kind: "function", Name: "OpenList"}, Matched: []string{"list"}, MinSymbolDF: 1, MaxSalience: 8.0}},
		Trace:    retrieve.Trace{TotalSymbols: 100},
	})
	if rel != Irrelevant {
		t.Fatalf("weak 1-word generic match must not fall through to Ambiguous, got %s", rel)
	}

	// 3) Terse rare-domain match passes when !HasAbsentAnchor, PromptTerms <= 4, and >=50% concepts present.
	rel, _ = relevanceGateV37(TaskSignals{BestSupport: 1, PresentRatio: 0.5, PromptTerms: 4, Concepts: 2}, retrieve.Result{
		Evidence: []retrieve.Evidence{{Hit: store.SymbolHit{Kind: "function", Name: "ConfirmWeight"}, Matched: []string{"peso→weight"}, MinSymbolDF: 1, MaxSalience: 8.0}},
		Trace:    retrieve.Trace{TotalSymbols: 100},
	})
	if rel != Relevant {
		t.Fatalf("terse rare-domain match with >=50%% repo coverage should be Relevant, got %s", rel)
	}

	// 4) Single common/rare word on a mostly-absent non-software prompt must abstain.
	rel, _ = relevanceGateV37(TaskSignals{BestSupport: 1, PresentRatio: 0.2, PromptTerms: 5, Concepts: 1}, retrieve.Result{
		Evidence: []retrieve.Evidence{{Hit: store.SymbolHit{Kind: "method", Name: "addChild"}, Matched: []string{"child"}, MinSymbolDF: 1, MaxSalience: 8.0}},
		Trace:    retrieve.Trace{TotalSymbols: 100},
	})
	if rel != Irrelevant {
		t.Fatalf("1-of-5 non-software match (PresentRatio=0.2) must be Irrelevant, got %s", rel)
	}
}

func TestV37FlowRouteBudgetAndEndToEndRun(t *testing.T) {
	ix := fixture(t)
	prompt := "how does ConfirmWeight validate the truck weight?"
	_, d36, err := Run(context.Background(), ix.Store(), prompt, Options{
		Retrieve:          retrieve.Options{Root: ix.Root()},
		RequireCodeAnchor: true,
		Refined:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d36.Action != ActionExploreAvailable || d36.Budget != leanBudget || d36.MaxItems != leanItems {
		t.Fatalf("V3.6 flow route drifted: %+v", d36)
	}

	res37, d37, err := Run(context.Background(), ix.Store(), prompt, Options{
		Retrieve: retrieve.Options{Root: ix.Root()},
		V37:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d37.Engine != Version37 || d37.Action != ActionExploreAvailable || d37.Budget != flowBudget || d37.MaxItems != flowItems {
		t.Fatalf("V3.7 flow route got engine=%s action=%s budget=%d maxItems=%d, want %s/%s/%d/%d",
			d37.Engine, d37.Action, d37.Budget, d37.MaxItems, Version37, ActionExploreAvailable, flowBudget, flowItems)
	}
	if len(res37.Evidence) == 0 {
		t.Fatal("expected evidence for flow query under V3.7")
	}

	// Off-topic and foreign-entity prompts abstain under V3.7.
	for _, neg := range []string{
		"quanto tempo leva para cozinhar arroz integral na panela de pressão?",
		"where is parseKafkaOffsets implemented?",
		"abrir entrada",
	} {
		nres, nd, err := Run(context.Background(), ix.Store(), neg, Options{
			Retrieve: retrieve.Options{Root: ix.Root()},
			V37:      true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if nd.Action != ActionAbstain || len(nres.Evidence) != 0 {
			t.Errorf("V3.7 negative %q should abstain, got action=%s ev=%d reasons=%v", neg, nd.Action, len(nres.Evidence), nd.AbstainReasons)
		}
	}
}

func TestV37SQLTableKindAndMemberKeysEndToEnd(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"sql/schema.sql": "CREATE TABLE weighing_records (\n  id INTEGER PRIMARY KEY,\n  plate TEXT NOT NULL\n);\n",
		"pkg/noise.go":   "package pkg\n\nfunc GetRecords() int { return 0 }\n",
		"http/resp.js":   "export const send = (v) => v;\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}

	res, d, err := Run(context.Background(), ix.Store(), "qual tabela guarda os registros de pesagem?", Options{
		Retrieve: retrieve.Options{Root: root},
		V37:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Action == ActionAbstain || len(res.Evidence) == 0 || res.Evidence[0].Hit.Name != "weighing_records" {
		t.Fatalf("expected weighing_records first under V3.7, got action=%s ev=%+v", d.Action, res.Evidence)
	}

	mres, md, err := Run(context.Background(), ix.Store(), "how does res.send work?", Options{
		Retrieve: retrieve.Options{Root: root},
		V37:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if md.Action == ActionAbstain || len(mres.Evidence) == 0 || mres.Evidence[0].Hit.Name != "send" {
		t.Fatalf("expected res.send -> send under V3.7, got action=%s ev=%+v", md.Action, mres.Evidence)
	}
}

func TestPlannerDeduplicatesExploreCodeSectionHeaderTokens(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/repo", WorktreeID: "w1", SessionID: "s1", WorktreeRevision: "rev1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)

	snippetItem := ctxbuild.Item{
		Section: ctxbuild.Code, Key: "fn|a.go|Alpha", Fingerprint: "fp-a",
		Text: "Alpha in a.go:1", Source: "explore",
		Body: []string{"1\tfunc Alpha() {}"},
	}
	secondItem := ctxbuild.Item{
		Section: ctxbuild.Code, Key: "fn|b.go|Beta", Fingerprint: "fp-b",
		Text: "Beta in b.go:10", Source: "exact",
	}

	refA, _, err := registry.Add(contract.EvidenceRef{
		Path: "a.go", Fingerprint: "fp-a", LineStart: 1, LineEnd: 1,
		Confidence: contract.ConfidenceVerified, Freshness: contract.FreshnessCurrent,
		CostEstimate: ctxbuild.PackItemTokens(snippetItem),
	}, retrieve.Evidence{ID: "ev-a", Section: ctxbuild.Code})
	if err != nil {
		t.Fatal(err)
	}
	refB, _, err := registry.Add(contract.EvidenceRef{
		Path: "b.go", Fingerprint: "fp-b", LineStart: 10, LineEnd: 12,
		Confidence: contract.ConfidenceVerified, Freshness: contract.FreshnessCurrent,
		CostEstimate: ctxbuild.PackItemTokens(secondItem),
	}, retrieve.Evidence{ID: "ev-b", Section: ctxbuild.Code})
	if err != nil {
		t.Fatal(err)
	}

	proposal := contract.GuidanceProposal{
		Kind:           ctxbuild.Code,
		EvidenceKeys:   []string{refA.Key},
		Priority:       115,
		TokenEstimate:  ctxbuild.SectionFrameTokens(ctxbuild.Code, 1) + ctxbuild.PackItemTokens(snippetItem),
		Items:          1,
		DeliveryReason: "explore.bounded_source_snippet",
	}
	// Exact tight budget without double-counting the RELEVANT CODE section header:
	exactBudget := ctxbuild.PackFrameTokens() +
		ctxbuild.SectionFrameTokens(ctxbuild.Code, 2) +
		ctxbuild.PackItemTokens(snippetItem) +
		ctxbuild.PackItemTokens(secondItem)

	input := PlanInput{
		Task: contract.TaskContext[TaskSignals]{
			Scope:    scope,
			Phase:    contract.PhaseExplore,
			Evidence: []contract.EvidenceRef{refA, refB},
			Guidance: []contract.GuidanceProposal{proposal},
		},
		Decision: Decision{
			Action:       ActionExploreAvailable,
			Budget:       exactBudget,
			MaxItems:     2,
			Included:     []Choice{{ID: "ev-b"}},
			RetrievalRan: true,
		},
		Evidence:      registry,
		BudgetTokens:  exactBudget,
		PriorityAware: true,
	}

	plan := (Planner{}).Plan(input)
	if len(plan.Sections) != 2 {
		t.Fatalf("expected both RELEVANT CODE evidence and explore proposal allocated at exact budget %d, got sections=%+v notDelivered=%+v",
			exactBudget, plan.Sections, plan.NotDelivered)
	}
	packed := ctxbuild.PackWith(
		ctxbuild.PackOptions{Ranked: true, DeduplicateEvidenceKeys: true},
		plan,
		registry,
		func(entry contract.EvidenceEntry[retrieve.Evidence]) []ctxbuild.Item {
			if entry.Value.ID == "ev-b" {
				return []ctxbuild.Item{secondItem}
			}
			return nil
		},
		[]ctxbuild.GuidanceProposal{{Proposal: proposal, Items: []ctxbuild.Item{snippetItem}}},
		nil,
	)
	if len(packed.Items) != 2 || packed.EstimatedTokens > exactBudget || !strings.Contains(packed.Text, "Alpha") || !strings.Contains(packed.Text, "Beta") {
		t.Fatalf("packed output mismatch: items=%d est=%d budget=%d text=%q notDelivered=%+v",
			len(packed.Items), packed.EstimatedTokens, exactBudget, packed.Text, packed.NotDelivered)
	}
}
