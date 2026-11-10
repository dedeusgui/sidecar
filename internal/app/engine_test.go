package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/engine"
)

// The hook keeps the v1 pipeline unless SIDECAR_ENGINE explicitly selects an opt-in engine.
func TestHookEngineSwitchDefaultsToV1(t *testing.T) {
	t.Setenv("SIDECAR_PROFILE", "")
	for _, value := range []string{"", "v1", "V3", "V2", "v4"} {
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
	t.Setenv("SIDECAR_ENGINE", "v3")
	t.Setenv("SIDECAR_FEATURES", "adaptive")
	if o := hookRetrieveOptions("/repo", filepath.Join(t.TempDir(), "s.json")); o.EngineVersion != "v3" || o.Adaptive || o.Seen == nil {
		t.Errorf("SIDECAR_ENGINE=v3 gave engine %q adaptive=%v seen=%v; v3 owns its policy and reads session memory", o.EngineVersion, o.Adaptive, o.Seen != nil)
	}
	t.Setenv("SIDECAR_ENGINE", engine.Version35)
	if o := hookRetrieveOptions("/repo", filepath.Join(t.TempDir(), "s.json")); o.EngineVersion != engine.Version35 || o.Adaptive || o.Seen == nil {
		t.Errorf("SIDECAR_ENGINE=v3.5 gave engine %q adaptive=%v seen=%v; v3.5 owns its policy and reads session memory", o.EngineVersion, o.Adaptive, o.Seen != nil)
	}
	t.Setenv("SIDECAR_PROFILE", "minimal")
	if o := hookRetrieveOptions("/repo", ""); o.Profile != "minimal" {
		t.Errorf("profile = %q, want minimal", o.Profile)
	}
}

func TestHookV3AbstainsOnConversationAndAnswersCode(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	t.Setenv("SIDECAR_ENGINE", "v3")
	t.Setenv("SIDECAR_FEATURES", "")
	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, root, "bom dia! tudo bem com você?", "s3"), &spawned)
	if strings.Contains(out, "additionalContext") {
		t.Fatalf("v3 must emit nothing for a greeting: %q", out)
	}
	if !strings.Contains(log, `"v3_action":"ABSTAIN"`) || !strings.Contains(log, `"engine":"v3"`) {
		t.Fatalf("decision not logged: %s", log)
	}
	out, log = runHook(t, adapter.Claude, payload(t, root, "onde o ManualReleaseHandler libera o veículo da fila?", "s3"), &spawned)
	if !strings.Contains(out, "additionalContext") || !strings.Contains(out, "ManualReleaseHandler") {
		t.Fatalf("v3 dropped a code question: %q\nlog: %s", out, log)
	}
	if strings.Contains(log, "libera o ve") || strings.Contains(log, "ManualReleaseHandler") {
		t.Fatal("prompt text or locator was written to the diagnostic log")
	}
	var rec map[string]any
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["v3_action"] == "ABSTAIN" || rec["v3_relevance"] != "relevant" {
		t.Fatalf("unexpected decision: %v", rec)
	}
}
