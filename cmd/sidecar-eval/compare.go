package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// splits lists the case splits in report order. The evaluation split is never used for tuning.
var splits = []string{"tune", "eval", "holdout"}

// splitsOf lists the fixed non-holdout splits first. Holdout is listed only when present in the
// outcomes, then any other split in order of appearance. Scenario suites and external manifests
// bring their own names.
func splitsOf(outs []caseOutcome) []string {
	present := map[string]bool{}
	for _, o := range outs {
		present[o.Split] = true
	}
	out := append([]string{}, splits[:2]...)
	seen := map[string]bool{"tune": true, "eval": true}
	if present["holdout"] {
		out = append(out, "holdout")
		seen["holdout"] = true
	}
	for _, o := range outs {
		if !seen[o.Split] {
			seen[o.Split] = true
			out = append(out, o.Split)
		}
	}
	return out
}

// baseline is a results file written earlier by --out. Only the fields the comparison needs are read.
type baseline struct {
	Hardware map[string]any `json:"hardware"`
	Outcomes []caseOutcome  `json:"outcomes"`
}

// summary holds the headline numbers of one method on one split. Percentages are 0-100.
type summary struct {
	Hit1, Hit5, Recall10, MRR, Precision, Empty, FalsePos float64
	P50, P95, Tokens                                      float64
	Positives, Negatives                                  int
}

// deltaRow compares one method on one split against the baseline.
type deltaRow struct {
	Split, Method string
	Cur, Base     summary
}

// caseChange is a single case whose result changed between the baseline and the current run.
type caseChange struct {
	Method, Split, Case, Detail string
}

// comparison is everything --compare reports.
type comparison struct {
	Hardware     map[string]any
	Rows         []deltaRow
	Skipped      []string // methods absent from the baseline, so not compared
	Incomparable []string // method/split with missing, duplicate or inconsistent cases
	Regressions  []caseChange
	Improvements []caseChange
}

type agg struct {
	pos, neg, hit1, hit5, empty, falsePos, precN int
	recall, rr, precision                        float64
	latencies, tokens                            []float64
}

// loadBaseline reads a results file produced by `sidecar-eval --out`.
func loadBaseline(path string) (baseline, error) {
	var b baseline
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, fmt.Errorf("%s: %w", path, err)
	}
	if len(b.Outcomes) == 0 {
		return b, fmt.Errorf("%s: no outcomes; expected a file written by sidecar-eval --out", path)
	}
	return b, nil
}

// compareRuns matches the current run against a baseline by method, split and case ID.
func compareRuns(base baseline, cur []caseOutcome, methods []string) comparison {
	c := comparison{Hardware: base.Hardware}
	inBase := map[string]bool{}
	for _, o := range base.Outcomes {
		inBase[o.Method] = true
	}
	allOutcomes := make([]caseOutcome, 0, len(base.Outcomes)+len(cur))
	allOutcomes = append(allOutcomes, base.Outcomes...)
	allOutcomes = append(allOutcomes, cur...)
	currentSplits := make(map[string]bool)
	for _, o := range cur {
		currentSplits[o.Split] = true
	}
	validGroups := make(map[string]map[string]bool)
	for _, m := range methods {
		if !inBase[m] {
			c.Skipped = append(c.Skipped, m)
			continue
		}
		for _, split := range splitsOf(allOutcomes) {
			if split == "holdout" && !currentSplits[split] {
				continue
			}
			ok, nonempty := comparableCases(m, split, base.Outcomes, cur)
			if !nonempty {
				continue
			}
			if !ok {
				c.Incomparable = append(c.Incomparable, m+" / "+split)
				continue
			}
			if validGroups[m] == nil {
				validGroups[m] = make(map[string]bool)
			}
			validGroups[m][split] = true
			c.Rows = append(c.Rows, deltaRow{
				Split: split, Method: m,
				Cur:  summarize(m, split, cur),
				Base: summarize(m, split, base.Outcomes),
			})
		}
	}
	var comparableCurrent []caseOutcome
	for _, o := range cur {
		if validGroups[o.Method][o.Split] {
			comparableCurrent = append(comparableCurrent, o)
		}
	}
	c.Regressions, c.Improvements = detectChanges(base.Outcomes, comparableCurrent)
	return c
}

// comparableCases requires the same unique case IDs and polarity in both runs.
func comparableCases(method, split string, base, cur []caseOutcome) (ok, nonempty bool) {
	collect := func(outs []caseOutcome) (map[string]bool, bool) {
		cases := make(map[string]bool)
		for _, o := range outs {
			if o.Method != method || o.Split != split {
				continue
			}
			if _, exists := cases[o.Case]; exists {
				return cases, false
			}
			cases[o.Case] = o.Negative
		}
		return cases, true
	}
	b, bUnique := collect(base)
	c, cUnique := collect(cur)
	if len(b) == 0 && len(c) == 0 {
		return false, false
	}
	if !bUnique || !cUnique || len(b) != len(c) {
		return false, true
	}
	for id, negative := range b {
		if currentNegative, found := c[id]; !found || currentNegative != negative {
			return false, true
		}
	}
	return true, true
}

