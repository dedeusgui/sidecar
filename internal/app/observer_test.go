package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/paths"
)

const testRoot = "/work/repo"

func read(path string) adapter.ToolCall {
	return adapter.ToolCall{Agent: adapter.Claude, Phase: "pre", Kind: adapter.KindRead, Path: path}
}

func edit(path string) adapter.ToolCall {
	return adapter.ToolCall{Agent: adapter.Claude, Phase: "pre", Kind: adapter.KindEdit, Path: path}
}

func test() adapter.ToolCall {
	return adapter.ToolCall{Agent: adapter.Claude, Phase: "pre", Kind: adapter.KindTest, Query: "go test ./..."}
}

func search(query string) adapter.ToolCall {
	return adapter.ToolCall{Agent: adapter.Claude, Phase: "pre", Kind: adapter.KindSearch, Query: query}
}

func failure(text string) adapter.ToolCall {
	return adapter.ToolCall{Agent: adapter.Claude, Phase: "post", Result: "error", Error: text}
}

// run applies calls in order and returns every signal raised, in order.
func run(s *brief.SessionSignals, calls ...adapter.ToolCall) []string {
	var all []string
	for _, c := range calls {
		all = append(all, applyTool(s, c, testRoot)...)
	}
	return all
}

func fresh() brief.SessionSignals {
	return brief.NewSignals("head-1", time.Now())
}

func TestLegitimateEditCyclesAreNotLoops(t *testing.T) {
	s := fresh()
	// Each epoch (the span between edits) holds at most two reads of a.go. Reading a file
	// again after every edit, and re-running tests after every fix, is the normal cycle.
	got := run(&s,
		read("a.go"), read("a.go"), edit("a.go"), read("a.go"), test(),
		edit("a.go"), read("a.go"), test(), edit("a.go"), read("a.go"), test(),
		search("Release"), edit("a.go"), search("Release"),
	)
	if len(got) != 0 {
		t.Fatalf("edit->read, edit->test and re-search after an edit were flagged: %v", got)
	}
}

func TestRedundantReadFlagsThirdReopenWithoutEdit(t *testing.T) {
	s := fresh()
	got := run(&s, read("a.go"), read("a.go"), read("a.go"), read("a.go"))
	if len(got) != 1 || got[0] != SignalRedundantRead {
		t.Fatalf("signals = %v, want exactly one %s", got, SignalRedundantRead)
	}
}

func TestRepeatedSearchNeedsNoEditInBetween(t *testing.T) {
	s := fresh()
	if got := run(&s, search("Release"), search("Release")); len(got) != 1 || got[0] != SignalRepeatedSearch {
		t.Fatalf("repeated search signals = %v", got)
	}
	if got := run(&s, edit("a.go"), search("Release")); len(got) != 0 {
		t.Fatalf("search after an edit flagged: %v", got)
	}
}

func TestRepeatedErrorMasksNumbersAndResetsOnEdit(t *testing.T) {
	s := fresh()
	if got := run(&s, failure("line 12: undefined: Foo"), failure("line 40: undefined: Foo")); len(got) != 1 || got[0] != SignalRepeatedError {
		t.Fatalf("same error with other line numbers = %v", got)
	}
	s = fresh()
	if got := run(&s, failure("boom"), edit("a.go"), failure("boom")); len(got) != 0 {
		t.Fatalf("error repeated after a fix was flagged: %v", got)
	}
}

func TestClaudeAndCodexPostEventsDoNotRaiseErrors(t *testing.T) {
	s := fresh()
	post := adapter.ToolCall{Agent: adapter.Codex, Phase: "post", Result: "unknown"}
	if got := run(&s, post, post, post); len(got) != 0 {
		t.Fatalf("unknown results raised %v", got)
	}
}

