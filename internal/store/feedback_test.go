package store

import (
	"database/sql"
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

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Each expected value is the fixed formula worked out by hand: raw = 0.20·S_agent + 0.35·S_trace +
// 0.45·S_oracle, delta = 0.25·raw, clamped to ±0.25.
func TestComputeReconciledWeightSignals(t *testing.T) {
	cases := []struct {
		label                  string
		polarity               int
		trace, oracle          int
		additionalSearchNeeded bool
		want                   float64
	}{
		// raw = 0.2·1 + 0.35·1 + 0.45·1 = 1.0 → clamped to the bound.
		{"helpful, edited, oracle passed", 1, TraceEdited, 1, false, 0.25},
		// raw = 0.2·0.25 + 0.35·0 + 0.45·1: an unobserved claim is discounted, the passed oracle still counts.
		{"helpful, untouched, oracle passed", 1, TraceUntouched, 1, false, 0.125},
		// raw = 0.2·1 + 0.35·0.5 + 0.45·0: a read is weak, and an unknown oracle adds nothing.
		{"helpful, read, oracle unknown", 1, TraceRead, 0, false, 0.09375},
		// raw = 0.2·(−1) + 0.35·(−0.25) + 0.45·(−1): flagged after reading, and the oracle failed.
		{"misleading, read and edited, oracle failed", -1, TraceEdited, -1, true, -0.184375},
		// Not reading a misleading item is not evidence against the flag: the trace adds nothing.
		{"misleading, untouched", -1, TraceUntouched, 0, false, -0.05},
		{"misleading, no trace", -1, TraceNotObserved, 0, false, -0.05},
	}
	for _, c := range cases {
		if got := ComputeReconciledWeight(c.polarity, "high", c.trace, c.oracle, c.additionalSearchNeeded); !near(got, c.want) {
			t.Errorf("%s: weight = %v, want %v", c.label, got, c.want)
		}
	}
	// An unknown oracle must not score like a failure: the completion claim is never the oracle.
	if unknown, failed := ComputeReconciledWeight(1, "high", TraceEdited, 0, false), ComputeReconciledWeight(1, "high", TraceEdited, -1, false); unknown <= failed {
		t.Fatalf("unknown oracle (%v) scored no better than a failed one (%v)", unknown, failed)
	}
	// Every combination stays inside the bound.
	for _, pol := range []int{1, -1} {
		for _, conf := range []string{"high", "medium", "low", "other"} {
			for _, tr := range []int{TraceUntouched, TraceNotObserved, TraceRead, TraceEdited} {
				for _, oracle := range []int{-1, 0, 1} {
					for _, more := range []bool{false, true} {
						w := ComputeReconciledWeight(pol, conf, tr, oracle, more)
						if w < -feedbackMaxDelta || w > feedbackMaxDelta {
							t.Fatalf("weight %v out of bounds for %v", w, []any{pol, conf, tr, oracle, more})
						}
					}
				}
			}
		}
	}
}

// Tiers: only an independent oracle with a known outcome is strong; an edit in the session makes it
// medium; a self-report alone is weak.
func TestClassifyFeedbackTiers(t *testing.T) {
	cases := []struct {
		name       string
		oracle     int
		provenance string
		presented  bool
		trace      int
		want       string
	}{
		{"independent pass on delivered evidence", 1, ProvenanceHarness, true, TraceNotObserved, TierStrong},
		{"human failure on delivered evidence", -1, ProvenanceHuman, true, TraceUntouched, TierStrong},
		{"independent oracle on evidence never delivered", 1, ProvenanceHarness, false, TraceEdited, TierWeak},
		{"no outcome from a harness is not strong", 0, ProvenanceHarness, true, TraceEdited, TierMedium},
		{"self-reported pass without edit", 1, ProvenanceAgentReport, true, TraceNotObserved, TierWeak},
		{"self-reported pass with an edit of delivered evidence", 1, ProvenanceAgentReport, true, TraceEdited, TierMedium},
		{"edit of evidence never delivered", 1, ProvenanceAgentReport, false, TraceEdited, TierWeak},
		{"opening a file is not use", 0, ProvenanceNone, true, TraceRead, TierWeak},
		{"rows from before v6", 1, ProvenanceLegacy, true, TraceRead, TierWeak},
	}
	for _, c := range cases {
		if got := ClassifyFeedback(c.oracle, c.provenance, c.presented, c.trace); got != c.want {
			t.Errorf("%s: ClassifyFeedback = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEvidenceUseFromTrace(t *testing.T) {
	want := map[int]string{TraceUntouched: EvidenceUnknown, TraceNotObserved: EvidenceUnknown, TraceRead: EvidenceInferred, TraceEdited: EvidenceConfirmed}
	for trace, use := range want {
		if got := EvidenceUse(trace); got != use {
			t.Errorf("EvidenceUse(%d) = %q, want %q", trace, got, use)
		}
	}
}

func openFeedbackStore(t *testing.T, files ...FileRecord) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, f := range files {
		if err := s.ReplaceFile(f, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestFeedbackLifecycleAndInvalidation(t *testing.T) {
	s := openFeedbackStore(t, FileRecord{Path: "a.go", Hash: "h1"}, FileRecord{Path: "b.go", Hash: "h2"}, FileRecord{Path: "c.go", Hash: "h3"})
	const wt = "/repo"
	insert := func(rec FeedbackRecord) {
		t.Helper()
		rec.Tier = TierStrong // this test is about lifecycle; tier scaling has its own test
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

// Strong feedback applies in full, medium at half the factor, and weak feedback never applies.
func TestFeedbackTiersScaleMultipliers(t *testing.T) {
	s := openFeedbackStore(t, FileRecord{Path: "a.go", Hash: "h1"}, FileRecord{Path: "b.go", Hash: "h2"}, FileRecord{Path: "c.go", Hash: "h3"})
	const wt = "/repo"
	add := func(path, symbol, hash, tier string, weight float64) {
		t.Helper()
		if _, err := s.InsertFeedback(FeedbackRecord{Worktree: wt, EvidenceID: path, FilePath: path, Symbol: symbol,
			FileHash: hash, Polarity: -1, ReconciledWeight: weight, Tier: tier, CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	add("a.go", "Alpha", "h1", TierStrong, -0.2) // 1 + (-0.2)
	add("b.go", "Beta", "h2", TierMedium, -0.2)  // 1 + 0.5 · (-0.2)
	add("c.go", "", "h3", TierWeak, -0.2)        // self-report only: not applied
	mults, err := s.FeedbackMultipliers(wt)
	if err != nil {
		t.Fatal(err)
	}
	if !near(mults["a.go#Alpha"], 0.8) || !near(mults["b.go#Beta"], 0.9) {
		t.Fatalf("multipliers = %v, want strong 0.8 and medium 0.9", mults)
	}
	if _, ok := mults["c.go"]; ok {
		t.Fatalf("weak feedback changed the ranking: %v", mults)
	}
}

// A report that leaves its fields empty is stored as the weakest record, so no caller can strengthen
// a record by omission.
func TestInsertFeedbackDefaultsToWeakest(t *testing.T) {
	s := openFeedbackStore(t, FileRecord{Path: "a.go", Hash: "h1"})
	if _, err := s.InsertFeedback(FeedbackRecord{Worktree: "/repo", EvidenceID: "a.go#Alpha", FilePath: "a.go", Symbol: "Alpha", FileHash: "h1", Polarity: 1, ReconciledWeight: 0.2, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListFeedback("/repo")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListFeedback = %d (%v), want 1", len(list), err)
	}
	r := list[0]
	if r.OracleStatus != OracleUnknown || r.Provenance != ProvenanceNone || r.EvidenceUsed != EvidenceUnknown || r.Tier != TierWeak {
		t.Fatalf("defaults = oracle %q, provenance %q, used %q, tier %q; want unknown, none, unknown, weak",
			r.OracleStatus, r.Provenance, r.EvidenceUsed, r.Tier)
	}
	if mults, _ := s.FeedbackMultipliers("/repo"); len(mults) != 0 {
		t.Fatalf("weak default changed the ranking: %v", mults)
	}
}

// A database at schema v5 keeps its rows through the v6 migration. Those rows carried the agent's
// completion claim as their oracle, so they become weak and stop affecting the ranking.
func TestMigrationFromV5KeepsLegacyFeedbackWeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for v := 0; v < 5; v++ {
		for _, stmt := range migrations[v] {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("v%d: %v", v, err)
			}
		}
	}
	if _, err := db.Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files (path, hash) VALUES ('a.go', 'h1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO evidence_feedback (worktree, session_id, evidence_id, file_path, symbol, file_hash,
		polarity, reason_code, agent_confidence, trace_corroborated, oracle_passed, reconciled_weight, created_at)
		VALUES ('/repo', 's1', 'a.go#Alpha', 'a.go', 'Alpha', 'h1', 1, 'wrong_file', 'high', 1, 1, 0.2, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("migrate v5 -> v6: %v", err)
	}
	defer func() { _ = s.Close() }()
	list, err := s.ListFeedback("/repo")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListFeedback = %d (%v), want the legacy row", len(list), err)
	}
	r := list[0]
	if r.Tier != TierWeak || r.Provenance != ProvenanceLegacy || r.OracleStatus != OracleUnknown || r.AgentCompleted {
		t.Fatalf("legacy row = tier %q, provenance %q, oracle %q, completed %v; want weak, legacy, unknown, false",
			r.Tier, r.Provenance, r.OracleStatus, r.AgentCompleted)
	}
	if mults, _ := s.FeedbackMultipliers("/repo"); len(mults) != 0 {
		t.Fatalf("legacy feedback still changes the ranking: %v", mults)
	}
}
