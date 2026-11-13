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
		ORDER BY f.path, s.line, s.name, s.kind, s.id LIMIT ?`, args...)
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
		WHERE s.parent_id = ? ORDER BY f.path, s.line, s.name, s.kind, s.id LIMIT ?`, id, limit)
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
	rows, err := s.queryCached(`SELECT `+symbolColumns("t")+`, CASE
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
		ORDER BY (CASE WHEN f.kind = 'test' THEN 1 ELSE 0 END),
			(CASE WHEN t.file_id = src.file_id THEN 0 WHEN EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id) THEN 1 ELSE 2 END),
			f.path, t.line, t.name, t.kind, t.id
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
	rows, err := s.queryCached(`SELECT `+symbolColumns("src")+`, CASE
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
		ORDER BY (CASE WHEN f.kind = 'test' THEN 1 ELSE 0 END),
			(CASE WHEN src.file_id = t.file_id THEN 0 WHEN EXISTS (SELECT 1 FROM imports i WHERE i.file_id = src.file_id AND i.resolved_file_id = t.file_id) THEN 1 ELSE 2 END),
			f.path, src.line, src.name, src.kind, src.id
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
	rows, err := s.queryCached(`SELECT DISTINCT `+symbolColumns("s")+`
		FROM refs r
		JOIN files f ON f.id = r.file_id AND f.kind = 'test'
		JOIN symbols s ON s.file_id = f.id AND s.kind = 'file'
		WHERE r.to_name = ? ORDER BY f.path, s.line, s.name, s.kind, s.id LIMIT ?`, name, limit)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

// SubtypesOf returns non-test type/class/struct definitions that reference baseName as a parent or interface.
func (s *Store) SubtypesOf(baseName string, limit int) ([]SymbolHit, error) {
	if baseName == "" || limit <= 0 {
		return nil, nil
	}
	rows, err := s.queryCached(`SELECT DISTINCT `+symbolColumns("t")+`
		FROM refs r
		JOIN symbols t ON t.id = r.from_symbol_id
		JOIN files f ON f.id = t.file_id AND f.kind <> 'test'
		WHERE r.kind = 'type' AND r.to_name = ? AND t.kind IN ('class', 'struct', 'type', 'interface')
		ORDER BY f.path, t.line, t.name, t.id LIMIT ?`, baseName, limit)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

// SubtypeMethods resolves concrete or abstract overrides of method (up to two inheritance hops)
// using indexed type references in non-test files.
func (s *Store) SubtypeMethods(method SymbolHit, limit int) ([]Related, error) {
	if method.Name == "" || method.FileID <= 0 || limit <= 0 {
		return nil, nil
	}
	rows, err := s.queryCached(`SELECT DISTINCT `+symbolColumns("m")+`
		FROM symbols base
		JOIN refs r1 ON r1.kind = 'type' AND r1.to_name = base.name
		JOIN symbols sub1 ON sub1.id = r1.from_symbol_id
		LEFT JOIN refs r2 ON r2.kind = 'type' AND r2.to_name = sub1.name
		LEFT JOIN symbols sub2 ON sub2.id = r2.from_symbol_id
		JOIN symbols m ON m.name = ? AND m.kind IN ('method', 'function')
			AND ((m.file_id = sub1.file_id AND m.line BETWEEN sub1.line AND CASE WHEN sub1.end_line > sub1.line THEN sub1.end_line ELSE sub1.line + 100000 END)
			  OR (sub2.id IS NOT NULL AND m.file_id = sub2.file_id AND m.line BETWEEN sub2.line AND CASE WHEN sub2.end_line > sub2.line THEN sub2.end_line ELSE sub2.line + 100000 END))
		JOIN files f ON f.id = m.file_id AND f.kind <> 'test'
		WHERE base.file_id = ? AND base.kind IN ('interface', 'class', 'struct', 'type')
		  AND base.line <= ? AND (base.end_line >= ? OR base.end_line <= base.line)
		  AND m.id <> ?
		ORDER BY CASE WHEN f.path LIKE '%Abstract%' THEN 0 ELSE 1 END, f.path, m.line, m.id
		LIMIT ?`, method.Name, method.FileID, method.Line, method.Line, method.ID, limit)
	if err != nil {
		return nil, err
	}
	hits, err := collectHits(rows)
	if err != nil {
		return nil, err
	}
	return toRelated(hits, "implements", "import"), nil
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
		FROM symbols s JOIN files f ON f.id = s.file_id WHERE s.kind <> 'file'
		ORDER BY f.path, s.line, s.name, s.kind, s.id`)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

// DocPaths lists indexed documentation files (Markdown, README, CHANGELOG), sorted by path.
func (s *Store) DocPaths(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM files WHERE kind = 'doc' ORDER BY path LIMIT ?`, limit)
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

// TestPaths lists indexed test files, sorted by path.
func (s *Store) TestPaths(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT path FROM files WHERE kind = 'test' ORDER BY path LIMIT ?`, limit)
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

// DocstringOwners returns definitions whose extracted doc comment contains name verbatim.
func (s *Store) DocstringOwners(name string, limit int) ([]SymbolHit, error) {
	rows, err := s.queryCached(`SELECT `+symbolColumns("s")+`
		FROM symbols s JOIN files f ON f.id = s.file_id
		WHERE s.doc <> '' AND instr(s.doc, ?) > 0 ORDER BY s.id LIMIT ?`, name, limit)
	if err != nil {
		return nil, err
	}
	return collectHits(rows)
}

// SymbolsInFile returns the definitions of one file, excluding the file and section rows, in source order.
func (s *Store) SymbolsInFile(path string, limit int) ([]SymbolHit, error) {
	rows, err := s.db.Query(`SELECT `+symbolColumns("sym")+` FROM symbols sym JOIN files f ON f.id = sym.file_id
		WHERE f.path = ? AND sym.kind NOT IN ('file', 'section')
		ORDER BY sym.line, sym.id LIMIT ?`, path, limit)
	if err != nil {
		return nil, err
	}
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

// ImportersOf returns the paths of files whose imports resolve to path. An import that did not resolve
// to a file in the repository is not listed: absence here is not proof that nobody imports the file.
func (s *Store) ImportersOf(path string, limit int) ([]string, error) {
	rows, err := s.queryCached(`SELECT DISTINCT f.path FROM imports i
		JOIN files f ON f.id = i.file_id JOIN files t ON t.id = i.resolved_file_id
		WHERE t.path = ? AND f.id <> t.id ORDER BY f.path LIMIT ?`, path, limit)
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
