package retrieve

import (
	"os"
	"strings"
)

// Policy says which optional analyses a request can benefit from. It is a pure function of the
// intent, so it is deterministic, explainable (Trace.PolicySkipped) and free of repository state.
type Policy struct {
	// SkipContracts skips contract links, conflict notes and documentation evidence. They answer
	// "what must this change preserve?", which a plain locate or general question does not ask.
	SkipContracts bool
}

// PolicyFor derives the policy. Any intent that edits, tests, explains, debugs or asks about impact
// or contracts keeps every analysis: the cost saving applies only to pure lookups.
func PolicyFor(in Intent) Policy {
	if len(in.Kinds) == 0 {
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
