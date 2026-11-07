package store

import (
	"math"
	"path/filepath"
	"testing"
)

func TestParseEvidenceID(t *testing.T) {
	cases := []struct{ id, path, symbol string }{
		{"internal/retrieve/retrieve.go#function:Query@180", "internal/retrieve/retrieve.go", "Query"},
		{"path/to/file.go#Symbol", "path/to/file.go", "Symbol"},
		{"path/to/file.go", "path/to/file.go", ""},
	}
	for _, c := range cases {
		path, symbol := ParseEvidenceID(c.id)
		if path != c.path || symbol != c.symbol {
			t.Errorf("ParseEvidenceID(%q) = (%q, %q), want (%q, %q)", c.id, path, symbol, c.path, c.symbol)
		}
	}
}

func TestComputeReconciledWeightSignals(t *testing.T) {
	// Helpful, read by the agent, task passed: the strongest positive case.
	if w := ComputeReconciledWeight(1, "high", 1, true, false); w <= 0.18 {
		t.Fatalf("corroborated helpful weight = %v, want > 0.18", w)
	}
	// Helpful but the trace says the file was never read: praise is discounted, not removed.
	discounted := ComputeReconciledWeight(1, "high", -1, true, false)
	if want := 0.08125; math.Abs(discounted-want) > 1e-9 {
		t.Fatalf("contradicted helpful weight = %v, want %v", discounted, want)
	}
	if corroborated := ComputeReconciledWeight(1, "high", 1, true, false); discounted >= corroborated {
		t.Fatalf("contradicted weight %v not below corroborated %v", discounted, corroborated)
	}
	// Misleading, and the agent needed another search: a clear negative.
	if w := ComputeReconciledWeight(-1, "medium", 0, true, true); w >= -0.15 {
		t.Fatalf("misleading weight = %v, want < -0.15", w)
	}
	// Every combination stays inside the bound.
	for _, pol := range []int{1, -1} {
		for _, conf := range []string{"high", "medium", "low", "other"} {
			for _, tr := range []int{-1, 0, 1} {
				for _, ok := range []bool{false, true} {
					for _, more := range []bool{false, true} {
						w := ComputeReconciledWeight(pol, conf, tr, ok, more)
						if w < -feedbackMaxDelta || w > feedbackMaxDelta {
							t.Fatalf("weight %v out of bounds for %v", w, []any{pol, conf, tr, ok, more})
						}
					}
				}
			}
		}
	}
}

func TestFeedbackLifecycleAndInvalidation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	const wt = "/repo"
	for _, f := range []FileRecord{{Path: "a.go", Hash: "h1"}, {Path: "b.go", Hash: "h2"}, {Path: "c.go", Hash: "h3"}} {
		if err := s.ReplaceFile(f, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(rec FeedbackRecord) {
		t.Helper()
		if _, err := s.InsertFeedback(rec); err != nil {
			t.Fatal(err)
		}
	}
	insert(FeedbackRecord{Worktree: wt, EvidenceID: "a.go#function:Alpha@1", FilePath: "a.go", Symbol: "Alpha", FileHash: "h1", Polarity: -1, ReconciledWeight: -0.2, CreatedAt: 1})
	insert(FeedbackRecord{Worktree: wt, EvidenceID: "b.go#Beta", FilePath: "b.go", Symbol: "Beta", FileHash: "h2", Polarity: 1, ReconciledWeight: 0.2, CreatedAt: 1})
	// Same file and no symbol: the file key. Older +0.2 and newer -0.1 on one target decay to +0.07.
	insert(FeedbackRecord{Worktree: wt, EvidenceID: "c.go", FilePath: "c.go", FileHash: "", Polarity: 1, ReconciledWeight: 0.2, CreatedAt: 1})
	insert(FeedbackRecord{Worktree: wt, EvidenceID: "c.go", FilePath: "c.go", FileHash: "h3", Polarity: -1, ReconciledWeight: -0.1, CreatedAt: 2})

	list, err := s.ListFeedback(wt)
	if err != nil || len(list) != 4 {
		t.Fatalf("ListFeedback = %d records (%v), want 4", len(list), err)
	}
	for _, r := range list {
		if r.Status != FeedbackValid {
			t.Fatalf("record %d status = %q, want valid", r.ID, r.Status)
		}
	}

	check := func(label string, mults map[string]float64, key string, want float64) {
		t.Helper()
		if got, ok := mults[key]; !ok || math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s: multiplier[%s] = %v (present %v), want %v", label, key, got, ok, want)
		}
	}
	mults, err := s.FeedbackMultipliers(wt)
	if err != nil {
		t.Fatal(err)
	}
	check("initial", mults, "a.go#Alpha", 0.8)
	check("initial", mults, "b.go#Beta", 1.2)
	check("initial", mults, "c.go", 1.07)
	if other, _ := s.FeedbackMultipliers("/elsewhere"); len(other) != 0 {
		t.Fatalf("feedback leaked into another worktree: %v", other)
	}

	// Changing a.go and re-indexing it with a new hash makes its feedback stale: it stops
	// applying at once, and nothing else is affected.
	if err := s.ReplaceFile(FileRecord{Path: "a.go", Hash: "h1-changed"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListFeedback(wt)
	if list[0].Status != FeedbackStale || list[1].Status != FeedbackValid {
		t.Fatalf("after change: statuses %q, %q; want stale, valid", list[0].Status, list[1].Status)
	}
	mults, _ = s.FeedbackMultipliers(wt)
	if _, ok := mults["a.go#Alpha"]; ok {
		t.Fatal("stale feedback still multiplies a.go#Alpha")
	}
	check("after change", mults, "b.go#Beta", 1.2)

	n, err := s.PurgeFeedback(wt, false)
	if err != nil || n != 1 {
		t.Fatalf("PurgeFeedback(stale only) = %d (%v), want 1", n, err)
	}
	if n, err := s.PurgeFeedback(wt, true); err != nil || n != 3 {
		t.Fatalf("PurgeFeedback(all) = %d (%v), want 3", n, err)
	}
}
