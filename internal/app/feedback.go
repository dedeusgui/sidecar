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
	// The oracle is the outcome of a test or build run. The agent's completion claim is recorded
	// on its own and never used as the outcome: an unknown oracle contributes nothing.
	oracle := store.OracleOutcome(report.OracleStatus)
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
				AgentConfidence: report.Confidence, AgentCompleted: report.TaskCompleted,
				TraceCorroborated: trace, OracleStatus: report.OracleStatus, Provenance: report.OracleProvenance,
				OraclePassed: oracle, EvidenceUsed: store.EvidenceUse(trace),
				Tier:             store.ClassifyFeedback(oracle, report.OracleProvenance, trace),
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
	if err := normalizeOracle(r); err != nil {
		return err
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
		switch {
		case s.Edits[h] > 0:
			return store.TraceEdited
		case s.Reads[h] > 0:
			return store.TraceRead
		}
		return store.TraceUntouched
	}
	return store.TraceNotObserved
}

// normalizeOracle reads the oracle outcome of a report. Only an explicit oracle_status or
// oracle_passed sets it: the agent's task_completed is a claim about the task, not a test result,
// so it never becomes the oracle.
func normalizeOracle(r *store.AgentFeedbackReport) error {
	switch r.OracleStatus {
	case "":
		switch {
		case r.OraclePassed == nil:
			r.OracleStatus = store.OracleUnknown
		case *r.OraclePassed:
			r.OracleStatus = store.OraclePassed
		default:
			r.OracleStatus = store.OracleFailed
		}
	case store.OraclePassed, store.OracleFailed, store.OracleUnknown:
		if r.OraclePassed != nil {
			want := store.OracleFailed
			if *r.OraclePassed {
				want = store.OraclePassed
			}
			if r.OracleStatus != want {
				return fmt.Errorf("oracle_status %q contradicts oracle_passed %v", r.OracleStatus, *r.OraclePassed)
			}
		}
	default:
		return fmt.Errorf("oracle_status %q is not one of passed, failed, unknown", r.OracleStatus)
	}
	if r.OracleStatus == store.OracleUnknown {
		r.OracleProvenance = store.ProvenanceNone
		return nil
	}
	if r.OracleProvenance == "" {
		r.OracleProvenance = store.ProvenanceAgentReport
	}
	switch r.OracleProvenance {
	case store.ProvenanceHarness, store.ProvenanceHuman, store.ProvenanceAgentReport:
		return nil
	}
	return fmt.Errorf("oracle_provenance %q is not one of harness, human, agent_report", r.OracleProvenance)
}
