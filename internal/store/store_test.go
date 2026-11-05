package store

import (
	"os"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
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
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
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
	s.Close()
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
	defer s.Close()
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
