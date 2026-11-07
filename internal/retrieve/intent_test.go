package retrieve

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClassifyIntentLabelsPortugueseAndEnglish(t *testing.T) {
	cases := []struct {
		prompt  string
		want    []string // every wanted kind must be present
		minConf float64
	}{
		{"Onde fica a fila de tickets?", []string{"locate"}, 0.85},
		{"where is the ticket queue defined", []string{"locate"}, 0.7},
		{"panic: nil pointer at queue/ticket.go:42 when calling Enqueue", []string{"debug"}, 0.9},
		{"Como um hook com erro evita interromper o agente?", []string{"debug"}, 0.8},
		{"quem usa o método Enqueue?", []string{"impact"}, 0.85},
		{"o que quebra se eu mudar a assinatura de Release", []string{"impact"}, 0.85},
		{"criar um novo endpoint para cupons", []string{"extend"}, 0.8},
		{"como criar outro handler seguindo o padrão do existente", []string{"extend"}, 0.8},
		{"teste de reserva de estoque", []string{"test"}, 0.9},
		{"which tests cover the release handler", []string{"test"}, 0.85},
		{"explique como o contexto do hook é montado", []string{"explain"}, 0.8},
		{"explain how the hook context is assembled", []string{"explain"}, 0.8},
		{"qual a regra de negócio do limite de itens por pedido?", []string{"contract"}, 0.8},
		{"a documentação diverge do código no limite de itens", []string{"contract"}, 0.85},
	}
	for _, c := range cases {
		in := ClassifyIntent(c.prompt)
		for _, k := range c.want {
			if !slices.Contains(in.Kinds, k) {
				t.Errorf("%q: kinds %v lack %q (signals %v)", c.prompt, in.Kinds, k, in.Signals)
			}
		}
		if in.Confidence < c.minConf {
			t.Errorf("%q: confidence %.2f, want >= %.2f", c.prompt, in.Confidence, c.minConf)
		}
	}
}

func TestClassifyIntentIsMultiLabel(t *testing.T) {
	in := ClassifyIntent("como testar e quem chama a função Release?")
	if !slices.Contains(in.Kinds, "test") || !slices.Contains(in.Kinds, "impact") {
		t.Fatalf("kinds = %v, want test and impact together", in.Kinds)
	}
}

func TestClassifyIntentFallsBackToGeneral(t *testing.T) {
	for _, p := range []string{"", "qual a capital da Austrália?", "obrigado pela atenção"} {
		in := ClassifyIntent(p)
		if !reflect.DeepEqual(in.Kinds, []string{"general"}) || in.Confidence != 0 {
			t.Errorf("%q: got %+v, want general with confidence 0", p, in)
		}
	}
}

func TestWeakSignalStaysBelowRoutingThreshold(t *testing.T) {
	in := ClassifyIntent("adicionar algo")
	if !slices.Contains(in.Kinds, "extend") {
		t.Fatalf("kinds = %v, want extend", in.Kinds)
	}
	if in.Confidence >= intentMinConfidence {
		t.Fatalf("confidence %.2f reaches the routing threshold %.2f for a generic word", in.Confidence, intentMinConfidence)
	}
}

func TestAssinaturaAloneIsSubscriptionNotContract(t *testing.T) {
	in := ClassifyIntent("cobrança mensal das assinaturas recorrentes")
	if slices.Contains(in.Kinds, "contract") {
		t.Fatalf("subscription wording routed as contract: %+v", in)
	}
	if got := ClassifyIntent("assinatura do método Release"); !slices.Contains(got.Kinds, "contract") {
		t.Fatalf("code signature not recognised: %+v", got)
	}
}

func TestClassifyIntentIsDeterministicAndCaseInsensitive(t *testing.T) {
	a := ClassifyIntent("ONDE FICA a Fila de Tickets?")
	b := ClassifyIntent("onde fica a fila de tickets?")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("case or accents changed the intent: %+v vs %+v", a, b)
	}
	if again := ClassifyIntent("ONDE FICA a Fila de Tickets?"); !reflect.DeepEqual(a, again) {
		t.Fatal("same prompt gave different intents")
	}
}

func TestSignalsNameTheRulesThatFired(t *testing.T) {
	in := ClassifyIntent("como testar ManualReleaseHandler em internal/app/app.go")
	for _, want := range []string{"test:where-to-test", "locate:path", "locate:symbol"} {
		if !slices.Contains(in.Signals, want) {
			t.Errorf("signals %v lack %q", in.Signals, want)
		}
	}
}

