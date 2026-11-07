package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func statusLineOf(t *testing.T, path string) map[string]any {
	t.Helper()
	sl, _ := readJSON(t, path)["statusLine"].(map[string]any)
	return sl
}

func writeSettings(t *testing.T, env Env, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(env.ClaudeSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.ClaudeSettings, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStatusLineIsOptInAndRollsBack(t *testing.T) {
	env := testEnv(t)
	original := `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"keep-me"}]}]}}`
	writeSettings(t, env, original)

	// Without --statusline nothing is added to the status line.
	opts, _ := yes(t, env)
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	if statusLineOf(t, env.ClaudeSettings) != nil {
		t.Fatal("status line installed without opt-in")
	}

	opts.StatusLine = true
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	sl := statusLineOf(t, env.ClaudeSettings)
	cmd, _ := sl["command"].(string)
	if sl["type"] != "command" || !IsOursStatusLine(cmd) {
		t.Fatalf("status line = %v", sl)
	}

	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if statusLineOf(t, env.ClaudeSettings) != nil {
		t.Fatal("status line remains after uninstall")
	}
	if !strings.Contains(string(mustRead(t, env.ClaudeSettings)), "keep-me") {
		t.Fatal("user's hook removed by uninstall")
	}
}

func TestForeignStatusLineIsPreserved(t *testing.T) {
	env := testEnv(t)
	writeSettings(t, env, `{"statusLine":{"type":"command","command":"other-tool"}}`)
	opts, _ := yes(t, env)
	opts.StatusLine = true
	done, err := Install(opts)
	if err != nil {
		t.Fatal(err)
	}
	if sl := statusLineOf(t, env.ClaudeSettings); sl["command"] != "other-tool" {
		t.Fatalf("foreign status line replaced: %v", sl)
	}
	if !containsLine(done, "preservada") {
		t.Fatalf("no notice about the preserved status line: %v", done)
	}
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if sl := statusLineOf(t, env.ClaudeSettings); sl["command"] != "other-tool" {
		t.Fatalf("uninstall removed a status line Sidecar did not add: %v", sl)
	}
}

func TestUninstallKeepsStatusLineEditedByUser(t *testing.T) {
	env := testEnv(t)
	writeSettings(t, env, `{}`)
	opts, _ := yes(t, env)
	opts.StatusLine = true
	if _, err := Install(opts); err != nil {
		t.Fatal(err)
	}
	// The user replaces Sidecar's status line after install: uninstall must not touch it.
	writeSettings(t, env, `{"statusLine":{"type":"command","command":"my-own-line"}}`)
	if _, err := Uninstall(UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	if sl := statusLineOf(t, env.ClaudeSettings); sl["command"] != "my-own-line" {
		t.Fatalf("user's status line removed: %v", sl)
	}
}

func containsLine(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
