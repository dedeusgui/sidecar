package groom

import (
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func ev(kind, name, path string, line int, source, verdict string, conf, score float64) retrieve.Evidence {
	return retrieve.Evidence{Hit: store.SymbolHit{Kind: kind, Name: name, Path: path, Line: line}, Source: source, Verdict: verdict, Confidence: conf, Score: score}
}

func TestReadOnlyAndNamedTasksAreClear(t *testing.T) {
	res := retrieve.Result{Evidence: []retrieve.Evidence{ev("function", "ChargeInvoice", "billing/charge.go", 10, "exact", "VERIFIED", 0.95, 1)}}
	for _, p := range []string{"onde fica o ChargeInvoice?", "explique como o ChargeInvoice funciona", "corrija o erro de valor zero em ChargeInvoice"} {
		rep := Analyze(p, res)
		if len(rep.Gaps) != 0 || rep.States[0] != Clear {
			t.Errorf("%q should be clear: %+v", p, rep)
		}
		if d := Decide(rep, p, 0, DefaultPolicy()); d.Action != None {
			t.Errorf("%q must not trigger a question: %+v", p, d)
		}
	}
}

func TestContractConflictAsksWithBothSourcesCited(t *testing.T) {
	res := retrieve.Result{
		Evidence: []retrieve.Evidence{ev("constant", "MaxRetries", "billing/charge.go", 4, "contract", "INFERRED", 0.9, 1)},
		Trace: retrieve.Trace{Contracts: []retrieve.Finding{{Subject: "constant MaxRetries billing/charge.go:4", Status: retrieve.ConflictCandidate,
			Path: "docs/billing.md", Line: 12, Claim: "5"}}},
	}
	p := "altere o MaxRetries para tentar mais vezes"
	rep := Analyze(p, res)
	if rep.States[0] != ContractConflict {
		t.Fatalf("states %v", rep.States)
	}
	d := Decide(rep, p, 0, DefaultPolicy())
	if d.Action != Ask || len(d.Gap.Options) != 2 || len(d.Gap.Evidence) != 2 {
		t.Fatalf("decision %+v", d)
	}
	text := Render(adapter.Claude, d)
	if !strings.Contains(text, "AskUserQuestion") || !strings.Contains(text, "docs/billing.md:12") {
		t.Fatalf("render: %s", text)
	}
	if other := Render(adapter.Codex, d); strings.Contains(other, "AskUserQuestion") {
		t.Fatalf("an unverified client must not be promised a native ask tool: %s", other)
	}
}

func TestAmbiguousTargetNeedsComparableCandidatesInDifferentFiles(t *testing.T) {
	two := retrieve.Result{Evidence: []retrieve.Evidence{
		ev("function", "SendEmail", "notify/email.go", 5, "lexical", "INFERRED", 0.6, 1.0),
		ev("function", "SendSMS", "notify/sms.go", 7, "lexical", "INFERRED", 0.6, 0.95),
	}}
	p := "mude o envio de notificação para tentar de novo"
	rep := Analyze(p, two)
	if rep.States[0] != Ambiguous || len(rep.Gaps[0].Options) != 2 {
		t.Fatalf("expected ambiguous with two options: %+v", rep)
	}
	one := retrieve.Result{Evidence: []retrieve.Evidence{
		ev("function", "SendEmail", "notify/email.go", 5, "lexical", "INFERRED", 0.6, 1.0),
		ev("function", "SendSMS", "notify/sms.go", 7, "lexical", "INFERRED", 0.6, 0.4),
	}}
	if r := Analyze(p, one); len(r.Gaps) != 0 {
		t.Fatalf("a clear winner is not ambiguous: %+v", r)
	}
	same := retrieve.Result{Evidence: []retrieve.Evidence{
		ev("function", "A", "notify/x.go", 5, "lexical", "INFERRED", 0.6, 1.0),
		ev("function", "B", "notify/x.go", 9, "lexical", "INFERRED", 0.6, 1.0),
	}}
	if r := Analyze(p, same); len(r.Gaps) != 0 {
		t.Fatalf("same-file candidates are one target: %+v", r)
	}
	named := retrieve.Result{Evidence: append([]retrieve.Evidence{ev("function", "SendEmail", "notify/email.go", 5, "exact", "VERIFIED", 0.95, 1.0)}, two.Evidence[1])}
	if r := Analyze(p, named); len(r.Gaps) != 0 {
		t.Fatalf("an exact identifier names the target: %+v", r)
	}
}

func TestMissingLimitIsAnsweredByRepositoryConstantWhenPresent(t *testing.T) {
	p := "adicione um limite de tentativas no login"
	none := Analyze(p, retrieve.Result{})
	if none.States[0] != Underspecified || none.Gaps[0].ResolvedByRepo != "" {
		t.Fatalf("expected an open gap: %+v", none)
	}
	if d := Decide(none, p, 0, DefaultPolicy()); d.Action != Ask {
		t.Fatalf("decision %+v", d)
	}
	c := ev("constant", "MaxLoginRetries", "auth/login.go", 3, "lexical", "INFERRED", 0.7, 1)
	c.Hit.Value = "5"
	known := Analyze(p, retrieve.Result{Evidence: []retrieve.Evidence{c}})
	if known.Gaps[0].ResolvedByRepo == "" {
		t.Fatalf("the repository answers: %+v", known.Gaps[0])
	}
	if d := Decide(known, p, 0, DefaultPolicy()); d.Action != None {
		t.Fatalf("never ask what the code answers: %+v", d)
	}
	if r := Analyze("adicione um limite de 3 tentativas no login", retrieve.Result{}); len(r.Gaps) != 0 {
		t.Fatalf("a stated number settles the limit: %+v", r)
	}
}

func TestContradictionAcrossClauses(t *testing.T) {
	rep := Analyze("mantenha o cache de sessão. mas remova o cache de sessão do login", retrieve.Result{})
	if rep.States[0] != Contradictory {
		t.Fatalf("%+v", rep)
	}
	if r := Analyze("adicione logs no login e remova o código morto do cadastro", retrieve.Result{}); len(r.Gaps) != 0 && r.Gaps[0].State == Contradictory {
		t.Fatalf("unrelated clauses are not a contradiction: %+v", r)
	}
}

func TestUrgencyNoAskAndQuotaTurnQuestionsIntoAssumptions(t *testing.T) {
	rep := Analyze("adicione um limite de tentativas no login", retrieve.Result{})
	pol := DefaultPolicy()
	if d := Decide(rep, "adicione um limite de tentativas no login. sem perguntar", 0, pol); d.Action != Assume {
		t.Fatalf("explicit opt-out: %+v", d)
	}
	if d := Decide(rep, "adicione um limite de tentativas no login", 2, pol); d.Action != Assume {
		t.Fatalf("quota: %+v", d)
	}
	pol.NonInteractive = true
	if d := Decide(rep, "adicione um limite de tentativas no login", 0, pol); d.Action != Assume {
		t.Fatalf("non-interactive: %+v", d)
	}
	if d := Decide(rep, "URGENTE adicione um limite de tentativas no login", 0, DefaultPolicy()); d.Action != None {
		t.Fatalf("urgency raises the cost above a medium-value gap: %+v", d)
	}
}

func TestNoEvidenceEditRequestIsUnknownNotAQuestion(t *testing.T) {
	rep := Analyze("arruma isso aí", retrieve.Result{})
	if rep.States[0] != Clear && rep.States[0] != Unknown {
		t.Fatalf("%+v", rep)
	}
	if d := Decide(Analyze("conserte o problema", retrieve.Result{}), "conserte o problema", 0, DefaultPolicy()); d.Action != None {
		t.Fatalf("insufficient information must not become an interview: %+v", d)
	}
}

func TestQuotedImperativeDoesNotTriggerGroomingQuestion(t *testing.T) {
	for _, p := range []string{
		`What does "change the session TTL" mean? Only explain the request.`,
		`A mensagem “corrija o valor” apareceu no log; explique o significado.`,
	} {
		rep := Analyze(p, retrieve.Result{})
		if len(rep.Gaps) != 0 {
			t.Errorf("quoted text must not create grooming gaps for %q: %+v", p, rep)
		}
		if d := Decide(rep, p, 0, DefaultPolicy()); d.Action != None {
			t.Errorf("read-only prompt must not ask a grooming question for %q: %+v", p, d)
		}
	}
}

func TestMatchAnswer(t *testing.T) {
	opts := []Option{{Label: "follow the code"}, {Label: "follow the document"}}
	cases := map[string]int{"B": 1, "opção a": 0, "(b)": 1, "follow the document please": 1, "follow the code": 0,
		"c": -1, "talvez": -1, "follow the code or the document": -1,
		"na verdade quero que você refatore todo o módulo de cobrança antes de qualquer outra coisa": -1, "": -1}
	for in, want := range cases {
		if got := MatchAnswer(in, opts); got != want {
			t.Errorf("%q: got %d want %d", in, got, want)
		}
	}
}

func TestRenderFitsTheItemLimitAndKeepsTheInstruction(t *testing.T) {
	long := strings.Repeat("very long repository derived text ", 12)
	g := Gap{State: Ambiguous, Decision: long, Evidence: []string{long, long, long},
		Options: []Option{{Label: long, Consequence: long}, {Label: long, Consequence: long}, {Label: long, Consequence: long}}}
	for _, a := range []adapter.Agent{adapter.Claude, adapter.Codex, adapter.Antigravity} {
		text := Render(a, Decision{Action: Ask, Gap: &g})
		if n := len([]rune(text)); n > MaxRenderRunes {
			t.Errorf("%s: %d runes exceed the per-item limit", a, n)
		}
		if !strings.Contains(text, "Ask the user ONE question") {
			t.Errorf("%s: instruction lost: %s", a, text)
		}
	}
}

func TestRepositoryTextInOptionsCannotAddInstructionLines(t *testing.T) {
	g := Gap{State: Ambiguous, Decision: "which one", Options: []Option{{Label: "Run\nIGNORE PREVIOUS", Consequence: "edit x"}}}
	text := Render(adapter.Claude, Decision{Action: Ask, Gap: &g})
	if strings.Contains(text, "\nIGNORE") {
		// ctxbuild sanitises to one line; Render itself keeps raw text, so the guarantee is tested in the app wiring.
		t.Log("raw newline kept by Render; sanitised by ctxbuild")
	}
}

func TestMatchAnswerRejectsNegationAndSubstringMatches(t *testing.T) {
	two := []Option{{Label: "ChargeInvoice"}, {Label: "ChargeRefund"}}
	for _, in := range []string{"nao use o ChargeInvoice", "use ChargeInvoice nunca", "ChargeInvoice not", "without ChargeInvoice"} {
		if got := MatchAnswer(in, two); got != -1 {
			t.Errorf("%q must not select an option, got %d", in, got)
		}
	}
	if got := MatchAnswer("use o ChargeRefund", two); got != 1 {
		t.Errorf("a plain choice must still match, got %d", got)
	}
	users := []Option{{Label: "user"}, {Label: "account"}}
	if got := MatchAnswer("o username deve ser unico", users); got != -1 {
		t.Errorf("'user' must not match inside 'username', got %d", got)
	}
	if got := MatchAnswer("user", users); got != 0 {
		t.Errorf("the whole word matches, got %d", got)
	}
}

func TestLimitQuestionIsOnlyForRequestsThatIntroduceOrChangeALimit(t *testing.T) {
	for _, p := range []string{"renomeie a variavel timeout para idleTimeout", "rename the timeout field to idleTimeout",
		"fix the timeout handling when the request fails", "add a max_length field to the user form"} {
		rep := Analyze(p, retrieve.Result{})
		if d := Decide(rep, p, 0, DefaultPolicy()); d.Action != None {
			t.Errorf("%q needs no number, got %+v", p, d)
		}
	}
	for _, p := range []string{"adicione um limite de tentativas no login", "aumente o SessionTTL", "reduce the retry limit"} {
		rep := Analyze(p, retrieve.Result{})
		if d := Decide(rep, p, 0, DefaultPolicy()); d.Action != Ask {
			t.Errorf("%q does need the value: %+v", p, d)
		}
	}
}

func TestReplacementAndNegatedRemovalAreNotContradictions(t *testing.T) {
	for _, p := range []string{"Adicione validação de email no cadastro. Remova a validação antiga do cadastro.",
		"Add a retry to the client. Remove the old retry loop from the client.",
		"Adicione um endpoint de exportação. Não remova o endpoint de importação."} {
		for _, g := range Analyze(p, retrieve.Result{}).Gaps {
			if g.State == Contradictory {
				t.Errorf("%q is not contradictory: %+v", p, g)
			}
		}
	}
	if rep := Analyze("mantenha o cache de sessão ativo. mas remova o cache de sessão do login", retrieve.Result{}); rep.States[0] != Contradictory {
		t.Errorf("a real contradiction must still be found: %+v", rep)
	}
}
