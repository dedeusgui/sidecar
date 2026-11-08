package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/install"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SIDECAR_HOME", filepath.Join(home, "cache"))
	return home
}

func makeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"release/handler.go": "package release\n\n// ManualReleaseHandler releases vehicles waiting in the ticket queue.\ntype ManualReleaseHandler struct{}\n\n// Release advances the first vehicle in the queue.\nfunc (h *ManualReleaseHandler) Release() error { return nil }\n",
		"queue/limits.go":    "package queue\n\n// MaxActiveTickets limits concurrent tickets.\nconst MaxActiveTickets = 1\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// hookJSON encodes a hook payload with encoding/json. Paths must be escaped by the encoder: on
// Windows they contain backslashes, and concatenating them into a JSON literal makes invalid
// escapes such as \U, so the observer rejects the whole event.
func hookJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// setUserHome points the user's home directory at dir. Go reads $HOME on Unix and %USERPROFILE%
// on Windows, so setting only HOME leaves the home guard untested there. The check fails loudly
// instead of letting a test pass without exercising the home directory.
func setUserHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	got, err := os.UserHomeDir()
	if err != nil || filepath.Clean(got) != filepath.Clean(dir) {
		t.Fatalf("user home = %q (%v), want %q", got, err, dir)
	}
}

func payload(t *testing.T, cwd, prompt, session string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{
		"session_id": session, "cwd": cwd, "prompt": prompt, "hook_event_name": "UserPromptSubmit",
	})
	return string(b)
}

func runHook(t *testing.T, agent adapter.Agent, input string, spawned *[]string) (string, string) {
	t.Helper()
	var out bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "hook.jsonl")
	Hook(HookOptions{
		Agent: agent, Stdin: strings.NewReader(input), Stdout: &out,
		LogPath: logPath, Budget: 5 * time.Second,
		Spawn: func(root string, history bool) error {
			*spawned = append(*spawned, root)
			return nil
		},
	})
	logData, _ := os.ReadFile(logPath)
	return out.String(), string(logData)
}

func TestHookInjectsEvidenceForPortuguesePrompt(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "s1"), &spawned)
	if !strings.Contains(out, "additionalContext") || !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("no context injected: %q", out)
	}
	if strings.Contains(log, "MAX_ACTIVE_TICKETS limita") {
		t.Fatal("prompt text was written to the diagnostic log")
	}
	if !strings.Contains(log, `"status":"ok"`) {
		t.Fatalf("log does not record status: %s", log)
	}
}

func TestHookFailsOpenOnBadPayload(t *testing.T) {
	setupHome(t)
	var spawned []string
	out, log := runHook(t, adapter.Claude, "{not json", &spawned)
	if out != "" {
		t.Fatalf("bad payload must not produce output, got %q", out)
	}
	if !strings.Contains(log, `"status":"bad_input"`) {
		t.Fatalf("failure not recorded for diagnostics: %s", log)
	}
}

func TestHookSkipsSlashCommandsAndTinyPrompts(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	for _, p := range []string{"/help", "ok", "   "} {
		if out, _ := runHook(t, adapter.Claude, payload(t, root, p, "s2"), &spawned); out != "" {
			t.Fatalf("prompt %q should be skipped, got %q", p, out)
		}
	}
}

func TestHookAnswersWithEmptyOutputWhenNothingRelates(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, root, "bom dia, tudo bem por aí?", "s3"), &spawned)
	if out != "" {
		t.Fatalf("unrelated prompt produced %q", out)
	}
	if !strings.Contains(log, `"status":"no_evidence"`) {
		t.Fatalf("log should say no evidence: %s", log)
	}
}

func TestCodexGetsJSONWhenThereIsNoEvidence(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	out, _ := runHook(t, adapter.Codex, payload(t, root, "bom dia, tudo bem por aí?", "s4"), &spawned)
	if out != "{}" {
		t.Fatalf("codex needs a JSON object on stdout, got %q", out)
	}
}

func TestSecondPromptInSameSessionIsNotRepeated(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	first, _ := runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "s5"), &spawned)
	second, _ := runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "s5"), &spawned)
	if first == "" {
		t.Fatal("first prompt got no context")
	}
	if second != "" {
		t.Fatalf("same evidence repeated in the same session: %q", second)
	}
}

func TestPartialSyncStillAnswersAndSchedulesBackgroundWork(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	var out bytes.Buffer
	Hook(HookOptions{
		Agent: adapter.Claude, Stdin: strings.NewReader(payload(t, root, "MAX_ACTIVE_TICKETS", "s6")),
		Stdout: &out, LogPath: filepath.Join(t.TempDir(), "l.jsonl"),
		Budget: time.Nanosecond,
		Spawn: func(r string, h bool) error {
			spawned = append(spawned, r)
			return nil
		},
	})
	if len(spawned) != 1 {
		t.Fatalf("background indexer not scheduled after a partial sync (spawned=%v)", spawned)
	}
}

