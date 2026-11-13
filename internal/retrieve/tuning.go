package retrieve

import "math"

// Tuning is the set of V2 ranking/abstention parameters that the offline replay (cmd/sidecar-eval
// --replay) may vary. The product always runs DefaultTuning: a nil Options.Tuning means defaults,
// and bench/replay/active.json is a versioned mirror of DefaultTuning that a test keeps in sync,
// so changing a default needs a visible, reviewable config change (rollback = revert both).
type Tuning struct {
	// OwnCoverCap bounds the distinct prompt terms a candidate must explain (the support denominator).
	OwnCoverCap float64 `json:"own_cover_cap"`
	// OwnAltWeight is the support of a translation-only match relative to the prompt's own word.
	OwnAltWeight float64 `json:"own_alt_weight"`
	// OwnConfBase and OwnConfSpan give confidence = base + span*support/denominator.
	OwnConfBase float64 `json:"own_conf_base"`
	OwnConfSpan float64 `json:"own_conf_span"`
	// MinBlockConfidence is the abstention gate: no block is sent unless one item reaches it.
	MinBlockConfidence float64 `json:"min_block_confidence"`
	// LongPromptTerms and LongPromptMinSupport are the length-aware specificity rule: a prompt with at
	// least LongPromptTerms distinct terms needs LongPromptMinSupport matched terms before a candidate
	// may open the block on its own. LongPromptTerms 0 disables the rule (the shipped default until a
	// replay on frozen external sets shows it helps without losing recall).
	LongPromptTerms      float64 `json:"long_prompt_terms"`
	LongPromptMinSupport float64 `json:"long_prompt_min_support"`
	// LongPromptExemptCoverage exempts a candidate whose salience-weighted coverage (rare terms weigh
	// more, see conceptSalience) reaches this value: a few rare matches are specific evidence even in a
	// long prompt. 0 means no exemption.
	LongPromptExemptCoverage float64 `json:"long_prompt_exempt_coverage"`
}

// DefaultTuning returns the shipped parameters. It is the single source for the constants below.
func DefaultTuning() Tuning {
	return Tuning{OwnCoverCap: ownCoverCap, OwnAltWeight: ownAltWeight, OwnConfBase: 0.3, OwnConfSpan: 0.6,
		MinBlockConfidence: minBlockConfidence, LongPromptTerms: 0, LongPromptMinSupport: 0, LongPromptExemptCoverage: 0}
}

// V37Tuning returns the deterministic V3.7 ranking and length-aware specificity parameters.
// DefaultTuning remains unchanged for V2/V3/V3.5/V3.6 compatibility.
func V37Tuning() Tuning {
	t := DefaultTuning()
	t.LongPromptTerms = 7
	t.LongPromptMinSupport = 2
	t.LongPromptExemptCoverage = 0.35
	return t
}

// Valid reports whether the parameters are usable; replay rejects invalid variants instead of running them.
func (t Tuning) Valid() bool {
	ok := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	return ok(t.OwnCoverCap) && t.OwnCoverCap >= 1 && ok(t.OwnAltWeight) && t.OwnAltWeight <= 2 &&
		ok(t.OwnConfBase) && ok(t.OwnConfSpan) && t.OwnConfBase+t.OwnConfSpan <= 0.95 &&
		ok(t.MinBlockConfidence) && t.MinBlockConfidence <= 1 && ok(t.LongPromptTerms) && ok(t.LongPromptMinSupport) && ok(t.LongPromptExemptCoverage)
}

func tuningOrDefault(t *Tuning) Tuning {
	if t == nil {
		return DefaultTuning()
	}
	return *t
}
