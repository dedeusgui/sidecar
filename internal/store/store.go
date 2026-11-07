// Package store persists the Sidecar index in SQLite (FTS5 for lexical search).
// The index is a regenerable cache: when it is unreadable it is moved aside and rebuilt.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver with FTS5; keeps the binary CGo-free
)

// SchemaVersion is bumped whenever the on-disk layout changes. Databases with a
// newer version are refused; older ones are migrated in place.
const SchemaVersion = 4

// ErrSchemaTooNew reports an index written by a newer Sidecar build.
var ErrSchemaTooNew = errors.New("index schema is newer than this sidecar build")

// Store is a handle to one worktree's index database.
type Store struct {
	db   *sql.DB
	path string
}

// FileRecord describes an indexed (or deliberately skipped) file.
type FileRecord struct {
	Path       string
	Lang       string
	Kind       string // code | test | doc | config | binary
	Size       int64
	MtimeNS    int64
	Hash       string
	Skipped    string // non-empty when content was not parsed (too_large, binary, sensitive...)
	PathTerms  string // analyzed path words for full-text search
	ExtractVer int    // version of the extraction logic that produced these rows
}

// SymbolRecord is one definition. Parent indexes the symbol slice passed to ReplaceFile
// (any order is accepted), or is -1. The *Terms fields hold analyzed text for search.
type SymbolRecord struct {
	Kind, Name, Signature, Doc, Value string
	Line, EndLine                     int
	Parent                            int
	NameTerms, SigTerms, DocTerms     string
}

// RefRecord is a name used from a symbol of the same file (From) or from file scope (-1).
type RefRecord struct {
	From int
	Name string
	Kind string
	Line int
}

// ImportRecord is a module imported by a file, before resolution to a file.
type ImportRecord struct {
	Module string
}

// PendingImport is an import whose target file is not known yet.
type PendingImport struct {
	ID     int64
	FileID int64
	Path   string
	Lang   string
	Module string
}

// SymbolHit is a search result joined with its file.
type SymbolHit struct {
	ID        int64
	FileID    int64
	Path      string
	Kind      string
	Name      string
	Signature string
	Line      int
	EndLine   int
	Doc       string
	Value     string
	FileHash  string
	Rank      float64 // bm25 value: lower is better
}

// CommitFile is one path changed by a commit.
type CommitFile struct {
	Status  string
	Path    string
	OldPath string
}

// CommitRecord is compact history metadata for one commit.
type CommitRecord struct {
	Hash         string
	Time         int64
	Subject      string
	SubjectTerms string
	Files        []CommitFile
}

const pragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"

// Open opens or creates the index at path and migrates it to SchemaVersion.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 25 * time.Millisecond)
		}
		s, err := openOnce(path)
		if err == nil {
			return s, nil
		}
		lastErr = err
		if errors.Is(err, ErrSchemaTooNew) || !transient(err) {
			break
		}
	}
	return nil, lastErr
}

func openOnce(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		return nil, err
	}
	// One connection keeps writes serialized inside this process.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// transient reports errors caused by another process touching the same files at the same
// moment (locks, WAL file creation or removal). They are worth one more try.
func transient(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "disk I/O error") ||
		strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "unable to open database")
}

// OpenOrRecover opens the index, moving an unreadable or incompatible file aside
// (path+".corrupt") and creating a fresh one. recovered reports whether that happened.
func OpenOrRecover(path string) (s *Store, recovered bool, err error) {
	s, err = Open(path)
	if err == nil {
		return s, false, nil
	}
	if qerr := Quarantine(path); qerr != nil {
		return nil, false, qerr
	}
	s, err = Open(path)
	return s, true, err
}

// Quarantine moves the index and its WAL files aside to path+".corrupt", replacing an earlier
// quarantine, so the next Open starts empty. Close the store before calling it.
func Quarantine(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := path + suffix
		if _, statErr := os.Stat(src); statErr == nil {
			dst := path + ".corrupt" + suffix
			_ = os.Remove(dst)
			if renameErr := os.Rename(src, dst); renameErr != nil {
				return fmt.Errorf("moving unreadable index aside: %w", renameErr)
			}
		}
	}
	return nil
}

