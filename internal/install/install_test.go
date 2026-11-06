package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testEnv(t *testing.T) Env {
	t.Helper()
	base := t.TempDir()
	src := filepath.Join(base, "build", "sidecar")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("#!fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return Env{
		Source:         src,
		InstallDir:     filepath.Join(base, "bin"),
		ClaudeSettings: filepath.Join(base, "claude", "settings.json"),
		CodexHooks:     filepath.Join(base, "codex", "hooks.json"),
		CodexConfig:    filepath.Join(base, "codex", "config.toml"),
		Manifest:       filepath.Join(base, "config", "install.json"),
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v\n%s", path, err, data)
	}
	return m
}

// countOurs counts handlers whose command is a sidecar hook for the agent.
func countOurs(t *testing.T, path, agent string) int {
	t.Helper()
	n := 0
	m := readJSON(t, path)
	hooks, _ := m["hooks"].(map[string]any)
	for _, g := range asList(hooks["UserPromptSubmit"]) {
		for _, h := range asList(g.(map[string]any)["hooks"]) {
			cmd, _ := h.(map[string]any)["command"].(string)
			if IsOurs(cmd, agent) {
				n++
			}
		}
	}
	return n
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func yes(t *testing.T, env Env) (Options, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return Options{Env: env, Yes: true, Out: &out, Agents: []string{"claude", "codex"}}, &out
}

func TestInstallPreservesUserSettingsAndAddsHook(t *testing.T) {
	env := testEnv(t)
	if err := os.MkdirAll(filepath.Dir(env.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"model":"opus","hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/usr/local/bin/notify-me"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"lint"}]}]},"permissions":{"allow":["Bash(ls)"]}}`
	if err := os.WriteFile(env.ClaudeSettings, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatalf("Install: %v", err)
	}
	m := readJSON(t, env.ClaudeSettings)
	if m["model"] != "opus" {
		t.Fatalf("user setting lost: %v", m)
	}
	if _, ok := m["permissions"]; !ok {
		t.Fatalf("permissions lost: %v", m)
	}
	hooks := m["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatal("unrelated hook event lost")
	}
	if countOurs(t, env.ClaudeSettings, "claude") != 1 {
		t.Fatal("sidecar hook not installed exactly once")
	}
	if !strings.Contains(string(mustRead(t, env.ClaudeSettings)), "notify-me") {
		t.Fatal("user's own UserPromptSubmit handler was removed")
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	env := testEnv(t)
	opts, _ := yes(t, env)
	for i := 0; i < 3; i++ {
		if _, err := Install(opts); err != nil {
			t.Fatalf("Install #%d: %v", i+1, err)
		}
	}
	if n := countOurs(t, env.ClaudeSettings, "claude"); n != 1 {
		t.Fatalf("hook duplicated: %d entries", n)
	}
	if n := countOurs(t, env.CodexHooks, "codex"); n != 1 {
		t.Fatalf("codex hook duplicated: %d entries", n)
	}
}

func TestInstallRequiresConsentWhenNotConfirmed(t *testing.T) {
	env := testEnv(t)
	var out bytes.Buffer
	_, err := Install(Options{Env: env, In: strings.NewReader("n\n"), Out: &out, Agents: []string{"claude"}})
	if err != nil {
		t.Fatalf("declining should not be an error: %v", err)
	}
	if _, statErr := os.Stat(env.ClaudeSettings); !os.IsNotExist(statErr) {
		t.Fatal("settings written without consent")
	}
	if _, statErr := os.Stat(env.InstallDir); !os.IsNotExist(statErr) {
		t.Fatal("binary installed without consent")
	}
}

func TestNonInteractiveInstallWithoutYesFails(t *testing.T) {
	env := testEnv(t)
	if _, err := Install(Options{Env: env, Out: &bytes.Buffer{}, Agents: []string{"claude"}}); err == nil {
		t.Fatal("install without a terminal and without --yes must not proceed silently")
	}
}

func TestCodexHooksJSONMergedWithExistingHandlers(t *testing.T) {
	env := testEnv(t)
	if err := os.MkdirAll(filepath.Dir(env.CodexHooks), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.CodexHooks, []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, env.CodexHooks)
	if _, ok := m["hooks"].(map[string]any)["SessionStart"]; !ok {
		t.Fatal("existing codex hook lost")
	}
	if countOurs(t, env.CodexHooks, "codex") != 1 {
		t.Fatal("codex sidecar hook missing")
	}
}

func TestBackupIsTakenOnceAndUninstallRemovesOnlyOurHooks(t *testing.T) {
	env := testEnv(t)
	if err := os.MkdirAll(filepath.Dir(env.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"keep-me"}]}]}}`
	if err := os.WriteFile(env.ClaudeSettings, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	backup := env.ClaudeSettings + BackupSuffix
	if string(mustRead(t, backup)) != original {
		t.Fatal("backup does not hold the original settings")
	}
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, backup)) != original {
		t.Fatal("second install overwrote the original backup")
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if countOurs(t, env.ClaudeSettings, "claude") != 0 {
		t.Fatal("sidecar hook remains after uninstall")
	}
	if !strings.Contains(string(mustRead(t, env.ClaudeSettings)), "keep-me") {
		t.Fatal("user's hook removed by uninstall")
	}
	if _, err := os.Stat(env.InstallDir); err == nil {
		entries, _ := os.ReadDir(env.InstallDir)
		for _, e := range entries {
			if e.Name() == "sidecar" {
				t.Fatal("installed binary not removed")
			}
		}
	}
}

