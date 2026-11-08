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
		if got := PolicyFor(Intent{Kinds: c.kinds}).SkipContracts; got != c.skip {
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