func TestHookRecoversFromCorruptIndex(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	runHook(t, adapter.Claude, payload(t, root, "MAX_ACTIVE_TICKETS", "s7"), &spawned)
	if err := os.WriteFile(paths.IndexDB(root), []byte("garbage garbage garbage garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, log := runHook(t, adapter.Claude, payload(t, root, "MAX_ACTIVE_TICKETS", "s8"), &spawned)
	if !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("corrupt index blocked the answer: %q (log %s)", out, log)
	}
}

func TestUnsafeRootIsNotIndexed(t *testing.T) {
	setupHome(t)
	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, string(filepath.Separator), "MAX_ACTIVE_TICKETS", "s9"), &spawned)
	if out != "" || !strings.Contains(log, "skipped_unsafe_root") {
		t.Fatalf("filesystem root should never be indexed: out=%q log=%s", out, log)
	}
}

func TestHookRejectsRepositoryRootAtUserHome(t *testing.T) {
	setupHome(t)
	home := t.TempDir()
	setUserHome(t, home)
	// A nested cwd does not make it safe to index a Git worktree rooted at HOME.
	if err := os.MkdirAll(filepath.Join(home, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(home, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, nested, "MAX_ACTIVE_TICKETS", "home-root"), &spawned)
	if out != "" || !strings.Contains(log, "skipped_unsafe_root") {
		t.Fatalf("home-rooted repository should not be indexed: out=%q log=%s", out, log)
	}
	if len(spawned) != 0 {
		t.Fatalf("unsafe repository started a background worker: %v", spawned)
	}
}

// dogfoodSnapshot is the frozen corpus for TestDogfoodQueries: the production files of the
// packages that own the pinned symbols. Tests and other packages are left out on purpose, so
// adding them cannot move the answers. Changing this list is a deliberate corpus change.
var dogfoodSnapshot = []string{
	"internal/app/app.go",
	"internal/app/diagnostics.go",
	"internal/app/spawn_unix.go",
	"internal/app/spawn_windows.go",
	"internal/gitx/gitx.go",
	"internal/install/install.go",
}

// dogfoodRoot copies dogfoodSnapshot from the repository into a temporary folder and returns it.
func dogfoodRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	for _, rel := range dogfoodSnapshot {
		body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dst, filepath.FromSlash(rel)), string(body))
	}
	return dst
}

