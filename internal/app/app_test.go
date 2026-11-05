package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/paths"
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
