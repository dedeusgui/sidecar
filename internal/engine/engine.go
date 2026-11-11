package engine

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Version is the engine name written to logs and explain output.
const Version = "v3"

// Version35 selects the opt-in unified context transaction built on the V3 decision layer.
const Version35 = "v3.5"

// Version36 selects the V3.6 refinement of the unified transaction: same architecture and
// contracts as V3.5, with the delivery fixes recorded in docs/v3.6. V3.5 stays byte-frozen
// as the comparison baseline.
const Version36 = "v3.6"

// Unified reports whether version runs the unified (V3.5-family) context transaction.
func Unified(version string) bool { return version == Version35 || version == Version36 }

// Action is what the engine decided to deliver. They are distinct actions, not a bundle that is
// always on: ABSTAIN emits zero bytes, EXPLORE_AVAILABLE only offers the on-demand tool.
type Action string

const (
	ActionAbstain          Action = "ABSTAIN"
	ActionLean             Action = "LEAN"
	ActionBrief            Action = "BRIEF"
	ActionExploreAvailable Action = "EXPLORE_AVAILABLE"
	ActionClarify          Action = "CLARIFY"
	ActionVerify           Action = "VERIFY"
)

// Choice records why one evidence item was kept or dropped.
type Choice struct {
	ID         string  `json:"id"`
	Path       string  `json:"path"`
	Line       int     `json:"line"`
	Source     string  `json:"source"`
	Verdict    string  `json:"verdict"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Decision is the explainable output of the router.
type Decision struct {
	Engine          string      `json:"engine"`
	Action          Action      `json:"action"`
	Relevance       Relevance   `json:"relevance,omitempty"`
	LowConfidence   []string    `json:"low_confidence,omitempty"` // why an ambiguous prompt got only a minimal answer
	Class           TaskClass   `json:"class"`
	Budget          int         `json:"budget_tokens"`
	MaxItems        int         `json:"max_items"`
	Providers       []string    `json:"providers"`
	Skipped         []string    `json:"skipped_providers,omitempty"`
	Included        []Choice    `json:"included,omitempty"`
	Excluded        []Choice    `json:"excluded,omitempty"`
	AbstainReasons  []string    `json:"abstain_reasons,omitempty"`
	ExploreHint     bool        `json:"explore_hint"`
	EstimatedTokens int         `json:"estimated_tokens"`
	Signals         TaskSignals `json:"signals"`
	RetrievalRan    bool        `json:"retrieval_ran"`
	ElapsedMS       float64     `json:"elapsed_ms"`
}

// Options configures Run. Retrieve carries the V2 options (root, profile, seen, LSP hooks); the
// engine forces EngineVersion "v2" and the adaptive context policy on it.
type Options struct {
	Retrieve          retrieve.Options
	RequireCodeAnchor bool // V3.5 abstains when retrieval has no anchored code symbol.
	// Refined selects the V3.6 corrections on top of RequireCodeAnchor. The code-anchor gate also
	// matches a symbol name through the dictionary translation of a prompt term, as retrieval
	// already does (a Portuguese "pedido" anchors OrderService). The route's item limit is kept:
	// V3.5 raised it to 12 because it then cut the selection to one anchor; V3.6 delivers the
	// ranked selection, so the V3 limits per action apply again.
	Refined bool
}

// Budgets per action, in estimated tokens. Heuristics chosen before measurement; not fitted to the
// evaluation splits. LEAN is deliberately small: a narrow question needs the place, not the tree.
const (
	leanBudget      = 420
	leanItems       = 5
	briefBudget     = 800
	briefItems      = 6
	deepBudget      = 1200
	deepItems       = 8
	ambiguousBudget = 200
	ambiguousItems  = 2
	minLeanSupport  = 1
	minCueSupport   = 2
	minCueCoverage  = 0.5
	minPresentRatio = 0.34
	// Domain prompts without software words or a location cue must be mostly made of the repository's
	// own vocabulary. Chosen on the dev splits only (cases tune, ten-repos development, negatives dev).
	domainPresentRatio = 0.6
	strictPresentRatio = 0.75
)

// Run reads the prompt, retrieves with the V2 pipeline when the prompt can be about the repository,
// and returns the evidence to deliver (possibly none) with the decision that produced it.
func Run(ctx context.Context, st *store.Store, prompt string, opts Options) (retrieve.Result, Decision, error) {
	start := time.Now()
	sig := readSignals(prompt)
	d := Decision{Engine: Version, Signals: sig}
	finish := func(res retrieve.Result) (retrieve.Result, Decision, error) {
		d.ElapsedMS = float64(time.Since(start).Microseconds()) / 1000
		return res, d, nil
	}
	if sig.Conversational {
		d.Action, d.Class = ActionAbstain, ClassOffRepo
		d.AbstainReasons = append(d.AbstainReasons, sig.Reasons...)
		d.Skipped = []string{"retrieval"}
		return finish(retrieve.Result{})
	}

	ro := opts.Retrieve
	ro.EngineVersion = "v2"
	ro.Adaptive = true // contracts and docs only when the intent can use them (retrieve.PolicyFor)
	if opts.RequireCodeAnchor && ro.MaxItems < 60 {
		// V3.5 may favor implementation files over nearby docs and tests after retrieval. Keep a
		// full ranked window so that choice is made from evidence the V2 result limit hid.
		ro.MaxItems = 60
	}
	ro.PreferImplementation = opts.RequireCodeAnchor
	res, err := retrieve.Search(ctx, st, prompt, ro)
	if err != nil {
		return res, d, err
	}
	d.RetrievalRan = true
	d.Providers = providersOf(res)
	d.Skipped = append(d.Skipped, res.Trace.PolicySkipped...)
	verifyLocators(st, &sig, res)
	measureSupport(&sig, res)
	d.Signals = sig

	rel, reasons := relevanceGate(sig, res)
	if opts.RequireCodeAnchor && rel != Irrelevant && !hasCodeAnchor(sig, res, opts.Refined) {
		rel = Irrelevant
		reasons = append(reasons, "no explicit code anchor or matching symbol name")
	}
	d.Relevance = rel
	if rel == Irrelevant {
		d.Action, d.Class = ActionAbstain, ClassOffRepo
		d.AbstainReasons = reasons
		for _, ev := range res.Evidence {
			d.Excluded = append(d.Excluded, choiceOf(ev, "relevance gate: "+reasons[0]))
		}
		res.Evidence = nil
		res.Trace.Final = 0
		return finish(res)
	}
	if opts.RequireCodeAnchor {
		preferImplementationEvidence(res.Evidence)
	}

	d.Class = classify(sig, res)
	d.Action, d.Budget, d.MaxItems = route(d.Class, sig)
	if opts.RequireCodeAnchor && !opts.Refined && d.MaxItems < 12 {
		// The V3.5 output planner can choose among several independent source anchors under one
		// global token budget; extra items remain subject to that budget.
		d.MaxItems = 12
	}
	if rel == Ambiguous {
		d.Action, d.Budget, d.MaxItems = ActionLean, ambiguousBudget, ambiguousItems
		d.LowConfidence = reasons
	}
	if b, ok := retrieve.ProfileBudget(ro.Profile); ok && b < d.Budget {
		d.Budget = b // an explicit minimal profile still wins
	}
	res = pack(res, &d)
	d.Signals = sig
	return finish(res)
}

// preferImplementationEvidence puts production source ahead of nearby docs, tests and benchmark
// helpers for V3.5's bounded output. The original V2 and V3 rankings remain untouched.
func preferImplementationEvidence(evidence []retrieve.Evidence) {
	score := func(ev retrieve.Evidence) int {
		path := strings.ToLower(filepath.ToSlash(ev.Hit.Path))
		ext := filepath.Ext(path)
		if !strings.Contains(" .go .py .js .jsx .ts .tsx .rs .java .kt .cs .php .rb .swift .c .cc .cpp .h .hpp .ex .exs .scala ", " "+ext+" ") {
			return 0
		}
		s := 2
		if strings.Contains(path, "/bench/") || strings.Contains(path, "/test/") || strings.Contains(path, "/tests/") ||
			strings.Contains(path, "_test.") || strings.Contains(path, ".test.") || strings.Contains(path, ".spec.") {
			s--
		}
		return s
	}
	sort.SliceStable(evidence, func(i, j int) bool { return score(evidence[i]) > score(evidence[j]) })
}

// Relevance is the gate verdict.
type Relevance string

const (
	Relevant   Relevance = "relevant"
	Ambiguous  Relevance = "ambiguous"  // lexical support cannot tell a domain prompt from another subject
	Irrelevant Relevance = "irrelevant" // clear evidence the prompt is not about this repository
)

// relevanceGate abstains only on clear evidence of irrelevance. A false positive costs a few hundred
// tokens of noise; a false negative costs the agent its own search, so the band where lexical support
// cannot separate a terse domain prompt ("limite de cargas no despacho") from another subject ("what
// time does the store open on sundays") gets a minimal lean answer instead of silence or a full block.
func relevanceGate(s TaskSignals, res retrieve.Result) (Relevance, []string) {
	if len(res.Evidence) == 0 {
		return Irrelevant, []string{"no evidence passed the V2 gates"}
	}
	for _, l := range s.Locators {
		if l.Verified {
			return Relevant, nil // the prompt names something the repository has
		}
	}
	software := len(s.SoftwareTerms) > 0
	switch {
	case s.BestSupport == 0:
		return Irrelevant, []string{"no evidence item matches a prompt concept by its own text"}
	case software && (s.PresentRatio >= minPresentRatio || s.RepoCue || s.TopSupport >= 2):
		return Relevant, nil
	case !software && s.RepoCue && s.TopSupport >= minCueSupport && s.Coverage >= minCueCoverage:
		return Relevant, nil // a location question in the project's own vocabulary ("onde registra uma visita")
	case !software && (s.BestSupport >= 3 && s.PresentRatio >= domainPresentRatio || s.BestSupport >= 2 && s.PresentRatio >= strictPresentRatio):
		return Relevant, nil // a terse request in the repository's domain language ("confirmar o peso do caminhão")
	case !software && !s.RepoCue && s.BestSupport <= 1 && s.PresentRatio < 0.5:
		return Irrelevant, []string{"one generic term matches and most prompt terms are absent from the repository"}
	case !software && !s.RepoCue && s.PresentRatio < 0.3:
		return Irrelevant, []string{"most prompt terms are absent from the repository and nothing points at code"}
	}
	var reasons []string
	if !software {
		reasons = append(reasons, "no software vocabulary or verified code anchor")
	}
	if !s.RepoCue {
		reasons = append(reasons, "no location or project cue")
	}
	if s.PresentRatio < domainPresentRatio {
		reasons = append(reasons, "many prompt terms are absent from the repository")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "weak lexical support")
	}
	return Ambiguous, reasons
}

// classify picks the effort class from intent, edit verbs, contract risk and evidence spread.
func classify(s TaskSignals, res retrieve.Result) TaskClass {
	kinds := map[string]bool{}
	for _, k := range s.Intent.Kinds {
		kinds[k] = true
	}
	switch {
	case kinds["debug"] || hasLocatorKind(s, "stack_frame"):
		return ClassDebug
	case s.Edit && (s.ContractRisk || kinds["contract"] || kinds["impact"]):
		return ClassContractChange
	case s.Edit && (kinds["extend"] || s.Files >= 3 && s.TopSupport >= 3):
		return ClassMultiFile
	case s.Edit:
		return ClassEdit
	case kinds["impact"]:
		return ClassContractChange
	case kinds["locate"] && !kinds["explain"]:
		return ClassLocate
	default:
		return ClassSimpleQuery
	}
}

// route maps a class to the smallest useful action and its budget.
func route(c TaskClass, s TaskSignals) (Action, int, int) {
	switch c {
	case ClassLocate:
		return ActionLean, leanBudget, leanItems
	case ClassSimpleQuery:
		if hasKind(s, "explain") {
			// A flow question ("how does X route requests") needs a chain, not a list: keep the block lean and
			// offer on-demand exploration instead of pushing a large block on every such prompt.
			return ActionExploreAvailable, leanBudget, leanItems
		}
		return ActionLean, leanBudget, leanItems
	case ClassEdit:
		return ActionBrief, briefBudget, briefItems
	case ClassDebug:
		return ActionBrief, briefBudget, briefItems
	case ClassContractChange:
		return ActionVerify, deepBudget, deepItems
	case ClassMultiFile:
		return ActionExploreAvailable, deepBudget, deepItems
	}
	return ActionLean, leanBudget, leanItems
}

// pack keeps the strongest items under the decision budget, preserving the V2 order (already packed
// by utility) and dropping items whose own text matches no prompt concept when better ones exist.
func pack(res retrieve.Result, d *Decision) retrieve.Result {
	var kept []retrieve.Evidence
	used := 0
	for i, ev := range res.Evidence {
		switch {
		case len(kept) >= d.MaxItems:
			d.Excluded = append(d.Excluded, choiceOf(ev, "item limit for "+string(d.Action)))
			continue
		case used+ev.TokenEstimate > d.Budget && len(kept) > 0:
			d.Excluded = append(d.Excluded, choiceOf(ev, "budget"))
			continue
		case d.Action == ActionLean && i > 0 && len(ev.Matched) == 0 && ev.Note == "" && ev.Source != "exact":
			d.Excluded = append(d.Excluded, choiceOf(ev, "lean: matches no prompt concept by itself"))
			continue
		}
		kept = append(kept, ev)
		used += ev.TokenEstimate
		d.Included = append(d.Included, choiceOf(ev, reasonOf(ev)))
	}
	res.Evidence = kept
	res.Trace.Final = len(kept)
	res.Trace.Budget = d.Budget
	d.EstimatedTokens = used
	d.ExploreHint = d.Action == ActionExploreAvailable || d.Action == ActionVerify
	return res
}

func reasonOf(ev retrieve.Evidence) string {
	switch {
	case ev.Note != "":
		return "contract note"
	case ev.Source == "exact":
		return "named in the prompt"
	case len(ev.Matched) > 0:
		return "matches: " + strings.Join(ev.Matched, ", ")
	case len(ev.Relations) > 0:
		return "related: " + ev.Relations[0]
	}
	return "ranked by V2"
}

func choiceOf(ev retrieve.Evidence, reason string) Choice {
	return Choice{ID: ev.ID, Path: ev.Hit.Path, Line: ev.Hit.Line, Source: ev.Source, Verdict: ev.Verdict,
		Confidence: ev.Confidence, Reason: reason}
}

func providersOf(res retrieve.Result) []string {
	p := []string{"lexical"}
	if res.Trace.Exact > 0 {
		p = append(p, "exact")
	}
	if res.Trace.Structural > 0 {
		p = append(p, "structural")
	}
	if len(res.Trace.Contracts) > 0 {
		p = append(p, "contracts")
	}
	return p
}

// verifyLocators marks each locator the index resolves: a path that is an indexed file, or an
// identifier whose last segment names an evidence item or an exact match.
func verifyLocators(st *store.Store, s *TaskSignals, res retrieve.Result) {
	names := map[string]bool{}
	for _, ev := range res.Evidence {
		names[strings.ToLower(ev.Hit.Name)] = true
	}
	for i := range s.Locators {
		l := &s.Locators[i]
		switch l.Kind {
		case "path", "stack_frame":
			p := strings.TrimPrefix(strings.TrimPrefix(l.Text, "./"), "/")
			if j := strings.LastIndex(p, ":"); j > 0 && l.Kind == "stack_frame" {
				p = p[:j]
			}
			if _, ok, err := st.FileIDByPath(p); err == nil && ok {
				l.Verified = true
			} else if ids, err := st.FileIDsWithSuffix(p); err == nil && len(ids) > 0 {
				l.Verified = true
			}
		default:
			text := strings.TrimRight(l.Text, "?!")
			parts := strings.FieldsFunc(text, func(r rune) bool { return r == '.' || r == ':' || r == '#' || r == ' ' })
			if len(parts) == 0 {
				continue
			}
			last := strings.ToLower(parts[len(parts)-1])
			if names[last] || names[strings.ToLower(text)] {
				l.Verified = true
				continue
			}
			if hits, err := st.ExactSymbols([]string{store.KeyOf(parts[len(parts)-1])}, 1); err == nil && len(hits) > 0 {
				l.Verified = true
			}
		}
	}
}

// measureSupport fills the evidence-dependent signals.
func measureSupport(s *TaskSignals, res retrieve.Result) {
	s.PromptTerms = len(res.Trace.Terms)
	s.Concepts = res.Trace.Concepts
	if s.PromptTerms > 0 {
		s.PresentRatio = round2(float64(min(s.Concepts, s.PromptTerms)) / float64(s.PromptTerms))
	}
	union := map[string]bool{}
	files := map[string]bool{}
	for i, ev := range res.Evidence {
		files[ev.Hit.Path] = true
		if ev.Verdict == "VERIFIED" {
			s.Verified++
		}
		if i == 0 {
			s.TopConfidence = ev.Confidence
		}
		if len(ev.Matched) > s.BestSupport {
			s.BestSupport = len(ev.Matched)
		}
		if i < 3 {
			for _, m := range ev.Matched {
				union[m] = true
			}
		}
	}
	s.TopSupport = len(union)
	s.Files = len(files)
	if s.Concepts > 0 {
		s.Coverage = round2(float64(s.TopSupport) / float64(s.Concepts))
	}
	if s.Edit {
		for _, ev := range res.Evidence {
			if ev.Note != "" || strings.Contains(strings.ToLower(ev.Hit.Kind), "interface") {
				s.ContractRisk = true // a recorded conflict or an interface others implement
				break
			}
		}
	}
	sort.Strings(s.SoftwareTerms)
}

func hasKind(s TaskSignals, kind string) bool {
	for _, k := range s.Intent.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func hasLocatorKind(s TaskSignals, kind string) bool {
	for _, l := range s.Locators {
		if l.Kind == kind {
			return true
		}
	}
	return false
}

func round2(x float64) float64 { return float64(int(x*100+0.5)) / 100 }
