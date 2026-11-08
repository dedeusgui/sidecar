package groom

// Action is the decision for the highest-value gap.
type Action string

const (
	None   Action = "none"   // nothing worth saying
	Ask    Action = "ask"    // one question, with options
	Assume Action = "assume" // not worth an interruption: state the premise and proceed
)

// Policy holds the cost side of the decision. Cost is what an interruption costs relative to the value
// of settling a gap, so the same gap is asked in a calm session and assumed in an urgent one.
type Policy struct {
	InteractionCost float64 // base cost of one question
	UrgentExtra     float64 // added when the prompt declares urgency
	Margin          float64 // value must beat the cost by this much
	MaxPerSession   int     // questions asked per session, across prompts
	NonInteractive  bool    // CI or cloud without a user: never ask
}

// DefaultPolicy is an uncalibrated starting point, to be measured on the clarification benchmark.
func DefaultPolicy() Policy {
	return Policy{InteractionCost: 0.30, UrgentExtra: 0.25, Margin: 0.10, MaxPerSession: 2}
}

// Decision is the outcome of Decide.
type Decision struct {
	Action Action `json:"action"`
	Gap    *Gap   `json:"gap,omitempty"`
	Reason string `json:"reason"`
}

// Decide picks at most one gap. A gap the repository answers, an explicit "do not ask", a
// non-interactive session and an exhausted session quota all turn a question into an assumption or nothing.
func Decide(rep Report, prompt string, askedInSession int, p Policy) Decision {
	if len(rep.Gaps) == 0 {
		return Decision{Action: None, Reason: "no open decision"}
	}
	urgentPrompt, noAskPrompt := Signals(prompt)
	cost := p.InteractionCost
	if urgentPrompt {
		cost += p.UrgentExtra
	}
	for i := range rep.Gaps {
		g := &rep.Gaps[i]
		if g.ResolvedByRepo != "" {
			continue // the repository already answers
		}
		if g.Value < cost+p.Margin {
			continue
		}
		switch {
		case noAskPrompt:
			return Decision{Action: Assume, Gap: g, Reason: "the user asked not to be questioned; state the premise"}
		case p.NonInteractive:
			return Decision{Action: Assume, Gap: g, Reason: "non-interactive session; state the premise"}
		case askedInSession >= p.MaxPerSession:
			return Decision{Action: Assume, Gap: g, Reason: "question quota for the session is used; state the premise"}
		}
		return Decision{Action: Ask, Gap: g, Reason: "value beats the interruption cost"}
	}
	return Decision{Action: None, Reason: "open decisions are answered by the repository or not worth an interruption"}
}
