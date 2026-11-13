package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// tailBytes bounds how much of the hook log one activity or statusline read touches, so
// refreshing the status line stays cheap however large the log grows.
const tailBytes = 256 << 10

// ActivityEntry is one hook run in user-facing form. It never carries prompt text, error
// text or stack traces; those stay in the local log only.
type ActivityEntry struct {
	At       string  `json:"at"`
	Agent    string  `json:"agent,omitempty"`
	Repo     string  `json:"repo,omitempty"`
	State    string  `json:"state"`
	Evidence int     `json:"evidence"`
	TotalMS  float64 `json:"total_ms"`
	session  string
}

// ActivityReport summarises the newest hook runs. Percentiles cover the entries listed.
type ActivityReport struct {
	Runs    int             `json:"runs"`
	P50MS   float64         `json:"p50_ms"`
	P95MS   float64         `json:"p95_ms"`
	Entries []ActivityEntry `json:"entries"`
}

// SessionKey is the value stored in the hook log for a session. The raw session ID is never
// logged, so the statusline matches sessions by this digest.
func SessionKey(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:16]
}

// ReadActivity returns the newest limit hook runs, newest first. A missing log is not an error.
func ReadActivity(logPath string, limit int) ActivityReport {
	recs := tailRecords(logPath)
	entries := []ActivityEntry{}
	for i := len(recs) - 1; i >= 0 && len(entries) < limit; i-- {
		entries = append(entries, entryOf(recs[i]))
	}
	rep := ActivityReport{Runs: len(entries), Entries: entries}
	rep.P50MS, rep.P95MS = percentiles(entries)
	return rep
}

// StatusLine is the one-line summary for the Claude Code status line. It reports the newest
// hook run of the given session (any session when session is ""). It never fails.
func StatusLine(logPath, session string) string {
	recs := tailRecords(logPath)
	for i := len(recs) - 1; i >= 0; i-- {
		if session != "" && str(recs[i]["session"]) != session {
			continue
		}
		e := entryOf(recs[i])
		switch e.State {
		case "injected", "partial":
			return fmt.Sprintf("Sidecar: %s · %d refs · %.0fms", e.State, e.Evidence, e.TotalMS)
		case "failed":
			return "Sidecar: failed"
		}
		return "Sidecar: no match"
	}
	return "Sidecar: no match"
}

// stateOf maps a log status to what the user needs to know. "injected" means context was
// written to the prompt; "no_evidence" means nothing relevant was found or it was already sent.
func stateOf(rec logRecord) string {
	switch str(rec["status"]) {
	case "ok":
		if num(rec["injected_chars"]) > 0 {
			return "injected"
		}
		return "no_evidence"
	case "partial":
		return "partial"
	case "no_evidence":
		return "no_evidence"
	case "skipped", "skipped_unsafe_root", "skipped_invocation":
		return "skipped"
	case "error", "panic", "bad_input", "write_error":
		return "failed"
	}
	return "unknown"
}

func entryOf(rec logRecord) ActivityEntry {
	return ActivityEntry{
		At: str(rec["at"]), Agent: str(rec["agent"]), Repo: str(rec["repo"]),
		State: stateOf(rec), Evidence: int(num(rec["evidence"])), TotalMS: num(rec["total_ms"]),
		session: str(rec["session"]),
	}
}

// percentiles uses the nearest-rank method on the entries' totals.
func percentiles(entries []ActivityEntry) (p50, p95 float64) {
	if len(entries) == 0 {
		return 0, 0
	}
	totals := make([]float64, len(entries))
	for i, e := range entries {
		totals[i] = e.TotalMS
	}
	slices.Sort(totals)
	return totals[len(totals)/2], totals[min(len(totals)-1, len(totals)*95/100)]
}

// tailRecords parses the JSON lines in the last tailBytes of the log, oldest first. A
// partial first line from the seek is dropped; unparseable lines are skipped.
func tailRecords(path string) []logRecord {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	var start int64
	if info.Size() > tailBytes {
		start = info.Size() - tailBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, tailBytes))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	var recs []logRecord
	for _, l := range lines {
		var rec logRecord
		if json.Unmarshal([]byte(l), &rec) == nil && rec != nil {
			recs = append(recs, rec)
		}
	}
	return recs
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}
