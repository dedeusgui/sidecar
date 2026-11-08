// Package groom detects task decisions that are still open and decides, by impact and cost, whether
// asking the user is worth the interruption. It is deterministic and uses no model: every gap cites
// the repository evidence or the prompt words that raised it, and a gap the repository already answers
// is never turned into a question.
package groom

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// State is one reading of how well specified the task is. Several may hold at once.
type State string

const (
	Clear            State = "clear"
	Underspecified   State = "underspecified"
	Ambiguous        State = "ambiguous"
	Contradictory    State = "contradictory"
	ContractConflict State = "contract_conflict"
	Unknown          State = "unknown" // not enough information to say anything else
)

// Option is one plausible answer with the concrete consequence of choosing it.
type Option struct {
	Label       string `json:"label"`
	Consequence string `json:"consequence"`
}

// Gap is one open decision.
type Gap struct {
	State          State    `json:"state"`
	Decision       string   `json:"decision"`                   // what is undefined
	Evidence       []string `json:"evidence,omitempty"`         // path:line or quoted prompt words that raised it
	Options        []Option `json:"options,omitempty"`          // alternatives compatible with the repository
	Impact         float64  `json:"impact"`                     // 0..1: cost of choosing wrong
	Uncertainty    float64  `json:"uncertainty"`                // 0..1: how little the evidence settles it
	Rework         float64  `json:"rework"`                     // 0..1: breadth of what would be redone
	ChangesTests   bool     `json:"changes_tests"`              // the answer alters implementation or tests
	ResolvedByRepo string   `json:"resolved_by_repo,omitempty"` // evidence that already answers it; such a gap is never asked
	Value          float64  `json:"value"`                      // impact × uncertainty × rework factor
}

// Report is the analysis of one prompt.
type Report struct {
	States []State `json:"states"`
	Gaps   []Gap   `json:"gaps,omitempty"`
}

var (
	editVerbs = regexp.MustCompile(`\b(adicion\w*|acrescent\w*|cri(e|ar|a)|implement\w*|alter(e|ar|a)|mud(e|ar|a)|modific\w*|troc(e|ar|a)|substitu\w*|remov\w*|exclu\w*|refator\w*|corrij\w*|conserte|migr(e|ar|a)|renome\w*|permit\w*|habilit\w*|desabilit\w*|aument\w*|reduz\w*|diminu\w*|ajust\w*|increase|decrease|raise|lower|adjust|bump|add|create|implement|change|update|modify|replace|remove|delete|refactor|fix|migrate|rename|support|enable|disable|make|allow)\b`)
	// numbers, durations and sizes: a stated value for a limit
	hasNumber  = regexp.MustCompile(`\d`)
	changeVerb = regexp.MustCompile(`\b(aument\w*|reduz\w*|diminu\w*|mud(e|ar|a)|alter(e|ar|a)|troc(e|ar|a)|ajust\w*|increase|decrease|raise|lower|change|update|adjust|bump)\b`)
	limitWords = regexp.MustCompile(`\b(limite|limit|maximo|max|minimo|min|timeout|ttl|quota|cota|tamanho|size|taxa|rate|intervalo|interval|tentativas|retries|retry|expir\w*)\b`)
	// Explicit opt-outs and urgency, in the user's own words.
	noAsk  = regexp.MustCompile(`\b(nao pergunte|sem perguntar|sem precisar perguntar|nao precisa perguntar|don'?t ask|do not ask|no questions|just do it|so faca|apenas faca)\b`)
	urgent = regexp.MustCompile(`\b(urgente|urgencia|rapido|agora|asap|urgent|quick|quickly|right now|imediato|hotfix|prod esta fora|em producao|production is down)\b`)
)

// Analyze reads the prompt together with the retrieval evidence. res must come from the V2 pipeline so
// that contract findings and ranking confidence are present; without evidence the result is Unknown or
// Clear, never a guess.
func Analyze(prompt string, res retrieve.Result) Report {
	text := lexical.Fold(prompt)
	var rep Report
	if !editVerbs.MatchString(text) {
		rep.States = []State{Clear} // questions, locating and explaining need no decision
		return rep
	}
	var gaps []Gap
	if g := contractConflict(text, res); g != nil {
		gaps = append(gaps, *g)
	}
	if g := contradiction(text); g != nil {
		gaps = append(gaps, *g)
	}
	if g := ambiguousTarget(res); g != nil {
		gaps = append(gaps, *g)
	}
	if g := missingLimit(prompt, text, res); g != nil {
		gaps = append(gaps, *g)
	}
	for i := range gaps {
		gaps[i].Value = round2(gaps[i].Impact * gaps[i].Uncertainty * (0.5 + 0.5*gaps[i].Rework))
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Value > gaps[j].Value })
	rep.Gaps = gaps
	seen := map[State]bool{}
	for _, g := range gaps {
		if !seen[g.State] {
			seen[g.State] = true
			rep.States = append(rep.States, g.State)
		}
	}
	if len(rep.States) == 0 {
		if hasTarget(res) {
			rep.States = []State{Clear}
		} else {
			rep.States = []State{Unknown}
		}
	}
	return rep
}

