package retrieve

import (
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/store"
)

// item is an exact, verified evidence item; the packer reads only the fields set here.
func item(name, path string, score float64, matched ...string) Evidence {
	return Evidence{
		ID:         name,
		Hit:        store.SymbolHit{Kind: "function", Name: name, Path: path, Line: 1, EndLine: 3},
		Verdict:    "VERIFIED",
		Section:    ctxbuild.Code,
		Source:     "exact",
		Score:      score,
		Matched:    matched,
		Confidence: 0.9,
	}
}

func selectedIDs(sel []Evidence) map[string]bool {
	out := map[string]bool{}
	for _, e := range sel {
		out[e.ID] = true
	}
	return out
}

func TestPackAbstainsBelowBlockGate(t *testing.T) {
	weak := item("weak", "a.go", 1, "fila")
	weak.Confidence = 0.4 // below minBlockConfidence: no item may carry the block alone
	sel, excluded, pt := Pack([]Evidence{weak}, PackOptions{Budget: 800})
	if len(sel) != 0 || !pt.Abstained || pt.Used != 0 {
		t.Fatalf("selected %d, abstained %v, used %d; want nothing sent", len(sel), pt.Abstained, pt.Used)
	}
	if excluded["weak"] == "" {
		t.Errorf("abstention gave no reason for the excluded item")
	}
	if sel, _, pt := Pack(nil, PackOptions{Budget: 800}); len(sel) != 0 || !pt.Abstained {
		t.Errorf("no candidates must abstain, got %d items", len(sel))
	}
}

func TestPackStaysWithinBudget(t *testing.T) {
	var cands []Evidence
	for i, name := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		cands = append(cands, item(name, name+".go", float64(10-i), "fila"))
	}
	budget := costOf(cands[0]) + costOf(cands[1])
	sel, _, pt := Pack(cands, PackOptions{Budget: budget})
	if pt.Used > budget {
		t.Fatalf("used %d tokens of a %d budget", pt.Used, budget)
	}
	if len(sel) == 0 || len(sel) >= len(cands) {
		t.Errorf("selected %d of %d under a two-item budget", len(sel), len(cands))
	}
}

// Two items that cover the same concept from one file: the second adds nothing new, so when
// the budget holds two items the packer takes the other file's item instead.
func TestPackPrefersNewConceptsAndOtherFiles(t *testing.T) {
	a := item("alpha", "queue.go", 1, "fila")
	b := item("bravo", "queue.go", 1, "fila")
	c := item("charlie", "dispatch.go", 1, "liberacao")
	budget := costOf(a) + costOf(c)
	sel, excluded, _ := Pack([]Evidence{a, b, c}, PackOptions{Budget: budget})
	got := selectedIDs(sel)
	if !got["alpha"] || !got["charlie"] || got["bravo"] {
		t.Fatalf("selected %v; want alpha and charlie, not the repeat bravo", got)
	}
	if excluded["bravo"] == "" {
		t.Errorf("the repeated item has no exclusion reason")
	}
}

// The selection keeps the ranking order even when a lower-ranked item is worth more per token.
func TestPackKeepsRankingOrder(t *testing.T) {
	top := item("top", "a.go", 1, "fila")
	second := item("second", "b.go", 0.5, "liberacao")
	sel, _, _ := Pack([]Evidence{top, second}, PackOptions{Budget: 10000})
	if len(sel) != 2 {
		t.Fatalf("selected %d items, want 2", len(sel))
	}
	if sel[0].ID != "top" || sel[1].ID != "second" {
		t.Errorf("order = %s, %s; want the ranking order top, second", sel[0].ID, sel[1].ID)
	}
}

// Session memory is the "new?" question: evidence already delivered is never sent again.
func TestPackSkipsEvidenceAlreadyDelivered(t *testing.T) {
	a := item("alpha", "a.go", 1, "fila")
	b := item("bravo", "b.go", 1, "liberacao")
	seen := func(key, _ string) bool { return key == a.Key() }
	sel, excluded, _ := Pack([]Evidence{a, b}, PackOptions{Budget: 10000, Seen: seen})
	if got := selectedIDs(sel); got["alpha"] || !got["bravo"] {
		t.Fatalf("selected %v; want bravo only", got)
	}
	if excluded["alpha"] != "already delivered in this session" {
		t.Errorf("reason = %q", excluded["alpha"])
	}
}

// A generated file is demoted: the same evidence from a hand-written file is worth more.
func TestGeneratedFilesAreWorthLess(t *testing.T) {
	gen := item("client", "api/client.pb.go", 1, "fila")
	hand := item("client", "api/client.go", 1, "fila")
	st := &packState{covered: map[string]bool{}, perFile: map[string]int{}}
	if st.value(gen) >= st.value(hand) {
		t.Errorf("generated value %.3f >= hand-written %.3f", st.value(gen), st.value(hand))
	}
}

// A conflict is the divergence itself; it is sent with the other evidence when it fits.
func TestConflictConcernIsSelected(t *testing.T) {
	note := conflictHead + " · código: constant MaxSlots = 3 (a.go:4) · doc: d.md:5 diz 4"
	concern := Evidence{ID: "conflict|x", Section: ctxbuild.Concerns, Source: "contract", Verdict: "VERIFIED",
		Confidence: 0.8, Note: note, TokenEstimate: ctxbuild.EstimateTokens(note)}
	sel, _, _ := Pack([]Evidence{item("alpha", "a.go", 1, "fila"), concern}, PackOptions{Budget: 10000})
	if !selectedIDs(sel)["conflict|x"] {
		t.Fatalf("conflict was not selected: %v", selectedIDs(sel))
	}
}

func TestInjectionPolicyRefusals(t *testing.T) {
	weak := item("weak", "a.go", 1)
	weak.Confidence = 0.2
	if why := (InjectionPolicy{}).Refusal(weak); why == "" {
		t.Errorf("confidence 0.2 was allowed")
	}
	if why := (InjectionPolicy{}).Refusal(item("ok", "a.go", 1)); why != "" {
		t.Errorf("fresh confident item refused: %s", why)
	}
	seen := func(string, string) bool { return true }
	if why := (InjectionPolicy{Seen: seen}).Refusal(item("ok", "a.go", 1)); why == "" {
		t.Errorf("already delivered item was allowed")
	}
}

func TestProfileBudgets(t *testing.T) {
	want := map[string]int{"minimal": 300, "balanced": 800, "deep": 1500, " DEEP ": 1500}
	for name, budget := range want {
		if got, ok := ProfileBudget(name); !ok || got != budget {
			t.Errorf("ProfileBudget(%q) = %d, %v; want %d", name, got, ok, budget)
		}
	}
	if _, ok := ProfileBudget("huge"); ok {
		t.Errorf("unknown profile accepted")
	}
}
