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
	// EngineVersion "v2" enables intent routing; "" and "v1" keep the v1 pipeline.
	EngineVersion string
	// IntentRouting enables intent routing regardless of EngineVersion.
	IntentRouting bool
	// Root is the repository root. The v2 contract check reads Markdown and test bodies from
	// it; empty means the index alone is used.
	Root string
	// Profile sets the context budget: "minimal" (300), "balanced" (800) or "deep" (1500).
	// Unknown names are ignored and the budget follows the evidence size.
	Profile string
	// Seen reports evidence already delivered in this session; nil means none was.
	Seen   ctxbuild.Seen
	Ablate Ablation
	// Tuning overrides the V2 ranking/abstention parameters for offline replay; nil means DefaultTuning.
	Tuning *Tuning
	// LSPTrusted and LSPServer are the opt-in language-server hooks. The lsp package imports
	// retrieve, so the caller supplies them. Notes need SIDECAR_LSP=1 and LSPTrusted(Root); LSPServer
	// returns the server name that can confirm rel, or "" for none.
	LSPTrusted func(root string) bool
	LSPServer  func(ctx context.Context, root, rel string) string
	// feedbackMults holds the post-task feedback multipliers of Root, keyed "path#symbol" or "path".
	feedbackMults   map[string]float64
	hasAbsentAnchor bool
}

// Ablation switches off parts of the pipeline. It exists to measure each component's
// contribution (see cmd/sidecar-eval); the product always runs with the zero value.
type Ablation struct {
	NoTranslation bool // do not use PT→EN translations
	NoExact       bool // do not match identifiers exactly
	NoStructure   bool // do not expand structural neighbours
	NoGit         bool // do not use uncommitted state or co-change
	LexicalOnly   bool // return the raw lexical ranking, without gating or fusion
	NoIntent      bool // v2 without intent routing
	NoContract    bool // v2 without contract links and conflict notes
	NoPacker      bool // v2 without utility packing: the ranked top MaxItems are sent
	NoFeedback    bool // v2 without post-task feedback multipliers
}

// Evidence is one item of retrieved context.
type Evidence struct {
	Hit           store.SymbolHit
	Verdict       string // VERIFIED | INFERRED | UNKNOWN
	Section       string // ctxbuild section the item belongs to
	Score         float64
	Matched       []string // query concepts matched by the item's own text
	Relations     []string // structural links, e.g. "calls:Enqueue(same_file)"
	Dirty         bool     // the file has uncommitted changes in the working tree
	ID            string   // deterministic identity: path, kind, name and start line
	Source        string   // exact | lexical | contract | structural | git | lsp
	Confidence    float64  // heuristic in [0, 1]; not fitted to labelled data
	TokenEstimate int      // estimate of the item's rendered metadata line
	// Note replaces the rendered line; set only for contract conflicts, which have no symbol of their own.
	Note string
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
	Feedback         string  `json:"feedback,omitempty"` // feedback_boost | feedback_penalty
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Confidence       float64 `json:"confidence"`
	TokenEstimate    int     `json:"token_estimate"`
	ReasonSelected   string  `json:"reason_selected,omitempty"`   // why the item was eligible
	ReasonSuppressed string  `json:"reason_suppressed,omitempty"` // why it was not shown
}

