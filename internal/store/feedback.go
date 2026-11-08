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

// Oracle status is the outcome of a test or build run. Unknown is not a failure: it contributes
// nothing to the weight. The agent's statement that it completed the task is a separate field
// (agent_reported_completion) and never becomes an oracle outcome.
const (
	OraclePassed  = "passed"
	OracleFailed  = "failed"
	OracleUnknown = "unknown"
)

// Provenance says who produced the oracle outcome. Only a run outside the agent (harness) or a
// person (human) can make feedback strong; a report written by the agent is a self-report.
const (
	ProvenanceHarness     = "harness"
	ProvenanceHuman       = "human"
	ProvenanceAgentReport = "agent_report"
	ProvenanceNone        = "none"
	// ProvenanceLegacy marks rows written before schema v6, whose oracle value was the agent's
	// completion claim. They are kept for diagnosis and never applied.
	ProvenanceLegacy = "legacy"
)

// Tiers follow the evidence hierarchy of the evaluation plan. Only strong and medium feedback
// changes the ranking; weak feedback is stored and never applied on its own.
const (
	// TierStrong: an oracle outcome from an independent run or a human-verified annotation.
	TierStrong = "strong"
	// TierMedium: the evidence file was edited in the session, objectively tying it to the patch
	// and corroborated by the observer trace.
	TierMedium = "medium"
	// TierWeak: a self-report, a citation, a file opened or an inferred use.
	TierWeak = "weak"
)

// Observer trace levels, stored in trace_corroborated. Absence of a read is not evidence that
// the item was useless: TraceUntouched only discounts a praise the trace cannot back up.
const (
	TraceUntouched   = -1 // the session had tool activity and never read or edited the file
	TraceNotObserved = 0  // no trace for this session
	TraceRead        = 1  // the file was read in the session
	TraceEdited      = 2  // the file was edited in the session (patch association)
)

