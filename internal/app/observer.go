package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// Loop signals. They are diagnostics, not verdicts: each one only says that the same
// thing repeated with no edit in between.
const (
	SignalRepeatedSearch = "repeated_search"
	SignalRedundantRead  = "redundant_read"
	SignalRepeatedError  = "repeated_error"
)

// loopThreshold is how many reads of one file, with no edit in between, raise redundant_read.
const loopThreshold = 3

// errorThreshold is how many identical errors, with no edit in between, raise repeated_error.
const errorThreshold = 2

// EventV1 is one observed tool event in the agent-feedback schema v1. Identifiers are
// hashes, so the record can be logged without prompts, paths or tool output.
type EventV1 struct {
	SchemaVersion      int     `json:"schema_version"`
	Client             string  `json:"client"`
	EventType          string  `json:"event_type"`
	ToolKind           string  `json:"tool_kind"`
	SessionIDHash      string  `json:"session_id_hash"`
	RepoIDHash         string  `json:"repo_id_hash"`
	PathIDHash         string  `json:"path_id_hash,omitempty"`
	ToolCallIDHash     string  `json:"tool_call_id_hash,omitempty"`
	MonotonicElapsedMS float64 `json:"monotonic_elapsed_ms"`
	Result             string  `json:"result"`
	Coverage           string  `json:"coverage"`
}

// Observe records one PreToolUse or PostToolUse event. It is passive: it writes nothing to
// stdout, never changes the tool call, and returns 0 whatever happens.
func Observe(agent adapter.Agent, event string, in io.Reader) (exit int) {
	start := time.Now()
	rec := logRecord{"at": start.UTC().Format(time.RFC3339), "agent": string(agent), "event": event}
	defer func() {
		if r := recover(); r != nil {
			rec["status"] = "panic"
			rec["error"] = fmt.Sprint(r)
		}
		rec["total_ms"] = ms(time.Since(start))
		writeLog(paths.HookLog(), rec)
	}()

	call, err := adapter.ParseTool(agent, event, in)
	if err != nil {
		rec["status"] = "bad_input"
		rec["error"] = err.Error()
		return 0
	}
	cwd := call.CWD
	if cwd == "" {
		cwd = "."
	}
	root, _ := repo.FindRoot(cwd)
	if unsafeRoot(cwd) || unsafeRoot(root) {
		rec["status"] = "skipped_unsafe_root"
		return 0
	}
	if call.Session == "" {
		rec["status"] = "no_session"
		return 0
	}
	statePath := paths.ObserverState(root, string(agent), call.Session)
	s := brief.LoadSignals(statePath, worktreeVersion(root), start)
	raised := applyTool(&s, call, root)
	if err := brief.SaveSignals(statePath, s); err != nil {
		rec["save_error"] = err.Error()
	}
	rec["tool"] = schemaV1(call, root, time.Since(start))
	rec["signals"] = raised
	rec["status"] = "observed"
	return 0
}

// applyTool updates the session with one tool call and returns the loop signals it raised.
// Use is counted on the pre-call event; the post-call event only contributes errors, so a
// client that sends both does not count a call twice.
func applyTool(s *brief.SessionSignals, c adapter.ToolCall, root string) []string {
	var raised []string
	raise := func(signal string) {
		s.Signals[signal]++
		raised = append(raised, signal)
	}
	path := ""
	if c.Path != "" {
		path = hashID(root, relPath(root, c.Path))
		addBounded(s.Files, path)
	}
	if c.Phase == "post" {
		if c.Result == "error" && c.Error != "" {
			key := hashID(root, normalizeError(c.Error))
			addCount(s.Errors, key)
			if s.Errors[key] == errorThreshold {
				raise(SignalRepeatedError)
			}
		}
		return raised
	}
	switch c.Kind {
	case adapter.KindEdit:
		if path != "" {
			addCount(s.Edits, path)
		}
		// Everything read or tried before a change is no longer evidence of a loop after it:
		// edit->read, edit->test and re-running a test after a fix are legitimate.
		s.ClearEpoch()
		s.Stage = brief.StageEdit
	case adapter.KindTest:
		s.Stage = nextStage(s.Stage)
	case adapter.KindSearch:
		key := hashID(root, c.Query)
		if s.Searches[key] {
			raise(SignalRepeatedSearch)
		}
		addBounded(s.Searches, key)
	case adapter.KindRead:
		if path != "" {
			addCount(s.Reads, path)
			if s.Reads[path] == loopThreshold {
				raise(SignalRedundantRead)
			}
		}
	}
	return raised
}

