package store

import "testing"

// Cached statements must observe later writes and tolerate Close.
func TestCachedQueriesSeeWrites(t *testing.T) {
	s, _ := openTemp(t)
	if _, ok, err := s.FileRecordOf("a.go"); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	if err := s.ReplaceFile(FileRecord{Path: "a.go", Lang: "go", Kind: "code", Hash: "h1"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	rec, ok, err := s.FileRecordOf("a.go")
	if err != nil || !ok || rec.Hash != "h1" {
		t.Fatalf("after write: %+v ok=%v err=%v", rec, ok, err)
	}
	if err := s.ReplaceFile(FileRecord{Path: "a.go", Lang: "go", Kind: "code", Hash: "h2"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if rec, _, _ = s.FileRecordOf("a.go"); rec.Hash != "h2" {
		t.Fatalf("stale cached statement: %+v", rec)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
