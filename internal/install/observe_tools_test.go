package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// oursFor matches Sidecar's command for an agent: its prompt hook when observed is empty, or
// the observer of one tool event otherwise.
func oursFor(agent, observed string) func(string) bool {
	if observed == "" {
		return func(cmd string) bool { return IsOurs(cmd, agent) }
	}
	return func(cmd string) bool {
		suffix := " observe --agent " + agent + " --event " + observed
		return strings.HasSuffix(cmd, suffix) && isSidecarBinary(strings.TrimSuffix(cmd, suffix))
	}
}

// countEvent counts the handlers under one client event that match. Claude Code nests handlers
// in matcher groups; Antigravity CLI keeps them in Sidecar's group.
func countEvent(t *testing.T, path, agent, event string, match func(string) bool) int {
	t.Helper()
	obj := readJSON(t, path)
	var handlers []any
	if agent == "antigravity" {
		group, _ := obj[antigravityGroup].(map[string]any)
		handlers = asList(group[event])
	} else {
		hooks, _ := obj["hooks"].(map[string]any)
		for _, g := range asList(hooks[event]) {
			handlers = append(handlers, asList(g.(map[string]any)["hooks"])...)
		}
	}
	n := 0
	for _, h := range handlers {
		cmd, _ := h.(map[string]any)["command"].(string)
		if match(cmd) {
			n++
		}
	}
	return n
}

// sameJSON reports whether the file at path holds the same JSON document as original.
func sameJSON(t *testing.T, path, original string) bool {
	t.Helper()
	var got, want any
	if err := json.Unmarshal(mustRead(t, path), &got); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if err := json.Unmarshal([]byte(original), &want); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(got, want)
}

func TestInstallAndUninstallObserveTools(t *testing.T) {
	env := antigravityEnv(t)
	claudeOriginal := `{"model":"keep","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"user-guard"}]}],` +
		`"UserPromptSubmit":[{"hooks":[{"type":"command","command":"user-prompt"}]}]}}`
	antigravityOriginal := `{"userGroup":{"PreInvocation":[{"type":"command","command":"user-agy"}]}}`
	for path, body := range map[string]string{env.ClaudeSettings: claudeOriginal, env.AntigravityHooks: antigravityOriginal} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	opts := Options{Env: env, Agents: []string{"claude", "antigravity"}, Yes: true, Out: &out, ObserveTools: true}
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PreToolUse, PostToolUse") || !strings.Contains(out.String(), "PreTool, PostTool") {
		t.Fatalf("consent plan does not show the tool observation:\n%s", out.String())
	}

	claudeObserve := map[string]string{"PreToolUse": "PreToolUse", "PostToolUse": "PostToolUse"}
	for event, observed := range claudeObserve {
		if n := countEvent(t, env.ClaudeSettings, "claude", event, oursFor("claude", observed)); n != 1 {
			t.Fatalf("claude %s observers = %d, want 1", event, n)
		}
	}
	if n := countEvent(t, env.ClaudeSettings, "claude", "UserPromptSubmit", oursFor("claude", "")); n != 1 {
		t.Fatalf("claude prompt hook = %d, want 1", n)
	}
	// Tool observers match every tool, so the group carries the "*" matcher.
	if !strings.Contains(string(mustRead(t, env.ClaudeSettings)), `"matcher": "*"`) {
		t.Fatal("tool observer group has no \"*\" matcher")
	}
	for event, observed := range map[string]string{"PreTool": "PreToolUse", "PostTool": "PostToolUse"} {
		if n := countEvent(t, env.AntigravityHooks, "antigravity", event, oursFor("antigravity", observed)); n != 1 {
			t.Fatalf("antigravity %s observers = %d, want 1", event, n)
		}
	}
	if n := countEvent(t, env.AntigravityHooks, "antigravity", "PreInvocation", oursFor("antigravity", "")); n != 1 {
		t.Fatalf("antigravity prompt hook = %d, want 1", n)
	}

	// Each handler is one manifest entry, so uninstall can remove exactly what was added.
	m, err := readManifest(env.Manifest)
	if err != nil || len(m.Entries) != 7 {
		t.Fatalf("manifest entries = %d, %v; want 7 (claude prompt + PreToolUse + PostToolUse + PreCompact, antigravity prompt + two tool hooks)", len(m.Entries), err)
	}
	if n := countEvent(t, env.ClaudeSettings, "claude", "PreCompact", oursFor("claude", "PreCompact")); n != 1 {
		t.Fatalf("claude PreCompact observer = %d, want 1 (compaction resets what the agent has seen)", n)
	}

	// Installing again changes nothing: no duplicate handlers and no duplicate entries.
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if n := countEvent(t, env.ClaudeSettings, "claude", "PreToolUse", oursFor("claude", "PreToolUse")); n != 1 {
		t.Fatalf("second install duplicated the observer: %d", n)
	}
	if m, _ := readManifest(env.Manifest); len(m.Entries) != 7 {
		t.Fatalf("second install changed the manifest: %d entries", len(m.Entries))
	}

	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !sameJSON(t, env.ClaudeSettings, claudeOriginal) {
		t.Fatalf("claude settings not restored:\n%s", mustRead(t, env.ClaudeSettings))
	}
	if !sameJSON(t, env.AntigravityHooks, antigravityOriginal) {
		t.Fatalf("antigravity hooks not restored:\n%s", mustRead(t, env.AntigravityHooks))
	}
}

