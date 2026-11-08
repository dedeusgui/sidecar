// Package progress classifies an agent's observed tool events into useful iteration, exploration and stagnation.
// It is pure and deterministic. It only sees events a hook delivered: a missing event is "unobserved", never proof
// that the agent did nothing, and no classification here interrupts or blocks anything. Intervention decisions are
// produced in shadow mode (what Sidecar would say and why) with the delivery limits already applied, so their
// rate and false-alarm cost can be measured before any transport exists.
package progress

import (
	"sort"
	"strings"
	"unicode"
)

// Event is one observed tool call, already reduced to what classification needs. Key is the normalised path for
// reads and edits, the normalised pattern for searches and the normalised message for errors. Nothing here is
// persisted by this package.
type Event struct {
	Kind string // search | read | edit | test | other
	Key  string
	Err  string // normalised error text for a failed call; "" otherwise
}

// Labels given to one event.
const (
	New           = "new"            // first time this key appears in the epoch
	ExactRepeat   = "exact_repeat"   // same search, same epoch
	NearRepeat    = "near_repeat"    // search with almost the same terms, same epoch
	Revisit       = "revisit"        // same file read again, same epoch
	RepeatedError = "repeated_error" // same error, no edit in between
	IteratedError = "iterated_error" // same error after an edit: useful iteration
	Progress      = "progress"       // an edit or a test run
	Neutral       = "neutral"
)

// Window states, over the last WindowSize events.
const (
	Exploring  = "exploring"  // mostly new keys
	Iterating  = "iterating"  // edits or tests present
	Stagnating = "stagnating" // several redundant events and no edit or test
)

// Policy bounds. They are uncalibrated starting points.
const (
	WindowSize       = 8
	StagnationRedund = 4 // redundant events in a window with no edit/test to call it stagnation
	NearRepeatSim    = 0.8
	ShadowGap        = 12 // events between two shadow decisions
	ShadowMax        = 3  // shadow decisions per session
)

// Redundant says whether a label is wasted work: a repeat with no new input.
func Redundant(label string) bool {
	switch label {
	case ExactRepeat, NearRepeat, Revisit, RepeatedError:
		return true
	}
	return false
}

// Shadow is a decision Sidecar would have delivered, recorded without delivering it.
type Shadow struct {
	At      int    `json:"at"`     // event index
	Reason  string `json:"reason"` // stagnation | repeated_error
	Window  string `json:"window"`
	Deliver bool   `json:"deliver"` // always false: there is no verified transport yet
}

// Report is the classification of a sequence.
type Report struct {
	Events          int            `json:"events"`
	Labels          map[string]int `json:"labels"`
	Redundant       int            `json:"redundant"`
	Exploratory     int            `json:"exploratory"`   // searches and reads
	NoveltyRatio    float64        `json:"novelty_ratio"` // new keys / exploratory events
	StagnantWindows int            `json:"stagnant_windows"`
	Longest         int            `json:"longest_stagnation"` // events in the longest run of stagnating windows
	Shadows         []Shadow       `json:"shadow_decisions"`
	Per             []string       `json:"per_event"`
}

// Analyze classifies events in order. An edit starts a new epoch: what was read or tried before a change no
// longer predicts a loop after it, so edit→read, edit→test and re-running a test after a fix are legitimate.
func Analyze(events []Event) Report {
	rep := Report{Labels: map[string]int{}, Events: len(events), Shadows: []Shadow{}}
	seenSearch := map[string]bool{}
	var searchSets [][]string
	reads := map[string]bool{}
	errEpoch := map[string]int{} // normalised error -> epoch of its last occurrence
	epoch := 0
	newKeys, exploratory := 0, 0
	var recent []string
	lastShadow, run, shadowCount := -ShadowGap, 0, 0
	for i, e := range events {
		label := Neutral
		switch e.Kind {
		case "edit":
			epoch++
			seenSearch, searchSets, reads = map[string]bool{}, nil, map[string]bool{}
			label = Progress
		case "test":
			label = Progress
		case "search":
			if e.Key == "" {
				break // a query with no usable terms says nothing: unobserved, never a repeat
			}
			exploratory++
			terms := termSet(e.Key)
			switch {
			case seenSearch[e.Key]:
				label = ExactRepeat
			case nearDuplicate(terms, searchSets):
				label = NearRepeat
			default:
				label = New
				newKeys++
			}
			seenSearch[e.Key] = true
			searchSets = append(searchSets, terms)
		case "read":
			if e.Key == "" {
				break
			}
			exploratory++
			if reads[e.Key] {
				label = Revisit
			} else {
				label = New
				newKeys++
			}
			reads[e.Key] = true
		}
		if e.Err != "" {
			if last, ok := errEpoch[e.Err]; ok {
				if last == epoch {
					label = RepeatedError
				} else if label == Neutral {
					label = IteratedError
				}
			}
			errEpoch[e.Err] = epoch
		}
		rep.Labels[label]++
		rep.Per = append(rep.Per, label)
		if Redundant(label) {
			rep.Redundant++
		}
		recent = append(recent, label)
		if len(recent) > WindowSize {
			recent = recent[1:]
		}
		state := windowState(recent)
		if state == Stagnating {
			rep.StagnantWindows++
			run++
			rep.Longest = max(rep.Longest, run)
		} else {
			run = 0
		}
		reason := ""
		switch {
		case state == Stagnating:
			reason = "stagnation"
		case label == RepeatedError && rep.Labels[RepeatedError] >= 2:
			reason = "repeated_error"
		}
		if reason != "" && i-lastShadow >= ShadowGap && shadowCount < ShadowMax {
			rep.Shadows = append(rep.Shadows, Shadow{At: i, Reason: reason, Window: state})
			lastShadow = i
			shadowCount++
		}
	}
	rep.Exploratory = exploratory
	if exploratory > 0 {
		rep.NoveltyRatio = float64(newKeys) / float64(exploratory)
	}
	return rep
}

func windowState(labels []string) string {
	redundant, progress, fresh := 0, 0, 0
	for _, l := range labels {
		switch {
		case Redundant(l):
			redundant++
		case l == Progress || l == IteratedError:
			progress++
		case l == New:
			fresh++
		}
	}
	switch {
	case progress > 0:
		return Iterating
	case redundant >= StagnationRedund:
		return Stagnating
	}
	return Exploring
}

// termSet returns the sorted distinct lowercase terms of a query, so word order and punctuation do not hide a repeat.
func termSet(q string) []string {
	f := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	set := map[string]bool{}
	for _, t := range f {
		if len(t) >= 2 {
			set[t] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func nearDuplicate(terms []string, prior [][]string) bool {
	if len(terms) < 2 {
		return false // a one-word search has no room for "almost the same"
	}
	for _, p := range prior {
		if jaccard(terms, p) >= NearRepeatSim {
			return true
		}
	}
	return false
}

func jaccard(a, b []string) float64 {
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	inter := 0
	for _, t := range b {
		if set[t] {
			inter++
		}
	}
	union := len(set) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
