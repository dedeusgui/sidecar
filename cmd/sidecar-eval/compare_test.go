package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func caseIDs(list []caseChange) string {
	var ids []string
	for _, c := range list {
		ids = append(ids, c.Case)
	}
	return strings.Join(ids, ",")
}

func TestSummarizeAggregatesOneSplitAndMethod(t *testing.T) {
	outs := []caseOutcome{
		{Method: "m", Split: "tune", Case: "T1", Hit5: true, RR: 1, Recall10: 1, Precision: 0.5, Tokens: 100, LatencyMS: 1},
		{Method: "m", Split: "tune", Case: "T2", Empty: true, Tokens: 0, LatencyMS: 3},
		{Method: "m", Split: "tune", Case: "N1", Negative: true, Empty: false, LatencyMS: 5},
		{Method: "m", Split: "tune", Case: "N2", Negative: true, Empty: true, LatencyMS: 7},
		// Another split and another method must not leak into this summary.
		{Method: "m", Split: "eval", Case: "E1", Hit5: true, RR: 1, Recall10: 1, Precision: 1, Tokens: 999, LatencyMS: 100},
		{Method: "other", Split: "tune", Case: "X1", Hit5: true, RR: 1, Recall10: 1, Precision: 1, Tokens: 999, LatencyMS: 100},
	}
	s := summarize("m", "tune", outs)
	checks := map[string][2]float64{
		"Hit5":      {s.Hit5, 50},
		"Recall10":  {s.Recall10, 50},
		"MRR":       {s.MRR, 0.5},
		"Precision": {s.Precision, 50},
		"Empty":     {s.Empty, 50},
		"FalsePos":  {s.FalsePos, 50},
		"P50":       {s.P50, 3},
		"P95":       {s.P95, 7},
		"Tokens":    {s.Tokens, 50},
	}
	for name, c := range checks {
		if !near(c[0], c[1]) {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	if s.Positives != 2 || s.Negatives != 2 {
		t.Errorf("counts = %d positives / %d negatives, want 2 / 2", s.Positives, s.Negatives)
	}
}

func TestCompareRunsReportsCurrentMinusBaseline(t *testing.T) {
	base := baseline{Outcomes: []caseOutcome{
		{Method: "m", Split: "tune", Case: "T1", Hit5: false, Tokens: 100, LatencyMS: 2},
		{Method: "m", Split: "tune", Case: "N1", Negative: true, Empty: true, LatencyMS: 2},
	}}
	cur := []caseOutcome{
		{Method: "m", Split: "tune", Case: "T1", Hit5: true, RR: 1, Recall10: 1, Precision: 1, Tokens: 60, LatencyMS: 5},
		{Method: "m", Split: "tune", Case: "N1", Negative: true, Empty: false, LatencyMS: 5},
	}
	c := compareRuns(base, cur, []string{"m"})
	if len(c.Rows) != len(splits) {
		t.Fatalf("rows = %d, want one per split", len(c.Rows))
	}
	row := c.Rows[0]
	if row.Split != "tune" {
		t.Fatalf("first row split = %s, want tune", row.Split)
	}
	deltas := map[string][2]float64{
		"Hit5":      {row.Cur.Hit5 - row.Base.Hit5, 100},
		"Recall10":  {row.Cur.Recall10 - row.Base.Recall10, 100},
		"Precision": {row.Cur.Precision - row.Base.Precision, 100},
		"FalsePos":  {row.Cur.FalsePos - row.Base.FalsePos, 100},
		"P50":       {row.Cur.P50 - row.Base.P50, 3},
		"Tokens":    {row.Cur.Tokens - row.Base.Tokens, -40},
	}
	for name, d := range deltas {
		if !near(d[0], d[1]) {
			t.Errorf("Δ %s = %v, want %v", name, d[0], d[1])
		}
	}
	if eval := c.Rows[1]; eval.Split != "eval" || eval.Cur.Hit5 != 0 || eval.Base.Hit5 != 0 {
		t.Errorf("split without cases should compare as zeros, got %+v", eval)
	}
}

func TestDetectsRegressionsAndImprovementsByCase(t *testing.T) {
	base := []caseOutcome{
		{Method: "m", Split: "eval", Case: "E1", Hit5: true},
		{Method: "m", Split: "eval", Case: "E2", Hit5: false},
		{Method: "m", Split: "eval", Case: "E3", Hit5: true},
		{Method: "m", Split: "eval", Case: "N1", Negative: true, Empty: true},
		{Method: "m", Split: "eval", Case: "N2", Negative: true, Empty: false},
	}
	cur := []caseOutcome{
		{Method: "m", Split: "eval", Case: "E1", Hit5: false},
		{Method: "m", Split: "eval", Case: "E2", Hit5: true},
		{Method: "m", Split: "eval", Case: "E3", Hit5: true},
		{Method: "m", Split: "eval", Case: "N1", Negative: true, Empty: false},
		{Method: "m", Split: "eval", Case: "N2", Negative: true, Empty: true},
	}
	reg, imp := detectChanges(base, cur)
	if got := caseIDs(reg); got != "E1,N1" {
		t.Errorf("regressions = %q, want E1,N1", got)
	}
	if got := caseIDs(imp); got != "E2,N2" {
		t.Errorf("improvements = %q, want E2,N2", got)
	}
	if !strings.Contains(reg[1].Detail, "falso positivo novo") {
		t.Errorf("new false positive not described: %q", reg[1].Detail)
	}
}

func TestCaseChangesMatchOnMethodSplitAndCase(t *testing.T) {
	base := []caseOutcome{
		{Method: "a", Split: "tune", Case: "C1", Hit5: true},
		{Method: "a", Split: "eval", Case: "C1", Hit5: false},
		{Method: "b", Split: "tune", Case: "C1", Hit5: true},
	}
	cur := []caseOutcome{
		{Method: "a", Split: "tune", Case: "C1", Hit5: true},
		{Method: "a", Split: "eval", Case: "C1", Hit5: true},
		{Method: "b", Split: "tune", Case: "C1", Hit5: false},
		{Method: "c", Split: "tune", Case: "C1", Hit5: false},
	}
	reg, imp := detectChanges(base, cur)
	if len(reg) != 1 || reg[0].Method != "b" {
		t.Errorf("regressions = %+v, want only method b", reg)
	}
	if len(imp) != 1 || imp[0].Split != "eval" {
		t.Errorf("improvements = %+v, want only split eval", imp)
	}
}

func TestMethodMissingFromBaselineIsSkipped(t *testing.T) {
	base := baseline{Outcomes: []caseOutcome{{Method: "old", Split: "tune", Case: "T1", Hit5: true}}}
	cur := []caseOutcome{{Method: "new", Split: "tune", Case: "T1", Hit5: false}}
	c := compareRuns(base, cur, []string{"new"})
	if len(c.Rows) != 0 || len(c.Regressions) != 0 {
		t.Errorf("unmatched method was compared: rows=%d regressions=%d", len(c.Rows), len(c.Regressions))
	}
	if len(c.Skipped) != 1 || c.Skipped[0] != "new" {
		t.Errorf("skipped = %v, want [new]", c.Skipped)
	}
}

func TestLoadBaselineReadsCommittedResults(t *testing.T) {
	b, err := loadBaseline(filepath.Join("..", "..", "bench", "results", "final-state.json"))
	if err != nil {
		t.Fatalf("committed baseline does not load: %v", err)
	}
	if len(b.Outcomes) == 0 || b.Hardware["date"] == nil {
		t.Fatalf("baseline missing outcomes or hardware: %d outcomes", len(b.Outcomes))
	}
}

func TestLoadBaselineRejectsFilesWithoutOutcomes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	for _, body := range []string{`{"hardware":{}}`, `not json`} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadBaseline(path); err == nil {
			t.Errorf("loadBaseline accepted %q", body)
		}
	}
}

func TestMarkdownReportHasSplitsAndComparison(t *testing.T) {
	methods := []method{{name: "m"}}
	cur := []caseOutcome{{Method: "m", Split: "tune", Case: "T1", Hit5: false}}
	base := baseline{Outcomes: []caseOutcome{{Method: "m", Split: "tune", Case: "T1", Hit5: true}}}
	c := compareRuns(base, cur, methodNames(methods))

	path := filepath.Join(t.TempDir(), "report.md")
	writeReportMD(path, methods, cur, &c)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		"# Relatório sidecar-eval", "### Split: tune", "### Split: holdout",
		"### Comparação com baseline", "#### Regressões (1)", "caso T1 (tune): Hit@5 verdadeiro → falso",
		"#### Melhorias (0)", "Nenhuma melhoria.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
	var plain bytes.Buffer
	printTables(&plain, methods, cur)
	if strings.Contains(plain.String(), "Comparação") {
		t.Error("tables alone must not contain the comparison")
	}
}
