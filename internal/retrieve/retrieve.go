// Package retrieve turns a natural-language prompt into ranked repository evidence.
// Candidates come from FTS5/BM25, exact identifiers and structural neighbours of the
// strongest matches. Rankings are fused with reciprocal rank fusion and diversified per
// file. Every result carries the factors that produced it.
package retrieve

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/fusion"
	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Options tunes retrieval. Zero values select the defaults.
type Options struct {
	MaxItems   int // evidence items returned (default 8)
	MaxPerFile int // items per file (default 2)
	Candidates int // lexical candidates examined (default 60)
	Seeds      int // strongest matches expanded structurally (default 8)
	Ablate     Ablation
}

// Ablation switches off parts of the pipeline. It exists to measure each component's
// contribution (see cmd/sidecar-eval); the product always runs with the zero value.
type Ablation struct {
	NoTranslation bool // do not use PT→EN translations
	NoExact       bool // do not match identifiers exactly
	NoStructure   bool // do not expand structural neighbours
	NoGit         bool // do not use uncommitted state or co-change
	LexicalOnly   bool // return the raw lexical ranking, without gating or fusion
}

// Evidence is one item of retrieved context.
type Evidence struct {
	Hit       store.SymbolHit
	Verdict   string // VERIFIED | INFERRED | UNKNOWN
	Section   string // ctxbuild section the item belongs to
	Score     float64
	Matched   []string // query concepts matched by the item's own text
	Relations []string // structural links, e.g. "calls:Enqueue(same_file)"
	Dirty     bool     // the file has uncommitted changes in the working tree
}

// CandidateTrace explains one candidate for diagnostics.
type CandidateTrace struct {
	Name, Path, Kind string
	Lexical          int // 1-based BM25 rank, 0 when not lexical
	Coverage         float64
	Exact            bool
	Relations        []string
	Score            float64
	Kept             bool
	Reason           string
}

// Trace records every factor behind a retrieval, for `sidecar explain`.
type Trace struct {
	Terms        []string
	Phrases      []string
	Translations map[string][]string
	Concepts     int
	Lexical      int
	Exact        int
	Structural   int
	Final        int
	Budget       int // suggested token budget for the context block
	Timings      map[string]time.Duration
	Candidates   []CandidateTrace
}

// Result is the outcome of Search.
type Result struct {
	Evidence []Evidence
	Trace    Trace
}

// concept is one prompt term, its repository-present translations, and whether the
// term itself occurs in the repository.
type concept struct {
	term        string
	termPresent bool
	alts        []string
	weight      float64 // inverse document frequency: rare terms discriminate more
}

type candidate struct {
	hit          store.SymbolHit
	lexRank      int
	exact        bool
	literal      bool
	matched      []string
	coverage     float64
	rels         []string
	neighbor     bool
	seedCoverage float64
	relBasis     []string
	git          bool
	reason       string
	suppressed   bool // dropped before fusion; never shown or ranked
	score        float64
}

