package retrieve

import (
	"math"
	"testing"
)

func TestDefaultTuningMatchesShippedConstants(t *testing.T) {
	d := DefaultTuning()
	if d.OwnCoverCap != ownCoverCap || d.OwnAltWeight != ownAltWeight || d.MinBlockConfidence != minBlockConfidence {
		t.Fatalf("default tuning drifted from constants: %+v", d)
	}
	if !d.Valid() {
		t.Fatal("default tuning must be valid")
	}
}

func TestTuningConfidenceDefaultsAreUnchanged(t *testing.T) {
	c := &candidate{v2Own: true, literal: true, directMatches: 1, promptTerms: 4}
	if got := confidenceOf(c); math.Abs(got-0.45) > 1e-9 {
		t.Fatalf("confidence changed for nil tuning: %v", got)
	}
	c.tun = &Tuning{OwnCoverCap: 1, OwnAltWeight: 1, OwnConfBase: 0.3, OwnConfSpan: 0.6, MinBlockConfidence: 0.5}
	if got := confidenceOf(c); math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("cap 1 should make one term full coverage, got %v", got)
	}
}

func TestTuningValidRejectsNonsense(t *testing.T) {
	bad := []Tuning{{OwnCoverCap: 0}, {OwnCoverCap: 4, OwnAltWeight: 3}, {OwnCoverCap: 4, OwnConfBase: 0.5, OwnConfSpan: 0.6},
		{OwnCoverCap: 4, MinBlockConfidence: 1.5}}
	for i, b := range bad {
		if b.Valid() {
			t.Fatalf("case %d should be invalid: %+v", i, b)
		}
	}
}

func TestV37TuningIsValidAndLeavesDefaultUnchanged(t *testing.T) {
	d := DefaultTuning()
	v := V37Tuning()
	if !v.Valid() {
		t.Fatalf("V37Tuning must be valid: %+v", v)
	}
	if d.LongPromptTerms != 0 || d.LongPromptMinSupport != 0 || d.LongPromptExemptCoverage != 0 {
		t.Fatalf("DefaultTuning drifted: %+v", d)
	}
	if v.LongPromptTerms != 7 || v.LongPromptMinSupport != 2 || v.LongPromptExemptCoverage <= 0 {
		t.Fatalf("unexpected V37Tuning: %+v", v)
	}
}
