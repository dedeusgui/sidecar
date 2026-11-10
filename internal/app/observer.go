package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/progress"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// Loop signals. They are diagnostics, not verdicts: each one only says that the same
// thing repeated with no edit in between.
const (
	SignalRepeatedSearch = "repeated_search"
	SignalRedundantRead  = "redundant_read"
	SignalRepeatedError  = "repeated_error"
	SignalNearRepeat     = "near_repeat_search" // a search with almost the same terms as an earlier one, same epoch
	SignalStagnation     = "stagnation"         // a window of redundant events with no edit or test (shadow only)
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

// Observe records one PreToolUse, PostToolUse or PreCompact event. It is passive: it writes
// nothing to stdout, never changes the tool call, and returns 0 whatever happens.
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
	version := worktreeVersion(root)
	scope := observerScope(root, call.Session, version)
	s := brief.LoadSignals(statePath, version, start)
	if s.ScopeFingerprint != "" && s.ScopeFingerprint != brief.ScopeFingerprint(scope) {
		s = brief.NewSignals(version, start)
	}
	s.ScopeFingerprint = brief.ScopeFingerprint(scope)
	if call.Phase == "compact" {
		// The agent lost what it saw before the compaction, so evidence suppressed as already
		// sent must be sent again; reads before it are not loops after it.
		_ = os.Remove(paths.SessionState(string(agent), call.Session))
		s.Compactions++
		s.ClearEpoch()
		s.Trace = nil // what the agent did before compaction says nothing about what it does after it
		rec["status"] = "compacted"
	} else {
		rec["signals"] = applyTool(&s, call, root)
		rec["tool"] = schemaV1(call, root, time.Since(start))
		rec["status"] = "observed"
	}
	if err := brief.SaveSignals(statePath, s); err != nil {
		rec["save_error"] = err.Error()
	}
	return 0
}

// applyTool updates the session with one tool call and returns the loop signals it raised.
// Use is counted on the pre-call event; the post-call event only contributes errors, so a
// client that sends both does not count a call twice.
func applyTool(s *brief.SessionSignals, c adapter.ToolCall, root string) []string {
	raised := applyToolBasic(s, c, root)
	return append(raised, classifyProgress(s, c, root)...)
}

// classifyProgress appends the event to the bounded trace and raises the passive progress signals. Keys are
// hashes (a search is the sorted set of its hashed terms), so near-duplicate detection needs no clear text.
// A decision to intervene is only counted (Shadows): there is no verified transport to deliver one.
func classifyProgress(s *brief.SessionSignals, c adapter.ToolCall, root string) []string {
	var ev progress.Event
	switch {
	case c.Phase == "post":
		if c.Result != "error" || c.Error == "" {
			return nil
		}
		ev = progress.Event{Kind: "other", Key: "err", Err: hashID(root, normalizeError(c.Error))}
	case c.Kind == adapter.KindEdit:
		ev = progress.Event{Kind: "edit", Key: hashID(root, relPath(root, c.Path))}
	case c.Kind == adapter.KindTest:
		ev = progress.Event{Kind: "test", Key: "test"}
	case c.Kind == adapter.KindRead && c.Path != "":
		ev = progress.Event{Kind: "read", Key: hashID(root, relPath(root, c.Path))}
	case c.Kind == adapter.KindSearch:
		ev = progress.Event{Kind: "search", Key: hashedTerms(root, c.Query)}
	default:
		return nil
	}
	s.Trace = append(s.Trace, ev)
	if len(s.Trace) > brief.MaxTrace {
		s.Trace = s.Trace[len(s.Trace)-brief.MaxTrace:]
	}
	rep := progress.Analyze(s.Trace)
	var raised []string
	raise := func(sig string) {
		s.Signals[sig]++
		raised = append(raised, sig)
	}
	if n := len(rep.Per); n > 0 && rep.Per[n-1] == progress.NearRepeat {
		raise(SignalNearRepeat)
	}
	if k := len(rep.Shadows); k > 0 && rep.Shadows[k-1].At == len(s.Trace)-1 && s.Shadows < progress.ShadowMax {
		s.Shadows++
		raise(SignalStagnation)
	}
	return raised
}

// hashedTerms reduces a search to the sorted hashes of its distinct terms. Two searches with the same words in
// another order, or one extra word, stay comparable without storing either text.
func hashedTerms(root, query string) string {
	f := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	set := map[string]bool{}
	for _, t := range f {
		if len(t) >= 2 {
			set[hashID(root, "term", t)[:10]] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

func applyToolBasic(s *brief.SessionSignals, c adapter.ToolCall, root string) []string {
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

// ObservationStateFromCoverage treats missing or unknown coverage as
// unsupported/unobserved. It only reports Seen for an event explicitly
// recorded by the observer.
func ObservationStateFromCoverage(coverage string) contract.ObservationState {
	switch strings.ToLower(strings.TrimSpace(coverage)) {
	case "observed", "seen":
		return contract.ObservationSeen
	case "not_observed", "not_seen":
		return contract.ObservationNotSeen
	default:
		return contract.ObservationUnsupported
	}
}

// recordEvidence notes in the session signals what the prompt was given: a count, the files
// and the symbols, all as hashes. It is bookkeeping, so failures are ignored.
func recordEvidence(root string, agent adapter.Agent, session string, sent []string) {
	statePath := paths.ObserverState(root, string(agent), session)
	now := time.Now()
	version := worktreeVersion(root)
	scope := observerScope(root, session, version)
	s := brief.LoadSignals(statePath, version, now)
	if s.ScopeFingerprint != "" && s.ScopeFingerprint != brief.ScopeFingerprint(scope) {
		s = brief.NewSignals(version, now)
	}
	s.ScopeFingerprint = brief.ScopeFingerprint(scope)
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
		addBounded(s.Presented, presentedPathKey(root, parts[1]))
		if len(parts) == 3 {
			addBounded(s.Presented, presentedSymbolKey(root, parts[1], parts[2]))
		}
	}
	_ = brief.SaveSignals(statePath, s)
}

func observerScope(root, session, revision string) contract.Scope {
	return contract.NormalizeScope(contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: session, WorktreeRevision: revision})
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

// presentedPathKey and presentedSymbolKey are the observer-state keys of evidence the hook delivered.
// They are hashes, like every other identifier in the state, so no path or name is stored in clear.
func presentedPathKey(root, path string) string {
	return hashID(root, "presented", "path", path)
}

func presentedSymbolKey(root, path, name string) string {
	return hashID(root, "presented", "symbol", path, name)
}
