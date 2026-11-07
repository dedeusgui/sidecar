package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openDecisionStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func indexFile(t *testing.T, s *Store, path, hash string, syms ...SymbolRecord) {
	t.Helper()
	if err := s.ReplaceFile(FileRecord{Path: path, Lang: "go", Kind: "code", Hash: hash}, syms, nil, nil); err != nil {
		t.Fatal(err)
	}
}

var releaseSymbol = SymbolRecord{Kind: "func", Name: "Release", Line: 1, EndLine: 3, Parent: -1}

func diskAt(hash string, exists bool) func(string) (string, bool) {
	return func(string) (string, bool) { return hash, exists }
}

func decisionFor(worktree, file, symbol, hash string, created time.Time, ttl time.Duration) DecisionRecord {
	return DecisionRecord{
		RepoRoot: "/repo", Worktree: worktree, SymbolOrPath: symbol, FilePath: file, Symbol: symbol,
		FileHash: hash, Summary: "usar fila única", Origin: "manual",
		CreatedAt: created.Unix(), TTLSeconds: int64(ttl.Seconds()),
	}
}

func statusOf(t *testing.T, s *Store, worktree string, now time.Time, cur func(string) (string, bool)) string {
	t.Helper()
	list, err := s.RefreshDecisions(worktree, now, cur)
	if err != nil || len(list) != 1 {
		t.Fatalf("RefreshDecisions = %d records, %v", len(list), err)
	}
	return list[0].Status
}

func TestDecisionStatusFollowsFileHashAndSymbol(t *testing.T) {
	s := openDecisionStore(t)
	now := time.Now()
	indexFile(t, s, "release/handler.go", "h1", releaseSymbol)
	if _, err := s.InsertDecision(decisionFor("/wt", "release/handler.go", "Release", "h1", now, time.Hour)); err != nil {
		t.Fatal(err)
	}

	if got := statusOf(t, s, "/wt", now, diskAt("h1", true)); got != DecisionValid {
		t.Fatalf("unchanged file: status %q, want valid", got)
	}
	if got := statusOf(t, s, "/wt", now, diskAt("h2", true)); got != DecisionStale {
		t.Fatalf("changed file: status %q, want stale", got)
	}

	indexFile(t, s, "release/handler.go", "h2") // the symbol was removed from the index
	if got := statusOf(t, s, "/wt", now, diskAt("h2", true)); got != DecisionInvalidated {
		t.Fatalf("removed symbol: status %q, want invalidated", got)
	}
}

func TestDecisionInvalidatedWhenFileIsMissingOrTTLExpires(t *testing.T) {
	s := openDecisionStore(t)
	now := time.Now()
	indexFile(t, s, "release/handler.go", "h1", releaseSymbol)
	if _, err := s.InsertDecision(decisionFor("/wt", "release/handler.go", "Release", "h1", now, time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, s, "/wt", now, diskAt("", false)); got != DecisionInvalidated {
		t.Fatalf("deleted file: status %q, want invalidated", got)
	}
	if got := statusOf(t, s, "/wt", now.Add(2*time.Hour), diskAt("h1", true)); got != DecisionInvalidated {
		t.Fatalf("expired TTL: status %q, want invalidated", got)
	}
}

func TestDecisionsAreIsolatedByWorktreeAndPurgeOnlyInvalidated(t *testing.T) {
	s := openDecisionStore(t)
	now := time.Now()
	indexFile(t, s, "release/handler.go", "h1", releaseSymbol)
	for _, wt := range []string{"/wt/a", "/wt/b"} {
		if _, err := s.InsertDecision(decisionFor(wt, "release/handler.go", "Release", "h1", now, time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	listA, err := s.ListDecisions("/wt/a")
	if err != nil || len(listA) != 1 || listA[0].Worktree != "/wt/a" {
		t.Fatalf("worktree A sees %+v, %v", listA, err)
	}

	// Only worktree A's file disappears; B's decision must survive A's refresh and purge.
	if _, err := s.RefreshDecisions("/wt/a", now, diskAt("", false)); err != nil {
		t.Fatal(err)
	}
	n, err := s.PurgeDecisions("/wt/a", false)
	if err != nil || n != 1 {
		t.Fatalf("purge invalidated in A = %d, %v; want 1", n, err)
	}
	if listB, _ := s.ListDecisions("/wt/b"); len(listB) != 1 {
		t.Fatalf("purge in A removed B's decision: %+v", listB)
	}
	if n, _ := s.PurgeDecisions("/wt/b", true); n != 1 {
		t.Fatalf("purge all in B = %d, want 1", n)
	}
}
