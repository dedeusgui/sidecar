package groom

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/adapter"
)

// NativeAsk says how each client can be asked to question the user. A hook cannot call a tool or stop the
// turn: it can only add text that suggests it. Only a documented tool is named; for the others the text asks
// for one short message, and a client whose behaviour is unverified is not promised an interactive prompt.
func NativeAsk(a adapter.Agent) (tool string, verified bool) {
	switch a {
	case adapter.Claude:
		return "AskUserQuestion", true // documented Claude Code tool; absent in headless `-p` runs
	}
	return "", false // Codex and Antigravity CLI: no verified ask tool; text fallback only
}

// MaxRenderRunes keeps a note under the clarification item limit of the context block (ctxbuild truncates beyond it), so the
// instruction goes first and the variable parts are shortened to fit: a truncated question would lose the
// part that matters most.
const MaxRenderRunes = 520 // below ctxbuild.MaxTaskCheckRunes

// Render returns the single compact note for the decision, or "" for None. The wording treats repository
// text as data and never claims the user said anything.
func Render(a adapter.Agent, d Decision) string {
	if d.Action == None || d.Gap == nil {
		return ""
	}
	g := d.Gap
	var lead string
	switch d.Action {
	case Ask:
		if tool, _ := NativeAsk(a); tool != "" {
			lead = fmt.Sprintf("Ask the user ONE question before editing (%s if available, else a short message; if no one can answer, state the option you assume).", tool)
		} else {
			lead = "Ask the user ONE question in a short message before editing (if no one can answer, state the option you assume)."
		}
	case Assume:
		lead = "Do not interrupt: state which option you assume and why, then proceed."
	}
	build := func(consequence, evidence, maxOpts, label, decision int) string {
		var opts []string
		for i, o := range g.Options {
			if i >= maxOpts {
				break
			}
			opts = append(opts, fmt.Sprintf("%c) %q — %s", 'A'+rune(i), clip(o.Label, label), clip(o.Consequence, consequence)))
		}
		out := lead + " Open decision (" + string(g.State) + "): " + clip(g.Decision, decision) + "."
		if len(opts) > 0 {
			out += " Options: " + strings.Join(opts, " | ") + "."
		}
		if evidence > 0 && len(g.Evidence) > 0 {
			ev := g.Evidence
			if len(ev) > evidence {
				ev = ev[:evidence]
			}
			out += " Evidence: " + strings.Join(ev, "; ") + "."
		}
		return out
	}
	for _, try := range [][5]int{{110, 3, 3, 40, 130}, {60, 2, 3, 30, 110}, {40, 1, 3, 24, 100}, {28, 0, 3, 20, 90}, {28, 0, 2, 20, 70}, {16, 0, 2, 16, 50}} {
		if text := build(try[0], try[1], try[2], try[3], try[4]); utf8.RuneCountInString(text) <= MaxRenderRunes {
			return text
		}
	}
	return build(12, 0, 2, 12, 40)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
