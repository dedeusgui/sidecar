package retrieve

import (
	"os"
	"regexp"
	"strings"

	"github.com/dedeusgui/sidecar/internal/lexical"
)

// Policy says which optional analyses a request can benefit from. It is a pure function of the
// intent, so it is deterministic, explainable (Trace.PolicySkipped) and free of repository state.
type Policy struct {
	// SkipContracts skips contract links, conflict notes and documentation evidence. They answer
	// "what must this change preserve?", which a plain locate or general question does not ask.
	SkipContracts bool
}

// editVerbs recognises a request to change something, in Portuguese and English. The intent classifier does not:
// "Change the session TTL" reads as general. It is shared with the clarification policy, so both agree on what an
// edit request is.
const editVerbAlts = `adicion\w*|acrescent\w*|cri(e|ar|a)|implement\w*|alter(e|ar|a)|mud(e|ar|a)|modific\w*|troc(e|ar|a)|substitu\w*|remov\w*|exclu\w*|refator\w*|corrij\w*|conserte|migr(e|ar|a)|renome\w*|permit\w*|habilit\w*|desabilit\w*|aument\w*|reduz\w*|diminu\w*|ajust\w*|increase|decrease|reduce|raise|lower|adjust|bump|add|create|implement|change|update|modify|replace|remove|delete|refactor|fix|migrate|rename|support|enable|disable|make|allow`

var editVerbs = regexp.MustCompile(`\b(` + editVerbAlts + `)\b`)

// negatedEdit matches an edit verb the user forbids ("não altere nada", "don't change anything"). Such a prompt is read-only.
var negatedEdit = regexp.MustCompile(`\b(nao|not|never|nunca|don'?t|dont|do not|sem|without)\s+(\w+\s+){0,2}(` + editVerbAlts + `)\b`)

// IsEditRequest reports whether the prompt asks to change code or configuration.
func IsEditRequest(prompt string) bool {
	text := negatedEdit.ReplaceAllString(lexical.Fold(prompt), " ")
	return editVerbs.MatchString(text)
}

// PolicyFor derives the policy. Any intent that edits, tests, explains, debugs or asks about impact
// or contracts keeps every analysis: the cost saving applies only to pure lookups, never to an edit request
// (whose contracts decide what the change must preserve), whatever the intent classifier made of it.
func PolicyFor(in Intent, prompt string) Policy {
	if len(in.Kinds) == 0 || IsEditRequest(prompt) {
		return Policy{}
	}
	for _, k := range in.Kinds {
		if k != "locate" && k != "general" {
			return Policy{}
		}
	}
	return Policy{SkipContracts: true}
}

// FeatureEnabled reports whether name is listed in SIDECAR_FEATURES (comma separated). Features are
// opt-in experiments: unset means off, and unknown names are ignored.
func FeatureEnabled(name string) bool {
	for _, f := range strings.Split(os.Getenv("SIDECAR_FEATURES"), ",") {
		if strings.TrimSpace(f) == name {
			return true
		}
	}
	return false
}