func TestUninstallDeletesFilesItCreated(t *testing.T) {
	env := testEnv(t)
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.ClaudeSettings); !os.IsNotExist(err) {
		t.Fatalf("settings file created by sidecar should be removed when left empty, stat err=%v", err)
	}
	if _, err := os.Stat(env.Manifest); !os.IsNotExist(err) {
		t.Fatal("manifest not removed")
	}
}

func TestInstalledBinaryIsExecutableCopy(t *testing.T) {
	env := testEnv(t)
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(env.InstallDir, BinaryName))
	if err != nil {
		t.Fatalf("binary not installed: %v", err)
	}
	// Windows does not expose executable bits through os.FileMode. The .exe
	// extension identifies native executables there; on Unix check permissions.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("installed binary is not executable: %v", info.Mode())
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("installed binary is not a regular file: %v", info.Mode())
	}
	if got, want := string(mustRead(t, filepath.Join(env.InstallDir, BinaryName))), string(mustRead(t, env.Source)); got != want {
		t.Fatalf("installed binary contents differ from source")
	}
}

func TestHookCommandQuotesPathsWithSpaces(t *testing.T) {
	cmd := HookCommand("/Users/me/My Apps/sidecar", "claude")
	if cmd != `"/Users/me/My Apps/sidecar" hook --agent claude` && !strings.HasPrefix(cmd, `"`) {
		t.Fatalf("path with spaces not quoted: %s", cmd)
	}
	if !IsOurs(cmd, "claude") || IsOurs(cmd, "codex") || IsOurs("notify-me", "claude") {
		t.Fatal("ownership detection is wrong")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInstallRefusesToOverwriteUnmanagedBinary(t *testing.T) {
	env := testEnv(t)
	dst := filepath.Join(env.InstallDir, BinaryName)
	if err := os.MkdirAll(env.InstallDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("user-owned executable, do not replace")
	if err := os.WriteFile(dst, original, 0o755); err != nil {
		t.Fatal(err)
	}
	opts, _ := yes(t, env)
	if _, err := Install(opts); err == nil || !strings.Contains(err.Error(), "unmanaged binary") {
		t.Fatalf("expected a safe refusal, got %v", err)
	}
	if got := mustRead(t, dst); !bytes.Equal(got, original) {
		t.Fatal("install overwrote an existing unmanaged executable")
	}
	if _, err := os.Stat(env.ClaudeSettings); !os.IsNotExist(err) {
		t.Fatal("install changed hooks despite refusing the binary replacement")
	}
}

func TestUninstallPreservesDifferentSidecarExecutableHook(t *testing.T) {
	env := testEnv(t)
	other := HookCommand(filepath.Join(env.InstallDir, "elsewhere", BinaryName), "claude")
	if err := os.MkdirAll(filepath.Dir(env.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
		"hooks": map[string]any{
			"UserPromptSubmit": []any{map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": other}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.ClaudeSettings, data, 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := yes(t, env)
	opts.Agents = []string{"claude"}
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mustRead(t, env.ClaudeSettings)), other) {
		t.Fatal("uninstall removed an unrelated user's sidecar hook")
	}
}