func TestObserveToolsIsOptIn(t *testing.T) {
	env := testEnv(t)
	if _, err := Install(Options{Env: env, Agents: []string{"claude"}, Yes: true, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if n := countEvent(t, env.ClaudeSettings, "claude", "PreToolUse", oursFor("claude", "PreToolUse")); n != 0 {
		t.Fatalf("tool observer installed without --observe-tools: %d", n)
	}
	if m, _ := readManifest(env.Manifest); len(m.Entries) != 1 {
		t.Fatalf("manifest entries = %d, want 1 (the prompt hook)", len(m.Entries))
	}
}

func TestInstallOmitsUnverifiedCodexObservationEvents(t *testing.T) {
	env := testEnv(t)
	if _, err := Install(Options{Env: env, Agents: []string{"codex"}, Yes: true, Out: &bytes.Buffer{}, ObserveTools: true}); err != nil {
		t.Fatal(err)
	}
	obj := readJSON(t, env.CodexHooks)
	hooks, _ := obj["hooks"].(map[string]any)
	if len(hooks) != 1 || len(asList(hooks["UserPromptSubmit"])) != 1 {
		t.Fatalf("Codex should receive only its verified prompt hook, got %v", hooks)
	}
	for _, event := range []string{"PreToolUse", "PostToolUse", "PreCompact"} {
		if len(asList(hooks[event])) != 0 {
			t.Fatalf("Codex received unverified %s event: %v", event, hooks[event])
		}
	}
	if m, err := readManifest(env.Manifest); err != nil || len(m.Entries) != 1 || m.Entries[0].Event != "UserPromptSubmit" {
		t.Fatalf("manifest should record only the prompt hook, entries=%+v err=%v", m.Entries, err)
	}
}

// A manifest written before tool observation has no event field; its entries are prompt hooks
// and must still be removed by uninstall.
func TestUninstallReadsManifestWithoutEvents(t *testing.T) {
	env := testEnv(t)
	if _, err := Install(Options{Env: env, Agents: []string{"claude"}, Yes: true, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(env.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Entries {
		m.Entries[i].Event = ""
	}
	if err := saveManifest(env.Manifest, m); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(env.ClaudeSettings); !os.IsNotExist(err) {
		t.Fatalf("settings file created by sidecar still present: %v", err)
	}
}