func TestStageFollowsEditAndTest(t *testing.T) {
	s := fresh()
	run(&s, read("a.go"))
	if s.Stage != brief.StageLocate {
		t.Fatalf("stage after read = %q", s.Stage)
	}
	run(&s, edit("a.go"))
	run(&s, test())
	if s.Stage != brief.StageTest {
		t.Fatalf("stage after edit+test = %q", s.Stage)
	}
	run(&s, test())
	if s.Stage != brief.StageVerify {
		t.Fatalf("stage after second test = %q", s.Stage)
	}
}

func TestSignalsInvalidateOnNewVersionAndTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.signals.json")
	now := time.Now()
	s := brief.NewSignals("head-1", now)
	run(&s, read("a.go"), read("a.go"))
	if err := brief.SaveSignals(path, s); err != nil {
		t.Fatal(err)
	}

	if kept := brief.LoadSignals(path, "head-1", now); kept.Reads == nil || len(kept.Reads) != 1 {
		t.Fatalf("same version lost its reads: %+v", kept.Reads)
	}
	if moved := brief.LoadSignals(path, "head-2", now); len(moved.Reads) != 0 {
		t.Fatal("a new commit must start fresh signals")
	}
	if expired := brief.LoadSignals(path, "head-1", now.Add(brief.SessionTTL+time.Minute)); len(expired.Reads) != 0 {
		t.Fatal("expired signals must start fresh")
	}
	if err := os.WriteFile(path, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if bad := brief.LoadSignals(path, "head-1", now); bad.Version != "head-1" || len(bad.Reads) != 0 {
		t.Fatal("corrupt state must not be an error")
	}
}

func TestInjectionPolicyIsPassiveForToolsAndFirstAntigravityCall(t *testing.T) {
	if !InjectionAllowed(adapter.Claude, 0) || !InjectionAllowed(adapter.Codex, 0) {
		t.Fatal("prompt events of Claude and Codex must stay allowed")
	}
	if !InjectionAllowed(adapter.Antigravity, 0) {
		t.Fatal("the first Antigravity model call of a turn must be allowed")
	}
	if InjectionAllowed(adapter.Antigravity, 1) {
		t.Fatal("later Antigravity model calls repeat the prompt and must not inject")
	}
}

func TestObserveRecordsHashesOnlyAndStaysSilent(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	payload := `{"session_id":"s9","cwd":"` + root + `","tool_name":"Read","tool_input":{"file_path":"` +
		filepath.Join(root, "queue", "limits.go") + `"},"tool_use_id":"secret-call-id"}`
	if code := Observe(adapter.Claude, "PreToolUse", strings.NewReader(payload)); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	s := brief.LoadSignals(paths.ObserverState(root, "claude", "s9"), worktreeVersion(root), time.Now())
	if len(s.Files) != 1 || len(s.Reads) != 1 {
		t.Fatalf("state after one read = files %d reads %d", len(s.Files), len(s.Reads))
	}
	log, err := os.ReadFile(paths.HookLog())
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"limits.go", "queue", "secret-call-id", root} {
		if bytes.Contains(log, []byte(leaked)) {
			t.Fatalf("observer log contains %q: %s", leaked, log)
		}
	}
	var rec struct {
		Tool EventV1 `json:"tool"`
	}
	line := bytes.TrimSpace(bytes.Split(log, []byte("\n"))[0])
	if err := json.Unmarshal(line, &rec); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, line)
	}
	if rec.Tool.SchemaVersion != 1 || rec.Tool.ToolKind != "read" || rec.Tool.Coverage != "observed" ||
		rec.Tool.EventType != "tool_invoked" || rec.Tool.PathIDHash == "" {
		t.Fatalf("schema v1 event = %+v", rec.Tool)
	}
}

