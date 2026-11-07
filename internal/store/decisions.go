package store

import (
	"time"
)

// Decision statuses. A decision is trusted only while its file hash, its symbol and its TTL
// still match the code: a changed file makes it stale, a missing file, a missing symbol or
// an expired TTL makes it invalidated.
const (
	DecisionValid       = "valid"
	DecisionStale       = "stale"
	DecisionInvalidated = "invalidated"
)

// DecisionRecord is an explicit technical decision about one file or symbol. It stores a
// summary written by the user and hashes of the code it refers to, never code or prompts.
type DecisionRecord struct {
	ID           int64  `json:"id"`
	RepoRoot     string `json:"repo_root"`
	Worktree     string `json:"worktree"`
	SymbolOrPath string `json:"symbol_or_path"`
	FilePath     string `json:"file_path"`
	Symbol       string `json:"symbol,omitempty"`
	CommitHash   string `json:"commit_hash"`
	FileHash     string `json:"file_hash"`
	Summary      string `json:"summary"`
	Origin       string `json:"origin"`
	SessionID    string `json:"session_id,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	TTLSeconds   int64  `json:"ttl_seconds"`
	Status       string `json:"status"`
}

// InsertDecision stores a decision and returns its ID.
func (s *Store) InsertDecision(d DecisionRecord) (int64, error) {
	var id int64
	err := s.withWriteTx(func(tx execer) error {
		res, err := tx.Exec(`INSERT INTO decisions (repo_root, worktree, symbol_or_path, file_path, symbol,
			commit_hash, file_hash, summary, origin, session_id, created_at, ttl_seconds, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.RepoRoot, d.Worktree, d.SymbolOrPath, d.FilePath, d.Symbol, d.CommitHash, d.FileHash,
			d.Summary, d.Origin, d.SessionID, d.CreatedAt, d.TTLSeconds, DecisionValid)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// ListDecisions returns the decisions recorded for one worktree, oldest first, with the
// status stored at the last refresh.
func (s *Store) ListDecisions(worktree string) ([]DecisionRecord, error) {
	rows, err := s.db.Query(`SELECT id, repo_root, worktree, symbol_or_path, file_path, symbol,
		commit_hash, file_hash, summary, origin, session_id, created_at, ttl_seconds, status
		FROM decisions WHERE worktree = ? ORDER BY id`, worktree)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []DecisionRecord{}
	for rows.Next() {
		var d DecisionRecord
		if err := rows.Scan(&d.ID, &d.RepoRoot, &d.Worktree, &d.SymbolOrPath, &d.FilePath, &d.Symbol,
			&d.CommitHash, &d.FileHash, &d.Summary, &d.Origin, &d.SessionID, &d.CreatedAt,
			&d.TTLSeconds, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RefreshDecisions recomputes the status of every decision of a worktree and stores it.
// current reports the file's content hash now and whether the file exists. The symbol
// check reads the index, so it reflects the last index sync.
func (s *Store) RefreshDecisions(worktree string, now time.Time, current func(path string) (string, bool)) ([]DecisionRecord, error) {
	list, err := s.ListDecisions(worktree)
	if err != nil {
		return nil, err
	}
	// Judge before taking the write lock: the single connection cannot serve a read while
	// a transaction holds it.
	for i := range list {
		if list[i].Status, err = s.judge(list[i], now, current); err != nil {
			return nil, err
		}
	}
	err = s.withWriteTx(func(tx execer) error {
		for _, d := range list {
			if _, err := tx.Exec(`UPDATE decisions SET status = ? WHERE id = ?`, d.Status, d.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return list, err
}

func (s *Store) judge(d DecisionRecord, now time.Time, current func(string) (string, bool)) (string, error) {
	if now.Unix() > d.CreatedAt+d.TTLSeconds {
		return DecisionInvalidated, nil
	}
	hash, exists := current(d.FilePath)
	if !exists {
		return DecisionInvalidated, nil
	}
	if d.Symbol != "" {
		var n int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM symbols s JOIN files f ON f.id = s.file_id
			WHERE f.path = ? AND s.name = ? AND s.kind NOT IN ('file', 'section')`, d.FilePath, d.Symbol).Scan(&n)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return DecisionInvalidated, nil
		}
	}
	if hash != d.FileHash {
		return DecisionStale, nil
	}
	return DecisionValid, nil
}

// PurgeDecisions deletes a worktree's invalidated decisions, or all of them when all is set.
// It returns how many were deleted.
func (s *Store) PurgeDecisions(worktree string, all bool) (int64, error) {
	query := `DELETE FROM decisions WHERE worktree = ? AND status = ?`
	args := []any{worktree, DecisionInvalidated}
	if all {
		query = `DELETE FROM decisions WHERE worktree = ?`
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
