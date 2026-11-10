package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/install"
	"github.com/dedeusgui/sidecar/internal/paths"
)

// buildSidecar compiles cmd/sidecar into a temporary folder and returns the binary path.
func buildSidecar(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), install.BinaryName)
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/sidecar")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// runCLI runs one invocation of the binary in dir with the test's environment and returns its
// stdout and exit code. Stderr is logged, so a failing invocation explains itself.
func runCLI(t *testing.T, bin, dir, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("sidecar %v timed out", args)
	}
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("sidecar %v: %v", args, err)
	}
	if code != 0 {
		t.Logf("sidecar %v stderr:\n%s", args, stderr.String())
	}
	return stdout.String(), code
}

// readJSONFile parses a JSON object file.
func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(readFile(t, path), &m); err != nil {
		t.Fatalf("%s is not a JSON object: %v", path, err)
	}
	return m
}

// sameJSONFile reports whether the file at path holds the same JSON document as want.
func sameJSONFile(t *testing.T, path, want string) bool {
	t.Helper()
	var got, expected any
	if err := json.Unmarshal(readFile(t, path), &got); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(got, expected)
}

// claudeHandlers returns the commands and matchers registered under one event of a Claude Code
// or Codex hook file, one entry per handler.
func claudeHandlers(t *testing.T, path, event string) (commands, matchers []string) {
	t.Helper()
	hooks, _ := readJSONFile(t, path)["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	for _, g := range groups {
		group := g.(map[string]any)
		matcher, _ := group["matcher"].(string)
		for _, h := range group["hooks"].([]any) {
			cmd, _ := h.(map[string]any)["command"].(string)
			commands = append(commands, cmd)
			matchers = append(matchers, matcher)
		}
	}
	return commands, matchers
}

// antigravitySidecarGroup is the group Sidecar owns in Antigravity CLI's hooks file (the value of
// the installer's private antigravityGroup).
const antigravitySidecarGroup = "sidecar"

// antigravityHandlers returns the commands registered under one event of Sidecar's group in
// Antigravity CLI's hooks file.
func antigravityHandlers(t *testing.T, path, event string) []string {
	t.Helper()
	group, _ := readJSONFile(t, path)[antigravitySidecarGroup].(map[string]any)
	var commands []string
	for _, h := range group[event].([]any) {
		cmd, _ := h.(map[string]any)["command"].(string)
		commands = append(commands, cmd)
	}
	return commands
}

// hookReply is the stdout of a Claude Code or Codex prompt hook that injects context.
type hookReply struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// antigravityReply is the stdout of an Antigravity CLI PreInvocation hook that injects context.
type antigravityReply struct {
	InjectSteps []struct {
		EphemeralMessage string `json:"ephemeralMessage"`
	} `json:"injectSteps"`
}