// nextStage advances the task stage after a test run: a test right after an edit starts
// verification; another test with no edit in between is verification again.
func nextStage(stage string) string {
	switch stage {
	case brief.StageEdit:
		return brief.StageTest
	case brief.StageTest, brief.StageVerify:
		return brief.StageVerify
	}
	return brief.StageTest
}

// InjectionAllowed is the injection policy for prompt-time events. Tool events never get
// context: observation is passive. Antigravity runs PreInvocation once per model call, and
// only the first call of a turn may receive context, since the later ones repeat the prompt.
func InjectionAllowed(agent adapter.Agent, invocation int) bool {
	return agent != adapter.Antigravity || invocation == 0
}

// recordEvidence notes in the session signals what the prompt was given: a count, the files
// and the symbols, all as hashes. It is bookkeeping, so failures are ignored.
func recordEvidence(root string, agent adapter.Agent, session string, sent []string) {
	statePath := paths.ObserverState(root, string(agent), session)
	s := brief.LoadSignals(statePath, worktreeVersion(root), time.Now())
	s.EvidenceSent += len(sent)
	for _, key := range sent {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) < 2 {
			continue
		}
		addBounded(s.Files, hashID(root, parts[1]))
		if len(parts) == 3 && parts[0] != "file" {
			addBounded(s.Symbols, hashID(root, key))
		}
	}
	_ = brief.SaveSignals(statePath, s)
}

func schemaV1(c adapter.ToolCall, root string, elapsed time.Duration) EventV1 {
	ev := EventV1{
		SchemaVersion: 1, Client: string(c.Agent), EventType: "tool_invoked", ToolKind: string(c.Kind),
		SessionIDHash: hashID(string(c.Agent), c.Session), RepoIDHash: hashID(root),
		Result: c.Result, Coverage: "observed",
		// Pre and post run in different processes, so no tool duration exists here. This
		// measures the observer's own work within this process.
		MonotonicElapsedMS: ms(elapsed),
	}
	if c.Phase == "post" {
		ev.EventType = "tool_completed"
	}
	if c.Path != "" {
		ev.PathIDHash = hashID(root, relPath(root, c.Path))
	}
	if c.CallID != "" {
		ev.ToolCallIDHash = hashID(c.CallID)
	}
	return ev
}

// worktreeVersion identifies the code a session state describes: the HEAD commit. A new
// commit, or a checkout of another one, starts fresh signals.
func worktreeVersion(root string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	head, err := gitx.Head(ctx, root)
	if err != nil {
		return "no-head"
	}
	return head
}

// hashID is an opaque local identifier: a truncated SHA-256 over its parts. It keeps raw
// paths, queries and errors out of the state and the log. It is not a secret.
func hashID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// relPath makes an absolute path relative to root. Other paths are returned unchanged.
func relPath(root, p string) string {
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return filepath.ToSlash(rel)
}

var digitRun = regexp.MustCompile(`\d+`)

// normalizeError keeps the first line of an error with numbers masked, so the same failure
// with a different line number or timestamp still counts as the same error.
func normalizeError(text string) string {
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	return digitRun.ReplaceAllString(strings.ToLower(first), "#")
}

// addBounded records key in a set unless the set is full.
func addBounded(set map[string]bool, key string) {
	if set[key] || len(set) < brief.MaxSignalEntries {
		set[key] = true
	}
}

// addCount increments a counter unless the map is full and key is new.
func addCount(m map[string]int, key string) {
	if _, ok := m[key]; ok || len(m) < brief.MaxSignalEntries {
		m[key]++
	}
}