// TestDogfoodQueries runs the pinned queries from bench/dogfood_cases.json against the frozen
// snapshot of this repository (dogfoodSnapshot). If a pinned symbol is renamed or its file is
// moved, update the case file and the snapshot list in the same change.
func TestDogfoodQueries(t *testing.T) {
	setupHome(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "bench", "dogfood_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			ID       string `json:"id"`
			Prompt   string `json:"prompt"`
			Negative bool   `json:"negative"`
			Relevant []struct {
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"relevant"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 6 {
		t.Fatalf("dogfood set has %d cases, want at least 6", len(file.Cases))
	}
	root := dogfoodRoot(t)
	for _, c := range file.Cases {
		rep, err := Explain(root, c.Prompt, retrieve.Options{})
		if err != nil {
			t.Fatalf("%s: explain: %v", c.ID, err)
		}
		if c.Negative {
			if rep.Context != "" {
				t.Errorf("%s: unrelated prompt injected %d evidence items", c.ID, len(rep.Evidence))
			}
			continue
		}
		if len(c.Relevant) == 0 {
			t.Fatalf("%s: positive case without expected symbols", c.ID)
		}
		top := rep.Evidence
		if len(top) > 8 {
			top = top[:8]
		}
		for _, g := range c.Relevant {
			found := false
			var names []string
			for _, e := range top {
				names = append(names, e.Name+"@"+e.Path)
				if e.Path == g.Path && e.Name == g.Name {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: %s %s not in top 8: %v", c.ID, g.Path, g.Name, names)
			}
		}
	}
}

// writeFile creates parent folders and writes body to path.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// installEnv points every path the installer touches at folders under dir.
func installEnv(t *testing.T, dir string) install.Env {
	t.Helper()
	src := filepath.Join(dir, "build", "sidecar")
	writeFile(t, src, "#!fake-binary")
	return install.Env{
		Source:         src,
		InstallDir:     filepath.Join(dir, "bin"),
		ClaudeSettings: filepath.Join(dir, "claude", "settings.json"),
		CodexHooks:     filepath.Join(dir, "codex", "hooks.json"),
		CodexConfig:    filepath.Join(dir, "codex", "config.toml"),
		Manifest:       filepath.Join(dir, "config", "install.json"),
	}
}

// sidecarCommands returns the hook commands Sidecar wrote for an agent in a settings file.
func sidecarCommands(t *testing.T, path, agent string) []string {
	t.Helper()
	var doc struct {
		Hooks struct {
			UserPromptSubmit []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"UserPromptSubmit"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(readFile(t, path), &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var out []string
	for _, g := range doc.Hooks.UserPromptSubmit {
		for _, h := range g.Hooks {
			if install.IsOurs(h.Command, agent) {
				out = append(out, h.Command)
			}
		}
	}
	return out
}

func doctorStatus(t *testing.T, env install.Env, name string) string {
	t.Helper()
	for _, c := range Doctor(env, t.TempDir()) {
		if c.Name == name {
			return c.Status
		}
	}
	t.Fatalf("doctor has no %q check", name)
	return ""
}

// gitInit makes root a Git repository with one commit, so the Git code paths run.
func gitInit(t *testing.T, root string) {
	t.Helper()
	steps := [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.name=sidecar-test", "-c", "user.email=sidecar-test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// TestSmokeInstallDoctorHookReinstallUninstall follows the path a user takes and checks
// that other tools' hooks and settings survive every step.
func TestSmokeInstallDoctorHookReinstallUninstall(t *testing.T) {
	setupHome(t)
	env := installEnv(t, t.TempDir())
	writeFile(t, env.ClaudeSettings, `{"model":"opus","permissions":{"allow":["Bash(ls)"]},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"lint"}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"/usr/local/bin/notify-me"}]}]}}`)
	writeFile(t, env.CodexHooks, `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`)
	opts := install.Options{Env: env, Yes: true, Out: &bytes.Buffer{}, Agents: []string{"claude", "codex"}}

	if _, err := install.Install(opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if s := doctorStatus(t, env, "claude code"); s != "ok" {
		t.Fatalf("doctor after install: claude code = %s", s)
	}
	if s := doctorStatus(t, env, "codex"); s != "ok" {
		t.Fatalf("doctor after install: codex = %s", s)
	}

	root := makeProject(t)
	var spawned []string
	question := "o MAX_ACTIVE_TICKETS limita a fila?"
	out, _ := runHook(t, adapter.Claude, payload(t, root, question, "smoke-claude"), &spawned)
	if !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("claude hook injected nothing: %q", out)
	}
	out, _ = runHook(t, adapter.Codex, payload(t, root, question, "smoke-codex"), &spawned)
	var codexOut struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &codexOut); err != nil || !strings.Contains(codexOut.HookSpecificOutput.AdditionalContext, "MaxActiveTickets") {
		t.Fatalf("codex hook output is not the expected JSON: %v %q", err, out)
	}

	if _, err := install.Install(opts); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if n := len(sidecarCommands(t, env.ClaudeSettings, "claude")); n != 1 {
		t.Fatalf("after reinstall: %d claude hooks, want 1", n)
	}
	if n := len(sidecarCommands(t, env.CodexHooks, "codex")); n != 1 {
		t.Fatalf("after reinstall: %d codex hooks, want 1", n)
	}

	if _, err := install.Uninstall(install.UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if n := len(sidecarCommands(t, env.ClaudeSettings, "claude")) + len(sidecarCommands(t, env.CodexHooks, "codex")); n != 0 {
		t.Fatalf("sidecar hooks left after uninstall: %d", n)
	}
	var claude map[string]any
	if err := json.Unmarshal(readFile(t, env.ClaudeSettings), &claude); err != nil {
		t.Fatal(err)
	}
	if claude["model"] != "opus" {
		t.Errorf("claude settings lost the user's model: %v", claude)
	}
	if _, ok := claude["permissions"]; !ok {
		t.Errorf("claude settings lost permissions: %v", claude)
	}
	if hooks, _ := claude["hooks"].(map[string]any); hooks["PreToolUse"] == nil {
		t.Errorf("claude settings lost the PreToolUse hook: %v", claude)
	}
	if raw := string(readFile(t, env.ClaudeSettings)); !strings.Contains(raw, "notify-me") {
		t.Errorf("claude settings lost the user's UserPromptSubmit handler: %s", raw)
	}
	if codex := string(readFile(t, env.CodexHooks)); !strings.Contains(codex, "echo hi") {
		t.Errorf("codex hooks lost the user's SessionStart handler: %s", codex)
	}
	for _, gone := range []string{env.Manifest, env.ClaudeSettings + install.BackupSuffix, env.CodexHooks + install.BackupSuffix, filepath.Join(env.InstallDir, install.BinaryName)} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s should be removed by uninstall (stat err=%v)", gone, err)
		}
	}
	if s := doctorStatus(t, env, "claude code"); s != "warn" {
		t.Errorf("doctor after uninstall: claude code = %s, want warn", s)
	}
}

// TestRepositoryWithSpacesAndUnicodeIsIndexedAndAnswered covers a Git worktree whose path and
// file names carry spaces and accents, which the hook must index and answer like any other.
func TestRepositoryWithSpacesAndUnicodeIsIndexedAndAnswered(t *testing.T) {
	setupHome(t)
	root := filepath.Join(t.TempDir(), "repositório com espaço ção")
	writeFile(t, filepath.Join(root, "queue", "limites da fila.go"), "package queue\n\n// MaxActiveTickets limits concurrent tickets.\nconst MaxActiveTickets = 1\n")
	writeFile(t, filepath.Join(root, "docs", "operação ção.md"), "# Operação\n\nFila de caminhões.\n")
	gitInit(t, root)

	var spawned []string
	out, log := runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "unicode-repo"), &spawned)
	if !strings.Contains(out, "MaxActiveTickets") || !strings.Contains(out, "limites da fila.go") {
		t.Fatalf("unicode/space repository not answered: %q (log %s)", out, log)
	}
	if !strings.Contains(log, `"status":"ok"`) {
		t.Fatalf("hook did not report ok: %s", log)
	}
	if _, err := os.Stat(paths.IndexDB(root)); err != nil {
		t.Fatalf("index not created for the unicode repository: %v", err)
	}
}

// TestInstalledHookRunsThroughShellFromSpacedUnicodePaths executes the exact hook commands
// that install wrote, through sh as an agent does, with a binary and repository whose paths
// contain spaces and accents. It builds the real sidecar binary, so it is skipped in -short.
func TestInstalledHookRunsThroughShellFromSpacedUnicodePaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook commands are run through a POSIX shell here")
	}
	if testing.Short() {
		t.Skip("builds the sidecar binary")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not on PATH")
	}
	setupHome(t)
	base := t.TempDir()
	built := filepath.Join(base, "compilação ção", "sidecar")
	if err := os.MkdirAll(filepath.Dir(built), 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(goTool, "build", "-o", built, "./cmd/sidecar")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	env := install.Env{
		Source:         built,
		InstallDir:     filepath.Join(base, "bin com espaço", "ção"),
		ClaudeSettings: filepath.Join(base, "claude config", "settings.json"),
		CodexHooks:     filepath.Join(base, "codex", "hooks.json"),
		CodexConfig:    filepath.Join(base, "codex", "config.toml"),
		Manifest:       filepath.Join(base, "cfg ção", "install.json"),
	}
	if _, err := install.Install(install.Options{Env: env, Yes: true, Out: &bytes.Buffer{}, Agents: []string{"claude", "codex"}}); err != nil {
		t.Fatalf("install: %v", err)
	}
	repo := filepath.Join(base, "repo com espaço ção")
	writeFile(t, filepath.Join(repo, "queue", "limits.go"), "package queue\n\n// MaxActiveTickets limits concurrent tickets.\nconst MaxActiveTickets = 1\n")
	gitInit(t, repo)

	for _, agent := range []string{"claude", "codex"} {
		file := env.ClaudeSettings
		if agent == "codex" {
			file = env.CodexHooks
		}
		cmds := sidecarCommands(t, file, agent)
		if len(cmds) != 1 {
			t.Fatalf("%s: %d hook commands installed, want 1", agent, len(cmds))
		}
		var stdout, stderr bytes.Buffer
		c := exec.Command(shell, "-c", cmds[0])
		c.Dir = repo
		c.Stdin = strings.NewReader(payload(t, repo, "o MAX_ACTIVE_TICKETS limita a fila?", "shell-"+agent))
		c.Stdout = &stdout
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			t.Fatalf("%s hook command %q failed: %v\nstderr: %s", agent, cmds[0], err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "MaxActiveTickets") {
			t.Fatalf("%s hook answered without evidence: %q (stderr %s)", agent, stdout.String(), stderr.String())
		}
	}

	if _, err := install.Uninstall(install.UninstallOptions{Env: env, Out: &bytes.Buffer{}}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.InstallDir, install.BinaryName)); !os.IsNotExist(err) {
		t.Fatalf("installed binary left behind (stat err=%v)", err)
	}
}

// TestConcurrentHooksAndBackgroundIndexerShareOneIndex runs several prompts and background
// indexers against one worktree at the same time. No hook may fail, and the index must
// still pass an integrity check afterwards.
func TestConcurrentHooksAndBackgroundIndexerShareOneIndex(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	logPath := filepath.Join(t.TempDir(), "hook.jsonl")
	const runs, indexers = 6, 3
	inputs := make([]string, runs)
	for i := range inputs {
		inputs[i] = payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", fmt.Sprintf("conc-%d", i))
	}
	outs := make([]string, runs)
	indexerErrs := make(chan error, indexers)
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out bytes.Buffer
			Hook(HookOptions{
				Agent: adapter.Claude, Stdin: strings.NewReader(inputs[i]), Stdout: &out,
				LogPath: logPath, Spawn: func(string, bool) error { return nil },
			})
			outs[i] = out.String()
		}()
	}
	for i := 0; i < indexers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			indexerErrs <- RunIndexer(root, true)
		}()
	}
	wg.Wait()
	close(indexerErrs)
	for err := range indexerErrs {
		if err != nil {
			t.Errorf("background indexer: %v", err)
		}
	}
	for i, out := range outs {
		if !strings.Contains(out, "MaxActiveTickets") {
			t.Errorf("hook %d answered without evidence: %q", i, out)
		}
	}
	logData := string(readFile(t, logPath))
	if strings.Contains(logData, `"status":"error"`) || strings.Contains(logData, `"status":"panic"`) {
		t.Fatalf("hook failures under concurrency: %s", logData)
	}
	st, err := store.Open(paths.IndexDB(root))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if check, err := st.IntegrityCheck(); err != nil || check != "ok" {
		t.Fatalf("index integrity after concurrency: %q, %v", check, err)
	}
}