func TestRecordEvidenceCountsWhatWasSentAsHashes(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	recordEvidence(root, adapter.Claude, "s5", []string{
		"symbol|queue/limits.go|MaxActiveTickets",
		"file|queue/limits.go|",
		"dirty|queue/limits.go",
	})
	s := brief.LoadSignals(paths.ObserverState(root, "claude", "s5"), worktreeVersion(root), time.Now())
	if s.EvidenceSent != 3 || len(s.Files) != 1 || len(s.Symbols) != 1 {
		t.Fatalf("evidence = sent %d files %d symbols %d", s.EvidenceSent, len(s.Files), len(s.Symbols))
	}
	for key := range s.Symbols {
		if strings.Contains(key, "MaxActiveTickets") {
			t.Fatal("symbol name stored in clear")
		}
	}
}

func TestAntigravityLaterInvocationGetsNoContext(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	input := `{"conversationId":"c1","workspacePaths":["` + root + `"],"lastUserInput":"o MAX_ACTIVE_TICKETS limita a fila?","invocationNum":1}`
	var spawned []string
	out, _ := runHook(t, adapter.Antigravity, input, &spawned)
	if out != "{}" {
		t.Fatalf("later invocation output = %q, want {}", out)
	}
	input = strings.Replace(input, `"invocationNum":1`, `"invocationNum":0`, 1)
	out, _ = runHook(t, adapter.Antigravity, input, &spawned)
	if !strings.Contains(out, "injectSteps") || !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("first invocation got no evidence: %q", out)
	}
}

// TestObserverConcurrentWrites runs tool events in parallel, as Claude Code does for parallel
// tool calls: sixteen on one session and sixteen each on their own session. Every state must
// load whole; a torn or shared temporary file would make LoadSignals return fresh state.
func TestObserverConcurrentWrites(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	target := filepath.Join(root, "queue", "limits.go")
	event := func(session string) string {
		return `{"session_id":"` + session + `","cwd":"` + root + `","tool_name":"Read","tool_input":{"file_path":"` + target + `"}}`
	}
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			Observe(adapter.Claude, "PreToolUse", strings.NewReader(event("shared")))
		}()
		go func(i int) {
			defer wg.Done()
			Observe(adapter.Claude, "PreToolUse", strings.NewReader(event(fmt.Sprintf("own-%d", i))))
		}(i)
	}
	wg.Wait()

	// Concurrent updates of one session may overwrite each other, so only the shape is asserted.
	shared := brief.LoadSignals(paths.ObserverState(root, "claude", "shared"), worktreeVersion(root), time.Now())
	if len(shared.Files) != 1 || len(shared.Reads) != 1 {
		t.Fatalf("shared session state is torn or lost: files=%d reads=%d", len(shared.Files), len(shared.Reads))
	}
	for i := 0; i < n; i++ {
		s := brief.LoadSignals(paths.ObserverState(root, "claude", fmt.Sprintf("own-%d", i)), worktreeVersion(root), time.Now())
		if len(s.Files) != 1 || len(s.Reads) != 1 {
			t.Fatalf("own-%d state = files %d reads %d, want exactly one read", i, len(s.Files), len(s.Reads))
		}
	}
}

// TestObserverPreCompactResetsSessionSeen checks that after a compaction the evidence the
// session already received is sent again, and that the compaction is counted.
func TestObserverPreCompactResetsSessionSeen(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	var spawned []string
	const prompt = "o MAX_ACTIVE_TICKETS limita a fila?"

	if out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "c1"), &spawned); !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("no evidence before compaction: %q", out)
	}
	if out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "c1"), &spawned); strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("evidence repeated within the session; the premise of this test does not hold: %q", out)
	}

	compact := `{"session_id":"c1","cwd":"` + root + `","hook_event_name":"PreCompact"}`
	if code := Observe(adapter.Claude, "PreCompact", strings.NewReader(compact)); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "c1"), &spawned); !strings.Contains(out, "MaxActiveTickets") {
		t.Fatalf("evidence not sent again after compaction: %q", out)
	}
	s := brief.LoadSignals(paths.ObserverState(root, "claude", "c1"), worktreeVersion(root), time.Now())
	if s.Compactions != 1 {
		t.Fatalf("compactions = %d, want 1", s.Compactions)
	}
}
