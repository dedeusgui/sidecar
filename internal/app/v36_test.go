package app

import (
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestV36HookDeliversDeterministicEvidence(t *testing.T) {
	setupHome(t)
	root := makeV35Project(t)
	t.Setenv("SIDECAR_ENGINE", engine.Version36)
	t.Setenv("SIDECAR_FEATURES", "")
	const prompt = "How does Router.Serve work in this repository?"
	var spawned []string
	first, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
	second, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
	if first != second || !strings.Contains(first, "router/router.go") {
		t.Fatalf("hook output missing evidence or nondeterministic: first=%s second=%s", first, second)
	}
	report, err := Explain(root, prompt, retrieve.Options{EngineVersion: engine.Version36})
	if err != nil {
		t.Fatal(err)
	}
	if report.Engine != engine.Version36 || report.V35Plan == nil || report.V35Tokens > report.V35Plan.BudgetTokens || !strings.Contains(report.Context, "router/router.go") {
		t.Fatalf("invalid V3.6 explain: %+v", report)
	}
}

func TestV36AbstainsOnConversationalNoise(t *testing.T) {
	setupHome(t)
	root := makeV35Project(t)
	t.Setenv("SIDECAR_ENGINE", engine.Version36)
	t.Setenv("SIDECAR_FEATURES", "")
	var spawned []string
	for _, prompt := range []string{"obrigado", "certo mais vai melhorando o que tu puder ok"} {
		out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
		if out != "" {
			t.Errorf("noise %q produced context: %s", prompt, out)
		}
	}
}

func TestV36SnippetFallbackWhenProposalRejected(t *testing.T) {
	setupHome(t)
	root := t.TempDir()
	scope := contract.NormalizeScope(contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"})
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	ev := retrieve.Evidence{
		ID:      "router.go",
		Hit:     store.SymbolHit{Path: "router/router.go", Kind: "file", Name: "router.go"},
		Section: ctxbuild.Code,
	}
	metadata := contract.EvidenceRef{
		Key: ev.Key(), Path: "router/router.go", Fingerprint: ev.Fingerprint(),
		Freshness: contract.FreshnessCurrent, Confidence: contract.ConfidenceCandidate,
	}
	metadata.CostEstimate = max(1, ctxbuild.PackItemTokens(v36EvidenceItem(contract.EvidenceEntry[retrieve.Evidence]{Ref: metadata, Value: ev})))
	ref, _, err := registry.Add(metadata, ev)
	if err != nil {
		t.Fatal(err)
	}

	choices := []engine.Choice{{ID: "router.go"}}
	prompt := "How does Router.Serve work?"

	// A two-item proposal cannot fit a one-item decision, even though a fallback can.
	snippetItem := ctxbuild.Item{
		Section: ctxbuild.Code, Key: ev.Key(), Text: "func (r Router) Serve() string",
		Fingerprint: "fp-snippet-1", Priority: 115, Source: "explore", Role: "snippet",
		Qualifier: "CANDIDATE", Provenance: "router/router.go:3-6",
		Body: []string{"func (r Router) Serve() string {", "\treturn r.authorize()", "}"},
	}
	proposal := contract.GuidanceProposal{
		Kind: ctxbuild.Code, EvidenceKeys: []string{ref.Key}, Priority: 115,
		TokenEstimate: 80, Items: 2, DeliveryReason: "explore.bounded_source_snippet",
	}
	otherSnippet := snippetItem
	otherSnippet.Key = "other"
	proposals := []ctxbuild.GuidanceProposal{{Proposal: proposal, Items: []ctxbuild.Item{snippetItem, otherSnippet}}}
	task := contract.TaskContext[engine.TaskSignals]{
		Scope: scope, Evidence: []contract.EvidenceRef{ref},
		Guidance: []contract.GuidanceProposal{proposal},
	}

	decision := engine.Decision{
		Action: engine.ActionExploreAvailable, Budget: 200, MaxItems: 1,
		RetrievalRan: true, Included: choices,
	}

	plan, snippetKeys := v36Plan(engine.PlanInput{
		Task: task, Decision: decision, Evidence: registry,
		BudgetTokens: decision.Budget, PriorityAware: true,
	}, preferNamedFiles(decision.Included, prompt), proposals, nil)

	packed := ctxbuild.PackWith(ctxbuild.PackOptions{Ranked: true}, plan, registry, func(entry contract.EvidenceEntry[retrieve.Evidence]) []ctxbuild.Item {
		if _, hasSnippet := snippetKeys[entry.Value.Key()]; hasSnippet {
			return nil
		}
		return []ctxbuild.Item{v36EvidenceItem(entry)}
	}, proposals, nil)

	t.Logf("plan.Sections: %+v", plan.Sections)
	t.Logf("plan.NotDelivered: %+v", plan.NotDelivered)
	t.Logf("packed.Items: %+v", packed.Items)
	t.Logf("packed.Text: %q", packed.Text)

	if len(packed.Items) == 0 {
		t.Fatal("rejected snippet suppressed the fallback evidence")
	}
	if packed.EstimatedTokens > decision.Budget || packed.Items[0].Source == "explore" {
		t.Fatalf("invalid fallback: %+v", packed)
	}

	proposal.TokenEstimate = ctxbuild.SectionFrameTokens(ctxbuild.Code, 1) + ctxbuild.PackItemTokens(snippetItem)
	proposal.Items = 1
	proposals[0].Proposal = proposal
	proposals[0].Items = []ctxbuild.Item{snippetItem}
	task.Guidance[0] = proposal
	plan, snippetKeys = v36Plan(engine.PlanInput{Task: task, Decision: decision, Evidence: registry, BudgetTokens: decision.Budget, PriorityAware: true}, choices, proposals, nil)
	packed = ctxbuild.PackWith(ctxbuild.PackOptions{Ranked: true}, plan, registry, func(entry contract.EvidenceEntry[retrieve.Evidence]) []ctxbuild.Item {
		if _, hasSnippet := snippetKeys[entry.Value.Key()]; hasSnippet {
			return nil
		}
		return []ctxbuild.Item{v36EvidenceItem(entry)}
	}, proposals, nil)
	if len(packed.Items) != 1 || packed.Items[0].Source != "explore" || packed.EstimatedTokens > decision.Budget {
		t.Fatalf("accepted snippet should appear once within budget: %+v", packed)
	}
}
