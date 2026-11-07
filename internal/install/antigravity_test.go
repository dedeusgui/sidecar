package install

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func antigravityEnv(t *testing.T) Env {
	t.Helper()
	env := testEnv(t)
	env.AntigravityHooks = filepath.Join(filepath.Dir(env.Manifest), "gemini", "config", "hooks.json")
	return env
}

// countAntigravity counts Sidecar's PreInvocation handlers in Antigravity's hooks.json.
func countAntigravity(t *testing.T, path string) int {
	t.Helper()
	n := 0
	group, _ := readJSON(t, path)[antigravityGroup].(map[string]any)
	for _, h := range asList(group["PreInvocation"]) {
		cmd, _ := h.(map[string]any)["command"].(string)
		if IsOurs(cmd, "antigravity") {
			n++
		}
	}
	return n
}

func TestDetectAntigravityFromConfigFolder(t *testing.T) {
	env := antigravityEnv(t)
	env.ClaudeSettings = filepath.Join(t.TempDir(), "none", "settings.json")
	env.CodexHooks = filepath.Join(t.TempDir(), "none", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(env.AntigravityHooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if !contains(Detect(env), "antigravity") {
		t.Fatal("antigravity not detected from its config folder")
	}
	env.AntigravityHooks = ""
	for _, a := range Detect(env) {
		if a == "antigravity" {
			t.Fatal("antigravity detected without a hooks path")
		}
	}
}

func TestInstallAntigravityCreatesGroupAndIsIdempotent(t *testing.T) {
	env := antigravityEnv(t)
	for i := 0; i < 2; i++ {
		if _, err := Install(Options{Env: env, Agents: []string{"antigravity"}, Yes: true, Out: &bytes.Buffer{}}); err != nil {
			t.Fatal(err)
		}
	}
	if n := countAntigravity(t, env.AntigravityHooks); n != 1 {
		t.Fatalf("Sidecar handlers = %d, want 1", n)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestInstallAntigravityPreservesUserGroupsAndUninstallRemovesOnlyOurs(t *testing.T) {
	env := antigravityEnv(t)
	userHooks := `{"my-linter":{"PostToolUse":[{"matcher":"run_command","hooks":[{"type":"command","command":"./lint.sh"}]}]}}`
	if err := os.MkdirAll(filepath.Dir(env.AntigravityHooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.AntigravityHooks, []byte(userHooks), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(Options{Env: env, Agents: []string{"antigravity"}, Yes: true, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.AntigravityHooks + BackupSuffix); err != nil {
		t.Fatalf("no backup of the existing hooks file: %v", err)
	}
	obj := readJSON(t, env.AntigravityHooks)
	if _, ok := obj["my-linter"]; !ok || countAntigravity(t, env.AntigravityHooks) != 1 {
		t.Fatalf("install changed user hooks: %v", obj)
	}

	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	obj = readJSON(t, env.AntigravityHooks)
	if _, ok := obj["my-linter"]; !ok {
		t.Fatalf("uninstall removed a user group: %v", obj)
	}
	if _, ok := obj[antigravityGroup]; ok {
		t.Fatalf("uninstall left Sidecar's group behind: %v", obj)
	}
}

func TestUninstallRemovesAntigravityFileItCreated(t *testing.T) {
	env := antigravityEnv(t)
	if _, err := Install(Options{Env: env, Agents: []string{"antigravity"}, Yes: true, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.AntigravityHooks); !os.IsNotExist(err) {
		t.Fatalf("hooks file created by Sidecar should be removed, stat err = %v", err)
	}
}
