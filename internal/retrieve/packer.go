package retrieve

import (
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// Packing constants are heuristics. minWorth was calibrated on the tune split only: values from
// 0.001 to 0.004 were compared, and anything above 0.001 removed relevant items from the v1 set.
const (
	minBlockConfidence = 0.5   // no block is sent unless one item reaches this confidence
	minItemConfidence  = 0.3   // an item below this never enters the block on its own
	minWorth           = 0.001 // marginal utility per token below this is not worth sending
)

var profileBudgets = map[string]int{"minimal": 300, "balanced": 800, "deep": 1500}

// ProfileBudget returns the token budget of a named profile: minimal, balanced or deep.
func ProfileBudget(name string) (int, bool) {
	b, ok := profileBudgets[strings.ToLower(strings.TrimSpace(name))]
	return b, ok
}

// PackOptions configures Pack.
type PackOptions struct {
	Budget int           // tokens available to the context block
	Seen   ctxbuild.Seen // evidence already delivered in this session; nil when none
	Tuning *Tuning       // nil = DefaultTuning
}

// PackTrace explains the packing decision for `sidecar explain`.
type PackTrace struct {
	Budget    int    `json:"budget_tokens"`
	Used      int    `json:"used_tokens"`
	Selected  int    `json:"selected"`
	Abstained bool   `json:"abstained"`
	Reason    string `json:"reason,omitempty"`
}

// InjectionPolicy answers the questions that come before an item may enter the block: is it
// new for this session, and is it trusted enough to send at all. Whether it is worth its tokens
// depends on what is already selected, so the packer's value test answers that instead.
type InjectionPolicy struct {
	Seen ctxbuild.Seen
}

// Refusal names why an item must not be sent, or returns "" when the policy allows it.
func (p InjectionPolicy) Refusal(e Evidence) string {
	if p.Seen != nil && p.Seen(e.Key(), e.Fingerprint()) {
		return "already delivered in this session"
	}
	if e.Confidence < minItemConfidence {
		return "confidence below the item gate"
	}
	return ""
}

// Pack chooses evidence greedily by marginal utility per token: at each step it takes the
// affordable item whose value, given what is already selected, is highest per token, and stops
// when no item is worth its tokens. Tokens are the limit, not a count of items: a count would let
// cheap, weak items displace valuable ones. Abstaining is the default: when no item clears the
// block gate, nothing is sent. Utility per token decides what is sent, not the order: the
// selection keeps the ranking order of cands. It returns the selection and the reason each other
// candidate was dropped, keyed by evidence ID.
func Pack(cands []Evidence, o PackOptions) ([]Evidence, map[string]string, PackTrace) {
	pt := PackTrace{Budget: o.Budget}
	excluded := map[string]string{}
	policy := InjectionPolicy{Seen: o.Seen}
	var open []Evidence
	rank := map[string]int{}
	best := 0.0
	for i, e := range cands {
		if why := policy.Refusal(e); why != "" {
			excluded[e.ID] = why
			continue
		}
		open = append(open, e)
		rank[e.ID] = i
		best = max(best, e.Confidence)
	}
	if len(open) == 0 || best < tuningOrDefault(o.Tuning).MinBlockConfidence {
		pt.Abstained, pt.Reason = true, "no evidence reaches the block confidence gate"
		for _, e := range open {
			excluded[e.ID] = "block abstained: no item reaches the confidence gate"
		}
		return nil, excluded, pt
	}

	st := &packState{covered: map[string]bool{}, perFile: map[string]int{}}
	for _, e := range cands {
		st.top = max(st.top, e.Score)
	}
	var selected []Evidence
	for {
		pick, pickU := -1, 0.0
		for k, e := range open {
			v, c, uc := st.value(e), costOf(e), utilityCostOf(e)
			if v/float64(uc) < minWorth || pt.Used+c > o.Budget {
				continue
			}
			if u := v / float64(uc); u > pickU {
				pick, pickU = k, u
			}
		}
		if pick < 0 {
			break
		}
		e := open[pick]
		open = append(open[:pick], open[pick+1:]...)
		st.take(e)
		selected = append(selected, e)
		pt.Used += costOf(e)
	}
	for _, e := range open {
		if st.value(e)/float64(utilityCostOf(e)) < minWorth {
			excluded[e.ID] = "not worth its tokens: marginal utility per token below threshold"
		} else {
			excluded[e.ID] = "dropped: token budget"
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return rank[selected[i].ID] < rank[selected[j].ID] })
	pt.Selected = len(selected)
	return selected, excluded, pt
}

// isRanked tells a retrieved item from a concern; concerns have their own, smaller limit.
func isRanked(e Evidence) bool { return e.Section != ctxbuild.Concerns }

// packState is what the selection has covered so far.
type packState struct {
	top     float64         // best fused score among the candidates
	covered map[string]bool // query concepts matched by a selected item
	perFile map[string]int  // selected items per file
	chosen  []Evidence
}

// value is the item's marginal utility: confidence × expected usefulness × trust × novelty ×
// diversity × complement. Each factor is a heuristic named where it is defined.
func (s *packState) value(e Evidence) float64 {
	return e.Confidence * s.expected(e) * trustOf(e) * s.novelty(e) * s.diversity(e) * s.complement(e)
}

// expected is the item's place in the fused ranking, as a share of the best score: the ranking
// is the signal that v1 already validates. A conflict is the divergence itself, so it counts fully.
func (s *packState) expected(e Evidence) float64 {
	if !isRanked(e) || s.top <= 0 {
		return 1
	}
	return e.Score / s.top
}

func (s *packState) take(e Evidence) {
	for _, m := range e.Matched {
		s.covered[m] = true
	}
	s.perFile[e.Hit.Path]++
	s.chosen = append(s.chosen, e)
}

// novelty rewards query concepts that no selected item covers yet. An item that matches no
// concept, such as a structural neighbour, gets a neutral factor.
func (s *packState) novelty(e Evidence) float64 {
	if len(e.Matched) == 0 {
		return 0.5
	}
	fresh := 0
	for _, m := range e.Matched {
		if !s.covered[m] {
			fresh++
		}
	}
	return 0.3 + 0.7*float64(fresh)/float64(len(e.Matched))
}

// diversity penalises a file that already has selected items, and generated files.
func (s *packState) diversity(e Evidence) float64 {
	d := 1 / float64(1+s.perFile[e.Hit.Path])
	if repo.IsGenerated(e.Hit.Path, nil) {
		d *= 0.5
	}
	return d
}

// complement rewards a test beside the selected code it checks, and code whose test is selected.
func (s *packState) complement(e Evidence) float64 {
	c := 1.0
	isTest := repo.Kind(e.Hit.Path) == "test"
	for _, rel := range e.Relations {
		if strings.Contains(rel, "[DOCUMENTED]") || strings.Contains(rel, "[TEST_EXPECTED]") {
			c = max(c, 1.2)
		}
	}
	for _, sel := range s.chosen {
		if isTest && hasString(sel.Relations, "tested_by:"+e.Hit.Path) {
			c = max(c, 1.25)
		}
		if hasString(e.Relations, "tested_by:"+sel.Hit.Path) {
			c = max(c, 1.1)
		}
	}
	return c
}

// trustOf discounts evidence that is inferred rather than verified.
func trustOf(e Evidence) float64 {
	switch e.Verdict {
	case "VERIFIED":
		return 1
	case "INFERRED":
		return 0.8
	}
	return 0.6
}

// costOf is the item's rendered size in tokens, never less than one.
func costOf(e Evidence) int {
	return max(1, ctxbuild.EstimateTokens(Describe(e)))
}

// utilityCostOf caps the token denominator used for marginal utility so that rich docstrings
// and contract annotations ([DOCUMENTED], [TEST_EXPECTED]) do not penalize well-documented code.
func utilityCostOf(e Evidence) int {
	return max(1, min(costOf(e), 25))
}