func TestFamiliesForUnionsKindsAndIgnoresGeneral(t *testing.T) {
	if got := familiesFor([]string{"general"}); got != nil {
		t.Fatalf("general must expand everything (nil), got %v", got)
	}
	got := familiesFor([]string{"impact", "test"})
	for _, f := range []string{famCallers, famTests, famHistory, famParent} {
		if !got[f] {
			t.Errorf("impact+test families %v lack %q", got, f)
		}
	}
	if got[famMembers] {
		t.Errorf("impact+test must not expand members: %v", got)
	}
}

func idsOf(res Result) []string {
	var out []string
	for _, e := range res.Evidence {
		out = append(out, e.ID)
	}
	return out
}

func TestRoutingNarrowsExpansionOnlyWhenConfident(t *testing.T) {
	ix := buildIndex(t)
	v1, err := Search(context.Background(), ix.Store(), "Onde fica o Enqueue?", Options{})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Search(context.Background(), ix.Store(), "Onde fica o Enqueue?", Options{EngineVersion: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if v1.Trace.Routed || !v2.Trace.Routed {
		t.Fatalf("routed: v1=%v v2=%v, want false and true", v1.Trace.Routed, v2.Trace.Routed)
	}
	if !hasRelationPrefix(v1, "calls:") {
		t.Fatal("fixture no longer exercises caller expansion in v1; pick another prompt")
	}
	if hasRelationPrefix(v2, "calls:") {
		t.Fatalf("locate routing still expanded callers: %v", v2.Evidence)
	}
}

func TestRoutingFallsBackToV1BelowThreshold(t *testing.T) {
	ix := buildIndex(t)
	for _, p := range []string{"qual a capital da Austrália?", "adicionar algo"} {
		v1, err := Search(context.Background(), ix.Store(), p, Options{})
		if err != nil {
			t.Fatal(err)
		}
		v2, err := Search(context.Background(), ix.Store(), p, Options{IntentRouting: true})
		if err != nil {
			t.Fatal(err)
		}
		if v2.Trace.Routed {
			t.Errorf("%q: routed with confidence %.2f", p, v2.Trace.Intent.Confidence)
		}
		if !reflect.DeepEqual(idsOf(v1), idsOf(v2)) {
			t.Errorf("%q: low-confidence v2 differs from v1: %v vs %v", p, idsOf(v1), idsOf(v2))
		}
	}
}

func hasRelationPrefix(res Result, prefix string) bool {
	for _, e := range res.Evidence {
		if hasPrefixAny(e.Relations, prefix) {
			return true
		}
	}
	return false
}

func TestTraceExplainsEveryCandidate(t *testing.T) {
	ix := buildIndex(t)
	const prompt = "liberação de veículo na fila"
	res, err := Search(context.Background(), ix.Store(), prompt, Options{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Search(context.Background(), ix.Store(), prompt, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(idsOf(res), idsOf(again)) {
		t.Fatalf("evidence IDs are not stable across runs: %v vs %v", idsOf(res), idsOf(again))
	}
	for _, e := range res.Evidence {
		if e.ID == "" || e.Source == "" || e.TokenEstimate <= 0 || e.Confidence < 0 || e.Confidence > 1 {
			t.Errorf("evidence lacks observability fields: %+v", e)
		}
	}
	if res.Trace.TokenEstimate <= 0 {
		t.Errorf("trace token estimate = %d, want > 0", res.Trace.TokenEstimate)
	}
	for _, c := range res.Trace.Candidates {
		if c.ID == "" || c.Source == "" || c.Confidence < 0 || c.Confidence > 1 {
			t.Errorf("candidate lacks observability fields: %+v", c)
		}
		switch {
		case c.Kept && (c.ReasonSelected == "" || c.ReasonSuppressed != ""):
			t.Errorf("kept candidate has wrong reasons: %+v", c)
		case !c.Kept && (c.ReasonSuppressed == "" || c.ReasonSelected != ""):
			t.Errorf("suppressed candidate has wrong reasons: %+v", c)
		}
	}
}

func TestLongPromptReadsOnlyItsEnds(t *testing.T) {
	filler := strings.Repeat("linha sem sinal nenhum\n", 500)
	if in := ClassifyIntent("onde fica o ponto? " + filler); !slices.Contains(in.Kinds, "locate") {
		t.Errorf("cue at the start ignored: %+v", in)
	}
	if in := ClassifyIntent(filler + "como funciona o hook?"); !slices.Contains(in.Kinds, "explain") {
		t.Errorf("cue at the end ignored: %+v", in)
	}
	if in := ClassifyIntent(filler + "onde fica o ponto" + filler); !reflect.DeepEqual(in.Kinds, []string{"general"}) {
		t.Errorf("cue buried in the middle should not route: %+v", in)
	}
}
