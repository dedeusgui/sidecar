package app

import "testing"

// The hook keeps the v1 pipeline unless SIDECAR_ENGINE is exactly v2; only v2 reads session memory.
func TestHookEngineSwitchDefaultsToV1(t *testing.T) {
	t.Setenv("SIDECAR_PROFILE", "")
	for _, value := range []string{"", "v1", "v3", "V2"} {
		t.Setenv("SIDECAR_ENGINE", value)
		o := hookRetrieveOptions("/repo", "")
		if o.EngineVersion != "" || o.Seen != nil || o.Root != "/repo" {
			t.Errorf("SIDECAR_ENGINE=%q gave engine %q, seen=%v, root %q; want v1 with the root", value, o.EngineVersion, o.Seen != nil, o.Root)
		}
	}
	t.Setenv("SIDECAR_ENGINE", "v2")
	if o := hookRetrieveOptions("/repo", ""); o.EngineVersion != "v2" {
		t.Errorf("SIDECAR_ENGINE=v2 gave engine %q", o.EngineVersion)
	}
	t.Setenv("SIDECAR_PROFILE", "minimal")
	if o := hookRetrieveOptions("/repo", ""); o.Profile != "minimal" {
		t.Errorf("profile = %q, want minimal", o.Profile)
	}
}
