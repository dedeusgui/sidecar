// Package core contains the shared, read-only traversal for ranked evidence from the existing index:
// numbered source windows, resolved relations, tests, contracts, impact and analysis gaps. Callers
// provide ranked seeds; this package adds no indexer, network request or repository-code execution.
// Output is bounded by a character budget scaled to repository size.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Schema versions the JSON contract (docs/explore.md).
const Schema = "sidecar-explore/1"

// Status of a fact. VERIFIED: the index resolved it (same file or a resolved import). CANDIDATE:
// a heuristic link (a name defined once, a co-change, a symbol without known callers). UNKNOWN: not
// established either way.
const (
	Verified  = "VERIFIED"
	Candidate = "CANDIDATE"
	Unknown   = "UNKNOWN"
)

// Options configures the shared traversal.
type Options struct {
	Root     string
	Query    string
	MaxFiles int // files whose source is shown; 0 = scaled to the repository size
	MaxChars int // total output budget for source snippets; 0 = scaled to the repository size
	Impact   bool
	Retrieve retrieve.Options
}

// Symbol is one definition shown in a file section.
type Symbol struct {
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Line       int     `json:"line"`
	EndLine    int     `json:"end_line"`
	Signature  string  `json:"signature,omitempty"`
	Verdict    string  `json:"verdict"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Snippet is a contiguous slice of a file, each line prefixed with its 1-based number and a tab.
type Snippet struct {
	Start  int    `json:"start"`
	End    int    `json:"end"`
	Text   string `json:"text"`
	Elided bool   `json:"elided,omitempty"` // the symbol body was cut to fit the per-file budget
}

// File is one file section.
type File struct {
	Path     string    `json:"path"`
	Lang     string    `json:"lang,omitempty"`
	Kind     string    `json:"kind,omitempty"`
	Role     string    `json:"role"` // candidate | test
	Reason   string    `json:"reason"`
	Symbols  []Symbol  `json:"symbols"`
	Snippets []Snippet `json:"snippets,omitempty"`
	Stale    bool      `json:"stale,omitempty"` // the file on disk differs from the indexed version
}

// Ref points at a symbol or file outside the shown source.
type Ref struct {
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Status string `json:"status"`
	Basis  string `json:"basis,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Edge is a call or import relation between two places.
type Edge struct {
	From   Ref    `json:"from"`
	To     Ref    `json:"to"`
	Kind   string `json:"kind"` // calls | imports
	Status string `json:"status"`
	Basis  string `json:"basis"`
}

// Budget reports the limits applied.
type Budget struct {
	MaxFiles  int `json:"max_files"`
	MaxChars  int `json:"max_chars"`
	UsedChars int `json:"used_chars"`
	RepoFiles int `json:"repo_files"`
}

// Result is the explore answer.
type Result struct {
	Schema       string             `json:"schema"`
	Relevance    string             `json:"relevance"` // the V3 gate verdict; explore answers anyway because it was asked
	Class        string             `json:"class"`
	Intent       []string           `json:"intent"`
	EntryPoints  []Ref              `json:"entry_points"`
	Files        []File             `json:"files"`
	Relations    []Edge             `json:"relations"`
	Consumers    []Ref              `json:"consumers"`
	Dependencies []Ref              `json:"dependencies"`
	Tests        []Ref              `json:"tests"`
	Contracts    []retrieve.Finding `json:"contracts,omitempty"`
	Impact       []impact.Finding   `json:"impact,omitempty"`
	NotShown     []Ref              `json:"not_shown,omitempty"`
	Gaps         []string           `json:"gaps"`
	Budget       Budget             `json:"budget"`
	ElapsedMS    float64            `json:"elapsed_ms"`
}

const (
	maxRelationsPerSymbol = 6
	maxSeedsForRelations  = 4
	maxRefs               = 10
	contextLines          = 2
	mergeGap              = 6
	maxSnippetLines       = 60
	maxImpactFindings     = 12
	maxContracts          = 8
)

// BudgetFor scales the output to the repository size: a focused question on a small project must not
// receive a whole file's worth of source, while a large repository justifies more per call.
func BudgetFor(repoFiles int) (maxFiles, maxChars int) {
	switch {
	case repoFiles < 200:
		return 3, 6000
	case repoFiles < 2000:
		return 4, 9000
	case repoFiles < 15000:
		return 5, 12000
	}
	return 6, 16000
}

// BuildInput supplies ranked evidence from the caller and the metadata that the old CLI router
// attached to it. The source traversal itself stays shared by the CLI and library APIs.
type BuildInput struct {
	Evidence  retrieve.Result
	Relevance string
	Class     string
	Intent    []string
	Gaps      []string
	Records   map[string]store.FileRecord
	Started   time.Time
}

// EmptyResult creates the same initialized sidecar-explore/1 result used by the CLI before search.
func EmptyResult(opts Options, records map[string]store.FileRecord) Result {
	out := Result{Schema: Schema, EntryPoints: []Ref{}, Files: []File{}, Relations: []Edge{}, Consumers: []Ref{},
		Dependencies: []Ref{}, Tests: []Ref{}, Gaps: []string{}}
	if records == nil {
		return out
	}
	maxFiles, maxChars := BudgetFor(len(records))
	if opts.MaxFiles > 0 {
		maxFiles = opts.MaxFiles
	}
	if opts.MaxChars > 0 {
		maxChars = opts.MaxChars
	}
	out.Budget = Budget{MaxFiles: maxFiles, MaxChars: maxChars, RepoFiles: len(records)}
	return out
}

// Build renders the shared traversal result from already-ranked evidence. It performs no ranking or
// orchestration so callers can use their own compatible seed policy without a package cycle.
func Build(ctx context.Context, st *store.Store, opts Options, input BuildInput) (Result, error) {
	start := input.Started
	if start.IsZero() {
		start = time.Now()
	}
	recs := input.Records
	if recs == nil {
		var err error
		recs, err = st.FileRecords()
		if err != nil {
			return Result{}, err
		}
	}
	out := EmptyResult(opts, recs)
	out.Relevance, out.Class, out.Intent = input.Relevance, input.Class, input.Intent
	out.Gaps = append(out.Gaps, input.Gaps...)
	maxFiles, maxChars := out.Budget.MaxFiles, out.Budget.MaxChars
	res := input.Evidence
	if len(res.Evidence) == 0 {
		out.Gaps = append(out.Gaps, "no indexed symbol matched the query; try a symbol name, a path or the project's own words")
		out.ElapsedMS = ms(time.Since(start))
		return out, nil
	}

	// Group evidence by file in ranking order.
	type group struct {
		path  string
		items []retrieve.Evidence
	}
	var groups []*group
	byPath := map[string]*group{}
	for _, ev := range res.Evidence {
		if ev.Note != "" || ev.Hit.Path == "" {
			continue
		}
		g := byPath[ev.Hit.Path]
		if g == nil {
			g = &group{path: ev.Hit.Path}
			byPath[ev.Hit.Path] = g
			groups = append(groups, g)
		}
		g.items = append(g.items, ev)
	}
	perFile := maxChars / max(1, min(maxFiles, len(groups)))
	used := 0
	var seeds []store.SymbolHit
	for i, g := range groups {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		rec, indexed := recs[g.path]
		if i >= maxFiles || used >= maxChars {
			for _, ev := range g.items[:1] {
				out.NotShown = append(out.NotShown, Ref{Name: ev.Hit.Name, Kind: ev.Hit.Kind, Path: ev.Hit.Path, Line: ev.Hit.Line,
					Status: statusOfVerdict(ev.Verdict), Reason: "ranked below the shown files"})
			}
			continue
		}
		f := File{Path: g.path, Lang: rec.Lang, Kind: rec.Kind, Role: "candidate", Reason: reasonOf(g.items[0])}
		if rec.Kind == "test" {
			f.Role = "test"
		}
		for _, ev := range g.items {
			f.Symbols = append(f.Symbols, Symbol{Name: ev.Hit.Name, Kind: ev.Hit.Kind, Line: ev.Hit.Line, EndLine: ev.Hit.EndLine,
				Signature: ev.Hit.Signature, Verdict: ev.Verdict, Source: ev.Source, Confidence: ev.Confidence, Reason: reasonOf(ev)})
			if ev.Hit.Kind != "file" && ev.Hit.Kind != "section" && len(seeds) < maxSeedsForRelations && rec.Kind != "test" {
				seeds = append(seeds, ev.Hit)
			}
		}
		data, readErr := os.ReadFile(filepath.Join(opts.Root, filepath.FromSlash(g.path)))
		switch {
		case readErr != nil:
			out.Gaps = append(out.Gaps, fmt.Sprintf("%s: not readable on disk (%v); symbols come from the index only", g.path, readErr))
		case !indexed:
			out.Gaps = append(out.Gaps, g.path+": not in the index file table")
		default:
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != rec.Hash {
				f.Stale = true
				out.Gaps = append(out.Gaps, g.path+": changed on disk since indexing; line numbers may be off until the next sync")
			}
			budget := min(perFile, maxChars-used)
			f.Snippets = snippets(string(data), g.items, budget, focusTerms(opts.Query))
			for _, s := range f.Snippets {
				used += len(s.Text)
			}
		}
		if rec.Skipped != "" {
			out.Gaps = append(out.Gaps, fmt.Sprintf("%s: content not parsed (%s)", g.path, rec.Skipped))
		} else if rec.Lang == "" && rec.Kind == "code" {
			out.Gaps = append(out.Gaps, g.path+": no grammar for this language; symbols are lexical candidates")
		}
		out.Files = append(out.Files, f)
	}
	out.Budget.UsedChars = used

	shown := map[string]bool{}
	for _, f := range out.Files {
		shown[f.Path] = true
	}
	if err := relate(ctx, st, seeds, shown, &out); err != nil {
		return out, err
	}
	for _, f := range res.Trace.Contracts {
		if len(out.Contracts) >= maxContracts {
			out.Gaps = append(out.Gaps, fmt.Sprintf("%d more contract links not listed", len(res.Trace.Contracts)-maxContracts))
			break
		}
		if shown[f.Path] || f.Source == "doc" || f.Status != "TEST_EXPECTED" {
			out.Contracts = append(out.Contracts, f)
		}
	}
	if opts.Impact || input.Class == "contract_change" || input.Class == "multi_file" || input.Class == "edit" {
		var code []string
		for _, f := range out.Files {
			if f.Kind == "code" {
				code = append(code, f.Path)
			}
		}
		if len(code) > 0 {
			rep, err := impact.Analyze(st, opts.Root, code)
			if err == nil {
				for _, f := range rep.Findings {
					if len(out.Impact) >= maxImpactFindings {
						break
					}
					out.Impact = append(out.Impact, f)
				}
				if !rep.HistoryIndexed {
					out.Gaps = append(out.Gaps, "Git history is not indexed: co-change impact was not computed")
				}
			}
		}
	}
	out.Gaps = append(out.Gaps,
		"calls are linked by name within a file, through resolved imports, or when the name is defined once; dynamic dispatch, reflection, callbacks and generated code are not resolved",
		"entry points are CANDIDATE: a symbol with no indexed caller may still be called by a framework, a test runner or another repository")
	out.ElapsedMS = ms(time.Since(start))
	return out, nil
}

// memberKeys returns the member names of dotted code references in the query (`res.send` -> `send`,
// `Router.prototype.handle` -> `handle`), so the definition is matched exactly.
func MemberKeys(q string) []string {
	var out []string
	for _, tok := range strings.Fields(q) {
		tok = strings.Trim(tok, "`'\"(),;:?!")
		parts := strings.Split(tok, ".")
		if len(parts) < 2 {
			continue
		}
		last := strings.TrimSuffix(parts[len(parts)-1], "()")
		if len(last) >= 2 && len(parts[0]) >= 1 && IsIdent(last) && IsIdent(parts[0]) {
			out = append(out, last)
		}
	}
	return out
}

// IsIdent reports whether a token follows the identifier shape used by the CLI's exact-key lookup.
func IsIdent(s string) bool {
	for i, r := range s {
		letter := r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		digit := i > 0 && r >= '0' && r <= '9'
		if !letter && !digit {
			return false
		}
	}
	return s != ""
}

// relate fills relations, consumers, dependencies, tests and candidate entry points from the index.
func relate(ctx context.Context, st *store.Store, seeds []store.SymbolHit, shown map[string]bool, out *Result) error {
	addRef := func(list *[]Ref, r Ref) {
		if len(*list) >= maxRefs {
			return
		}
		for _, x := range *list {
			if x.Path == r.Path && x.Name == r.Name && x.Line == r.Line {
				return
			}
		}
		*list = append(*list, r)
	}
	for _, s := range seeds {
		if err := ctx.Err(); err != nil {
			return err
		}
		from := Ref{Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.Line, Status: Verified}
		callers, err := st.CallersOf(s.ID, maxRelationsPerSymbol)
		if err != nil {
			return err
		}
		callees, err := st.CalleesOf(s.ID, maxRelationsPerSymbol)
		if err != nil {
			return err
		}
		for _, c := range callers {
			r := Ref{Name: c.Name, Kind: c.Kind, Path: c.Path, Line: c.Line, Status: statusOfBasis(c.Basis), Basis: c.Basis, Reason: "calls " + s.Name}
			out.Relations = append(out.Relations, Edge{From: r, To: from, Kind: "calls", Status: r.Status, Basis: c.Basis})
			if c.Path != s.Path {
				addRef(&out.Consumers, r)
			}
		}
		for _, c := range callees {
			r := Ref{Name: c.Name, Kind: c.Kind, Path: c.Path, Line: c.Line, Status: statusOfBasis(c.Basis), Basis: c.Basis, Reason: "called by " + s.Name}
			out.Relations = append(out.Relations, Edge{From: from, To: r, Kind: "calls", Status: r.Status, Basis: c.Basis})
			if c.Path != s.Path {
				addRef(&out.Dependencies, r)
			}
		}
		if len(callers) == 0 && (s.Kind == "function" || s.Kind == "method" || s.Kind == "route") {
			addRef(&out.EntryPoints, Ref{Name: s.Name, Kind: s.Kind, Path: s.Path, Line: s.Line, Status: Candidate,
				Reason: "no indexed caller"})
		}
		for _, c := range callers {
			if more, err := st.CallersOf(c.ID, 1); err == nil && len(more) == 0 && c.Kind != "test" && !strings.HasPrefix(c.Name, "Test") {
				addRef(&out.EntryPoints, Ref{Name: c.Name, Kind: c.Kind, Path: c.Path, Line: c.Line, Status: Candidate,
					Reason: "top of the indexed call chain into " + s.Name})
			}
		}
		tests, err := st.TestFilesReferencing(s.Name, 3)
		if err != nil {
			return err
		}
		for _, t := range tests {
			addRef(&out.Tests, Ref{Path: t.Path, Status: Verified, Basis: "references", Reason: "test file references " + s.Name})
		}
	}
	files := map[string]bool{}
	for _, s := range seeds {
		files[s.Path] = true
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		imps, err := st.ImportersOf(p, maxRefs)
		if err != nil {
			return err
		}
		for _, q := range imps {
			out.Relations = append(out.Relations, Edge{From: Ref{Path: q, Status: Verified}, To: Ref{Path: p, Status: Verified}, Kind: "imports", Status: Verified, Basis: "resolved_import"})
			addRef(&out.Consumers, Ref{Path: q, Status: Verified, Basis: "resolved_import", Reason: "imports " + p})
		}
		deps, err := st.ResolvedImportPaths(p)
		if err != nil {
			return err
		}
		for _, q := range deps {
			out.Relations = append(out.Relations, Edge{From: Ref{Path: p, Status: Verified}, To: Ref{Path: q, Status: Verified}, Kind: "imports", Status: Verified, Basis: "resolved_import"})
			addRef(&out.Dependencies, Ref{Path: q, Status: Verified, Basis: "resolved_import", Reason: "imported by " + p})
		}
	}
	for _, f := range out.Files {
		if f.Role == "test" {
			addRef(&out.Tests, Ref{Path: f.Path, Status: Verified, Basis: "retrieved", Reason: "test file matched the query"})
		}
	}
	return nil
}

// snippets returns numbered source slices around the given symbols, merged when close and cut to budget.
// A symbol longer than maxSnippetLines keeps its first lines plus windows around the lines that mention
// a query term, so the part of a long body the question is about survives the cut.
func snippets(text string, items []retrieve.Evidence, budget int, terms []string) []Snippet {
	lines := strings.Split(text, "\n")
	type span struct {
		a, b   int
		elided bool
	}
	var spans []span
	for _, ev := range items {
		a, b := ev.Hit.Line, ev.Hit.EndLine
		if a <= 0 || ev.Hit.Kind == "file" || a > len(lines) {
			continue
		}
		b = min(max(a, b), len(lines))
		if b-a+1 <= maxSnippetLines {
			spans = append(spans, span{max(1, a-contextLines), min(len(lines), b+contextLines), false})
			continue
		}
		spans = append(spans, span{max(1, a-contextLines), a + headLines - 1, true})
		for _, i := range focusLines(lines, a+headLines, b, terms) {
			spans = append(spans, span{max(a, i-focusContext), min(b, i+focusContext), true})
		}
		spans = append(spans, span{b, min(len(lines), b), true}) // closing line shows where the body ends
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].a < spans[j].a })
	var merged []span
	for _, s := range spans {
		n := len(merged)
		// Two focus windows of a long body stay apart (with a gap marker) unless they touch.
		separateWindows := n > 0 && s.elided && merged[n-1].elided && s.a > merged[n-1].b+1
		if n > 0 && s.a <= merged[n-1].b+mergeGap && !separateWindows {
			merged[n-1].b = max(merged[n-1].b, s.b)
			merged[n-1].elided = merged[n-1].elided || s.elided
			continue
		}
		merged = append(merged, s)
	}
	var out []Snippet
	left := budget
	for _, s := range merged {
		if left <= 0 {
			break
		}
		var b strings.Builder
		end := s.a - 1
		for i := s.a; i <= s.b; i++ {
			line := fmt.Sprintf("%d\t%s\n", i, strings.TrimRight(lines[i-1], "\r"))
			if b.Len()+len(line) > left {
				s.elided = true
				break
			}
			b.WriteString(line)
			end = i
		}
		if end < s.a {
			break
		}
		out = append(out, Snippet{Start: s.a, End: end, Text: b.String(), Elided: s.elided || end < s.b})
		left -= b.Len()
	}
	return out
}

