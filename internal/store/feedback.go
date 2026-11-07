package store

import (
	"fmt"
	"math"
	"strings"
)

// Feedback statuses. A record is valid while the file it judged still has the hash it had
// when the feedback was written (or the record was written without a hash). A changed or
// removed file makes it stale; purge deletes stale records.
const (
	FeedbackValid = "valid"
	FeedbackStale = "stale"
)

// Signal weights of the reconciliation. They are fixed, not learned: the outcome of the task
// is the strongest signal and the agent's own report the weakest, because it can be
// post-rationalized.
const (
	weightAgent  = 0.20
	weightTrace  = 0.35
	weightOracle = 0.45
	// feedbackMaxDelta bounds one record's effect on retrieval, and the sum over a target.
	feedbackMaxDelta = 0.25
	// feedbackDecay makes each older record on the same target count less than the newer one.
	feedbackDecay = 0.85
)

// AgentFeedbackReport is the structured post-task output emitted by an agent
// (or recorded via `sidecar feedback record`) after finishing a task.
type AgentFeedbackReport struct {
	SchemaVersion          int      `json:"schema_version"`
	SessionID              string   `json:"session_id,omitempty"`
	TaskCompleted          bool     `json:"task_completed"`
	OraclePassed           *bool    `json:"oracle_passed,omitempty"` // optional explicit test/build result; defaults to TaskCompleted
	ContextHelpfulness     string   `json:"context_helpfulness"`     // helpful | neutral | harmful | unknown
	HelpfulEvidenceIDs     []string `json:"helpful_evidence_ids"`
	MisleadingEvidenceIDs  []string `json:"misleading_evidence_ids"`
	AdditionalSearchNeeded bool     `json:"additional_search_needed"`
	ReasonCode             string   `json:"reason_code"` // exact_match | wrong_file | stale_context | missing_caller | noisy_neighbor | other
	Confidence             string   `json:"confidence"`  // high | medium | low
}

// FeedbackRecord is one judged evidence item of one report.
type FeedbackRecord struct {
	ID                int64   `json:"id"`
	Worktree          string  `json:"worktree"`
	SessionID         string  `json:"session_id,omitempty"`
	EvidenceID        string  `json:"evidence_id"`
	FilePath          string  `json:"file_path"`
	Symbol            string  `json:"symbol,omitempty"`
	FileHash          string  `json:"file_hash"`
	Polarity          int     `json:"polarity"` // +1 (helpful) or -1 (misleading)
	ReasonCode        string  `json:"reason_code"`
	AgentConfidence   string  `json:"agent_confidence"`
	TraceCorroborated int     `json:"trace_corroborated"` // +1 corroborated, 0 unobserved, -1 contradicted
	OraclePassed      int     `json:"oracle_passed"`      // +1 passed, -1 failed
	ReconciledWeight  float64 `json:"reconciled_weight"`  // bounded delta in [-0.25, +0.25]
	CreatedAt         int64   `json:"created_at"`
	Status            string  `json:"status"` // valid | stale (computed on read by comparing FileHash with files.hash)
}

// ParseEvidenceID splits an evidence ID into its file path and symbol. Canonical IDs look like
// "path#kind:Name@line"; "path#Name" and a bare "path" are accepted too.
func ParseEvidenceID(id string) (filePath, symbol string) {
	filePath, rest, found := strings.Cut(id, "#")
	if !found {
		return id, ""
	}
	if _, name, ok := strings.Cut(rest, ":"); ok {
		rest = name
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[:i]
	}
	return filePath, rest
}

// ComputeReconciledWeight computes the bounded deterministic score delta in [-0.25, +0.25]
// for a single evidence item without any LLM or network call. polarity is +1 for an item the
// agent reported helpful and -1 for one it reported misleading. traceCorroborated is +1 when the
// observer saw the agent read or edit the file, -1 when it did not, and 0 when there is no trace.
//
// Weights: w_oracle = 0.45 (task/test outcome), w_trace = 0.35 (tool trace corroboration),
// w_agent = 0.20 (agent self-report scaled by its confidence).
func ComputeReconciledWeight(polarity int, confidence string, traceCorroborated int, oraclePassed bool, additionalSearchNeeded bool) float64 {
	confScale := 0.7
	switch confidence {
	case "high":
		confScale = 1.0
	case "low":
		confScale = 0.4
	}
	sAgent := float64(polarity) * confScale
	// A "helpful" claim that the trace contradicts (the file was never read or edited) counts for
	// a quarter, so praise the agent cannot back up does not raise the weight.
	if polarity > 0 && traceCorroborated < 0 {
		sAgent *= 0.25
	}

	var sTrace float64
	if polarity > 0 {
		switch {
		case traceCorroborated > 0:
			sTrace = 1
		case traceCorroborated < 0:
			sTrace = -0.5
		}
	} else if traceCorroborated <= 0 {
		sTrace = -1 // the trace agrees the item was not useful
	} else {
		sTrace = -0.25 // the agent read it and still flagged it
	}

	var sOracle float64
	if polarity > 0 {
		sOracle = -0.5
		if oraclePassed {
			sOracle = 1
		}
	} else {
		sOracle = -0.6
		if !oraclePassed || additionalSearchNeeded {
			sOracle = -1
		}
	}

	raw := weightAgent*sAgent + weightTrace*sTrace + weightOracle*sOracle
	return clamp(raw*0.25, -feedbackMaxDelta, feedbackMaxDelta)
}

