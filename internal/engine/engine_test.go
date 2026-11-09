package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// fixture is a small Go project with a domain vocabulary (trucks, scale, queue) and software code.
func fixture(t *testing.T) *index.Index {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"queue/queue.go":      "package queue\n\n// MaxActiveTickets limits the active tickets at the gate.\nconst MaxActiveTickets = 40\n\n// Enqueue puts a truck in the waiting queue.\nfunc Enqueue(truck string) int { return len(truck) }\n\n// ReleaseManual lets the operator release the next truck by hand.\nfunc ReleaseManual(operator string) bool { return operator != \"\" }\n",
		"scale/scale.go":      "package scale\n\n// ConfirmWeight confirms the truck weight on the scale.\nfunc ConfirmWeight(truck string, kg int) bool { return kg > 0 }\n",
		"scale/scale_test.go": "package scale\n\nimport \"testing\"\n\nfunc TestConfirmWeight(t *testing.T) { if !ConfirmWeight(\"a\", 1) { t.Fatal() } }\n",
		"api/routes.go":       "package api\n\n// ListTrucksHandler is the HTTP route that lists the trucks.\nfunc ListTrucksHandler() string { return \"/trucks\" }\n\n// Timeout cancels a request after the deadline.\nfunc Timeout(seconds int) int { return seconds }\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
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
	return ix
}

func run(t *testing.T, ix *index.Index, prompt string) (retrieve.Result, Decision) {
	t.Helper()
	res, d, err := Run(context.Background(), ix.Store(), prompt, Options{Retrieve: retrieve.Options{Root: ix.Root()}})
	if err != nil {
		t.Fatal(err)
	}
	return res, d
}

func TestConversationalPromptsAbstainWithoutRetrieval(t *testing.T) {
	ix := fixture(t)
	for _, p := range []string{"oi! tudo bem com você?", "thanks, that's all for now", "ok 👍", "😂😂", "write a haiku about autumn", "valeu, era só isso"} {
		res, d := run(t, ix, p)
		if d.Action != ActionAbstain || d.RetrievalRan || len(res.Evidence) != 0 {
			t.Errorf("%q: action=%s retrieval=%v evidence=%d, want ABSTAIN without retrieval", p, d.Action, d.RetrievalRan, len(res.Evidence))
		}
		if Render(res, d, nil) != "" {
			t.Errorf("%q: abstention must emit zero bytes", p)
		}
	}
}

func TestGreetingWithACodeTaskIsNotConversational(t *testing.T) {
	ix := fixture(t)
	_, d := run(t, ix, "oi, onde fica a função ConfirmWeight?")
	if d.Action == ActionAbstain {
		t.Fatalf("a greeting followed by a code question must not abstain: %+v", d.AbstainReasons)
	}
}

func TestVerifiedIdentifierIsRelevantEvenWithoutOtherWords(t *testing.T) {
	ix := fixture(t)
	res, d := run(t, ix, "ConfirmWeight?")
	if d.Relevance != Relevant || len(res.Evidence) == 0 {
		t.Fatalf("relevance=%s evidence=%d", d.Relevance, len(res.Evidence))
	}
	found := false
	for _, l := range d.Signals.Locators {
		found = found || strings.TrimRight(l.Text, "?") == "ConfirmWeight" && l.Verified
	}
	if !found {
		t.Fatalf("locator not verified: %+v", d.Signals.Locators)
	}
}

func TestUnknownIdentifierIsNotVerified(t *testing.T) {
	ix := fixture(t)
	_, d := run(t, ix, "where is parseKafkaOffsets implemented?")
	for _, l := range d.Signals.Locators {
		if l.Verified {
			t.Fatalf("absent identifier verified: %+v", l)
		}
	}
}

func TestOffTopicPromptWithOneGenericWordAbstains(t *testing.T) {
	ix := fixture(t)
	for _, p := range []string{"quanto tempo leva para cozinhar arroz integral na panela de pressão?", "qual o melhor horário para plantar tomate na primavera?"} {
		res, d := run(t, ix, p)
		if d.Action != ActionAbstain || len(res.Evidence) != 0 {
			t.Errorf("%q: action=%s evidence=%d signals=%+v", p, d.Action, len(res.Evidence), d.Signals)
		}
	}
}