const (
	headLines       = 12
	focusContext    = 3
	maxFocusWindows = 8
)

// focusTerms are the lowercase words of the query worth looking for inside a long body.
func focusTerms(q string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(w string) {
		w = strings.ToLower(strings.Trim(w, "`'\"(),;:?!."))
		if len([]rune(w)) >= 4 && !seen[w] && !stopFocus[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	for _, tok := range strings.Fields(q) {
		add(tok)
		for _, part := range strings.FieldsFunc(tok, func(r rune) bool { return r == '.' || r == '-' || r == '_' || r == '/' }) {
			add(part)
		}
	}
	for _, t := range lexical.Analyze(q) {
		add(t)
	}
	return out
}

var stopFocus = map[string]bool{"does": true, "what": true, "where": true, "which": true, "when": true, "with": true,
	"from": true, "that": true, "this": true, "como": true, "onde": true, "qual": true, "quais": true, "para": true,
	"decide": true, "work": true, "works": true, "funciona": true}

// focusLines picks up to maxFocusWindows lines in [a, b] that mention query terms, preferring lines with
// terms that are rare inside the body (a body that mentions "type" twenty times and "etag" twice is
// asked about at the "etag" lines). Returned in line order, at least 2*focusContext apart.
func focusLines(lines []string, a, b int, terms []string) []int {
	if len(terms) == 0 || a > b {
		return nil
	}
	freq := map[string]int{}
	hits := map[int][]string{}
	for i := a; i <= b; i++ {
		l := strings.ToLower(lines[i-1])
		for _, t := range terms {
			if strings.Contains(l, t) {
				freq[t]++
				hits[i] = append(hits[i], t)
			}
		}
	}
	type cand struct {
		line  int
		score float64
	}
	var cands []cand
	for i, ts := range hits {
		score := 0.0
		for _, t := range ts {
			score += 1 / float64(freq[t])
		}
		cands = append(cands, cand{i, score})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].line < cands[j].line
	})
	var picked []int
	for _, c := range cands {
		if len(picked) >= maxFocusWindows {
			break
		}
		near := false
		for _, p := range picked {
			if c.line-p < 2*focusContext && p-c.line < 2*focusContext {
				near = true
				break
			}
		}
		if !near {
			picked = append(picked, c.line)
		}
	}
	sort.Ints(picked)
	return picked
}

func reasonOf(ev retrieve.Evidence) string {
	switch {
	case ev.Source == "exact":
		return "named in the query"
	case len(ev.Matched) > 0:
		return "matches: " + strings.Join(ev.Matched, ", ")
	case len(ev.Relations) > 0:
		return "structural: " + ev.Relations[0]
	}
	return "ranked by retrieval (" + ev.Source + ")"
}

func statusOfBasis(basis string) string {
	switch basis {
	case "same_file", "import":
		return Verified
	case "unique_name":
		return Candidate
	}
	return Unknown
}

func statusOfVerdict(v string) string {
	if v == "VERIFIED" {
		return Verified
	}
	if v == "INFERRED" {
		return Candidate
	}
	return Unknown
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