func hasTarget(res retrieve.Result) bool {
	for _, ev := range res.Evidence {
		if ev.Note == "" && ev.Confidence >= 0.5 {
			return true
		}
	}
	return false
}

func isTestPath(p string) bool { _, k := repo.Classify(p); return k == "test" }

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func loc(ev retrieve.Evidence) string { return fmt.Sprintf("%s:%d", ev.Hit.Path, ev.Hit.Line) }

// contractConflict: the definition the request touches is described differently by code and by a document
// or test. Sidecar cites both and does not choose an authority.
func contractConflict(text string, res retrieve.Result) *Gap {
	top := map[string]retrieve.Evidence{}
	n := 0
	for _, ev := range res.Evidence {
		if ev.Note != "" || n >= 4 {
			continue
		}
		top[ev.Hit.Name] = ev
		n++
	}
	for _, f := range res.Trace.Contracts {
		if f.Status != retrieve.ConflictCandidate {
			continue
		}
		fields := strings.Fields(f.Subject)
		if len(fields) < 2 {
			continue
		}
		ev, ok := top[fields[1]]
		if !ok {
			continue
		}
		where := fmt.Sprintf("%s:%d", f.Path, f.Line)
		// A value stated in the request settles a numeric conflict: both sides are updated to it, and
		// which source was right no longer changes the patch.
		uncertainty := 1.0
		if hasNumber.MatchString(text) && hasNumber.MatchString(f.Claim) {
			uncertainty = 0.2
		}
		return &Gap{State: ContractConflict,
			Decision: fmt.Sprintf("which source is authoritative for %s: the code or %s", fields[1], f.Path),
			Evidence: []string{loc(ev), where},
			Options: []Option{
				{Label: "follow the code", Consequence: "keep " + loc(ev) + " as is; update " + f.Path + " and its claim (" + f.Claim + ")"},
				{Label: "follow the document", Consequence: "change " + loc(ev) + " to match " + f.Path + " (" + f.Claim + "); tests that encode the old value change too"},
			},
			Impact: 1.0, Uncertainty: uncertainty, Rework: 0.5, ChangesTests: true}
	}
	return nil
}

// ambiguousTarget: no exact identifier, and two comparably strong candidates in different files.
func ambiguousTarget(res retrieve.Result) *Gap {
	var cands []retrieve.Evidence
	for _, ev := range res.Evidence {
		if ev.Note != "" || ev.Hit.Kind == "file" || ev.Hit.Kind == "section" || isTestPath(ev.Hit.Path) {
			continue
		}
		if ev.Source == "exact" || ev.Verdict == "VERIFIED" {
			return nil // the prompt names its target
		}
		if ev.Source == "structural" || ev.Source == "git" {
			continue
		}
		cands = append(cands, ev)
	}
	if len(cands) < 2 || cands[0].Confidence < 0.5 || cands[1].Confidence < 0.5 {
		return nil
	}
	a, b := cands[0], cands[1]
	if a.Hit.Path == b.Hit.Path || a.Score <= 0 || b.Score/a.Score < 0.85 {
		return nil
	}
	opts := []Option{{Label: a.Hit.Name, Consequence: "edit " + loc(a)}, {Label: b.Hit.Name, Consequence: "edit " + loc(b)}}
	ev := []string{loc(a), loc(b)}
	if len(cands) > 2 && cands[2].Hit.Path != a.Hit.Path && cands[2].Hit.Path != b.Hit.Path &&
		cands[2].Confidence >= 0.5 && cands[2].Score/a.Score >= 0.85 {
		opts = append(opts, Option{Label: cands[2].Hit.Name, Consequence: "edit " + loc(cands[2])})
		ev = append(ev, loc(cands[2]))
	}
	return &Gap{State: Ambiguous, Decision: "which definition the change targets: the request does not name one and several fit equally well",
		Evidence: ev, Options: opts, Impact: 0.8, Uncertainty: 0.6, Rework: 1.0, ChangesTests: true}
}

