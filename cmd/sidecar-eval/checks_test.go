package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/store"
)

func hitAt(path, name string) store.SymbolHit { return store.SymbolHit{Path: path, Name: name} }

func TestNegativeCheckFailsWhenContextIsInjected(t *testing.T) {
	c := caseSpec{ID: "n", Negative: true}
	if got := checkFailures(c, callResult{}, caseOutcome{Empty: true}); got != nil {
		t.Fatalf("empty negative failed: %v", got)
	}
	if got := checkFailures(c, callResult{}, caseOutcome{Empty: false, Tokens: 12}); len(got) != 1 || !strings.HasPrefix(got[0], "no_evidence") {
		t.Fatalf("injected negative passed: %v", got)
	}
}

func TestGoldMustNotConflictAndContractChecks(t *testing.T) {
	c := caseSpec{
		ID:             "p",
		Relevant:       []gold{{Path: "a.go", Name: "F"}},
		MustNot:        []string{"gen/x.go"},
		ExpectConflict: true,
		ExpectContract: "TEST_EXPECTED",
	}
	res := callResult{hits: []store.SymbolHit{hitAt("gen/x.go", "F")}}
	o := caseOutcome{Found: 0, Gold: 1}
	failed := checkFailures(c, res, o)
	for _, want := range []string{"gold: 0/1", "must_not: gen/x.go", "conflict:", "contract: TEST_EXPECTED"} {
		if !slices.ContainsFunc(failed, func(f string) bool { return strings.HasPrefix(f, want) }) {
			t.Errorf("missing failure %q in %v", want, failed)
		}
	}
	ok := callResult{
		hits:      []store.SymbolHit{hitAt("a.go", "F")},
		notes:     []string{conflictPrefix + " · código: constant F = 2"},
		contracts: []string{"TEST_EXPECTED"},
	}
	if got := checkFailures(c, ok, caseOutcome{Found: 1, Gold: 1}); got != nil {
		t.Fatalf("satisfied checks still failed: %v", got)
	}
}

func TestFileLevelGoldMatchesAnySymbolInFile(t *testing.T) {
	g := gold{Path: "a.go"}
	if !g.matches(hitAt("a.go", "Anything")) || g.matches(hitAt("b.go", "Anything")) {
		t.Fatal("file-level gold must match every symbol of its file and nothing else")
	}
}

func TestSeenFailuresRequireGoldFirstThenBlockRepeats(t *testing.T) {
	c := caseSpec{Relevant: []gold{{Path: "a.go", Name: "F"}}}
	withGold := callResult{hits: []store.SymbolHit{hitAt("a.go", "F")}}
	if got := seenFailures(c, callResult{}, withGold); len(got) != 1 || !strings.HasPrefix(got[0], "seen: 1ª passada") {
		t.Fatalf("missing gold in first pass not reported: %v", got)
	}
	if got := seenFailures(c, withGold, withGold); len(got) != 1 || !strings.HasPrefix(got[0], "seen: gold repetido") {
		t.Fatalf("repeated gold not reported: %v", got)
	}
	if got := seenFailures(c, withGold, callResult{}); got != nil {
		t.Fatalf("suppressed rerun failed: %v", got)
	}
}

func TestSeenSetReportsOnlyTheGivenKeys(t *testing.T) {
	seen := seenSet([]string{"k1"})
	if !seen("k1", "") || seen("k2", "") {
		t.Fatal("seen set must answer true only for delivered keys")
	}
}
