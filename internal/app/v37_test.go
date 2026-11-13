package app

import (
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestV37HookContractsAndDeterminism(t *testing.T) {
	testUnifiedBinaryHookContracts(t, engine.Version37)

	setupHome(t)
	root := makeV35Project(t)
	t.Setenv("SIDECAR_ENGINE", engine.Version37)
	t.Setenv("SIDECAR_FEATURES", "")
	const prompt = "How does Router.Serve work in this repository?"
	var spawned []string
	first, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
	second, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
	if first != second || !strings.Contains(first, "router/router.go") {
		t.Fatalf("v3.7 hook nondeterministic or missing router.go: first=%s second=%s", first, second)
	}
	report, err := Explain(root, prompt, retrieve.Options{EngineVersion: engine.Version37})
	if err != nil {
		t.Fatal(err)
	}
	if report.Engine != engine.Version37 || report.V35Plan == nil || report.V35Tokens > report.V35Plan.BudgetTokens {
		t.Fatalf("invalid V3.7 explain report: %+v", report)
	}
}

func TestV37AbstainsOnConversationalAndOffTopicNoise(t *testing.T) {
	setupHome(t)
	root := makeV35Project(t)
	t.Setenv("SIDECAR_ENGINE", engine.Version37)
	t.Setenv("SIDECAR_FEATURES", "")
	var spawned []string
	for _, prompt := range []string{
		"obrigado",
		"certo mais vai melhorando o que tu puder ok",
		"Can we configure a PostgreSQL replica in Kubernetes with Terraform modules today?",
	} {
		out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, ""), &spawned)
		if out != "" {
			t.Errorf("noise %q produced context: %s", prompt, out)
		}
	}
}

func TestV37PrepareProposalsDropsSeenAndRedundantEntrypoint(t *testing.T) {
	proposals := []ctxbuild.GuidanceProposal{
		{
			Proposal: contract.GuidanceProposal{Kind: ctxbuild.TaskBrief, EvidenceKeys: []string{"k1", "k1"}},
			Items: []ctxbuild.Item{
				{Section: ctxbuild.TaskBrief, Key: "k1", Fingerprint: "fp1", Text: "[CONFIRMED] entrypoint: router.Serve", Role: "entrypoint"},
				{Section: ctxbuild.TaskBrief, Key: "brief|test|router_test.go", Fingerprint: "router_test.go", Text: "[CONFIRMED] test file references...", Role: "test"},
				{Section: ctxbuild.TaskBrief, Key: "brief|consumer|main.go", Fingerprint: "main.go", Text: "[CONFIRMED] caller: main", Role: "consumer"},
			},
		},
	}
	seen := func(key, fp string) bool { return key == "brief|consumer|main.go" }
	choices := []engine.Choice{{ID: "test-1", Source: "contract"}}
	snippets := map[string]string{"k1": "fp-snippet"}

	got, metas := v37PrepareProposals(proposals, choices, snippets, seen)
	if len(got) != 0 || len(metas) != 0 {
		t.Fatalf("expected all redundant/seen TaskBrief items filtered cleanly, got=%+v", got)
	}
}
