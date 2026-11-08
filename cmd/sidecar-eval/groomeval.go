package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

type groomOutcome struct {
	Case      string        `json:"case"`
	Category  string        `json:"category"`
	ExpectAsk bool          `json:"expect_ask"`
	States    []groom.State `json:"states"`
	Action    groom.Action  `json:"action"`
	Asked     bool          `json:"asked"`
	StateOK   *bool         `json:"state_ok,omitempty"`
	Decision  string        `json:"decision,omitempty"`
	Resolved  string        `json:"resolved_by_repo,omitempty"`
}

// runGroomEval measures the clarification policy against labels written before the run. The two numbers that
// decide whether the feature is safe are the unnecessary-question rate on cases where nobody should be
// interrupted and the recall on cases where a decision is open. Both come with their counts: the benchmark is
// small and authored by the detector's author, so it is a regression gate, not evidence of usefulness.
func runGroomEval(cf casesFile, indexes map[string]*index.Index, outPath string) error {
	var outs []groomOutcome
	for _, c := range cf.Cases {
		if c.ExpectAsk == nil {
			continue
		}
		ix := indexes[c.Repo]
		res, err := retrieve.Search(context.Background(), ix.Store(), c.Prompt, retrieve.Options{EngineVersion: "v2", Root: ix.Root()})
		if err != nil {
			return err
		}
		rep := groom.Analyze(c.Prompt, res)
		d := groom.Decide(rep, c.Prompt, 0, groom.DefaultPolicy())
		o := groomOutcome{Case: c.ID, Category: c.Category, ExpectAsk: *c.ExpectAsk, States: rep.States, Action: d.Action, Asked: d.Action == groom.Ask}
		if d.Gap != nil {
			o.Decision, o.Resolved = d.Gap.Decision, d.Gap.ResolvedByRepo
		}
		if c.ExpectState != "" {
			ok := false
			for _, s := range rep.States {
				ok = ok || string(s) == c.ExpectState
			}
			o.StateOK = &ok
		}
		outs = append(outs, o)
	}
	var tp, fp, fn, tn, stateOK, stateN int
	cats := map[string][2]int{} // asked, total per category
	for _, o := range outs {
		switch {
		case o.ExpectAsk && o.Asked:
			tp++
		case o.ExpectAsk && !o.Asked:
			fn++
		case !o.ExpectAsk && o.Asked:
			fp++
		default:
			tn++
		}
		if o.StateOK != nil {
			stateN++
			if *o.StateOK {
				stateOK++
			}
		}
		c := cats[o.Category]
		c[1]++
		if o.Asked {
			c[0]++
		}
		cats[o.Category] = c
	}
	fmt.Printf("clarification benchmark: %d cases (ask expected %d, no-ask expected %d)\n", len(outs), tp+fn, fp+tn)
	fmt.Printf("  asked correctly %d/%d (recall %.0f%%) | missed %d | unnecessary questions %d/%d (%.0f%%) | state agreement %d/%d\n",
		tp, tp+fn, pct(tp, tp+fn), fn, fp, fp+tn, pct(fp, fp+tn), stateOK, stateN)
	names := make([]string, 0, len(cats))
	for n := range cats {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("  %-24s asked %d/%d\n", n, cats[n][0], cats[n][1])
	}
	for _, o := range outs {
		if o.ExpectAsk != o.Asked || (o.StateOK != nil && !*o.StateOK) {
			fmt.Printf("  MISMATCH %-4s expect_ask=%v action=%s states=%v %s\n", o.Case, o.ExpectAsk, o.Action, o.States, o.Decision)
		}
	}
	if outPath != "" {
		data, _ := json.MarshalIndent(map[string]any{"schema": "groom-eval/1", "hardware": hardware(), "outcomes": outs,
			"summary": map[string]int{"tp": tp, "fn": fn, "fp": fp, "tn": tn, "state_ok": stateOK, "state_n": stateN}}, "", "  ")
		return os.WriteFile(outPath, data, 0o644)
	}
	return nil
}