// missingLimit: the request introduces or changes a limit but states no value. If the repository already
// carries a constant for it, the repository answers and nothing is asked.
func missingLimit(prompt, text string, res retrieve.Result) *Gap {
	m := limitTerm(prompt, text)
	if m == "" || hasNumber.MatchString(text) {
		return nil
	}
	g := &Gap{State: Underspecified, Decision: fmt.Sprintf("the value of the %s the request refers to is not stated", m),
		Evidence: []string{"prompt words: " + m + " (no number given)"},
		Options: []Option{
			{Label: "you give the value", Consequence: "implement with that value and test its boundaries"},
			{Label: "conservative default", Consequence: "add a named constant with a default; tests pin the default, easy to change later"},
		}, Impact: 0.7, Uncertainty: 1.0, Rework: 0.5, ChangesTests: true}
	changing := changeVerb.MatchString(text)
	for _, ev := range res.Evidence {
		if changing {
			// The request changes an existing limit without saying the new value: the constant in the code is the
			// old value and cannot answer it. Cite it so the question can state where things stand.
			if ev.Note == "" && (ev.Hit.Kind == "constant" || ev.Hit.Kind == "field") && ev.Hit.Value != "" && nameIsLimit(ev.Hit.Name) {
				g.Evidence = append(g.Evidence, fmt.Sprintf("current: %s = %s (%s)", ev.Hit.Name, ev.Hit.Value, loc(ev)))
				break
			}
			continue
		}
		if ev.Note == "" && (ev.Hit.Kind == "constant" || ev.Hit.Kind == "field") && ev.Hit.Value != "" && nameIsLimit(ev.Hit.Name) {
			g.ResolvedByRepo = fmt.Sprintf("%s = %s (%s)", ev.Hit.Name, ev.Hit.Value, loc(ev))
			g.Uncertainty = 0
			break
		}
	}
	return g
}

var (
	addPolarity    = regexp.MustCompile(`\b(adicion\w*|incluir|inclua|manter|mantenha|keep|add|include|enable|habilit\w*|permit\w*|allow)\b`)
	removePolarity = regexp.MustCompile(`\b(remov\w*|exclu\w*|delete|drop|tire|retire|desabilit\w*|disable|stop|pare de|nao (use|usar|mantenha|manter|inclua)|do not (use|keep|include)|don'?t (use|keep|include)|without|sem)\b`)
	clauseSplit    = regexp.MustCompile(`[.;!?\n]|\b(mas|porem|but|however|though)\b`)
)

// contradiction: two clauses give opposite directives about a shared content term. Low precision by
// design, so it carries lower uncertainty than a contract conflict.
func contradiction(text string) *Gap {
	var clauses []string
	for _, c := range clauseSplit.Split(text, -1) {
		if c = strings.TrimSpace(c); len(c) > 6 {
			clauses = append(clauses, c)
		}
	}
	for i := 0; i < len(clauses); i++ {
		for j := i + 1; j < len(clauses); j++ {
			pi, pj := polarity(clauses[i]), polarity(clauses[j])
			if pi == 0 || pj == 0 || pi == pj {
				continue
			}
			if shared := sharedTerm(clauses[i], clauses[j]); shared != "" {
				return &Gap{State: Contradictory, Decision: fmt.Sprintf("the request both asks to add/keep and to remove/avoid %q", shared),
					Evidence: []string{"prompt: \"" + clip(clauses[i], 80) + "\"", "prompt: \"" + clip(clauses[j], 80) + "\""},
					Impact:   0.9, Uncertainty: 0.8, Rework: 0.75, ChangesTests: true}
			}
		}
	}
	return nil
}

func polarity(c string) int {
	switch {
	case removePolarity.MatchString(c):
		return -1
	case addPolarity.MatchString(c):
		return 1
	}
	return 0
}

func sharedTerm(a, b string) string {
	set := map[string]bool{}
	for _, t := range lexical.Analyze(a) {
		if len(t) >= 4 {
			set[t] = true
		}
	}
	best := ""
	for _, t := range lexical.Analyze(b) {
		if set[t] && len(t) > len(best) && !editVerbs.MatchString(t) && !addPolarity.MatchString(t) && !removePolarity.MatchString(t) {
			best = t
		}
	}
	return best
}

// Signals read from the prompt that change the cost of interrupting.
func Signals(prompt string) (urgentPrompt, noAskPrompt bool) {
	t := lexical.Fold(prompt)
	return urgent.MatchString(t), noAsk.MatchString(t)
}

// nameIsLimit reports whether an identifier (camelCase, snake_case) contains a limit word as a term.
func nameIsLimit(name string) bool {
	for _, t := range lexical.Analyze(name) {
		if limitWords.MatchString(t) {
			return true
		}
	}
	return false
}

// limitTerm finds a limit word in the prompt, also inside identifiers (SessionTTL, MaxRetries), which the
// folded text keeps as one word.
func limitTerm(prompt, text string) string {
	if m := limitWords.FindString(text); m != "" {
		return m
	}
	for _, t := range lexical.Analyze(prompt) {
		if limitWords.MatchString(t) {
			return t
		}
	}
	return ""
}
