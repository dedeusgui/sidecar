package store

import (
	"database/sql"
	"errors"
	"strings"
)

var errNoRows = sql.ErrNoRows

// Related is a symbol reached from another symbol, with the kind of relation that links them.
// Relation is one of: member, member_of, calls, called_by, tested_by.
// For calls and called_by the Basis says how the link was established:
// import (the target file is imported and defines the name), same_file (nominal),
// or unique_name (nominal, the name is defined exactly once in the repository).
type Related struct {
	SymbolHit
	Relation string
	Basis    string
}

// symbolColumns lists the columns scanHit expects, for a symbol aliased as sym and its file as f.
func symbolColumns(sym string) string {
	return sym + `.id, ` + sym + `.file_id, f.path, ` + sym + `.kind, ` + sym + `.name, ` + sym + `.signature, ` +
		sym + `.line, ` + sym + `.end_line, ` + sym + `.doc, ` + sym + `.value, f.hash`
}

func scanHit(scan func(dest ...any) error) (SymbolHit, error) {
	var h SymbolHit
	err := scan(&h.ID, &h.FileID, &h.Path, &h.Kind, &h.Name, &h.Signature, &h.Line, &h.EndLine, &h.Doc, &h.Value, &h.FileHash)
	return h, err
}

// ExactSymbols returns definitions whose normalized key equals one of keys.
func (s *Store) ExactSymbols(keys []string, limit int) ([]SymbolHit, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(keys)+1)
	marks := make([]string, 0, len(keys))
	for _, k := range keys {
		marks = append(marks, "?")
		args = append(args, k)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT `+symbolColumns("s")+`
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE s.key IN (`+strings.Join(marks, ",")+`) AND s.kind NOT IN ('file', 'section')
		ORDER BY f.path, s.line LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

// SymbolCount returns how many symbols are indexed.
func (s *Store) SymbolCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&n)
	return n, err
}

// FileSymbol returns the per-file row for a path.
func (s *Store) FileSymbol(path string) (SymbolHit, bool, error) {
	row := s.db.QueryRow(`SELECT `+symbolColumns("s")+`
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE f.path = ? AND s.kind = 'file' LIMIT 1`, path)
	h, err := scanHit(row.Scan)
	if errors.Is(err, errNoRows) {
		return SymbolHit{}, false, nil
	}
	return h, err == nil, err
}

// MembersOf returns symbols declared directly inside a symbol (methods of a class, fields).
func (s *Store) MembersOf(id int64, limit int) ([]Related, error) {
	rows, err := s.db.Query(`SELECT `+symbolColumns("s")+`
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE s.parent_id = ? ORDER BY s.line LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	hits, err := collectHits(rows)
	return toRelated(hits, "member", "same_file"), err
}

// ParentOf returns the symbol that directly contains a symbol.
func (s *Store) ParentOf(id int64) (SymbolHit, bool, error) {
	row := s.db.QueryRow(`SELECT `+symbolColumns("s")+`
		FROM symbols s JOIN files f ON f.id = s.file_id
		JOIN symbols c ON c.parent_id = s.id WHERE c.id = ? LIMIT 1`, id)
	h, err := scanHit(row.Scan)
	if errors.Is(err, errNoRows) {
		return SymbolHit{}, false, nil
	}
	return h, err == nil, err
}

// CalleesOf returns definitions that a symbol references by name. A callee in the same
// file or in a file the caller imports is linked; a name defined only once in the whole
// repository is linked with the weaker unique_name basis. Other names are not linked.
func (s *Store) CalleesOf(id int64, limit int) ([]Related, error) {
	rows, err := s.db.Query(`SELECT `+symbolColumns("t")+`, CASE
			WHEN t.file_id = src.file_id THEN 'same_file'
			WHEN EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id) THEN 'import'
			ELSE 'unique_name' END AS basis
		FROM refs r
		JOIN symbols src ON src.id = r.from_symbol_id
		JOIN symbols t ON t.name = r.to_name AND t.kind NOT IN ('file', 'section', 'route', 'constant')
		JOIN files f ON f.id = t.file_id
		WHERE r.from_symbol_id = ? AND r.kind = 'call' AND t.id <> src.id AND (
			t.file_id = src.file_id
			OR EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id)
			OR (SELECT COUNT(*) FROM symbols x WHERE x.name = r.to_name AND x.kind NOT IN ('file', 'section', 'route')) = 1)
		GROUP BY t.id
		ORDER BY basis, t.file_id, t.line
		LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Related
	for rows.Next() {
		var basis string
		h, err := scanHit(func(dest ...any) error {
			return rows.Scan(append(dest, &basis)...)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Related{SymbolHit: h, Relation: "calls", Basis: basis})
	}
	return out, rows.Err()
}

// CallersOf returns symbols that call the given symbol's name, under the same linking rules.
func (s *Store) CallersOf(id int64, limit int) ([]Related, error) {
	rows, err := s.db.Query(`SELECT `+symbolColumns("src")+`, CASE
			WHEN src.file_id = t.file_id THEN 'same_file'
			WHEN EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id) THEN 'import'
			ELSE 'unique_name' END AS basis
		FROM symbols t
		JOIN refs r ON r.to_name = t.name AND r.kind = 'call'
		JOIN symbols src ON src.id = r.from_symbol_id
		JOIN files f ON f.id = src.file_id
		WHERE t.id = ? AND src.id <> t.id AND (
			src.file_id = t.file_id
			OR EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id)
			OR (SELECT COUNT(*) FROM symbols x WHERE x.name = t.name AND x.kind NOT IN ('file', 'section', 'route')) = 1)
		GROUP BY src.id
		ORDER BY basis, src.file_id, src.line
		LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Related
	for rows.Next() {
		var basis string
		h, err := scanHit(func(dest ...any) error {
			return rows.Scan(append(dest, &basis)...)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, Related{SymbolHit: h, Relation: "called_by", Basis: basis})
	}
	return out, rows.Err()
}

// TestFilesReferencing returns the file rows of test files that mention a name.
func (s *Store) TestFilesReferencing(name string, limit int) ([]SymbolHit, error) {
	rows, err := s.db.Query(`SELECT DISTINCT `+symbolColumns("s")+`
		FROM refs r
		JOIN files f ON f.id = r.file_id AND f.kind = 'test'
		JOIN symbols s ON s.file_id = f.id AND s.kind = 'file'
		WHERE r.to_name = ? ORDER BY f.path LIMIT ?`, name, limit)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

func collectHits(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}) ([]SymbolHit, error) {
	defer func() { _ = rows.Close() }()
	var out []SymbolHit
	for rows.Next() {
		h, err := scanHit(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func toRelated(hits []SymbolHit, relation, basis string) []Related {
	out := make([]Related, 0, len(hits))
	for _, h := range hits {
		out = append(out, Related{SymbolHit: h, Relation: relation, Basis: basis})
	}
	return out
}

// CommitCount returns how many commits are ingested into history.
func (s *Store) CommitCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM commits`).Scan(&n)
	return n, err
}

// IntegrityCheck runs SQLite's quick check; "ok" means the database is readable and consistent.
func (s *Store) IntegrityCheck() (string, error) {
	var out string
	err := s.db.QueryRow(`PRAGMA quick_check`).Scan(&out)
	return out, err
}

// AllSymbols returns every indexed definition except per-file rows, for baselines and tests.
func (s *Store) AllSymbols() ([]SymbolHit, error) {
	rows, err := s.db.Query(`SELECT ` + symbolColumns("s") + `
		FROM symbols s JOIN files f ON f.id = s.file_id WHERE s.kind <> 'file' ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}