// InsertFeedback stores one judged evidence item and returns its ID. rec.ReconciledWeight is
// computed by the caller, which knows whether the agent needed more search.
func (s *Store) InsertFeedback(rec FeedbackRecord) (int64, error) {
	if rec.Polarity != 1 && rec.Polarity != -1 {
		return 0, fmt.Errorf("feedback polarity must be +1 or -1, got %d", rec.Polarity)
	}
	var id int64
	err := s.withWriteTx(func(tx execer) error {
		res, err := tx.Exec(`INSERT INTO evidence_feedback (worktree, session_id, evidence_id, file_path, symbol,
			file_hash, polarity, reason_code, agent_confidence, trace_corroborated, oracle_passed,
			reconciled_weight, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.Worktree, rec.SessionID, rec.EvidenceID, rec.FilePath, rec.Symbol, rec.FileHash,
			rec.Polarity, rec.ReasonCode, rec.AgentConfidence, rec.TraceCorroborated, rec.OraclePassed,
			clamp(rec.ReconciledWeight, -feedbackMaxDelta, feedbackMaxDelta), rec.CreatedAt)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// ListFeedback returns the feedback recorded for one worktree, oldest first, with each record's
// status computed against the file hashes now in the index.
func (s *Store) ListFeedback(worktree string) ([]FeedbackRecord, error) {
	rows, err := s.db.Query(`SELECT ef.id, ef.worktree, ef.session_id, ef.evidence_id, ef.file_path, ef.symbol,
		ef.file_hash, ef.polarity, ef.reason_code, ef.agent_confidence, ef.trace_corroborated,
		ef.oracle_passed, ef.reconciled_weight, ef.created_at,
		f.path IS NOT NULL AND (ef.file_hash = '' OR ef.file_hash = f.hash)
		FROM evidence_feedback ef LEFT JOIN files f ON f.path = ef.file_path
		WHERE ef.worktree = ? ORDER BY ef.id`, worktree)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []FeedbackRecord{}
	for rows.Next() {
		var r FeedbackRecord
		var valid bool
		if err := rows.Scan(&r.ID, &r.Worktree, &r.SessionID, &r.EvidenceID, &r.FilePath, &r.Symbol,
			&r.FileHash, &r.Polarity, &r.ReasonCode, &r.AgentConfidence, &r.TraceCorroborated,
			&r.OraclePassed, &r.ReconciledWeight, &r.CreatedAt, &valid); err != nil {
			return nil, err
		}
		r.Status = FeedbackStale
		if valid {
			r.Status = FeedbackValid
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FeedbackMultipliers returns the retrieval multiplier (1 + delta) of every target with valid
// feedback in a worktree. A symbol's feedback is keyed "path#symbol" and a file's "path", so
// feedback on one symbol does not demote its neighbours. Newer records weigh more: the n-th
// newest record on a target counts 0.85^n. Each target's delta is clamped to [-0.25, +0.25].
func (s *Store) FeedbackMultipliers(worktree string) (map[string]float64, error) {
	rows, err := s.db.Query(`SELECT ef.file_path, ef.symbol, ef.reconciled_weight
		FROM evidence_feedback ef JOIN files f ON f.path = ef.file_path
		WHERE ef.worktree = ? AND (ef.file_hash = '' OR ef.file_hash = f.hash)
		ORDER BY ef.created_at DESC, ef.id DESC`, worktree)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	deltas := map[string]float64{}
	ranks := map[string]int{}
	for rows.Next() {
		var path, symbol string
		var weight float64
		if err := rows.Scan(&path, &symbol, &weight); err != nil {
			return nil, err
		}
		key := path
		if symbol != "" {
			key = path + "#" + symbol
		}
		deltas[key] += weight * math.Pow(feedbackDecay, float64(ranks[key]))
		ranks[key]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(deltas))
	for key, delta := range deltas {
		out[key] = 1 + clamp(delta, -feedbackMaxDelta, feedbackMaxDelta)
	}
	return out, nil
}

// PurgeFeedback deletes a worktree's stale feedback, or all of its feedback when all is set.
// It returns how many records were deleted.
func (s *Store) PurgeFeedback(worktree string, all bool) (int64, error) {
	query := `DELETE FROM evidence_feedback WHERE worktree = ? AND id NOT IN (
		SELECT ef.id FROM evidence_feedback ef JOIN files f ON f.path = ef.file_path
		WHERE ef.worktree = ? AND (ef.file_hash = '' OR ef.file_hash = f.hash))`
	args := []any{worktree, worktree}
	if all {
		query = `DELETE FROM evidence_feedback WHERE worktree = ?`
		args = []any{worktree}
	}
	var n int64
	err := s.withWriteTx(func(tx execer) error {
		res, err := tx.Exec(query, args...)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return n, err
}

func clamp(x, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, x))
}
