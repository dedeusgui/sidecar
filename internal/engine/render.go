package engine

import (
	"strconv"
	"strings"

	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// Render builds the block the hook sends for a decision, without session memory. ABSTAIN renders
// nothing. BRIEF and VERIFY use the Task Evidence Brief when the evidence supports one; LEAN keeps the
// standard block, already cut to the lean budget by Run.
func Render(res retrieve.Result, d Decision, seen ctxbuild.Seen) string {
	if d.Action == ActionAbstain || len(res.Evidence) == 0 {
		return ""
	}
	if useBrief(d) {
		if text, ok := brief.RenderTask(res, seen); ok {
			return text
		}
	}
	return brief.Render(res, seen)
}

// RenderSession is Render with the per-session memory of brief, so repeated evidence is not resent.
func RenderSession(res retrieve.Result, d Decision, statePath string) (string, []string) {
	if d.Action == ActionAbstain || len(res.Evidence) == 0 {
		return "", nil
	}
	if useBrief(d) {
		if text, keys, ok := brief.RenderTaskSession(res, statePath); ok {
			return text, keys
		}
	}
	return brief.RenderSession(res, statePath)
}

// useBrief is false by default: B12 favoured the brief but B15 did not repeat it, so the V3 router
// sends the standard block unless SIDECAR_FEATURES=brief asks for the experiment.
func useBrief(d Decision) bool {
	return (d.Action == ActionBrief || d.Action == ActionVerify) && retrieve.FeatureEnabled("brief")
}

// WithExploreHint appends one line offering on-demand exploration to a non-empty block when the decision
// asks for it. It never forces a call: EXPLORE_AVAILABLE only makes the tool known. exe is the command the
// agent can run (the hook's own executable), quoted when it contains spaces.
func WithExploreHint(block string, d Decision, exe string) string {
	const end = "[END SIDECAR CONTEXT]"
	if block == "" || !d.ExploreHint || exe == "" || !strings.HasSuffix(strings.TrimSpace(block), end) {
		return block
	}
	if strings.ContainsAny(exe, " \t") {
		exe = strconv.Quote(exe)
	}
	hint := "ON DEMAND: `" + exe + " explore --query \"<symbol, path or question>\"` returns numbered source, callers, tests and gaps for one topic.\n"
	i := strings.LastIndex(block, end)
	return block[:i] + hint + block[i:]
}
