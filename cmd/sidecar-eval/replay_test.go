package main

import (
	"os"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestCommittedActiveTuningMatchesDefaults(t *testing.T) {
	if err := checkActiveTuning("../../bench/replay/params.json"); err != nil {
		t.Fatal(err)
	}
}

func TestExpandVariantsOrderGridAndLimits(t *testing.T) {
	cfg := replayConfig{Version: "x", Active: retrieve.DefaultTuning(),
		Variants: map[string]retrieve.Tuning{"b": retrieve.DefaultTuning(), "a": retrieve.DefaultTuning()},
		Grid:     map[string][]float64{"own_cover_cap": {3, 5}, "own_alt_weight": {0.5, 1}}}
	got, err := expandVariants(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range got {
		names = append(names, v.Name)
	}
	if strings.Join(names[:3], ",") != "active,a,b" || len(got) != 3+4 {
		t.Fatalf("unexpected variants: %v", names)
	}
	if got[3].Tuning.OwnAltWeight != 0.5 || got[3].Tuning.OwnCoverCap != 3 {
		t.Fatalf("grid not applied: %+v", got[3])
	}
	big := replayConfig{Version: "x", Active: retrieve.DefaultTuning(),
		Grid: map[string][]float64{"own_cover_cap": {1, 2, 3, 4, 5, 6}, "own_alt_weight": {0, 0.5, 1, 1.5, 2, 0.25}}}
	if _, err := expandVariants(big); err == nil {
		t.Fatal("oversized grid must be rejected")
	}
}

func TestExpandVariantsRejectsInvalidAndUnknown(t *testing.T) {
	bad := retrieve.DefaultTuning()
	bad.OwnCoverCap = 0
	if _, err := expandVariants(replayConfig{Version: "x", Active: retrieve.DefaultTuning(), Variants: map[string]retrieve.Tuning{"bad": bad}}); err == nil {
		t.Fatal("invalid variant must be rejected")
	}
	if _, err := expandVariants(replayConfig{Version: "x", Active: retrieve.DefaultTuning(), Grid: map[string][]float64{"nope": {1}}}); err == nil {
		t.Fatal("unknown grid parameter must be rejected")
	}
}

func TestCheckActiveTuningDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.json"
	writeFile(t, path, `{"version":"x","active":{"own_cover_cap":9,"own_alt_weight":1,"own_conf_base":0.3,"own_conf_span":0.6,"min_block_confidence":0.5}}`)
	if err := checkActiveTuning(path); err == nil {
		t.Fatal("drift must be detected")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReplayKeepsTheHoldoutOutOfTheVariantComparison(t *testing.T) {
	cf := casesFile{Cases: []caseSpec{{ID: "a", Split: "tune"}, {ID: "b", Split: "holdout"}, {ID: "c", Split: "eval"}}}
	var kept []string
	for _, c := range cf.Cases {
		if c.Split != "holdout" {
			kept = append(kept, c.ID)
		}
	}
	if strings.Join(kept, ",") != "a,c" {
		t.Fatalf("holdout must be filtered out: %v", kept)
	}
}
