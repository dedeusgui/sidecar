package main

import "testing"

func TestFilterHoldoutCasesRequiresExplicitFlag(t *testing.T) {
	cases := []caseSpec{
		{ID: "dev", Split: "tune"},
		{ID: "sealed", Split: "holdout"},
	}

	got, skipped := filterHoldoutCases(cases, false)
	if len(got) != 1 || got[0].ID != "dev" || skipped != 1 {
		t.Fatalf("without flag: cases=%+v skipped=%d, want only dev and one omitted", got, skipped)
	}

	got, skipped = filterHoldoutCases(cases, true)
	if len(got) != 2 || got[0].ID != "dev" || got[1].ID != "sealed" || skipped != 0 {
		t.Fatalf("with flag: cases=%+v skipped=%d, want both cases and none omitted", got, skipped)
	}
}