// detectChanges lists positive cases that lost Hit@5 and negative cases that started
// returning context (a new false positive). Their opposites are listed as improvements.
func detectChanges(base, cur []caseOutcome) (regressions, improvements []caseChange) {
	type key struct{ method, split, id string }
	prev := map[key]caseOutcome{}
	for _, o := range base {
		prev[key{o.Method, o.Split, o.Case}] = o
	}
	for _, o := range cur {
		b, ok := prev[key{o.Method, o.Split, o.Case}]
		if !ok || b.Negative != o.Negative {
			continue
		}
		change := caseChange{Method: o.Method, Split: o.Split, Case: o.Case}
		switch {
		case o.Negative && b.Empty && !o.Empty:
			change.Detail = "falso positivo novo (passou a injetar contexto)"
			regressions = append(regressions, change)
		case o.Negative && !b.Empty && o.Empty:
			change.Detail = "falso positivo removido"
			improvements = append(improvements, change)
		case !o.Negative && b.Hit5 && !o.Hit5:
			change.Detail = "Hit@5 verdadeiro → falso"
			regressions = append(regressions, change)
		case !o.Negative && !b.Hit5 && o.Hit5:
			change.Detail = "Hit@5 falso → verdadeiro"
			improvements = append(improvements, change)
		}
	}
	return regressions, improvements
}

// summarize aggregates one method on one split. The table and the comparison both use it.
func summarize(name, split string, outs []caseOutcome) summary {
	a := aggregate(name, split, outs)
	return summary{
		Hit1:      pct(a.hit1, a.pos),
		Hit5:      pct(a.hit5, a.pos),
		Recall10:  100 * a.recall / float64(max(a.pos, 1)),
		MRR:       a.rr / float64(max(a.pos, 1)),
		Precision: 100 * a.precision / float64(max(a.precN, 1)),
		Empty:     pct(a.empty, a.pos),
		FalsePos:  pct(a.falsePos, a.neg),
		P50:       percentile(a.latencies, 50),
		P95:       percentile(a.latencies, 95),
		Tokens:    mean(a.tokens),
		Positives: a.pos,
		Negatives: a.neg,
	}
}

func aggregate(name, split string, outs []caseOutcome) *agg {
	a := &agg{}
	for _, o := range outs {
		if o.Method != name || (split != "all" && o.Split != split) {
			continue
		}
		a.latencies = append(a.latencies, o.LatencyMS)
		if o.Negative {
			a.neg++
			if !o.Empty {
				a.falsePos++
			}
			continue
		}
		a.pos++
		a.tokens = append(a.tokens, float64(o.Tokens))
		if o.Hit1 {
			a.hit1++
		}
		if o.Hit5 {
			a.hit5++
		}
		if o.Empty {
			a.empty++
		}
		a.recall += o.Recall10
		a.rr += o.RR
		if !o.Empty {
			a.precision += o.Precision
			a.precN++
		}
	}
	return a
}

// printComparison writes the delta table and the named regressions and improvements.
func printComparison(w io.Writer, c comparison) {
	_, _ = fmt.Fprintf(w, "\n### Comparação com baseline\n\n")
	_, _ = fmt.Fprintf(w, "Baseline: %s · %s · %s\n\n", hardwareField(c.Hardware, "date"), hardwareField(c.Hardware, "go"), hardwareField(c.Hardware, "cpu_model"))
	_, _ = fmt.Fprintln(w, "Δ = atual − baseline (Δ em % são pontos percentuais). Em Falso Positivo, P50 e Tokens, Δ positivo é pior; nos demais, positivo é melhor.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "| Split | Método | Δ Hit@5 | Δ Recall@10 | Δ MRR | Δ Precisão | Δ Falso Positivo | Δ P50 ms | Δ Tokens |")
	_, _ = fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|")
	for _, r := range c.Rows {
		_, _ = fmt.Fprintf(w, "| %s | %s | %+.1f | %+.1f | %+.3f | %+.1f | %+.1f | %+.2f | %+.0f |\n",
			r.Split, r.Method,
			r.Cur.Hit5-r.Base.Hit5, r.Cur.Recall10-r.Base.Recall10, r.Cur.MRR-r.Base.MRR,
			r.Cur.Precision-r.Base.Precision, r.Cur.FalsePos-r.Base.FalsePos,
			r.Cur.P50-r.Base.P50, r.Cur.Tokens-r.Base.Tokens)
	}
	if len(c.Skipped) > 0 {
		_, _ = fmt.Fprintf(w, "\nMétodos sem baseline (não comparados): %v\n", c.Skipped)
	}
	if len(c.Incomparable) > 0 {
		_, _ = fmt.Fprintf(w, "\nConjuntos de casos incompatíveis (não comparados; excluídos também das mudanças por caso): %v\n", c.Incomparable)
	}
	printChanges(w, fmt.Sprintf("Regressões (%d)", len(c.Regressions)), "Nenhuma regressão.", c.Regressions)
	printChanges(w, fmt.Sprintf("Melhorias (%d)", len(c.Improvements)), "Nenhuma melhoria.", c.Improvements)
}

func printChanges(w io.Writer, title, empty string, list []caseChange) {
	_, _ = fmt.Fprintf(w, "\n#### %s\n\n", title)
	if len(list) == 0 {
		_, _ = fmt.Fprintln(w, empty)
		return
	}
	for _, ch := range list {
		_, _ = fmt.Fprintf(w, "- %s · caso %s (%s): %s\n", ch.Method, ch.Case, ch.Split, ch.Detail)
	}
}

func hardwareField(hw map[string]any, key string) string {
	if v, ok := hw[key]; ok {
		return fmt.Sprint(v)
	}
	return "?"
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func percentile(xs []float64, p int) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64{}, xs...)
	sort.Float64s(c)
	idx := (p*len(c) + 99) / 100
	if idx > len(c) {
		idx = len(c)
	}
	return c[max(idx-1, 0)]
}
