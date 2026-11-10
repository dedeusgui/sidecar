// Package explore keeps the on-demand CLI adapter for the shared read-only exploration traversal.
// The V3 router stays here for compatibility; library clients use the leaf service package.
package explore

import (
	"context"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/explore/core"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Schema versions the JSON contract (docs/explore.md).
const Schema = core.Schema

const (
	Verified  = core.Verified
	Candidate = core.Candidate
	Unknown   = core.Unknown
)

// These aliases preserve the public sidecar-explore/1 Go API.
type Options = core.Options
type Symbol = core.Symbol
type Snippet = core.Snippet
type File = core.File
type Ref = core.Ref
type Edge = core.Edge
type Budget = core.Budget
type Result = core.Result

// BudgetFor scales the output to repository size. Kept as a compatibility alias for the CLI.
func BudgetFor(repoFiles int) (maxFiles, maxChars int) { return core.BudgetFor(repoFiles) }

// Run explores the repository for one query. It retains the existing V3 decision and V2 fallback
// policy; only the read-only source traversal is shared with the library service APIs.
func Run(ctx context.Context, st *store.Store, opts Options) (Result, error) {
	start := time.Now()
	recs, err := st.FileRecords()
	if err != nil {
		return core.EmptyResult(opts, nil), err
	}
	out := core.EmptyResult(opts, recs)

	ro := opts.Retrieve
	ro.Root = opts.Root
	ro.MaxItems = 12
	ro.MaxPerFile = 4
	ro.Profile = "deep"
	ro.NoExactDominance = true
	ro.ExtraExactKeys = core.MemberKeys(opts.Query)
	if words := strings.Fields(opts.Query); len(words) <= 3 {
		// A bare name ("Settle", "OpenGate handler") asked of explore is a symbol lookup.
		for _, w := range words {
			if w = strings.Trim(w, "`'\"(),;:?!"); len(w) >= 3 && core.IsIdent(w) {
				ro.ExtraExactKeys = append(ro.ExtraExactKeys, w)
			}
		}
	}
	res, dec, err := engine.Run(ctx, st, opts.Query, engine.Options{Retrieve: ro})
	if err != nil {
		return out, err
	}
	relevance, class, intent := string(dec.Relevance), string(dec.Class), dec.Signals.Intent.Kinds
	out.Relevance, out.Class, out.Intent = relevance, class, intent
	var gaps []string
	if dec.Action == engine.ActionAbstain {
		relevance = string(engine.Irrelevant)
		gaps = append(gaps, "the relevance gate found no repository evidence for this query: "+strings.Join(dec.AbstainReasons, "; "))
		// The tool was asked explicitly, so rerun without the gate and report candidates as such.
		r2, searchErr := retrieve.Search(ctx, st, opts.Query, withV2(ro))
		if searchErr != nil {
			out.Relevance = relevance
			out.Gaps = append(out.Gaps, gaps...)
			return out, searchErr
		}
		res = r2
	} else if len(res.Evidence) < 4 {
		if r2, searchErr := retrieve.Search(ctx, st, opts.Query, withV2(ro)); searchErr == nil && len(r2.Evidence) > len(res.Evidence) {
			res = r2 // explore wants the wider V2 pool; the router budget applies to the hook, not here
		}
	}
	return core.Build(ctx, st, opts, core.BuildInput{
		Evidence: res, Relevance: relevance, Class: class, Intent: intent,
		Gaps: gaps, Records: recs, Started: start,
	})
}

func memberKeys(q string) []string { return core.MemberKeys(q) }

func withV2(o retrieve.Options) retrieve.Options {
	o.EngineVersion = "v2"
	o.Adaptive = false
	return o
}