// TestDamagedIndexIsRebuiltAndHookKeepsAnswering covers damage that SQLite reports: a truncated
// file (caught when opening) and a zeroed second half (caught by a query). Both must be moved
// aside and rebuilt without blocking the answer. Silent damage that no query touches is not
// detected here; `sidecar doctor` reports it through quick_check.
func TestDamagedIndexIsRebuiltAndHookKeepsAnswering(t *testing.T) {
	damages := map[string]func([]byte) []byte{
		"truncated": func(b []byte) []byte { return b[:len(b)/2] },
		"zeroed second half": func(b []byte) []byte {
			c := append([]byte{}, b...)
			for i := len(c) / 2; i < len(c); i++ {
				c[i] = 0
			}
			return c
		},
	}
	for name, damage := range damages {
		t.Run(name, func(t *testing.T) {
			setupHome(t)
			root := makeProject(t)
			var spawned []string
			runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "damage-1"), &spawned)
			db := paths.IndexDB(root)
			raw := readFile(t, db)
			if len(raw) < 8192 {
				t.Fatalf("index too small to damage a page: %d bytes", len(raw))
			}
			writeFile(t, db, string(damage(raw)))

			out, log := runHook(t, adapter.Claude, payload(t, root, "o MAX_ACTIVE_TICKETS limita a fila?", "damage-2"), &spawned)
			if !strings.Contains(out, "MaxActiveTickets") {
				t.Fatalf("damaged index blocked the answer: %q (log %s)", out, log)
			}
			if !strings.Contains(log, `"index_recovered":true`) {
				t.Fatalf("recovery not recorded: %s", log)
			}
			if _, err := os.Stat(db + ".corrupt"); err != nil {
				t.Fatalf("damaged index was not kept aside: %v", err)
			}
		})
	}
}

// TestExplainCarriesIntentAndFingerprint checks the observability fields of `explain --json`.
func TestExplainCarriesIntentAndFingerprint(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	rep, err := Explain(root, "Onde fica o ManualReleaseHandler?", retrieve.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Fingerprint == nil || rep.Fingerprint.Hash == "" || rep.Fingerprint.Provenance == "" {
		t.Fatalf("fingerprint missing: %+v", rep.Fingerprint)
	}
	if len(rep.Intent.Kinds) == 0 || rep.Intent.Kinds[0] != "locate" {
		t.Fatalf("intent = %+v, want locate first", rep.Intent)
	}
	if rep.Routed {
		t.Fatal("explain runs the v1 pipeline; intent must not route it")
	}
	if _, ok := rep.TimingsMS["fingerprint"]; !ok {
		t.Fatalf("fingerprint timing missing: %v", rep.TimingsMS)
	}
	for _, e := range rep.Evidence {
		if e.ID == "" || e.Source == "" {
			t.Fatalf("evidence without ID or source: %+v", e)
		}
	}
}