// Evidence use: how much the trace says the agent interacted with the evidence. None of these
// proves that the evidence caused a change.
const (
	EvidenceConfirmed = "confirmed" // edited in the session
	EvidenceInferred  = "inferred"  // read in the session, no edit
	EvidenceUnknown   = "unknown"   // no observation, or the trace shows no interaction
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

// tierFactor scales a record's reconciled weight when it is applied to the ranking. Weak feedback
// has no factor: it never changes the ranking by itself.
var tierFactor = map[string]float64{TierStrong: 1, TierMedium: 0.5}

// AgentFeedbackReport is the structured post-task output emitted by an agent
// (or recorded via `sidecar feedback record`) after finishing a task.
type AgentFeedbackReport struct {
	SchemaVersion int    `json:"schema_version"`
	SessionID     string `json:"session_id,omitempty"`
	// TaskCompleted is the agent's own claim that it finished. It is never used as the oracle.
	TaskCompleted bool `json:"task_completed"`
	// OraclePassed is the outcome of a test or build run; nil means none was reported.
	OraclePassed *bool `json:"oracle_passed,omitempty"`
	// OracleStatus is passed, failed or unknown. When omitted it is derived from OraclePassed.
	OracleStatus string `json:"oracle_status,omitempty"`
	// OracleProvenance is harness, human or agent_report. Defaults to agent_report when the oracle is known.
	OracleProvenance       string   `json:"oracle_provenance,omitempty"`
	ContextHelpfulness     string   `json:"context_helpfulness"` // helpful | neutral | harmful | unknown
	HelpfulEvidenceIDs     []string `json:"helpful_evidence_ids"`
	MisleadingEvidenceIDs  []string `json:"misleading_evidence_ids"`
	AdditionalSearchNeeded bool     `json:"additional_search_needed"`
	ReasonCode             string   `json:"reason_code"` // exact_match | wrong_file | stale_context | missing_caller | noisy_neighbor | other
	Confidence             string   `json:"confidence"`  // high | medium | low
}

// FeedbackRecord is one judged evidence item of one report.
type FeedbackRecord struct {
	ID                int64  `json:"id"`
	Worktree          string `json:"worktree"`
	SessionID         string `json:"session_id,omitempty"`
	EvidenceID        string `json:"evidence_id"`
	FilePath          string `json:"file_path"`
	Symbol            string `json:"symbol,omitempty"`
	FileHash          string `json:"file_hash"`
	Polarity          int    `json:"polarity"` // feedback_label: +1 helpful, -1 misleading
	ReasonCode        string `json:"reason_code"`
	AgentConfidence   string `json:"agent_confidence"`
	AgentCompleted    bool   `json:"agent_reported_completion"`
	TraceCorroborated int    `json:"trace_corroborated"` // see TraceEdited, TraceRead, TraceNotObserved, TraceUntouched
	OracleStatus      string `json:"oracle_status"`      // passed | failed | unknown
	Provenance        string `json:"feedback_provenance"`
	OraclePassed      int    `json:"oracle_passed"` // oracle outcome: +1 passed, -1 failed, 0 unknown
	// Presented is true only when the evidence was injected into the session this report names.
	// Feedback on evidence the agent never received is kept for diagnosis, never applied.
	Presented        bool    `json:"evidence_presented"`
	OracleRunID      string  `json:"oracle_run_id,omitempty"`
	OracleCommit     string  `json:"oracle_commit,omitempty"`
	OracleSHA256     string  `json:"oracle_sha256,omitempty"`
	EvidenceUsed     string  `json:"evidence_used"` // confirmed | inferred | unknown
	Tier             string  `json:"feedback_confidence"`
	ReconciledWeight float64 `json:"reconciled_weight"` // bounded delta in [-0.25, +0.25], before the tier factor
	CreatedAt        int64   `json:"created_at"`
	Status           string  `json:"status"` // valid | stale (computed on read by comparing FileHash with files.hash)
}

// OracleOutcome maps an oracle status to +1 passed, -1 failed or 0 unknown.
func OracleOutcome(status string) int {
	switch status {
	case OraclePassed:
		return 1
	case OracleFailed:
		return -1
	}
	return 0
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
// agent reported helpful and -1 for one it reported misleading. trace is one of the Trace*
// levels. oracle is +1 passed, -1 failed or 0 unknown.
//
// An unknown oracle adds nothing: the agent's completion claim is not an oracle. A trace that
// does not show the file (TraceUntouched or TraceNotObserved) adds nothing for a misleading flag,
// because not reading a file does not show it was useless. For a helpful claim it only discounts
// the praise, because the agent may have used the injected context without opening the file.
//
// Weights: w_oracle = 0.45, w_trace = 0.35, w_agent = 0.20 (self-report scaled by its confidence).
func ComputeReconciledWeight(polarity int, confidence string, trace int, oracle int, additionalSearchNeeded bool) float64 {
	confScale := 0.7
	switch confidence {
	case "high":
		confScale = 1.0
	case "low":
		confScale = 0.4
	}
	sAgent := float64(polarity) * confScale
	// Praise the trace cannot back up (the file was never read or edited) counts for a quarter.
	if polarity > 0 && trace == TraceUntouched {
		sAgent *= 0.25
	}

	var sTrace float64
	if polarity > 0 {
		switch trace {
		case TraceEdited:
			sTrace = 1
		case TraceRead:
			sTrace = 0.5
		}
	} else if trace > 0 {
		sTrace = -0.25 // the agent read the item and still flagged it
	}

	var sOracle float64
	switch {
	case oracle == 0:
		sOracle = 0
	case polarity > 0 && oracle > 0:
		sOracle = 1
	case polarity > 0:
		sOracle = -0.5
	case oracle < 0 || additionalSearchNeeded:
		sOracle = -1
	default:
		sOracle = -0.6
	}

	raw := weightAgent*sAgent + weightTrace*sTrace + weightOracle*sOracle
	return clamp(raw*0.25, -feedbackMaxDelta, feedbackMaxDelta)
}

// ClassifyFeedback assigns the tier of one record. Nothing is strong or medium unless the evidence
// was presented to the session: attribution to evidence the agent never received is unsound. Strong
// then needs a known oracle outcome from an independent run or a person. Medium needs the file
// edited in the session. Everything else is weak and is never applied to the ranking.
func ClassifyFeedback(oracle int, provenance string, presented bool, trace int) string {
	if !presented {
		return TierWeak
	}
	if oracle != 0 && (provenance == ProvenanceHarness || provenance == ProvenanceHuman) {
		return TierStrong
	}
	if trace == TraceEdited {
		return TierMedium
	}
	return TierWeak
}

// EvidenceUse says what the trace shows of the agent's interaction with the evidence.
func EvidenceUse(trace int) string {
	switch trace {
	case TraceEdited:
		return EvidenceConfirmed
	case TraceRead:
		return EvidenceInferred
	}
	return EvidenceUnknown
}

// InsertFeedback stores one judged evidence item and returns its ID. rec.ReconciledWeight is
// computed by the caller, which knows whether the agent needed more search. Empty classification
// fields take the weakest value, so a caller that forgets them cannot strengthen a record.
func (s *Store) InsertFeedback(rec FeedbackRecord) (int64, error) {
	if rec.Polarity != 1 && rec.Polarity != -1 {
		return 0, fmt.Errorf("feedback polarity must be +1 or -1, got %d", rec.Polarity)
	}
	if rec.OracleStatus == "" {
		rec.OracleStatus = OracleUnknown
	}
	if rec.Provenance == "" {
		rec.Provenance = ProvenanceNone
	}
	if rec.EvidenceUsed == "" {
		rec.EvidenceUsed = EvidenceUnknown
	}
	if rec.Tier == "" {
		rec.Tier = TierWeak
	}
	agentCompleted, presented := 0, 0
	if rec.AgentCompleted {
		agentCompleted = 1
	}
	if rec.Presented {
		presented = 1
	}
	var id int64
	err := s.withWriteTx(func(tx execer) error {
		res, err := tx.Exec(`INSERT INTO evidence_feedback (worktree, session_id, evidence_id, file_path, symbol,
			file_hash, polarity, reason_code, agent_confidence, trace_corroborated, oracle_passed,
			reconciled_weight, created_at, agent_completed, oracle_status, feedback_provenance, evidence_used,
			feedback_confidence, evidence_presented, oracle_run_id, oracle_commit, oracle_sha256)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.Worktree, rec.SessionID, rec.EvidenceID, rec.FilePath, rec.Symbol, rec.FileHash,
			rec.Polarity, rec.ReasonCode, rec.AgentConfidence, rec.TraceCorroborated, rec.OraclePassed,
			clamp(rec.ReconciledWeight, -feedbackMaxDelta, feedbackMaxDelta), rec.CreatedAt,
			agentCompleted, rec.OracleStatus, rec.Provenance, rec.EvidenceUsed, rec.Tier,
			presented, rec.OracleRunID, rec.OracleCommit, rec.OracleSHA256)
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
		ef.file_hash, ef.polarity, ef.reason_code, ef.agent_confidence, ef.agent_completed,
		ef.trace_corroborated, ef.oracle_status, ef.feedback_provenance, ef.oracle_passed, ef.evidence_used,
		ef.feedback_confidence, ef.reconciled_weight, ef.created_at, ef.evidence_presented, ef.oracle_run_id,
		ef.oracle_commit, ef.oracle_sha256,
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
		var agentCompleted, presented int
		if err := rows.Scan(&r.ID, &r.Worktree, &r.SessionID, &r.EvidenceID, &r.FilePath, &r.Symbol,
			&r.FileHash, &r.Polarity, &r.ReasonCode, &r.AgentConfidence, &agentCompleted,
			&r.TraceCorroborated, &r.OracleStatus, &r.Provenance, &r.OraclePassed, &r.EvidenceUsed,
			&r.Tier, &r.ReconciledWeight, &r.CreatedAt, &presented, &r.OracleRunID, &r.OracleCommit,
			&r.OracleSHA256, &valid); err != nil {
			return nil, err
		}
		r.AgentCompleted = agentCompleted == 1
		r.Presented = presented == 1
		r.Status = FeedbackStale
		if valid {
			r.Status = FeedbackValid
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FeedbackMultipliers returns the retrieval multiplier (1 + delta) of every target with valid
// feedback in a worktree. Only strong and medium records count; each is scaled by its tier factor.
// A symbol's feedback is keyed "path#symbol" and a file's "path", so feedback on one symbol does
// not demote its neighbours. Newer applied records weigh more: the n-th newest counts 0.85^n. Each
// target's delta is clamped to [-0.25, +0.25].
func (s *Store) FeedbackMultipliers(worktree string) (map[string]float64, error) {
	rows, err := s.db.Query(`SELECT ef.file_path, ef.symbol, ef.reconciled_weight, ef.feedback_confidence
		FROM evidence_feedback ef JOIN files f ON f.path = ef.file_path
		WHERE ef.worktree = ? AND (ef.file_hash = '' OR ef.file_hash = f.hash)
		AND ef.feedback_confidence IN (?, ?)
		ORDER BY ef.created_at DESC, ef.id DESC`, worktree, TierStrong, TierMedium)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	deltas := map[string]float64{}
	ranks := map[string]int{}
	for rows.Next() {
		var path, symbol, tier string
		var weight float64
		if err := rows.Scan(&path, &symbol, &weight, &tier); err != nil {
			return nil, err
		}
		key := path
		if symbol != "" {
			key = path + "#" + symbol
		}
		deltas[key] += tierFactor[tier] * weight * math.Pow(feedbackDecay, float64(ranks[key]))
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