// TestCLIEndToEndWithAllAdapters runs the compiled binary the way the agents do: install with every
// optional feature, prompt hooks for each client, tool observation, status commands, then uninstall
// with purge. Every configuration file must come back as it was.
func TestCLIEndToEndWithAllAdapters(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the sidecar binary")
	}
	// Build before HOME is redirected: the go tool's module cache lives under the real HOME.
	bin := buildSidecar(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("SIDECAR_HOME", filepath.Join(home, "cache"))
	t.Setenv("SIDECAR_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("SIDECAR_ENGINE", "")

	// The user's own configuration, which install must merge into and uninstall must restore.
	// Codex has no file before the install, so uninstall must remove the one it creates.
	claudeOriginal := `{"model":"keep","hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"user-prompt"}]}]}}`
	antigravityOriginal := `{"userGroup":{"PreInvocation":[{"type":"command","command":"user-agy"}]}}`
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), claudeOriginal)
	writeFile(t, filepath.Join(home, ".gemini", "config", "hooks.json"), antigravityOriginal)

	env, err := install.DefaultEnv()
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(env.InstallDir, install.BinaryName)
	repo := makeProject(t)
	gitInit(t, repo)

	out, code := runCLI(t, bin, repo, "", "install", "--yes", "--agent", "claude", "--agent", "codex",
		"--agent", "antigravity", "--statusline", "--observe-tools")
	if code != 0 || !strings.Contains(out, "Pronto.") {
		t.Fatalf("install exit=%d:\n%s", code, out)
	}

	// The installed files point at the installed binary, and the user's handlers are still there.
	claudeCmds, _ := claudeHandlers(t, env.ClaudeSettings, "UserPromptSubmit")
	if len(claudeCmds) != 2 || claudeCmds[0] != "user-prompt" || claudeCmds[1] != install.HookCommand(installed, "claude") {
		t.Fatalf("claude UserPromptSubmit = %q", claudeCmds)
	}
	for _, ev := range []string{"PreToolUse", "PostToolUse"} {
		cmds, matchers := claudeHandlers(t, env.ClaudeSettings, ev)
		if len(cmds) != 1 || !strings.HasSuffix(cmds[0], " observe --agent claude --event "+ev) || matchers[0] != "*" {
			t.Fatalf("claude %s = %q matchers %q", ev, cmds, matchers)
		}
	}
	if sl, _ := readJSONFile(t, env.ClaudeSettings)["statusLine"].(map[string]any); sl == nil || !strings.HasSuffix(sl["command"].(string), " statusline") {
		t.Fatalf("statusLine not installed: %v", sl)
	}
	if codexCmds, _ := claudeHandlers(t, env.CodexHooks, "UserPromptSubmit"); len(codexCmds) != 1 || codexCmds[0] != install.HookCommand(installed, "codex") {
		t.Fatalf("codex hook = %q", codexCmds)
	}
	if cmds := antigravityHandlers(t, env.AntigravityHooks, "PreInvocation"); len(cmds) != 1 || cmds[0] != install.HookCommand(installed, "antigravity") {
		t.Fatalf("antigravity PreInvocation = %q", cmds)
	}
	for ev, observed := range map[string]string{"PreTool": "PreToolUse", "PostTool": "PostToolUse"} {
		cmds := antigravityHandlers(t, env.AntigravityHooks, ev)
		if len(cmds) != 1 || !strings.HasSuffix(cmds[0], " observe --agent antigravity --event "+observed) {
			t.Fatalf("antigravity %s = %q", ev, cmds)
		}
	}

	if out, code := runCLI(t, installed, repo, "", "doctor"); code != 0 || !strings.Contains(out, "hook UserPromptSubmit instalado") ||
		!strings.Contains(out, "V3.5 inativa") || !strings.Contains(out, "export SIDECAR_ENGINE=v3.5") ||
		!strings.Contains(out, "antigravity") || !strings.Contains(out, "prompt=unverified") {
		t.Fatalf("doctor exit=%d:\n%s", code, out)
	}

	// Prompt hooks: each client's payload in, its own reply schema out.
	const prompt = "o MAX_ACTIVE_TICKETS limita a fila?"
	for _, agent := range []string{"claude", "codex"} {
		in, _ := json.Marshal(map[string]any{"session_id": "e2e-" + agent, "cwd": repo, "hook_event_name": "UserPromptSubmit", "prompt": prompt})
		out, code := runCLI(t, installed, repo, string(in), "hook", "--agent", agent)
		var reply hookReply
		if code != 0 || json.Unmarshal([]byte(out), &reply) != nil {
			t.Fatalf("%s hook exit=%d, stdout %q is not the hookSpecificOutput schema", agent, code, out)
		}
		if reply.HookSpecificOutput.HookEventName != "UserPromptSubmit" ||
			!strings.Contains(reply.HookSpecificOutput.AdditionalContext, "[SIDECAR CONTEXT]") ||
			!strings.Contains(reply.HookSpecificOutput.AdditionalContext, "MaxActiveTickets") {
			t.Fatalf("%s hook reply = %+v", agent, reply)
		}
	}
	in, _ := json.Marshal(map[string]any{"conversationId": "e2e-agy", "workspacePaths": []string{repo}, "lastUserInput": prompt, "invocationNum": 0})
	out, code = runCLI(t, installed, repo, string(in), "hook", "--agent", "antigravity")
	var agy antigravityReply
	if code != 0 || json.Unmarshal([]byte(out), &agy) != nil || len(agy.InjectSteps) != 1 ||
		!strings.Contains(agy.InjectSteps[0].EphemeralMessage, "MaxActiveTickets") {
		t.Fatalf("antigravity hook exit=%d, stdout %q is not the injectSteps schema", code, out)
	}

	// Tool observation is silent and records metadata only.
	target := filepath.Join(repo, "queue", "limits.go")
	pre, _ := json.Marshal(map[string]any{"session_id": "e2e-claude", "cwd": repo, "hook_event_name": "PreToolUse",
		"tool_name": "Read", "tool_input": map[string]any{"file_path": target}, "tool_use_id": "e2e-call-1"})
	if out, code := runCLI(t, installed, repo, string(pre), "observe", "--agent", "claude", "--event", "PreToolUse"); code != 0 || out != "" {
		t.Fatalf("observe PreToolUse exit=%d stdout=%q", code, out)
	}
	post, _ := json.Marshal(map[string]any{"session_id": "e2e-claude", "cwd": repo, "hook_event_name": "PostToolUse",
		"tool_name": "Read", "tool_input": map[string]any{"file_path": target}, "tool_use_id": "e2e-call-1"})
	if out, code := runCLI(t, installed, repo, string(post), "observe", "--agent", "claude", "--event", "PostToolUse"); code != 0 || out != "" {
		t.Fatalf("observe PostToolUse exit=%d stdout=%q", code, out)
	}
	logData := readFile(t, paths.HookLog())
	observed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(logData)), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("hook log line is not JSON: %s", line)
		}
		if ev, _ := rec["event"].(string); rec["status"] == "observed" {
			observed[ev] = true
		}
	}
	if !observed["PreToolUse"] || !observed["PostToolUse"] {
		t.Fatalf("observed events = %v, log:\n%s", observed, logData)
	}
	for _, leaked := range []string{"limits.go", "e2e-claude", "e2e-call-1", repo, prompt} {
		if bytes.Contains(logData, []byte(leaked)) {
			t.Fatalf("hook log contains %q", leaked)
		}
	}

	// Status commands read what the hooks left behind.
	out, code = runCLI(t, installed, repo, "", "status", "--root", repo, "--json")
	var status map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || status["files"].(float64) < 1 ||
		status["engine"] != "v1" || status["v35_active"] != false ||
		!strings.Contains(status["engine_activation"].(string), "SIDECAR_ENGINE=v3.5") {
		t.Fatalf("status exit=%d:\n%s", code, out)
	}
	t.Setenv("SIDECAR_ENGINE", "v3.5")
	out, code = runCLI(t, installed, repo, "", "status", "--root", repo, "--json")
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || status["engine"] != "v3.5" || status["v35_active"] != true {
		t.Fatalf("status did not report the opt-in engine immediately: exit=%d\n%s", code, out)
	}
	t.Setenv("SIDECAR_ENGINE", "")
	out, code = runCLI(t, installed, repo, "", "status", "--root", repo, "--json")
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || status["engine"] != "v1" || status["v35_active"] != false {
		t.Fatalf("status did not report immediate rollback to default: exit=%d\n%s", code, out)
	}
	out, code = runCLI(t, installed, repo, "", "activity", "--json")
	var activity map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &activity) != nil || activity["runs"].(float64) < 1 {
		t.Fatalf("activity exit=%d:\n%s", code, out)
	}
	if out, code := runCLI(t, installed, repo, `{"session_id":"e2e-claude"}`, "statusline"); code != 0 || !strings.HasPrefix(out, "Sidecar:") {
		t.Fatalf("statusline exit=%d: %q", code, out)
	}

	// Uninstall with purge: the user's configuration comes back, and everything Sidecar created is gone.
	if out, code := runCLI(t, bin, repo, "", "uninstall", "--purge"); code != 0 || !strings.Contains(out, "Sidecar desinstalado") {
		t.Fatalf("uninstall exit=%d:\n%s", code, out)
	}
	if !sameJSONFile(t, env.ClaudeSettings, claudeOriginal) {
		t.Fatalf("claude settings not restored:\n%s", readFile(t, env.ClaudeSettings))
	}
	if !sameJSONFile(t, env.AntigravityHooks, antigravityOriginal) {
		t.Fatalf("antigravity hooks not restored:\n%s", readFile(t, env.AntigravityHooks))
	}
	for _, gone := range []string{env.CodexHooks, env.ClaudeSettings + install.BackupSuffix, env.Manifest, installed, paths.Home()} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone after uninstall --purge: %v", gone, err)
		}
	}
}