// Search ranks evidence for a prompt. An empty prompt or one without repository matches
// returns no evidence.
func Search(ctx context.Context, st *store.Store, prompt string, opts Options) (Result, error) {
	opts = withDefaults(opts)
	res := Result{Trace: Trace{Translations: map[string][]string{}, Timings: map[string]time.Duration{}}}
	start := time.Now()

	t := time.Now()
	terms := lexical.Analyze(prompt)
	res.Trace.Terms = terms
	res.Trace.Phrases = lexical.ExpandPhrases(prompt)
	total, err := st.SymbolCount()
	if err != nil {
		return res, err
	}
	concepts, err := buildConcepts(st, terms, res.Trace.Phrases, total, res.Trace.Translations, opts.Ablate.NoTranslation)
	if err != nil {
		return res, err
	}
	res.Trace.Concepts = len(concepts)
	res.Trace.Timings["analyze"] = time.Since(t)

	t = time.Now()
	cands := map[int64]*candidate{}
	var lexOrder []int64
	if match := ftsMatch(concepts); match != "" {
		hits, err := st.SearchSymbols(match, opts.Candidates)
		if err != nil {
			return res, err
		}
		for i, h := range hits {
			c := ensure(cands, h)
			if c.lexRank == 0 {
				c.lexRank = i + 1
				lexOrder = append(lexOrder, h.ID)
			}
		}
	}
	res.Trace.Lexical = len(lexOrder)
	res.Trace.Timings["lexical"] = time.Since(t)

	t = time.Now()
	var exactHits []store.SymbolHit
	if !opts.Ablate.NoExact {
		exactHits, err = st.ExactSymbols(identifierKeys(prompt), 20)
		if err != nil {
			return res, err
		}
	}
	for _, h := range exactHits {
		c := ensure(cands, h)
		c.exact = true
	}
	res.Trace.Exact = len(exactHits)
	for _, c := range cands {
		scoreOwnText(c, concepts)
	}
	res.Trace.Timings["exact"] = time.Since(t)

	if opts.Ablate.LexicalOnly {
		for i, id := range lexOrder {
			if i >= opts.MaxItems {
				break
			}
			c := cands[id]
			res.Evidence = append(res.Evidence, Evidence{Hit: c.hit, Verdict: "INFERRED", Section: ctxbuild.Code, Score: 1 / float64(i+1), Matched: c.matched})
		}
		res.Trace.Final = len(res.Evidence)
		res.Trace.Budget = budgetFor(res.Evidence)
		res.Trace.Timings["total"] = time.Since(start)
		return res, nil
	}

	t = time.Now()
	seeds := pickSeeds(cands, opts.Seeds)
	added := 0
	if !opts.Ablate.NoStructure {
		added, err = expand(st, seeds, cands, !opts.Ablate.NoGit)
		if err != nil {
			return res, err
		}
	}
	res.Trace.Structural = added
	res.Trace.Timings["structural"] = time.Since(t)

	t = time.Now()
	var dirty []string
	if !opts.Ablate.NoGit {
		dirty, err = st.DirtyPaths()
		if err != nil {
			return res, err
		}
	}
	dirtySet := map[string]bool{}
	for _, p := range dirty {
		dirtySet[p] = true
	}
	for _, c := range cands {
		c.git = dirtySet[c.hit.Path]
	}
	for _, c := range cands {
		c.reason = gate(c)
	}
	kept := fuse(cands, lexOrder, opts)
	res.Trace.Timings["fusion"] = time.Since(t)

	for _, c := range kept {
		res.Evidence = append(res.Evidence, evidenceOf(c))
	}
	res.Trace.Final = len(res.Evidence)
	res.Trace.Budget = budgetFor(res.Evidence)
	res.Trace.Candidates = traceOf(cands)
	res.Trace.Timings["total"] = time.Since(start)
	return res, nil
}

func withDefaults(o Options) Options {
	if o.MaxItems <= 0 {
		o.MaxItems = 8
	}
	if o.MaxPerFile <= 0 {
		o.MaxPerFile = 2
	}
	if o.Candidates <= 0 {
		o.Candidates = 60
	}
	if o.Seeds <= 0 {
		o.Seeds = 8
	}
	return o
}

