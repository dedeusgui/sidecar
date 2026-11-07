package app

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/store"
)

// The values an agent report may use. Anything else is rejected, so a report cannot write free
// text into the store.
var (
	feedbackHelpfulness = []string{"helpful", "neutral", "harmful", "unknown"}
	feedbackReasons     = []string{"exact_match", "wrong_file", "stale_context", "missing_caller", "noisy_neighbor", "other"}
	feedbackConfidence  = []string{"high", "medium", "low"}
)

// RecordFeedback stores an agent's post-task report: one record per evidence item it named,
// helpful (+1) or misleading (-1). Each record keeps the file hash the index holds now, so a later
// change to that file makes it stale. The session's observer trace, when there is one, corroborates
// or contradicts the report; without a trace it is neutral (0).
func RecordFeedback(root string, report store.AgentFeedbackReport) ([]store.FeedbackRecord, error) {
	if err := normalizeReport(&report); err != nil {
		return nil, err
	}
	st, ok, err := openIndexStore(root)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("no index for this worktree; nothing to attach the feedback to")
	}
	defer func() { _ = st.Close() }()
	files, err := st.FileRecords()
	if err != nil {
		return nil, err
	}
	oracle := report.TaskCompleted
	if report.OraclePassed != nil {
		oracle = *report.OraclePassed
	}
	oracleOutcome := -1
	if oracle {
		oracleOutcome = 1
	}
	now := time.Now()
	traces := map[string]int{}
	var out []store.FeedbackRecord
	for _, group := range []struct {
		ids      []string
		polarity int
	}{{report.HelpfulEvidenceIDs, 1}, {report.MisleadingEvidenceIDs, -1}} {
		seen := map[string]bool{}
		for _, id := range group.ids {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			path, symbol := store.ParseEvidenceID(id)
			trace, ok := traces[path]
			if !ok {
				trace = sessionTrace(root, report.SessionID, path)
				traces[path] = trace
			}
			file, indexed := files[path]
			rec := store.FeedbackRecord{
				Worktree: root, SessionID: report.SessionID, EvidenceID: id, FilePath: path, Symbol: symbol,
				FileHash: file.Hash, Polarity: group.polarity, ReasonCode: report.ReasonCode,
				AgentConfidence: report.Confidence, TraceCorroborated: trace, OraclePassed: oracleOutcome,
				ReconciledWeight: store.ComputeReconciledWeight(group.polarity, report.Confidence, trace, oracle, report.AdditionalSearchNeeded),
				CreatedAt:        now.Unix(), Status: store.FeedbackStale,
			}
			if indexed {
				rec.Status = store.FeedbackValid
			}
			if rec.ID, err = st.InsertFeedback(rec); err != nil {
				return nil, err
			}
			out = append(out, rec)
		}
	}
	return out, nil
}

// ListFeedback returns the feedback of the worktree at root, with each record's status.
// A worktree that was never indexed has none.
func ListFeedback(root string) ([]store.FeedbackRecord, error) {
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		return []store.FeedbackRecord{}, err
	}
	defer func() { _ = st.Close() }()
	return st.ListFeedback(root)
}

// PurgeFeedback deletes the stale feedback of the worktree at root, or all of it when all is set.
func PurgeFeedback(root string, all bool) (int64, error) {
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		return 0, err
	}
	defer func() { _ = st.Close() }()
	return st.PurgeFeedback(root, all)
}

// normalizeReport fills omitted labels with their neutral value and rejects values outside the
// documented ones.
func normalizeReport(r *store.AgentFeedbackReport) error {
	if r.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1, got %d", r.SchemaVersion)
	}
	if r.ContextHelpfulness == "" {
		r.ContextHelpfulness = "unknown"
	}
	if r.ReasonCode == "" {
		r.ReasonCode = "other"
	}
	if r.Confidence == "" {
		r.Confidence = "medium"
	}
	for _, check := range []struct {
		field, value string
		allowed      []string
	}{
		{"context_helpfulness", r.ContextHelpfulness, feedbackHelpfulness},
		{"reason_code", r.ReasonCode, feedbackReasons},
		{"confidence", r.Confidence, feedbackConfidence},
	} {
		if !slices.Contains(check.allowed, check.value) {
			return fmt.Errorf("%s %q is not one of %s", check.field, check.value, strings.Join(check.allowed, ", "))
		}
	}
	return nil
}

// sessionTrace says what the observer saw of file in the session: +1 when the agent read or edited
// it, -1 when the session made tool calls and never did, 0 when there is no trace. Reads are cleared
// by each edit, so a file read before the last edit and not edited reads as -1.
func sessionTrace(root, session, file string) int {
	if session == "" {
		return 0
	}
	for _, agent := range []adapter.Agent{adapter.Claude, adapter.Codex, adapter.Antigravity} {
		statePath := paths.ObserverState(root, string(agent), session)
		if _, err := os.Stat(statePath); err != nil {
			continue
		}
		s := brief.LoadSignals(statePath, worktreeVersion(root), time.Now())
		if len(s.Reads)+len(s.Edits)+len(s.Searches) == 0 {
			return 0
		}
		h := hashID(root, file)
		if s.Reads[h] > 0 || s.Edits[h] > 0 {
			return 1
		}
		return -1
	}
	return 0
}
