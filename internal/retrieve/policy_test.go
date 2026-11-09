package retrieve

import "testing"

func TestPolicyForSkipsOnlyPureLookups(t *testing.T) {
	cases := []struct {
		kinds []string
		skip  bool
	}{
		{[]string{"locate"}, true}, {[]string{"general"}, true}, {[]string{"locate", "general"}, true},
		{[]string{"locate", "debug"}, false}, {[]string{"contract"}, false}, {[]string{"impact"}, false},
		{[]string{"extend"}, false}, {[]string{"test"}, false}, {[]string{"explain"}, false}, {nil, false},
	}
	for _, c := range cases {
		if got := PolicyFor(Intent{Kinds: c.kinds}, "onde fica o MaxRetries").SkipContracts; got != c.skip {
			t.Errorf("kinds %v: skip=%v, want %v", c.kinds, got, c.skip)
		}
	}
}

func TestFeatureEnabledParsesList(t *testing.T) {
	t.Setenv("SIDECAR_FEATURES", " brief , adaptive,,x")
	if !FeatureEnabled("adaptive") || !FeatureEnabled("brief") || FeatureEnabled("groom") {
		t.Fatal("feature list parsed incorrectly")
	}
	t.Setenv("SIDECAR_FEATURES", "")
	if FeatureEnabled("adaptive") {
		t.Fatal("unset must be off")
	}
}

func TestPolicyNeverSkipsContractsForAnEditRequest(t *testing.T) {
	for _, p := range []string{"Change the session TTL to 30 minutes", "Altere o MaxRetries para 5", "Mude o limite de tentativas para 5", "aumente o MaxActiveTickets"} {
		in := ClassifyIntent(p)
		if PolicyFor(in, p).SkipContracts {
			t.Errorf("%q (intent %v) is an edit request: contracts decide what must be preserved", p, in.Kinds)
		}
	}
	if !PolicyFor(ClassifyIntent("onde fica o MaxRetries?"), "onde fica o MaxRetries?").SkipContracts {
		t.Error("a pure lookup keeps the saving")
	}
}

func TestForbiddenEditVerbsMakeAPromptReadOnly(t *testing.T) {
	readOnly := []string{
		"No middleware de throttle do chi, qual arquivo monta o header Retry-After? Só preciso da localização. Não altere nada no repositório.",
		"Where is the retry header built? Don't change anything, I only need the location.",
		"explique o fluxo sem modificar o código",
	}
	for _, p := range readOnly {
		if IsEditRequest(p) {
			t.Errorf("%q forbids editing and must read as read-only", p)
		}
	}
	edits := []string{"Altere o MaxRetries para 5, mas não mexa na documentação", "Change the TTL and do not touch the tests", "corrija o erro; não precisa explicar"}
	for _, p := range edits {
		if !IsEditRequest(p) {
			t.Errorf("%q asks for an edit (only a different action is forbidden)", p)
		}
	}
}

func TestEditIntentIgnoresQuotedInstructionsButKeepsOtherClauses(t *testing.T) {
	readOnly := []string{
		`What does "change the session TTL" mean? Only explain the request.`,
		`A mensagem “corrija o valor” apareceu no log; explique o significado.`,
		`The README says 'change the cache TTL'; where is that documented?`,
	}
	for _, p := range readOnly {
		if IsEditRequest(p) {
			t.Errorf("quoted imperative in %q is data, not an edit request", p)
		}
	}

	edits := []string{
		"Não altere o timeout; corrija o parser.",
		"Do not change the timeout; fix the parser.",
		`Explain why the log says "change the TTL", then update the parser.`,
	}
	for _, p := range edits {
		if !IsEditRequest(p) {
			t.Errorf("an unquoted positive clause in %q still asks for an edit", p)
		}
	}
}