// Trace records every factor behind a retrieval, for `sidecar explain`.
type Trace struct {
	Terms         []string
	Phrases       []string
	Translations  map[string][]string
	Concepts      int
	Lexical       int
	Exact         int
	Structural    int
	Final         int
	Budget        int // suggested token budget for the context block
	Timings       map[string]time.Duration
	Candidates    []CandidateTrace
	Intent        Intent            `json:"intent"`
	Routed        bool              `json:"intent_routed"` // the intent changed the structural expansion
	TokenEstimate int               `json:"token_estimate"`
	Fingerprint   *repo.Fingerprint `json:"repo_fingerprint,omitempty"` // set by the caller that has the root
	Contracts     []Finding         `json:"contracts,omitempty"`        // v2: every checked contract link
	Packing       *PackTrace        `json:"packing,omitempty"`          // v2: why the final set was chosen
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
	nameComplete bool
	matched      []string
	coverage     float64
	// directMatches and altMatches count the prompt terms matched by the symbol's own text, as the
	// prompt's own word and through a translation. promptTerms is the number of distinct prompt
	// terms, known or not; v2Own selects the coverage-based confidence (see ownTextConfidence).
	directMatches int
	altMatches    int
	promptTerms   int
	v2Own         bool
	tun           *Tuning // nil = DefaultTuning; set from Options.Tuning for V2 candidates
	rels          []string
	neighbor      bool
	seedCoverage  float64
	relBasis      []string
	git           bool
	reason        string
	feedback      string // feedback_boost | feedback_penalty when post-task feedback moved its score
	suppressed    bool   // dropped before fusion; never shown or ranked
	score         float64
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
	searchTerms := terms
	if len(terms) > 12 {
		if focal := focalTerms(prompt); len(focal) >= 2 {
			searchTerms = focal
		}
	}
	concepts, err := buildConcepts(st, searchTerms, res.Trace.Phrases, total, res.Trace.Translations, opts.Ablate.NoTranslation)
	if err != nil {
		return res, err
	}
	if len(concepts) == 0 && len(searchTerms) < len(terms) {
		concepts, err = buildConcepts(st, terms, res.Trace.Phrases, total, res.Trace.Translations, opts.Ablate.NoTranslation)
		if err != nil {
			return res, err
		}
	}
	if len(terms) <= 10 {
		absentTerms, _ := absentAnchorTerms(st, prompt, opts.Ablate.NoTranslation)
		if len(absentTerms) > 0 {
			opts.hasAbsentAnchor = true
			var filtered []concept
			for _, c := range concepts {
				if !absentTerms[c.term] {
					filtered = append(filtered, c)
				}
			}
			concepts = filtered
		}
	}
	res.Trace.Concepts = len(concepts)
	res.Trace.Timings["analyze"] = time.Since(t)

	// Intent is always reported; it changes the search only when v2 routing is on and confident.
	res.Trace.Intent = ClassifyIntent(prompt)
	v2 := opts.EngineVersion == "v2"
	var families map[string]bool
	if (opts.IntentRouting || v2) && !opts.Ablate.NoIntent && res.Trace.Intent.Confidence >= intentMinConfidence {
		families = familiesFor(res.Trace.Intent.Kinds)
		res.Trace.Routed = families != nil
	}

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
		if v2 {
			c.directMatches, c.altMatches = ownMatches(c.hit, terms, opts.Ablate.NoTranslation)
			c.promptTerms, c.v2Own, c.tun = len(uniqueStrings(terms)), true, opts.Tuning
		}
	}
	res.Trace.Timings["exact"] = time.Since(t)

	if opts.Ablate.LexicalOnly {
		for i, id := range lexOrder {
			if i >= opts.MaxItems {
				break
			}
			c := cands[id]
			res.Evidence = append(res.Evidence, Evidence{Hit: c.hit, Verdict: "INFERRED", Section: ctxbuild.Code, Score: 1 / float64(i+1), Matched: c.matched,
				ID: idOf(c.hit), Source: sourceOf(c), Confidence: confidenceOf(c), TokenEstimate: tokensOf(c)})
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
		added, err = expand(st, seeds, cands, !opts.Ablate.NoGit, families)
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
	if v2 && !opts.Ablate.NoFeedback && opts.Root != "" {
		// A failed read leaves the ranking as it was: feedback only ever adjusts it.
		if mults, err := st.FeedbackMultipliers(opts.Root); err == nil && len(mults) > 0 {
			opts.feedbackMults = mults
		}
	}
	kept := fuse(cands, lexOrder, opts)
	res.Trace.Timings["fusion"] = time.Since(t)

	var evidence []Evidence
	for _, c := range kept {
		evidence = append(evidence, evidenceOf(c))
	}
	budget := budgetFor(evidence[:min(len(evidence), opts.MaxItems)])
	if b, ok := ProfileBudget(opts.Profile); ok {
		budget = b
	}
	res.Trace.Budget = budget
	var excluded map[string]string
	if v2 {
		t = time.Now()
		evidence, excluded, err = v2Pipeline(ctx, st, prompt, evidence, opts, budget, &res.Trace)
		if err != nil {
			return res, err
		}
		res.Trace.Timings["contract_pack"] = time.Since(t)
	}
	res.Evidence = evidence
	for _, ev := range res.Evidence {
		res.Trace.TokenEstimate += ev.TokenEstimate
	}
	res.Trace.Final = len(res.Evidence)
	res.Trace.Candidates = traceOf(cands)
	markExcluded(res.Trace.Candidates, excluded)
	res.Trace.Timings["total"] = time.Since(start)
	return res, nil
}

// v2Pipeline adds contract evidence to the ranked pool, then (unless ablated) packs the set
// sent under the budget. It returns the evidence to send and, per excluded item, the reason.
func v2Pipeline(ctx context.Context, st *store.Store, prompt string, pool []Evidence, opts Options, budget int, tr *Trace) ([]Evidence, map[string]string, error) {
	var concerns []Evidence
	if !opts.Ablate.NoContract {
		findings, err := ContractEvidence(st, opts.Root, subjectsOf(pool))
		if err != nil {
			return nil, nil, err
		}
		tr.Contracts = findings
		concerns = annotate(pool, findings)
	}
	if opts.Root != "" {
		if tr.Fingerprint == nil {
			fp := store.DiscoverCached(ctx, opts.Root, st)
			tr.Fingerprint = &fp
		}
		top := 0.0
		for _, ev := range pool {
			top = max(top, ev.Score)
		}
		if top == 0 {
			top = 1.0
		}
		concerns = append(concerns, docEvidence(ctx, tr.Fingerprint, prompt, top)...)
		lspNotes(ctx, opts, pool)
	}
	if budget == 0 && len(concerns) > 0 {
		budget = budgetFor(concerns)
		tr.Budget = budget
	}
	if opts.Ablate.NoPacker {
		out := pool
		if len(out) > opts.MaxItems {
			out = out[:opts.MaxItems]
		}
		return append(out, concerns...), nil, nil
	}
	selected, excluded, pt := Pack(append(pool, concerns...), PackOptions{Budget: budget, Seen: opts.Seen, Tuning: opts.Tuning})
	tr.Packing = &pt
	return selected, excluded, nil
}

// markExcluded records on the candidate trace why the packer dropped an item that the
// ranking had kept.
func markExcluded(cands []CandidateTrace, excluded map[string]string) {
	for i := range cands {
		if reason, ok := excluded[cands[i].ID]; ok && cands[i].Kept {
			cands[i].Kept = false
			cands[i].ReasonSelected = ""
			cands[i].ReasonSuppressed = "packer: " + reason
		}
	}
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
	// One batched lookup for every term, translation and phrase. A statement per term made
	// Voice Mode prompts (70+ terms) pay for every round trip on each search.
	lookup := append([]string{}, terms...)
	if !noTranslation {
		for _, term := range terms {
			lookup = append(lookup, lexical.Translations(term)...)
		}
		lookup = append(lookup, phrases...)
	}
	freq, err := st.TermFreqs(lookup)
	if err != nil {
		return nil, err
	}
	var out []concept
	for _, term := range terms {
		present, df := freq[term].Doc, freq[term].Symbol
		c := concept{term: term}
		if present > 0 && df <= maxDF {
			c.termPresent = true
		}
		translations := lexical.Translations(term)
		if noTranslation {
			translations = nil
		}
		for _, en := range translations {
			if f := freq[en]; f.Doc > 0 && f.Symbol <= maxDF {
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
		if f := freq[en]; f.Doc > 0 && f.Symbol <= maxDF {
			out = append(out, concept{term: en, termPresent: true, weight: idf(f.Symbol, total)})
			trans["phrase"] = append(trans["phrase"], en)
		}
	}
	// Long / Voice Mode prompts contain many low-IDF conversational words that happen to
	// appear in comments or docs. Distilling to the 10 highest-salience concepts keeps FTS5
	// fast and prevents conversational filler from diluting coverage.
	const maxSalientConcepts = 10
	if len(out) > maxSalientConcepts {
		sort.SliceStable(out, func(i, j int) bool {
			return conceptSalience(out[i], freq, total) > conceptSalience(out[j], freq, total)
		})
		out = out[:maxSalientConcepts]
	}
	return out, nil
}

// conceptSalience scores a concept for long-prompt distillation using its effective symbol
// frequency across the term and its translations, preferring symbol matches over comment-only matches.
func conceptSalience(c concept, freq map[string]store.TermFreq, total int) float64 {
	bestSym := freq[c.term].Symbol
	bestDoc := freq[c.term].Doc
	for _, a := range c.alts {
		f := freq[a]
		if f.Symbol > 0 && (bestSym == 0 || f.Symbol < bestSym) {
			bestSym = f.Symbol
		}
		if f.Doc > 0 && (bestDoc == 0 || f.Doc < bestDoc) {
			bestDoc = f.Doc
		}
	}
	if bestSym > 0 {
		return idf(bestSym, total) * 2.0
	}
	return idf(bestDoc, total)
}

var interrogativeCues = map[string]bool{
	"qual": true, "quais": true, "onde": true, "cade": true, "como": true, "quem": true,
	"which": true, "where": true, "how": true, "what": true, "why": true,
}

// focalTerms extracts the core question and code-like identifiers from a long / Voice Mode prompt
// so spoken narrative filler around the question does not dilute concept coverage or FTS5 latency.
func focalTerms(prompt string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	clauses := splitClauses(prompt)
	for _, cl := range clauses {
		if strings.HasSuffix(cl, "?") && hasInterrogativeCue(cl) {
			for _, t := range lexical.Analyze(cl) {
				add(t)
			}
		}
	}
	if len(out) == 0 {
		for _, cl := range clauses {
			if hasInterrogativeCue(cl) && !strings.Contains(lexical.Fold(cl), "ponto de entrada") {
				for _, t := range lexical.Analyze(cl) {
					add(t)
				}
			}
		}
	}
	for _, tok := range identRe.FindAllString(prompt, -1) {
		if codeLike(tok) {
			for _, t := range lexical.Analyze(tok) {
				add(t)
			}
		}
	}
	return out
}

func splitClauses(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '!', ';', ':', '\n', '?':
			if seg := strings.TrimSpace(s[start : i+1]); seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	if seg := strings.TrimSpace(s[start:]); seg != "" {
		out = append(out, seg)
	}
	return out
}

func hasInterrogativeCue(clause string) bool {
	folded := lexical.Fold(clause)
	// Strip comparative "como eu / como tu / como voce" ("as I / like you") so spoken
	// filler clauses ending with ", sabe?" are not mistaken for "how" questions.
	for _, comp := range []string{"como eu ", "como tu ", "como voce ", "como to "} {
		folded = strings.ReplaceAll(folded, comp, " ")
	}
	for _, w := range strings.FieldsFunc(folded, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if interrogativeCues[w] {
			return true
		}
	}
	return false
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

// absentAnchorTerms reports analyzed terms from any code-like identifier or mid-sentence
// capitalized proper noun (e.g. "Kafka", "MongoDB", "Express", "gRPC") whose primary root
// and translations are absent from the repository's symbols.
func absentAnchorTerms(st *store.Store, prompt string, noTranslation bool) (map[string]bool, error) {
	var absent map[string]bool
	for _, loc := range identRe.FindAllStringIndex(prompt, -1) {
		tok := prompt[loc[0]:loc[1]]
		isCode := codeLike(tok)
		midSentenceProper := loc[0] > 0 && tok[0] >= 'A' && tok[0] <= 'Z' && len(tok) >= 4 && !afterSentenceBreak(prompt, loc[0]) && lexical.Fold(tok) != "code"
		if !isCode && !midSentenceProper {
			continue
		}
		sub := lexical.Analyze(tok)
		if len(sub) == 0 {
			continue
		}
		// For CamelCase technology names like "MongoDB" -> ["mongo", "db"] or "PostgreSQL" -> ["postgre", "sql"],
		// check the primary proper root rather than a generic 2-3 letter suffix ("db", "sql").
		checkTerms := sub
		if len(sub) > 1 && len(sub[0]) >= 4 {
			checkTerms = sub[:1]
		}
		isTermPresent := func(t string) (bool, error) {
			if isCode {
				n, err := st.TermDocFreq(t)
				return n > 0, err
			}
			// Mid-sentence proper noun (e.g. "Kafka", "Express", "Redis"): must appear in at least
			// one symbol name/signature/path or repeatedly in repository documentation (like "Rust").
			codeN, err := st.TermCodeFreq(t)
			if err != nil || codeN > 0 {
				return codeN > 0, err
			}
			docN, err := st.TermDocFreq(t)
			return docN >= 10, err
		}
		anyPresent := false
		for _, t := range checkTerms {
			if ok, err := isTermPresent(t); err != nil {
				return nil, err
			} else if ok {
				anyPresent = true
				break
			}
			if !noTranslation {
				for _, en := range lexical.Translations(t) {
					if ok, err := isTermPresent(en); err != nil {
						return nil, err
					} else if ok {
						anyPresent = true
						break
					}
				}
			}
			if anyPresent {
				break
			}
		}
		if !anyPresent {
			if absent == nil {
				absent = map[string]bool{}
			}
			for _, t := range sub {
				absent[t] = true
			}
		}
	}
	return absent, nil
}

func afterSentenceBreak(s string, pos int) bool {
	for i := pos - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t':
			continue
		case '.', '?', '!', '\n', '\r', ':':
			return true
		default:
			return false
		}
	}
	return true
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
	c.nameComplete = false
	matchedName := map[string]bool{}
	for _, cp := range concepts {
		switch {
		case cp.termPresent && all[cp.term]:
			c.matched = append(c.matched, cp.term)
			if nameSet[cp.term] {
				c.literal = true
				matchedName[cp.term] = true
			}
		default:
			// A translation match is a match, but never a literal one: only the prompt's own
			// word appearing in the name counts as VERIFIED or as uncommitted-state boost.
			for _, a := range cp.alts {
				if all[a] {
					c.matched = append(c.matched, cp.term+"→"+a)
					if nameSet[a] {
						matchedName[a] = true
					}
					break
				}
			}
		}
	}
	if c.hit.Kind != "file" && len(nameSet) >= 2 && len(matchedName) == len(nameSet) {
		c.nameComplete = true
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
	hasExact := false
	for _, c := range cands {
		if c.exact {
			hasExact = true
			break
		}
	}
	var seeds []*candidate
	for _, c := range cands {
		if hasExact && !c.exact && c.coverage < 1 {
			continue
		}
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
// A nil families map expands every family (v1). Otherwise only the listed families are queried.
func expand(st *store.Store, seeds []*candidate, cands map[int64]*candidate, withHistory bool, families map[string]bool) (int, error) {
	if len(seeds) == 0 {
		return 0, nil
	}
	want := func(f string) bool { return families == nil || families[f] }
	if withHistory && want(famHistory) {
		if n, err := st.CommitCount(); err != nil || n == 0 {
			withHistory = false
		}
	}
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
		kind := seed.hit.Kind
		isFileOrSection := kind == "file" || kind == "section"
		isContainer := !isFileOrSection && kind != "function" && kind != "method" && kind != "constant" && kind != "field"
		if isContainer && want(famMembers) {
			members, err := st.MembersOf(id, 6)
			if err != nil {
				return added, err
			}
			for _, m := range members {
				addNeighbor(seed, m.SymbolHit, "member_of:"+seed.hit.Name, m.Basis)
			}
		}
		if !isFileOrSection && want(famParent) {
			if parent, ok, err := st.ParentOf(id); err != nil {
				return added, err
			} else if ok {
				addNeighbor(seed, parent, "contains:"+seed.hit.Name, "same_file")
			}
		}
		if !isFileOrSection && want(famCallees) {
			callees, err := st.CalleesOf(id, 6)
			if err != nil {
				return added, err
			}
			for _, r := range callees {
				addNeighbor(seed, r.SymbolHit, fmt.Sprintf("called_by:%s(%s)", seed.hit.Name, r.Basis), r.Basis)
			}
		}
		if !isFileOrSection && want(famCallers) {
			callers, err := st.CallersOf(id, 4)
			if err != nil {
				return added, err
			}
			for _, r := range callers {
				addNeighbor(seed, r.SymbolHit, fmt.Sprintf("calls:%s(%s)", seed.hit.Name, r.Basis), r.Basis)
			}
		}
		if !isFileOrSection && want(famTests) {
			tests, err := st.TestFilesReferencing(seed.hit.Name, 2)
			if err != nil {
				return added, err
			}
			for _, f := range tests {
				addNeighbor(seed, f, "tested_by:"+f.Path, "same_file")
			}
		}
		if !withHistory || !want(famHistory) {
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
	hasNonGenerated := false
	for _, c := range cands {
		if c.exact {
			hasExact = true
		}
		if !repo.IsGenerated(c.hit.Path, nil) && (c.exact || c.coverage > 0) {
			hasNonGenerated = true
		}
	}
	var pool []*candidate
	for _, c := range cands {
		if hasNonGenerated && repo.IsGenerated(c.hit.Path, nil) {
			c.suppressed = true
			c.reason = "dropped: generated file"
			continue
		}
		if hasExact && !c.exact && !c.neighbor && len(c.rels) == 0 && c.coverage < 1 {
			c.suppressed = true
			c.reason = "dropped: partial word match next to an exact identifier"
			continue
		}
		if opts.hasAbsentAnchor && !hasExact && !c.exact && len(c.matched) <= 1 {
			c.suppressed = true
			c.reason = "dropped: named anchor identifier absent from repository"
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
	for _, c := range pool {
		if !c.exact && c.nameComplete {
			exactList = append(exactList, idx(c))
		}
	}
	byCoverage := append([]*candidate{}, pool...)
	sort.SliceStable(byCoverage, func(i, j int) bool {
		if byCoverage[i].coverage != byCoverage[j].coverage {
			return byCoverage[i].coverage > byCoverage[j].coverage
		}
		if byCoverage[i].nameComplete != byCoverage[j].nameComplete {
			return byCoverage[i].nameComplete
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
		if m, ok := feedbackMultiplier(c, opts.feedbackMults); ok {
			scores[idx(c)] *= m
			switch {
			case m > 1.01:
				c.feedback = "feedback_boost"
			case m < 0.99:
				c.feedback = "feedback_penalty"
			}
		}
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

// feedbackMultiplier returns the feedback multiplier of c's symbol, or of its file when the
// symbol has none of its own.
func feedbackMultiplier(c *candidate, mults map[string]float64) (float64, bool) {
	if m, ok := mults[c.hit.Path+"#"+c.hit.Name]; ok {
		return m, true
	}
	m, ok := mults[c.hit.Path]
	return m, ok
}

func evidenceOf(c *candidate) Evidence {
	ev := Evidence{Hit: c.hit, Score: c.score, Matched: c.matched, Relations: c.rels, Dirty: c.git}
	ev.Verdict = verdictOf(c)
	ev.Section = sectionOf(c)
	ev.ID, ev.Source, ev.Confidence, ev.TokenEstimate = idOf(c.hit), sourceOf(c), confidenceOf(c), tokensOf(c)
	return ev
}

// idOf identifies a symbol by path, kind, name and start line. It does not depend on the
// prompt, so the same symbol keeps its ID across runs.
func idOf(h store.SymbolHit) string {
	return fmt.Sprintf("%s#%s:%s@%d", h.Path, h.Kind, h.Name, h.Line)
}

// sourceOf names the candidate's principal origin among the retrieval sources.
func sourceOf(c *candidate) string {
	switch {
	case c.exact:
		return "exact"
	case c.neighbor:
		return "structural"
	case c.lexRank > 0 || c.coverage > 0:
		if sectionOf(c) == ctxbuild.Contracts {
			return "contract"
		}
		return "lexical"
	case c.git:
		return "git"
	}
	return "none"
}

// confidenceOf is a fixed heuristic over the evidence path, ordered by how direct the match is.
// It is not fitted to labelled data; calibrating it needs the evaluation set.
func confidenceOf(c *candidate) float64 {
	if c.exact {
		return 0.95
	}
	if c.v2Own && (c.literal || c.coverage > 0) {
		return ownTextConfidence(c)
	}
	switch {
	case c.exact || c.literal:
		return 0.95
	case c.coverage > 0:
		return 0.5 + 0.4*math.Min(c.coverage, 1)
	case c.neighbor:
		return 0.3 + 0.3*math.Min(c.seedCoverage, 1)
	case c.git:
		return 0.3
	}
	return 0.2
}

// ownCoverCap bounds the prompt terms that must be matched. Voice Mode prompts have many more terms
// than any answer needs, so the denominator stops growing at this count.
const ownCoverCap = 4

// ownAltWeight is the support of a translation-only match, relative to the prompt's own word. It
// is 1: the denominator, not a discount, is what keeps a dictionary guess from passing alone.
const ownAltWeight = 1.0

// ownMatches counts the distinct prompt terms that the symbol's own text contains, as the prompt's
// own word (direct) or only through a translation (alt). It reads every prompt term, including the
// ones the concept distillation drops, so the numerator and the denominator count the same words.
func ownMatches(h store.SymbolHit, terms []string, noTranslation bool) (direct, alt int) {
	all := hitTerms(h)
	for _, t := range uniqueStrings(terms) {
		switch {
		case all[t]:
			direct++
		case !noTranslation:
			for _, en := range lexical.Translations(t) {
				if all[en] {
					alt++
					break
				}
			}
		}
	}
	return direct, alt
}

func uniqueStrings(xs []string) []string {
	seen := make(map[string]bool, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ownTextConfidence scores a candidate by how much of the prompt its own text explains, in the V2
// pipeline. The denominator is the prompt's own terms, so a content word the repository does not
// know counts against the match. The old coverage was taken over the repository's terms only, so
// one known word out of four read as full coverage. A translation-only match counts half: a
// dictionary can map a common word onto a common code word (horário to time, which matches
// timeout). Confidence stays inside [0.3, 0.9]; only an exact identifier reaches 0.95.
func ownTextConfidence(c *candidate) float64 {
	t := tuningOrDefault(c.tun)
	support := float64(c.directMatches) + t.OwnAltWeight*float64(c.altMatches)
	denom := math.Min(float64(max(c.promptTerms, 1)), t.OwnCoverCap)
	cover := math.Min(support/denom, 1)
	return t.OwnConfBase + t.OwnConfSpan*cover
}

// tokensOf estimates the metadata line that the brief renders for the item.
func tokensOf(c *candidate) int {
	h := c.hit
	return ctxbuild.EstimateTokens(fmt.Sprintf("%s %s (%s:%d-%d) %s", h.Kind, h.Name, h.Path, h.Line, h.EndLine, strings.Join(c.rels, " ")))
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
		t := CandidateTrace{
			Name: c.hit.Name, Path: c.hit.Path, Kind: c.hit.Kind,
			Lexical: c.lexRank, Coverage: c.coverage, Exact: c.exact,
			Relations: c.rels, Score: c.score, Kept: c.reason == "kept", Reason: c.reason, Feedback: c.feedback,
			ID: idOf(c.hit), Source: sourceOf(c), Confidence: confidenceOf(c), TokenEstimate: tokensOf(c),
		}
		if t.Kept {
			t.ReasonSelected = gate(c)
		} else {
			t.ReasonSuppressed = c.reason
		}
		out = append(out, t)
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
