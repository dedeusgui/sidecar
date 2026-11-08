package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/gitx"
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

// RecordFeedback stores an agent's post-task report: one record per evidence item it named, helpful
// (+1) or misleading (-1). A report cannot claim independent provenance; that needs an oracle artifact
// and is recorded by RecordFeedbackWithOracle. Every ID must name evidence in this index and must have
// been delivered to the session, or the report is refused before anything is written.
func RecordFeedback(root string, report store.AgentFeedbackReport) ([]store.FeedbackRecord, error) {
	return recordFeedback(root, report, nil, nil)
}

// RecordFeedbackWithOracle records a report whose oracle outcome comes from an independent harness.
// The artifact must verify under key, belong to the report's session, and name the worktree's HEAD
// commit. Only then is the provenance harness, which makes an evidence item strong when it was delivered.
func RecordFeedbackWithOracle(root string, report store.AgentFeedbackReport, art OracleArtifact, key []byte) ([]store.FeedbackRecord, error) {
	return recordFeedback(root, report, &art, key)
}

func recordFeedback(root string, report store.AgentFeedbackReport, art *OracleArtifact, key []byte) ([]store.FeedbackRecord, error) {
	if err := normalizeReport(&report); err != nil {
		return nil, err
	}
	if art != nil {
		if err := verifyOracle(root, report.SessionID, *art, key); err != nil {
			return nil, err
		}
		report.OracleStatus = art.Status
		report.OracleProvenance = store.ProvenanceHarness
	}
	if err := checkContradictions(report); err != nil {
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

	type judged struct {
		id, path, symbol string
		polarity         int
	}
	var items []judged
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
			if _, indexed := files[path]; !indexed {
				return nil, fmt.Errorf("evidence %q is not in the index", id)
			}
			if symbol != "" {
				hits, err := st.ExactSymbols([]string{store.KeyOf(symbol)}, 50)
				if err != nil {
					return nil, err
				}
				if !slices.ContainsFunc(hits, func(h store.SymbolHit) bool { return h.Path == path && h.Name == symbol }) {
					return nil, fmt.Errorf("evidence %q names no symbol %q in %s", id, symbol, path)
				}
			}
			items = append(items, judged{id: id, path: path, symbol: symbol, polarity: group.polarity})
		}
	}

	oracle := store.OracleOutcome(report.OracleStatus)
	state, hasState := sessionState(root, report.SessionID)
	now := time.Now()
	var out []store.FeedbackRecord
	for _, it := range items {
		trace := traceOf(state, hasState, root, it.path)
		presented := hasState && wasPresented(state, root, it.path, it.symbol)
		file := files[it.path]
		rec := store.FeedbackRecord{
			Worktree: root, SessionID: report.SessionID, EvidenceID: it.id, FilePath: it.path, Symbol: it.symbol,
			FileHash: file.Hash, Polarity: it.polarity, ReasonCode: report.ReasonCode,
			AgentConfidence: report.Confidence, AgentCompleted: report.TaskCompleted,
			TraceCorroborated: trace, OracleStatus: report.OracleStatus, Provenance: report.OracleProvenance,
			OraclePassed: oracle, EvidenceUsed: store.EvidenceUse(trace), Presented: presented,
			Tier:             store.ClassifyFeedback(oracle, report.OracleProvenance, presented, trace),
			ReconciledWeight: store.ComputeReconciledWeight(it.polarity, report.Confidence, trace, oracle, report.AdditionalSearchNeeded),
			CreatedAt:        now.Unix(), Status: store.FeedbackStale,
		}
		if art != nil {
			rec.OracleRunID, rec.OracleCommit, rec.OracleSHA256 = art.RunID, art.Commit, art.OracleSHA256
		}
		rec.Status = store.FeedbackValid // every ID was checked against the index above
		if rec.ID, err = st.InsertFeedback(rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// verifyOracle checks an oracle artifact against the report and the worktree: the digest, the session
// and the commit. A mismatch in any of them is an error, so nothing is recorded under a foreign identity.
func verifyOracle(root, session string, art OracleArtifact, key []byte) error {
	if err := verifyOracleArtifact(key, art); err != nil {
		return err
	}
	if session == "" || art.SessionID != session {
		return fmt.Errorf("oracle artifact belongs to session %q, the report names %q", art.SessionID, session)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	head, err := gitx.Head(ctx, root)
	if err != nil {
		return fmt.Errorf("cannot read the worktree commit: %w", err)
	}
	if art.Commit != head {
		return fmt.Errorf("oracle artifact is for commit %s, the worktree is at %s", art.Commit, head)
	}
	return nil
}

// checkContradictions refuses a report that calls the same evidence both helpful and misleading.
func checkContradictions(r store.AgentFeedbackReport) error {
	for _, id := range r.HelpfulEvidenceIDs {
		if slices.Contains(r.MisleadingEvidenceIDs, id) {
			return fmt.Errorf("evidence %q is reported both helpful and misleading", id)
		}
	}
	return nil
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

// normalizeOracle reads the oracle outcome of a report. Only an explicit oracle_status or
// oracle_passed sets it: the agent's task_completed is a claim about the task, not a test result, so
// it never becomes the oracle. A report may not declare harness or human provenance: that needs an
// oracle artifact, because the report itself is written by the party whose result is being judged.
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
	switch r.OracleProvenance {
	case "", store.ProvenanceAgentReport:
		r.OracleProvenance = store.ProvenanceAgentReport
		return nil
	case store.ProvenanceHarness, store.ProvenanceHuman:
		return fmt.Errorf("oracle_provenance %q needs an oracle artifact from the harness; a report cannot declare it", r.OracleProvenance)
	}
	return fmt.Errorf("oracle_provenance %q is not one of harness, human, agent_report", r.OracleProvenance)
}

// sessionState loads the observer state of a session for the first agent that has one. A session
// with no state has nothing presented and no trace.
func sessionState(root, session string) (brief.SessionSignals, bool) {
	if session == "" {
		return brief.SessionSignals{}, false
	}
	for _, agent := range []adapter.Agent{adapter.Claude, adapter.Codex, adapter.Antigravity} {
		statePath := paths.ObserverState(root, string(agent), session)
		if _, err := os.Stat(statePath); err != nil {
			continue
		}
		return brief.LoadSignals(statePath, worktreeVersion(root), time.Now()), true
	}
	return brief.SessionSignals{}, false
}

// wasPresented reports whether the hook delivered this evidence to the session. A file-level ID needs
// the file to have been delivered; a symbol-level ID needs that symbol to have been delivered.
func wasPresented(s brief.SessionSignals, root, path, symbol string) bool {
	if symbol == "" {
		return s.Presented[presentedPathKey(root, path)]
	}
	return s.Presented[presentedSymbolKey(root, path, symbol)]
}

// traceOf says what the observer saw of the file in the session: +2 when the agent edited it, +1 when it
// read it, -1 when the session made tool calls and never touched it, 0 when there is no trace. An edit
// is an association with the patch, not proof that the snippet caused it.
func traceOf(s brief.SessionSignals, hasState bool, root, file string) int {
	if !hasState {
		return store.TraceNotObserved
	}
	if len(s.Reads)+len(s.Edits)+len(s.Searches) == 0 {
		return store.TraceNotObserved
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