func TestDomainLanguagePromptIsAnsweredLean(t *testing.T) {
	ix := fixture(t)
	res, d := run(t, ix, "confirm the truck weight on the scale")
	if d.Action == ActionAbstain || len(res.Evidence) == 0 {
		t.Fatalf("domain prompt abstained: %+v", d)
	}
	if res.Evidence[0].Hit.Path != "scale/scale.go" {
		t.Fatalf("top evidence %s", res.Evidence[0].Hit.Path)
	}
}

func TestLocateIsLeanAndBounded(t *testing.T) {
	ix := fixture(t)
	res, d := run(t, ix, "where is the route that lists the trucks?")
	if d.Action != ActionLean || d.Class != ClassLocate {
		t.Fatalf("action=%s class=%s", d.Action, d.Class)
	}
	if len(res.Evidence) > leanItems || d.EstimatedTokens > d.Budget && len(res.Evidence) > 1 {
		t.Fatalf("lean exceeded its limits: items=%d tokens=%d budget=%d", len(res.Evidence), d.EstimatedTokens, d.Budget)
	}
	if len(d.Included) != len(res.Evidence) {
		t.Fatalf("every delivered item needs a recorded reason")
	}
}

func TestNegatedAndQuotedEditsAreNotEdits(t *testing.T) {
	cases := map[string]bool{
		"não altere nada, só me explique onde fica a fila":                              false,
		"don't change anything, where is Enqueue?":                                      false,
		`o ticket diz "remova o timeout", mas só quero entender onde ele é configurado`: false,
		"não altere os testes; corrija o Timeout para aceitar zero":                     true,
		"keep the signature and change ConfirmWeight to reject negative weights":        true,
	}
	for prompt, want := range cases {
		if got := readSignals(prompt).Edit; got != want {
			t.Errorf("%q: edit=%v want %v", prompt, got, want)
		}
	}
}

func TestContractPreservingEditRoutesToVerify(t *testing.T) {
	ix := fixture(t)
	_, d := run(t, ix, "keep the signature and change ConfirmWeight to reject negative weights")
	if d.Class != ClassContractChange || d.Action != ActionVerify {
		t.Fatalf("class=%s action=%s", d.Class, d.Action)
	}
	if !d.ExploreHint {
		t.Fatal("verify must offer on-demand exploration")
	}
}

func TestStackFrameRoutesToDebug(t *testing.T) {
	ix := fixture(t)
	_, d := run(t, ix, "panic: runtime error\n\tscale/scale.go:4 ConfirmWeight")
	if d.Class != ClassDebug {
		t.Fatalf("class=%s", d.Class)
	}
	verified := false
	for _, l := range d.Signals.Locators {
		verified = verified || (l.Kind == "path" || l.Kind == "stack_frame") && l.Verified
	}
	if !verified {
		t.Fatalf("path in stack frame not verified: %+v", d.Signals.Locators)
	}
}

func TestDecisionIsDeterministic(t *testing.T) {
	ix := fixture(t)
	prompt := "onde o operador libera manualmente o próximo caminhão da fila?"
	_, a := run(t, ix, prompt)
	_, b := run(t, ix, prompt)
	a.ElapsedMS, b.ElapsedMS = 0, 0
	if a.Action != b.Action || a.Class != b.Class || len(a.Included) != len(b.Included) || strings.Join(a.AbstainReasons, ";") != strings.Join(b.AbstainReasons, ";") {
		t.Fatalf("non-deterministic decision:\n%+v\n%+v", a, b)
	}
}

func TestLongNoisyOffTopicPromptAbstains(t *testing.T) {
	ix := fixture(t)
	prompt := "então, estou organizando o aniversário da minha mãe no sábado, vão umas trinta pessoas, e queria uma lista do que comprar de comida e bebida, mais ou menos as quantidades, e se dá pra fazer tudo com uns quinhentos reais"
	res, d := run(t, ix, prompt)
	if len(res.Evidence) != 0 {
		t.Fatalf("long off-topic prompt delivered %d items: %+v", len(res.Evidence), d.Signals)
	}
}

func TestCodeIdentifierShapes(t *testing.T) {
	for tok, want := range map[string]bool{"res.redirect": true, "etag_matches?": true, "ThrottleBacklog": true, "ConfirmWeight": true,
		"Hello": false, "example.com": false, "it": false, "HTTP": false, "e.g": false} {
		if got := codeIdentifier(tok); got != want {
			t.Errorf("%q: %v want %v", tok, got, want)
		}
	}
}