// IsCorrupt reports errors that mean the index file itself is damaged. Busy or unavailable
// databases are not corrupt: quarantining one would discard a healthy index.
func IsCorrupt(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "malformed") || strings.Contains(msg, "not a database") ||
		strings.Contains(msg, "no such fts5 table")
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// SchemaVersion returns the version recorded in the database.
func (s *Store) SchemaVersion() (int, error) {
	var v int
	err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

// GetMeta returns a metadata value, or "" when absent.
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetMeta stores a metadata value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// execer is satisfied by *sql.Conn and *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// withWriteTx runs fn inside BEGIN IMMEDIATE. Taking the write lock up front makes a busy
// database wait (busy_timeout) instead of failing: a deferred transaction that reads and
// then writes gets SQLITE_BUSY at once when another process is writing.
func (s *Store) withWriteTx(fn func(q execer) error) error {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	if err := fn(connExec{ctx, conn}); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

// connExec adapts a dedicated connection to execer.
type connExec struct {
	ctx  context.Context
	conn *sql.Conn
}

func (c connExec) Exec(query string, args ...any) (sql.Result, error) {
	return c.conn.ExecContext(c.ctx, query, args...)
}

// ReplaceFile atomically replaces everything indexed for file.Path.
func (s *Store) ReplaceFile(file FileRecord, syms []SymbolRecord, refs []RefRecord, imports []ImportRecord) error {
	return s.withWriteTx(func(tx execer) error {
		return replaceFileTx(tx, file, syms, refs, imports)
	})
}

func replaceFileTx(tx execer, file FileRecord, syms []SymbolRecord, refs []RefRecord, imports []ImportRecord) error {
	if err := deleteFile(tx, file.Path); err != nil {
		return err
	}
	res, err := tx.Exec(`INSERT INTO files(path, lang, kind, size, mtime_ns, hash, skipped, extract_ver)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		file.Path, file.Lang, file.Kind, file.Size, file.MtimeNS, file.Hash, file.Skipped, file.ExtractVer)
	if err != nil {
		return err
	}
	fileID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	symIDs := make([]int64, len(syms))
	for i, sym := range syms {
		r, err := tx.Exec(`INSERT INTO symbols(file_id, kind, name, key, signature, doc, value, line, end_line)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fileID, sym.Kind, sym.Name, KeyOf(sym.Name), sym.Signature, sym.Doc, sym.Value, sym.Line, sym.EndLine)
		if err != nil {
			return err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		symIDs[i] = id
		if _, err := tx.Exec(`INSERT INTO symbol_fts(rowid, name_terms, sig_terms, doc_terms, path_terms)
			VALUES(?, ?, ?, ?, ?)`, id, sym.NameTerms, sym.SigTerms, sym.DocTerms, file.PathTerms); err != nil {
			return err
		}
	}
	// Parents are linked after every symbol exists, so declaration order does not matter.
	for i, sym := range syms {
		if sym.Parent >= 0 && sym.Parent < len(syms) && sym.Parent != i {
			if _, err := tx.Exec(`UPDATE symbols SET parent_id = ? WHERE id = ?`, symIDs[sym.Parent], symIDs[i]); err != nil {
				return err
			}
		}
	}
	for _, ref := range refs {
		from := sql.NullInt64{}
		if ref.From >= 0 && ref.From < len(symIDs) {
			from = sql.NullInt64{Int64: symIDs[ref.From], Valid: true}
		}
		if _, err := tx.Exec(`INSERT INTO refs(file_id, from_symbol_id, to_name, kind, line)
			VALUES(?, ?, ?, ?, ?)`, fileID, from, ref.Name, ref.Kind, ref.Line); err != nil {
			return err
		}
	}
	for _, imp := range imports {
		if _, err := tx.Exec(`INSERT INTO imports(file_id, module) VALUES(?, ?)`, fileID, imp.Module); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFile removes a file and everything derived from it.
func (s *Store) DeleteFile(path string) error {
	return s.withWriteTx(func(tx execer) error { return deleteFile(tx, path) })
}

func deleteFile(tx execer, path string) error {
	if _, err := tx.Exec(`DELETE FROM symbol_fts WHERE rowid IN
		(SELECT s.id FROM symbols s JOIN files f ON f.id = s.file_id WHERE f.path = ?)`, path); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM files WHERE path = ?`, path)
	return err
}

// FileRecords returns every indexed file keyed by path.
func (s *Store) FileRecords() (map[string]FileRecord, error) {
	rows, err := s.db.Query(`SELECT path, lang, kind, size, mtime_ns, hash, skipped, extract_ver FROM files`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]FileRecord{}
	for rows.Next() {
		var f FileRecord
		if err := rows.Scan(&f.Path, &f.Lang, &f.Kind, &f.Size, &f.MtimeNS, &f.Hash, &f.Skipped, &f.ExtractVer); err != nil {
			return nil, err
		}
		out[f.Path] = f
	}
	return out, rows.Err()
}

// SearchSymbols runs an FTS5 MATCH over symbol text. match must already be a valid FTS5
// expression built by the caller. Results are ordered by bm25 (best first).
func (s *Store) SearchSymbols(match string, limit int) ([]SymbolHit, error) {
	rows, err := s.db.Query(`SELECT s.id, s.file_id, f.path, s.kind, s.name, s.signature,
			s.line, s.end_line, s.doc, s.value, f.hash, bm25(symbol_fts, 10.0, 4.0, 2.0, 3.0)
		FROM symbol_fts
		JOIN symbols s ON s.id = symbol_fts.rowid
		JOIN files f ON f.id = s.file_id
		WHERE symbol_fts MATCH ?
		ORDER BY bm25(symbol_fts, 10.0, 4.0, 2.0, 3.0), f.path, s.line, s.name, s.kind, s.id
		LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []SymbolHit
	for rows.Next() {
		var h SymbolHit
		if err := rows.Scan(&h.ID, &h.FileID, &h.Path, &h.Kind, &h.Name, &h.Signature,
			&h.Line, &h.EndLine, &h.Doc, &h.Value, &h.FileHash, &h.Rank); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// TermDocFreq counts how often an analyzed term occurs in any indexed column, including
// the path. It answers whether the term is present in the repository at all.
func (s *Store) TermDocFreq(term string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(doc), 0) FROM symbol_vocab WHERE term = ?`, term).Scan(&n)
	return n, err
}

// TermSymbolFreq counts symbols whose own name, signature or documentation contains the term.
// Path words are excluded: they are copied into every symbol row of a file and would make
// a distinctive folder name look common.
func (s *Store) TermSymbolFreq(term string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(doc), 0) FROM symbol_vocab
		WHERE term = ? AND col IN ('name_terms', 'sig_terms', 'doc_terms')`, term).Scan(&n)
	return n, err
}

// DirtyPaths lists files that differed from HEAD at the previous sync.
func (s *Store) DirtyPaths() ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM dirty_paths ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetDirtyPaths replaces the set of files that differ from HEAD.
func (s *Store) SetDirtyPaths(paths []string) error {
	return s.withWriteTx(func(tx execer) error {
		if _, err := tx.Exec(`DELETE FROM dirty_paths`); err != nil {
			return err
		}
		for _, p := range paths {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO dirty_paths(path) VALUES(?)`, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// PendingImports lists imports whose target is still unresolved.
func (s *Store) PendingImports() ([]PendingImport, error) {
	rows, err := s.db.Query(`SELECT i.id, i.file_id, f.path, f.lang, i.module
		FROM imports i JOIN files f ON f.id = i.file_id
		WHERE i.resolved_file_id IS NULL ORDER BY i.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []PendingImport
	for rows.Next() {
		var p PendingImport
		if err := rows.Scan(&p.ID, &p.FileID, &p.Path, &p.Lang, &p.Module); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResolvedImportPaths lists the repository files a file imports, once resolved.
func (s *Store) ResolvedImportPaths(fromPath string) ([]string, error) {
	rows, err := s.db.Query(`SELECT t.path FROM imports i
		JOIN files f ON f.id = i.file_id
		JOIN files t ON t.id = i.resolved_file_id
		WHERE f.path = ? ORDER BY t.path`, fromPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetImportTarget records the file an import resolves to.
func (s *Store) SetImportTarget(importID, fileID int64) error {
	_, err := s.db.Exec(`UPDATE imports SET resolved_file_id = ? WHERE id = ?`, fileID, importID)
	return err
}

// StaleFilePaths lists files whose rows were produced by another extraction version. They
// are re-parsed a few at a time, so a large repository makes progress across syncs.
func (s *Store) StaleFilePaths(version, limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM files WHERE extract_ver <> ? ORDER BY path LIMIT ?`, version, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResetExtractVersion marks every file as produced by version v. Tests use it to simulate
// an index written by an older extraction.
func (s *Store) ResetExtractVersion(v int) error {
	_, err := s.db.Exec(`UPDATE files SET extract_ver = ?`, v)
	return err
}

// SetFileStat records the stat fingerprint of a file whose content did not change.
func (s *Store) SetFileStat(path string, size, mtimeNS int64) error {
	_, err := s.db.Exec(`UPDATE files SET size = ?, mtime_ns = ? WHERE path = ?`, size, mtimeNS, path)
	return err
}

// FileIDsWithSuffix returns files whose path equals suffix or ends with "/"+suffix.
func (s *Store) FileIDsWithSuffix(suffix string) ([]int64, error) {
	pattern := "%/" + likeEscape(suffix)
	rows, err := s.db.Query(`SELECT id FROM files WHERE path = ? OR path LIKE ? ESCAPE '\'`, suffix, pattern)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// FileIDByPath returns the id of an indexed file.
func (s *Store) FileIDByPath(path string) (int64, bool, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM files WHERE path = ?`, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// InsertCommits stores commit metadata and the files each commit touched. Commits that
// touch more than maxFiles paths keep their subject but not their file list, so bulk
// commits (formatting, vendoring) never contribute to co-change. Re-inserts are ignored.
func (s *Store) InsertCommits(commits []CommitRecord, maxFiles int) error {
	return s.withWriteTx(func(tx execer) error {
		for _, c := range commits {
			if _, err := tx.Exec(`INSERT OR IGNORE INTO commits(hash, ts, subject, subject_terms, file_count)
				VALUES(?, ?, ?, ?, ?)`, c.Hash, c.Time, c.Subject, c.SubjectTerms, len(c.Files)); err != nil {
				return err
			}
			if len(c.Files) > maxFiles {
				continue
			}
			for _, f := range c.Files {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO commit_files(commit_hash, path, old_path, status)
					VALUES(?, ?, ?, ?)`, c.Hash, f.Path, f.OldPath, f.Status); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// CoChanged returns the paths most often committed together with path, with counts.
func (s *Store) CoChanged(path string, limit int) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT other.path, COUNT(*) AS n
		FROM commit_files mine
		JOIN commit_files other ON other.commit_hash = mine.commit_hash AND other.path <> mine.path
		WHERE mine.path = ?
		GROUP BY other.path
		ORDER BY n DESC, other.path
		LIMIT ?`, path, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			return nil, err
		}
		out[p] = n
	}
	return out, rows.Err()
}

// LastCommitHash returns the newest ingested commit, or "".
func (s *Store) LastCommitHash() (string, error) {
	var h string
	err := s.db.QueryRow(`SELECT hash FROM commits ORDER BY ts DESC, hash LIMIT 1`).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return h, err
}

// migrate brings the schema up to SchemaVersion. The write lock is taken before the
// version is read, so processes opening a new index at the same moment run the migration
// one at a time, and the losers see the finished schema instead of half of it.
func (s *Store) migrate() error {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	var current int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return err
	}
	if current > SchemaVersion {
		return ErrSchemaTooNew
	}
	for v := current; v < SchemaVersion; v++ {
		for _, stmt := range migrations[v] {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migration to v%d: %w", v+1, err)
			}
		}
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	finished = true
	return nil
}

// migrations[v] upgrades a database at version v to v+1.
var migrations = [][]string{
	{ // v0 -> v1
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE files (
			id INTEGER PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			lang TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'code',
			size INTEGER NOT NULL DEFAULT 0,
			mtime_ns INTEGER NOT NULL DEFAULT 0,
			hash TEXT NOT NULL DEFAULT '',
			skipped TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE symbols (
			id INTEGER PRIMARY KEY,
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			parent_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			name TEXT NOT NULL,
			key TEXT NOT NULL DEFAULT '',
			signature TEXT NOT NULL DEFAULT '',
			doc TEXT NOT NULL DEFAULT '',
			value TEXT NOT NULL DEFAULT '',
			line INTEGER NOT NULL,
			end_line INTEGER NOT NULL
		)`,
		`CREATE INDEX symbols_name ON symbols(name)`,
		`CREATE INDEX symbols_key ON symbols(key)`,
		`CREATE INDEX symbols_file ON symbols(file_id)`,
		`CREATE INDEX symbols_parent ON symbols(parent_id)`,
		`CREATE VIRTUAL TABLE symbol_fts USING fts5(
			name_terms, sig_terms, doc_terms, path_terms,
			tokenize = 'unicode61 remove_diacritics 2')`,
		`CREATE VIRTUAL TABLE symbol_vocab USING fts5vocab(symbol_fts, 'col')`,
		`CREATE TABLE refs (
			id INTEGER PRIMARY KEY,
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			from_symbol_id INTEGER REFERENCES symbols(id) ON DELETE CASCADE,
			to_name TEXT NOT NULL,
			kind TEXT NOT NULL,
			line INTEGER NOT NULL
		)`,
		`CREATE INDEX refs_to ON refs(to_name)`,
		`CREATE INDEX refs_file ON refs(file_id)`,
		`CREATE INDEX refs_from ON refs(from_symbol_id)`,
		`CREATE TABLE imports (
			id INTEGER PRIMARY KEY,
			file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			module TEXT NOT NULL,
			resolved_file_id INTEGER REFERENCES files(id) ON DELETE SET NULL
		)`,
		`CREATE INDEX imports_file ON imports(file_id)`,
		`CREATE TABLE dirty_paths (path TEXT PRIMARY KEY)`,
		`CREATE TABLE commits (
			hash TEXT PRIMARY KEY,
			ts INTEGER NOT NULL,
			subject TEXT NOT NULL,
			subject_terms TEXT NOT NULL DEFAULT '',
			file_count INTEGER NOT NULL
		)`,
		`CREATE INDEX commits_ts ON commits(ts)`,
		`CREATE TABLE commit_files (
			commit_hash TEXT NOT NULL REFERENCES commits(hash) ON DELETE CASCADE,
			path TEXT NOT NULL,
			old_path TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			PRIMARY KEY (commit_hash, path)
		)`,
		`CREATE INDEX commit_files_path ON commit_files(path)`,
	},
	{ // v1 -> v2: vocabulary per column, so path words do not inflate symbol frequencies
		`DROP TABLE symbol_vocab`,
		`CREATE VIRTUAL TABLE symbol_vocab USING fts5vocab(symbol_fts, 'col')`,
	},
	{ // v2 -> v3: each file remembers which extraction logic produced its rows
		`ALTER TABLE files ADD COLUMN extract_ver INTEGER NOT NULL DEFAULT 0`,
	},
	{ // v3 -> v4: explicit decisions, written only by `sidecar memory add`
		`CREATE TABLE decisions (
			id INTEGER PRIMARY KEY,
			repo_root TEXT NOT NULL,
			worktree TEXT NOT NULL,
			symbol_or_path TEXT NOT NULL,
			file_path TEXT NOT NULL,
			symbol TEXT NOT NULL DEFAULT '',
			commit_hash TEXT NOT NULL DEFAULT '',
			file_hash TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL,
			origin TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			ttl_seconds INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'valid'
		)`,
		`CREATE INDEX decisions_worktree ON decisions(worktree)`,
	},
}

// KeyOf normalizes an identifier for exact matching: lowercase, underscores removed.
// MAX_ACTIVE_TICKETS and MaxActiveTickets share the key maxactivetickets.
func KeyOf(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}
