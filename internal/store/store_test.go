package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestOpenCreatesCurrentSchema(t *testing.T) {
	s, _ := openTemp(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != SchemaVersion {
		t.Fatalf("schema version = %d, want %d", v, SchemaVersion)
	}
}

func TestReopenKeepsData(t *testing.T) {
	s, path := openTemp(t)
	if err := s.SetMeta("indexed_head", "abc123"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()
	got, err := s2.GetMeta("indexed_head")
	if err != nil || got != "abc123" {
		t.Fatalf("GetMeta = %q, %v; want abc123", got, err)
	}
}

func TestNewerSchemaIsRejected(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.db.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	_ = s.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("Open accepted a schema newer than the binary understands")
	}
}

func TestReplaceFileDropsStaleSymbolsAndSearchRows(t *testing.T) {
	s, _ := openTemp(t)
	first := FileRecord{Path: "src/queue.go", Lang: "go", Kind: "code", Hash: "h1", Size: 10}
	err := s.ReplaceFile(first, []SymbolRecord{
		{Kind: "function", Name: "ManualRelease", NameTerms: "manual release", Signature: "func ManualRelease()", Line: 3, EndLine: 5, Parent: -1},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ReplaceFile v1: %v", err)
	}
	hits, err := s.SearchSymbols("manual release", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("expected 1 hit before update, got %d (%v)", len(hits), err)
	}

	second := first
	second.Hash = "h2"
	err = s.ReplaceFile(second, []SymbolRecord{
		{Kind: "function", Name: "AutomaticDispatch", NameTerms: "automatic dispatch", Line: 3, EndLine: 5, Parent: -1},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ReplaceFile v2: %v", err)
	}
	if hits, _ := s.SearchSymbols("manual release", 10); len(hits) != 0 {
		t.Fatalf("stale symbol still searchable after re-index: %+v", hits)
	}
	if hits, _ := s.SearchSymbols("automatic dispatch", 10); len(hits) != 1 {
		t.Fatalf("new symbol not searchable, got %d hits", len(hits))
	}
}

func TestCorruptIndexIsMovedAsideAndRebuilt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database at all, sorry"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, recovered, err := OpenOrRecover(path)
	if err != nil {
		t.Fatalf("OpenOrRecover: %v", err)
	}
	defer func() { _ = s.Close() }()
	if !recovered {
		t.Fatal("expected recovered=true for a corrupt file")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt file was not preserved aside: %v", err)
	}
	if v, err := s.SchemaVersion(); err != nil || v != SchemaVersion {
		t.Fatalf("rebuilt schema version = %d, %v", v, err)
	}
}

// A busy or missing database must not be quarantined: that would discard a healthy index.
func TestIsCorruptOnlyMatchesDamagedFiles(t *testing.T) {
	for msg, want := range map[string]bool{
		"database disk image is malformed (11)":               true,
		"file is not a database (26)":                         true,
		"SQL logic error: no such fts5 table: symbol_fts (1)": true,
		"database is locked (5)":                              false,
		"unable to open database file (14)":                   false,
		"disk I/O error":                                      false,
	} {
		if got := IsCorrupt(errors.New(msg)); got != want {
			t.Errorf("IsCorrupt(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestParentMayBeDeclaredAfterChild(t *testing.T) {
	s, _ := openTemp(t)
	// Go declares methods before their type in some files: index 0 is the method, 1 the type.
	err := s.ReplaceFile(FileRecord{Path: "h.go", Lang: "go", Kind: "code", Hash: "x"}, []SymbolRecord{
		{Kind: "method", Name: "Release", Parent: 1, Line: 3, EndLine: 4},
		{Kind: "struct", Name: "Handler", Parent: -1, Line: 1, EndLine: 2},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ReplaceFile: %v", err)
	}
	var parent string
	err = s.db.QueryRow(`SELECT p.name FROM symbols c JOIN symbols p ON p.id = c.parent_id WHERE c.name = 'Release'`).Scan(&parent)
	if err != nil || parent != "Handler" {
		t.Fatalf("parent of Release = %q (%v), want Handler", parent, err)
	}
}

func TestDirtyPathsRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.SetDirtyPaths([]string{"b.go", "a.go"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.DirtyPaths()
	if err != nil || len(got) != 2 || got[0] != "a.go" {
		t.Fatalf("DirtyPaths = %v, %v", got, err)
	}
	if err := s.SetDirtyPaths(nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.DirtyPaths(); len(got) != 0 {
		t.Fatalf("dirty set not cleared: %v", got)
	}
}

func TestTermDocumentFrequencyFromSearchVocabulary(t *testing.T) {
	s, _ := openTemp(t)
	for i, name := range []string{"ManualRelease", "ReleaseQueue"} {
		err := s.ReplaceFile(FileRecord{Path: "f" + string(rune('a'+i)) + ".go", Lang: "go", Kind: "code", Hash: "h"},
			[]SymbolRecord{{Kind: "function", Name: name, NameTerms: "release " + name, Parent: -1}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	df, err := s.TermDocFreq("release")
	if err != nil || df != 2 {
		t.Fatalf("TermDocFreq(release) = %d, %v; want 2", df, err)
	}
	if df, _ := s.TermDocFreq("absent"); df != 0 {
		t.Fatalf("absent term df = %d, want 0", df)
	}
}

func TestImportTargetsAreResolvedOnce(t *testing.T) {
	s, _ := openTemp(t)
	err := s.ReplaceFile(FileRecord{Path: "web/app.ts", Lang: "typescript", Kind: "code", Hash: "a"}, nil, nil,
		[]ImportRecord{{Module: "./billing/invoice"}})
	if err != nil {
		t.Fatal(err)
	}
	err = s.ReplaceFile(FileRecord{Path: "web/billing/invoice.ts", Lang: "typescript", Kind: "code", Hash: "b"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingImports()
	if err != nil || len(pending) != 1 || pending[0].Module != "./billing/invoice" {
		t.Fatalf("PendingImports = %+v, %v", pending, err)
	}
	target, ok, err := s.FileIDByPath("web/billing/invoice.ts")
	if err != nil || !ok {
		t.Fatalf("FileIDByPath: %v %v", ok, err)
	}
	if err := s.SetImportTarget(pending[0].ID, target); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.PendingImports(); len(left) != 0 {
		t.Fatalf("import still pending after resolution: %+v", left)
	}
}

func TestCoChangeIgnoresMassiveCommits(t *testing.T) {
	s, _ := openTemp(t)
	commits := []CommitRecord{
		{Hash: "c1", Time: 1, Subject: "feat queue", Files: []CommitFile{{Status: "M", Path: "queue.go"}, {Status: "M", Path: "queue_test.go"}}},
		{Hash: "c2", Time: 2, Subject: "fix queue", Files: []CommitFile{{Status: "M", Path: "queue.go"}, {Status: "M", Path: "queue_test.go"}}},
		{Hash: "c3", Time: 3, Subject: "reformat everything", Files: manyFiles(200)},
	}
	if err := s.InsertCommits(commits, 50); err != nil {
		t.Fatalf("InsertCommits: %v", err)
	}
	got, err := s.CoChanged("queue.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got["queue_test.go"] != 2 {
		t.Fatalf("co-change count = %v, want queue_test.go:2 and no noise from the massive commit", got)
	}
	if len(got) != 1 {
		t.Fatalf("massive commit leaked into co-change: %v", got)
	}
	// Re-inserting the same commits must not duplicate history.
	if err := s.InsertCommits(commits[:2], 50); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CoChanged("queue.go", 10); got["queue_test.go"] != 2 {
		t.Fatalf("duplicate insert changed counts: %v", got)
	}
}

func manyFiles(n int) []CommitFile {
	var out []CommitFile
	for i := 0; i < n; i++ {
		out = append(out, CommitFile{Status: "M", Path: "gen/file" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".go"})
	}
	return out
}

func TestFileIDsWithSuffixMatchesOnPathSegments(t *testing.T) {
	s, _ := openTemp(t)
	for _, p := range []string{"app/billing/Invoice.java", "legacy/xbilling/Invoice.java", "Invoice.java"} {
		if err := s.ReplaceFile(FileRecord{Path: p, Lang: "java", Kind: "code", Hash: p}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := s.FileIDsWithSuffix("billing/Invoice.java")
	if err != nil || len(ids) != 1 {
		t.Fatalf("suffix billing/Invoice.java matched %v (%v); want exactly the segment-aligned file", ids, err)
	}
}

func TestPathDuplicationDoesNotInflateSymbolFrequency(t *testing.T) {
	s, _ := openTemp(t)
	// Every symbol of owner/ carries the path word "owner"; only one symbol names it.
	for i := 0; i < 50; i++ {
		err := s.ReplaceFile(FileRecord{Path: "owner/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".java",
			Lang: "java", Kind: "code", Hash: "h", PathTerms: "owner"},
			[]SymbolRecord{{Kind: "method", Name: "m", NameTerms: "m", Parent: -1}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReplaceFile(FileRecord{Path: "owner/OwnerController.java", Lang: "java", Kind: "code", Hash: "z", PathTerms: "owner"},
		[]SymbolRecord{{Kind: "class", Name: "OwnerController", NameTerms: "owner controller", Parent: -1}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	sym, err := s.TermSymbolFreq("owner")
	if err != nil || sym != 1 {
		t.Fatalf("TermSymbolFreq(owner) = %d, %v; want 1 (path copies must not count)", sym, err)
	}
	present, _ := s.TermDocFreq("owner")
	if present == 0 {
		t.Fatal("path term must still count as present for file matching")
	}
}

func TestMigrationUpgradesVocabularyFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the layout written by the first release: row-level vocabulary, version 1.
	for _, stmt := range []string{
		`ALTER TABLE files DROP COLUMN extract_ver`,
		`DROP TABLE decisions`,
		`DROP TABLE IF EXISTS evidence_feedback`,
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
		t.Fatalf("upgrade from v1 failed: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if v, _ := s2.SchemaVersion(); v != SchemaVersion {
		t.Fatalf("version after upgrade = %d, want %d", v, SchemaVersion)
	}
	if _, err := s2.TermSymbolFreq("anything"); err != nil {
		t.Fatalf("column-level vocabulary unavailable after upgrade: %v", err)
	}
}

func TestConcurrentFirstOpenMigratesExactlyOnce(t *testing.T) {
	// Several hook processes can meet a brand-new index at the same moment. Each must end
	// up with a usable, fully migrated database; none may fail on a half-applied migration.
	for round := 0; round < 5; round++ {
		path := filepath.Join(t.TempDir(), "race.db")
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s, err := Open(path)
				if err != nil {
					errs <- err
					return
				}
				defer func() { _ = s.Close() }()
				if _, err := s.SearchSymbols("probe", 1); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: concurrent first open failed: %v", round, err)
		}
	}
}

func TestWritesDoNotFailWhenAnotherWriterHoldsTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	// Another process is in the middle of a write transaction.
	conn, err := a.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- b.ReplaceFile(FileRecord{Path: "x.go", Lang: "go", Kind: "code", Hash: "h"},
			[]SymbolRecord{{Kind: "function", Name: "X", Parent: -1}}, nil, nil)
	}()
	time.Sleep(150 * time.Millisecond)
	if _, err := conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("write failed instead of waiting for the lock: %v", err)
	}
}

func TestStaleExtractionVersionsAreReportedPerFile(t *testing.T) {
	s, _ := openTemp(t)
	for _, p := range []string{"a.go", "b.go"} {
		if err := s.ReplaceFile(FileRecord{Path: p, Lang: "go", Kind: "code", Hash: p, ExtractVer: 1}, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ReplaceFile(FileRecord{Path: "c.go", Lang: "go", Kind: "code", Hash: "c", ExtractVer: 2}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	stale, err := s.StaleFilePaths(2, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 2 || stale[0] != "a.go" || stale[1] != "b.go" {
		t.Fatalf("StaleFilePaths(2) = %v, want [a.go b.go]", stale)
	}
	recs, _ := s.FileRecords()
	if recs["c.go"].ExtractVer != 2 {
		t.Fatalf("extract version not stored: %+v", recs["c.go"])
	}
}
