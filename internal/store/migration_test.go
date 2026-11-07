package store

import (
	"path/filepath"
	"testing"
)

// seedIndex writes one file, one symbol and one meta value into a fresh index at path.
func seedIndex(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()
	if err := s.ReplaceFile(FileRecord{Path: "src/queue.go", Lang: "go", Kind: "code", Hash: "h1"},
		[]SymbolRecord{{Kind: "function", Name: "ManualRelease", Line: 3, EndLine: 5, Parent: -1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("indexed_head", "abc123"); err != nil {
		t.Fatal(err)
	}
}

// requireSeedIntact checks the rows seedIndex wrote and that the database is consistent.
func requireSeedIntact(t *testing.T, s *Store) {
	t.Helper()
	if n, err := s.SymbolCount(); err != nil || n != 1 {
		t.Fatalf("symbols after reopen = %d (%v), want 1", n, err)
	}
	if got, err := s.GetMeta("indexed_head"); err != nil || got != "abc123" {
		t.Fatalf("meta after reopen = %q (%v), want abc123", got, err)
	}
	if hits, err := s.AllSymbols(); err != nil || len(hits) != 1 || hits[0].Name != "ManualRelease" {
		t.Fatalf("AllSymbols after reopen = %+v (%v)", hits, err)
	}
	if check, err := s.IntegrityCheck(); err != nil || check != "ok" {
		t.Fatalf("integrity check = %q (%v), want ok", check, err)
	}
}

// A current database reopens unchanged, and its rows survive the reopen.
func TestReopenCurrentDatabaseKeepsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	seedIndex(t, path)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s.Close() }()
	if v, _ := s.SchemaVersion(); v != SchemaVersion {
		t.Fatalf("version after reopen = %d, want %d", v, SchemaVersion)
	}
	requireSeedIntact(t, s)
}

// An index written by an older build is migrated in place, and its rows survive the upgrade.
func TestOlderDatabaseMigratesCleanlyAndKeepsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	seedIndex(t, path)

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// The layout the first release wrote, as in TestMigrationUpgradesVocabularyFromV1.
	for _, stmt := range []string{
		`ALTER TABLE files DROP COLUMN extract_ver`,
		`DROP TABLE decisions`,
		`DROP TABLE symbol_vocab`,
		`CREATE VIRTUAL TABLE symbol_vocab USING fts5vocab(symbol_fts, 'row')`,
		`PRAGMA user_version = 1`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("migration from v1 failed: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if v, _ := s2.SchemaVersion(); v != SchemaVersion {
		t.Fatalf("version after migration = %d, want %d", v, SchemaVersion)
	}
	requireSeedIntact(t, s2)
}
