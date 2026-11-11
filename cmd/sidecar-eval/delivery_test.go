package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHardNegativesExcludeHoldout(t *testing.T) {
	file := filepath.Join(t.TempDir(), "negatives.json")
	if err := os.WriteFile(file, []byte(`{"splits":{"dev":["dev prompt"],"validation":["validation prompt"],"holdout":["sealed prompt"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cases, err := hardNegatives(file, []string{"one", "two"})
	if err != nil || len(cases) != 4 {
		t.Fatalf("cases=%+v err=%v", cases, err)
	}
	for _, c := range cases {
		if !c.Negative || strings.Contains(c.Prompt, "sealed") {
			t.Fatalf("invalid negative: %+v", c)
		}
	}
}

func TestScoreDeliveryDetectsGoldAndConflictingLabels(t *testing.T) {
	c := caseSpec{ID: "one", Relevant: []gold{{Path: "src/router.go", Name: "Serve"}}}
	context := "[SIDECAR CONTEXT]\nRELEVANT CODE\n- function Serve [VERIFIED] [CANDIDATE] (source: src/router.go:1-3)\n[END SIDECAR CONTEXT]"
	row := scoreDelivery("v3.6", c, context)
	if !row.GoldDelivered || !row.GoldSymbol || row.BriefOnly || row.GoldLine != 1 || row.LabelConflict != 1 {
		t.Fatalf("unexpected delivery score: %+v", row)
	}
}

func TestScoreDeliveryDoesNotPairUnrelatedSymbolsOrPathPrefixes(t *testing.T) {
	c := caseSpec{Relevant: []gold{{Path: "src/router.go", Name: "Serve"}}}
	for _, test := range []struct {
		context      string
		path, symbol bool
	}{
		{"RELEVANT CODE\n- Serve (source: src/router.go.backup:1)", false, false},
		{"RELEVANT CODE\n- Other (source: src/router.go:1)\n- Serve (source: src/other.go:1)", true, false},
		{"RELEVANT CODE\n- ServeHTTP (source: src/router.go:1)", true, false},
	} {
		got := scoreDelivery("v3.6", c, test.context)
		if got.GoldDelivered != test.path || got.GoldSymbol != test.symbol {
			t.Fatalf("context %q: %+v", test.context, got)
		}
	}
}

func TestDeliveryRejectsUnknownEngine(t *testing.T) {
	if err := runDelivery(nil, nil, []string{"v3.typo"}, t.TempDir(), "", "", false); err == nil {
		t.Fatal("unknown engine silently selected default")
	}
}
