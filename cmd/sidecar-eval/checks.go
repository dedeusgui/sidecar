package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/store"
)

// conflictPrefix starts every contract conflict note (retrieve's conflictHead). The eval
// matches the text because that constant is unexported.
const conflictPrefix = "CONFLICT_CANDIDATE"

// checkFailures returns the mechanical checks that did not hold for one case. A check the case
// does not declare is never evaluated, so older suites keep the same numbers.
func checkFailures(c caseSpec, res callResult, o caseOutcome) []string {
	if c.Negative {
		if !o.Empty || o.Tokens != 0 {
			return []string{"no_evidence: contexto injetado"}
		}
		return nil
	}
	var failed []string
	if o.Found < o.Gold {
		failed = append(failed, fmt.Sprintf("gold: %d/%d no top 10", o.Found, o.Gold))
	}
	for _, p := range c.MustNot {
		if pathInTop(res.hits, p, 10) {
			failed = append(failed, "must_not: "+p)
		}
	}
	if c.ExpectConflict && !anyPrefix(res.notes, conflictPrefix) {
		failed = append(failed, "conflict: nota CONFLICT_CANDIDATE ausente")
	}
	if c.ExpectContract != "" && !slices.Contains(res.contracts, c.ExpectContract) {
		failed = append(failed, "contract: "+c.ExpectContract+" ausente")
	}
	return failed
}

// seenSet reports as already delivered exactly the evidence keys of an earlier pass.
func seenSet(keys []string) ctxbuild.Seen {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return func(key, _ string) bool { return set[key] }
}

// seenFailures is the continuation check. The first pass must present all the gold, and a
// second pass that already delivered that evidence must not present it again.
func seenFailures(c caseSpec, first, rerun callResult) []string {
	if goldInTop(c, first.hits, 10) < len(c.Relevant) {
		return []string{"seen: 1ª passada sem todo o gold"}
	}
	if goldInTop(c, rerun.hits, 10) > 0 {
		return []string{"seen: gold repetido após envio"}
	}
	return nil
}

// printScenarios summarises the cases that carry a category: per method, how many passed
// every check and which failed. It prints nothing for suites without categories.
func printScenarios(w io.Writer, methods []string, outs []caseOutcome) {
	any := false
	for _, o := range outs {
		if o.Category != "" {
			any = true
			break
		}
	}
	if !any {
		return
	}
	_, _ = fmt.Fprintf(w, "\n### Cenários (checks mecânicos)\n\n")
	for _, m := range methods {
		total, passed := 0, 0
		var failed []string
		for _, o := range outs {
			if o.Method != m || o.Category == "" {
				continue
			}
			total++
			if len(o.Failed) == 0 {
				passed++
				continue
			}
			failed = append(failed, fmt.Sprintf("%s (%s)", o.Case, strings.Join(o.Failed, "; ")))
		}
		list := "nenhuma"
		if len(failed) > 0 {
			list = strings.Join(failed, " · ")
		}
		_, _ = fmt.Fprintf(w, "- %s: %d/%d passam · falhas: %s\n", m, passed, total, list)
	}
}

func goldInTop(c caseSpec, hits []store.SymbolHit, n int) int {
	found := 0
	for _, g := range c.Relevant {
		for i := 0; i < min(len(hits), n); i++ {
			if g.matches(hits[i]) {
				found++
				break
			}
		}
	}
	return found
}

func pathInTop(hits []store.SymbolHit, path string, n int) bool {
	for i := 0; i < min(len(hits), n); i++ {
		if hits[i].Path == path {
			return true
		}
	}
	return false
}

func anyPrefix(xs []string, prefix string) bool {
	for _, x := range xs {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}
