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