// buildConcepts keeps only prompt terms that can discriminate: present in the repository
// (or with a present translation) and not so common that they match a large share of symbols.
func buildConcepts(st *store.Store, terms, phrases []string, total int, trans map[string][]string, noTranslation bool) ([]concept, error) {
	// No frequency cut-off: a topical word (owner, in a repository about owners) is still the
	// best discriminator available. Common terms are down-weighted by IDF and BM25 instead.
	maxDF := total + 1
	var out []concept
	for _, term := range terms {
		present, err := st.TermDocFreq(term)
		if err != nil {
			return nil, err
		}
		df, err := st.TermSymbolFreq(term)
		if err != nil {
			return nil, err
		}
		c := concept{term: term}
		if present > 0 && df <= maxDF {
			c.termPresent = true
		}
		translations := lexical.Translations(term)
		if noTranslation {
			translations = nil
		}
		for _, en := range translations {
			epresent, err := st.TermDocFreq(en)
			if err != nil {
				return nil, err
			}
			edf, err := st.TermSymbolFreq(en)
			if err != nil {
				return nil, err
			}
			if epresent > 0 && edf <= maxDF {
				c.alts = append(c.alts, en)
			}
		}
		if len(c.alts) > 0 {
			trans[term] = c.alts
		}
		c.weight = idf(df, total)
		if c.termPresent || len(c.alts) > 0 {
			out = append(out, c)
		}
	}
	for _, en := range phrases {
		if noTranslation {
			break
		}
		present, err := st.TermDocFreq(en)
		if err != nil {
			return nil, err
		}
		df, err := st.TermSymbolFreq(en)
		if err != nil {
			return nil, err
		}
		if present > 0 && df <= maxDF {
			out = append(out, concept{term: en, termPresent: true, weight: idf(df, total)})
			trans["phrase"] = append(trans["phrase"], en)
		}
	}
	return out, nil
}

// ftsMatch builds an OR query over every present term and translation. Coverage is
// computed separately, so the FTS expression only has to find candidates.
func ftsMatch(concepts []concept) string {
	var parts []string
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] && isWord(t) {
			seen[t] = true
			parts = append(parts, `"`+t+`"`)
		}
	}
	for _, c := range concepts {
		if c.termPresent {
			add(c.term)
		}
		for _, a := range c.alts {
			add(a)
		}
	}
	return strings.Join(parts, " OR ")
}

func isWord(t string) bool {
	for _, r := range t {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r <= 127 {
			return false
		}
	}
	return t != ""
}

var identRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// identifierKeys finds code-like identifiers in the prompt for exact matching. A token
// counts only when it carries code signals (underscore, digit or an inner capital), so
// plain words that happen to equal a table or file name stay ordinary lexical matches.
func identifierKeys(prompt string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, tok := range identRe.FindAllString(prompt, -1) {
		if !codeLike(tok) || len(lexical.Analyze(tok)) == 0 {
			continue
		}
		k := store.KeyOf(tok)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

func codeLike(tok string) bool {
	if strings.ContainsAny(tok, "_0123456789") {
		return true
	}
	for i, r := range tok {
		if i > 0 && r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func ensure(cands map[int64]*candidate, h store.SymbolHit) *candidate {
	if c, ok := cands[h.ID]; ok {
		return c
	}
	c := &candidate{hit: h}
	cands[h.ID] = c
	return c
}

// hitTerms are the analyzed words of a symbol's own text.
func hitTerms(h store.SymbolHit) map[string]bool {
	set := map[string]bool{}
	for _, text := range []string{h.Name, h.Path, h.Signature, h.Doc} {
		for _, t := range lexical.Analyze(text) {
			set[t] = true
		}
	}
	return set
}

// scoreOwnText records which concepts the symbol's own name, path, signature or doc match.
// A match on the name itself is literal; a match only through a translation is not.
func scoreOwnText(c *candidate, concepts []concept) {
	if len(concepts) == 0 {
		return
	}
	all := hitTerms(c.hit)
	nameSet := map[string]bool{}
	for _, t := range lexical.Analyze(c.hit.Name) {
		nameSet[t] = true
	}
	c.matched = nil
	c.literal = false
	for _, cp := range concepts {
		switch {
		case cp.termPresent && all[cp.term]:
			c.matched = append(c.matched, cp.term)
			if nameSet[cp.term] {
				c.literal = true
			}
		default:
			// A translation match is a match, but never a literal one: only the prompt's own
			// word appearing in the name counts as VERIFIED or as uncommitted-state boost.
			for _, a := range cp.alts {
				if all[a] {
					c.matched = append(c.matched, cp.term+"→"+a)
					break
				}
			}
		}
	}
	var matchedWeight, totalWeight float64
	for _, cp := range concepts {
		totalWeight += cp.weight
	}
	for _, m := range c.matched {
		term := strings.SplitN(m, "→", 2)[0]
		for _, cp := range concepts {
			if cp.term == term {
				matchedWeight += cp.weight
				break
			}
		}
	}
	if totalWeight > 0 {
		c.coverage = matchedWeight / totalWeight
	}
}

// idf weights a term by how rarely it occurs among the indexed symbols.
func idf(df, total int) float64 {
	if total <= 0 {
		return 1
	}
	return math.Log(float64(total+1)/float64(df+1)) + 1
}

// pickSeeds chooses the strongest matches to expand: exact first, then coverage, then BM25.
func pickSeeds(cands map[int64]*candidate, n int) []*candidate {
	var seeds []*candidate
	for _, c := range cands {
		if c.exact || c.coverage > 0 {
			seeds = append(seeds, c)
		}
	}
	sort.Slice(seeds, func(i, j int) bool {
		a, b := seeds[i], seeds[j]
		if a.exact != b.exact {
			return a.exact
		}
		if a.coverage != b.coverage {
			return a.coverage > b.coverage
		}
		ar, br := rankOrMax(a.lexRank), rankOrMax(b.lexRank)
		if ar != br {
			return ar < br
		}
		return a.hit.ID < b.hit.ID
	})
	if len(seeds) > n {
		seeds = seeds[:n]
	}
	return seeds
}

func rankOrMax(r int) int {
	if r == 0 {
		return 1 << 30
	}
	return r
}

// expand adds structural neighbours of the seeds. Each link is labelled with its kind so
// the context and the trace can say why an item is there.
func expand(st *store.Store, seeds []*candidate, cands map[int64]*candidate, withHistory bool) (int, error) {
	added := 0
	addNeighbor := func(seed *candidate, h store.SymbolHit, label, basis string) {
		c, ok := cands[h.ID]
		if !ok {
			c = &candidate{hit: h, neighbor: true}
			cands[h.ID] = c
			scoreOwnText(c, nil) // no concepts here: coverage stays 0
			added++
		}
		if !hasString(c.rels, label) {
			c.rels = append(c.rels, label)
		}
		if basis != "" && !hasString(c.relBasis, basis) {
			c.relBasis = append(c.relBasis, basis)
		}
		if seed.coverage > c.seedCoverage {
			c.seedCoverage = seed.coverage
		}
		if seed.exact && !c.exact {
			c.seedCoverage = max(c.seedCoverage, 1)
		}
	}
	for _, seed := range seeds {
		id := seed.hit.ID
		members, err := st.MembersOf(id, 6)
		if err != nil {
			return added, err
		}
		for _, m := range members {
			addNeighbor(seed, m.SymbolHit, "member_of:"+seed.hit.Name, m.Basis)
		}
		if parent, ok, err := st.ParentOf(id); err != nil {
			return added, err
		} else if ok {
			addNeighbor(seed, parent, "contains:"+seed.hit.Name, "same_file")
		}
		callees, err := st.CalleesOf(id, 6)
		if err != nil {
			return added, err
		}
		for _, r := range callees {
			addNeighbor(seed, r.SymbolHit, fmt.Sprintf("called_by:%s(%s)", seed.hit.Name, r.Basis), r.Basis)
		}
		callers, err := st.CallersOf(id, 4)
		if err != nil {
			return added, err
		}
		for _, r := range callers {
			addNeighbor(seed, r.SymbolHit, fmt.Sprintf("calls:%s(%s)", seed.hit.Name, r.Basis), r.Basis)
		}
		tests, err := st.TestFilesReferencing(seed.hit.Name, 2)
		if err != nil {
			return added, err
		}
		for _, f := range tests {
			addNeighbor(seed, f, "tested_by:"+f.Path, "same_file")
		}
		if !withHistory {
			continue
		}
		co, err := st.CoChanged(seed.hit.Path, 3)
		if err != nil {
			return added, err
		}
		paths := make([]string, 0, len(co))
		for p := range co {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		// Co-change annotates the seed rather than adding new items: history explains a
		// match, it is not a reason to show an unrelated file. One shared commit is not evidence.
		for _, p := range paths {
			if co[p] >= 2 && !hasString(seed.rels, fmt.Sprintf("cochange:%s(%d)", p, co[p])) {
				seed.rels = append(seed.rels, fmt.Sprintf("cochange:%s(%d)", p, co[p]))
			}
		}
	}
	return added, nil
}

// gate says why a candidate may be shown, or why it is not.
func gate(c *candidate) string {
	switch {
	case c.exact:
		return "exact identifier"
	case c.coverage > 0:
		return "own text matches"
	case c.neighbor && c.seedCoverage > 0:
		return "structural neighbour of a match"
	}
	return "no evidence"
}

func gated(c *candidate) bool { return !c.suppressed && c.reason != "no evidence" }

// fuse combines lexical, exact, coverage, structural and git rankings, then diversifies.
// It returns the kept candidates in order and sets each candidate's score and reason.
func fuse(cands map[int64]*candidate, lexOrder []int64, opts Options) []*candidate {
	// A file's own row only adds value when nothing more specific from that file matched.
	specificFiles := map[string]bool{}
	for _, c := range cands {
		if c.hit.Kind != "file" && (c.exact || c.coverage > 0 || c.neighbor) {
			specificFiles[c.hit.Path] = true
		}
	}
	// When the prompt names an identifier that exists, partial word matches are noise:
	// keep the exact symbols, their structural neighbours, and anything matching every concept.
	hasExact := false
	for _, c := range cands {
		if c.exact {
			hasExact = true
		}
	}
	var pool []*candidate
	for _, c := range cands {
		if hasExact && !c.exact && !c.neighbor && c.coverage < 1 {
			c.suppressed = true
			c.reason = "dropped: partial word match next to an exact identifier"
			continue
		}
		if c.hit.Kind == "file" && specificFiles[c.hit.Path] && !hasPrefixAny(c.rels, "tested_by:") {
			c.suppressed = true
			c.reason = "dropped: more specific symbols from the same file"
		}
		if gated(c) {
			pool = append(pool, c)
		}
	}
	idx := func(c *candidate) string { return strconv.FormatInt(c.hit.ID, 10) }
	sort.Slice(pool, func(i, j int) bool { return pool[i].hit.ID < pool[j].hit.ID })

	var lexList, exactList, covList, structList, gitList []string
	for _, id := range lexOrder {
		if c, ok := cands[id]; ok && gated(c) {
			lexList = append(lexList, idx(c))
		}
	}
	for _, c := range pool {
		if c.exact {
			exactList = append(exactList, idx(c))
		}
	}
	byCoverage := append([]*candidate{}, pool...)
	sort.SliceStable(byCoverage, func(i, j int) bool {
		if byCoverage[i].coverage != byCoverage[j].coverage {
			return byCoverage[i].coverage > byCoverage[j].coverage
		}
		return rankOrMax(byCoverage[i].lexRank) < rankOrMax(byCoverage[j].lexRank)
	})
	for _, c := range byCoverage {
		if c.coverage > 0 {
			covList = append(covList, idx(c))
		}
	}
	for _, c := range pool {
		if c.neighbor {
			structList = append(structList, idx(c))
		}
	}
	// Uncommitted state only reinforces symbols whose own name matches the prompt.
	for _, c := range byCoverage {
		if c.git && c.literal {
			gitList = append(gitList, idx(c))
		}
	}

	scores := fusion.RRF([][]string{lexList, exactList, covList, structList, gitList}, 60)
	byKey := map[string]*candidate{}
	for _, c := range pool {
		byKey[idx(c)] = c
		c.score = scores[idx(c)]
	}
	var scored []fusion.Scored
	for _, key := range fusion.Order(scores) {
		c := byKey[key]
		scored = append(scored, fusion.Scored{Key: key, File: c.hit.Path, Score: c.score})
	}
	// Direct matches claim the per-file budget before structural neighbours do: a neighbour
	// that matches nothing in the prompt is supporting context, not the answer.
	sort.SliceStable(scored, func(i, j int) bool {
		di, dj := byKey[scored[i].Key].direct(), byKey[scored[j].Key].direct()
		return di && !dj
	})
	kept := fusion.Diversify(scored, opts.MaxPerFile, opts.MaxItems)
	keptSet := map[string]bool{}
	for _, k := range kept {
		keptSet[k.Key] = true
	}
	var out []*candidate
	for _, k := range kept {
		c := byKey[k.Key]
		c.reason = "kept"
		out = append(out, c)
	}
	for _, c := range pool {
		if !keptSet[idx(c)] {
			c.reason = "dropped: per-file cap or item limit"
		}
	}
	return out
}

// direct reports whether the candidate matches the prompt through its own text.
func (c *candidate) direct() bool { return c.exact || c.coverage > 0 }

func evidenceOf(c *candidate) Evidence {
	ev := Evidence{Hit: c.hit, Score: c.score, Matched: c.matched, Relations: c.rels, Dirty: c.git}
	ev.Verdict = verdictOf(c)
	ev.Section = sectionOf(c)
	return ev
}

func verdictOf(c *candidate) string {
	switch {
	case c.exact || c.literal:
		return "VERIFIED"
	case hasString(c.relBasis, "import"):
		return "VERIFIED"
	case c.neighbor && len(c.relBasis) == 1 && c.relBasis[0] == "unique_name":
		return "UNKNOWN"
	}
	return "INFERRED"
}

func sectionOf(c *candidate) string {
	kind := repo.Kind(c.hit.Path)
	switch {
	case kind == "test":
		return ctxbuild.Contracts
	case c.hit.Kind == "constant":
		return ctxbuild.Contracts
	case c.neighbor && c.coverage == 0 && hasPrefixAny(c.rels, "cochange:"):
		return ctxbuild.Git
	case c.neighbor && c.coverage == 0:
		return ctxbuild.Relations
	}
	return ctxbuild.Code
}

func traceOf(cands map[int64]*candidate) []CandidateTrace {
	list := make([]*candidate, 0, len(cands))
	for _, c := range cands {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].hit.ID < list[j].hit.ID
	})
	if len(list) > 40 {
		list = list[:40]
	}
	out := make([]CandidateTrace, 0, len(list))
	for _, c := range list {
		out = append(out, CandidateTrace{
			Name: c.hit.Name, Path: c.hit.Path, Kind: c.hit.Kind,
			Lexical: c.lexRank, Coverage: c.coverage, Exact: c.exact,
			Relations: c.rels, Score: c.score, Kept: c.reason == "kept", Reason: c.reason,
		})
	}
	return out
}

// budgetFor sizes the context block: a single exact hit is a small task; several
// related items are a common one; broad multi-part evidence gets the complex budget.
func budgetFor(ev []Evidence) int {
	switch {
	case len(ev) == 0:
		return 0
	case len(ev) <= 3 && hasVerdict(ev, "VERIFIED"):
		return 300
	case len(ev) <= 5:
		return 800
	}
	return 1500
}

func hasVerdict(ev []Evidence, v string) bool {
	for _, e := range ev {
		if e.Verdict == v {
			return true
		}
	}
	return false
}

func hasString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func hasPrefixAny(xs []string, prefix string) bool {
	for _, x := range xs {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}
