// Package impact answers "what else does this change touch?" from the index: consumers that were not
// changed, tests that name the changed symbols, documents and tests that state a value the change may
// have made false, and gaps where no test is known. It is diff-aware (the changed files are an input),
// bounded, deterministic, and never runs repository code: verification commands are candidates only.
package impact

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Status follows the Task Evidence Brief: CONFIRMED is a checkable fact in the index; CANDIDATE is a
// nominal relation; UNKNOWN is a gap; CONFLICT is two sources that disagree.
type Status string

const (
	Confirmed Status = "CONFIRMED"
	Candidate Status = "CANDIDATE"
	Unknown   Status = "UNKNOWN"
	Conflict  Status = "CONFLICT"
)

// Bounds keep the analysis proportional to the change.
const (
	MaxChangedFiles   = 12
	MaxSymbolsPerFile = 40
	maxCoChange       = 5
	minCoChange       = 2 // past commits that touched both files before a pair counts
	maxCallers        = 6
	fetchCallers      = 60 // rows read before filtering: the bound applies to what is shown, not to what is searched
	fetchImporters    = 60
	fetchCoChange     = 40
	maxTestsPerSym    = 3
	maxFindings       = 40
)

// Finding is one traceable statement about the change.
type Finding struct {
	Kind   string `json:"kind"` // consumer | importer | test | doc | contract | gap | verify
	Status Status `json:"status"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Symbol string `json:"symbol,omitempty"` // the changed symbol the finding is about
	Text   string `json:"text"`
}

// Report is the analysis of one changed set.
type Report struct {
	Changed   []string  `json:"changed"`
	Skipped   []string  `json:"skipped,omitempty"` // changed files beyond the bound, doc or unindexed files
	Findings  []Finding `json:"findings"`
	Surface   []string  `json:"surface"` // changed files plus the files the findings name
	Truncated bool      `json:"truncated,omitempty"`
	// HistoryIndexed says whether Git history was in the index. Without it the co-change signal was not computed, so
	// an empty result must not be read as "nothing is related".
	HistoryIndexed bool `json:"history_indexed"`
}

// Analyze relates the changed files to what depends on them. changed holds repository-relative paths;
// deleted or unindexed paths are reported as skipped, not guessed about.
func Analyze(st *store.Store, root string, changed []string) (Report, error) {
	var rep Report
	set := map[string]bool{}
	for _, c := range changed {
		c = strings.TrimPrefix(path.Clean(strings.ReplaceAll(c, "\\", "/")), "./")
		if c != "" && c != "." && !set[c] {
			set[c] = true
			rep.Changed = append(rep.Changed, c)
		}
	}
	sort.Strings(rep.Changed)
	var code []string
	for _, c := range rep.Changed {
		switch _, kind := repo.Classify(c); {
		case kind == "code" && len(code) < MaxChangedFiles:
			code = append(code, c)
		case kind == "code":
			rep.Skipped = append(rep.Skipped, c+" (over the bound)")
		case kind == "test":
			// A changed test is evidence the change is being verified, not something to analyse.
		default:
			rep.Skipped = append(rep.Skipped, c+" (not code)")
		}
	}
	seen := map[string]bool{}
	add := func(f Finding) {
		key := f.Kind + "|" + f.Path + "|" + f.Symbol + "|" + f.Text
		if seen[key] {
			return
		}
		seen[key] = true
		if len(rep.Findings) >= maxFindings {
			rep.Truncated = true
			return
		}
		rep.Findings = append(rep.Findings, f)
	}
	var subjects []store.SymbolHit
	for _, f := range code {
		syms, err := st.SymbolsInFile(f, MaxSymbolsPerFile)
		if err != nil {
			return rep, err
		}
		if len(syms) == 0 {
			rep.Skipped = append(rep.Skipped, f+" (no symbols indexed)")
			continue
		}
		untested := 0
		for _, sym := range syms {
			callers, err := st.CallersOf(sym.ID, fetchCallers)
			if err != nil {
				return rep, err
			}
			shown := 0
			for _, c := range callers {
				if set[c.Path] {
					continue
				}
				if _, kind := repo.Classify(c.Path); kind == "test" {
					continue // tests are reported below
				}
				if shown >= maxCallers {
					rep.Truncated = true
					break
				}
				shown++
				status := Candidate
				if c.Basis == "import" {
					status = Confirmed
				}
				add(Finding{Kind: "consumer", Status: status, Path: c.Path, Line: c.Line, Symbol: sym.Name,
					Text: fmt.Sprintf("%s %s calls %s (%s, basis %s) and was not changed", c.Kind, c.Name, sym.Name, c.Path, c.Basis)})
			}
			tests, err := st.TestFilesReferencing(sym.Name, maxTestsPerSym)
			if err != nil {
				return rep, err
			}
			for _, tf := range tests {
				state := "was not changed"
				if set[tf.Path] {
					state = "was changed too"
				}
				add(Finding{Kind: "test", Status: Confirmed, Path: tf.Path, Line: tf.Line, Symbol: sym.Name,
					Text: fmt.Sprintf("test file %s references %s and %s", tf.Path, sym.Name, state)})
			}
			if len(tests) == 0 && isCallable(sym.Kind) && untested < 3 && hasUpper(sym.Name) {
				untested++
				add(Finding{Kind: "gap", Status: Unknown, Path: sym.Path, Line: sym.Line, Symbol: sym.Name,
					Text: fmt.Sprintf("no test file in the index references exported %s %s", sym.Kind, sym.Name)})
			}
			if retrieveKind(sym.Kind) {
				subjects = append(subjects, sym)
			}
		}
		co, err := st.CoChanged(f, fetchCoChange)
		if err != nil {
			return rep, err
		}
		names := make([]string, 0, len(co))
		for p, n := range co {
			if n >= minCoChange && !set[p] && p != f {
				names = append(names, p)
			}
		}
		sort.Slice(names, func(i, j int) bool {
			if co[names[i]] != co[names[j]] {
				return co[names[i]] > co[names[j]]
			}
			return names[i] < names[j]
		})
		shownCo := 0
		for _, p := range names {
			if _, kind := repo.Classify(p); kind == "code" {
				if shownCo >= maxCoChange {
					rep.Truncated = true
					break
				}
				shownCo++
				add(Finding{Kind: "cochange", Status: Candidate, Path: p,
					Text: fmt.Sprintf("%s changed together with %s in %d past commits and was not changed now", p, f, co[p])})
			}
		}
		importers, err := st.ImportersOf(f, fetchImporters)
		if err != nil {
			return rep, err
		}
		shownImp := 0
		for _, imp := range importers {
			if set[imp] {
				continue
			}
			if _, kind := repo.Classify(imp); kind != "code" {
				continue
			}
			if shownImp >= 8 {
				rep.Truncated = true
				break
			}
			shownImp++
			add(Finding{Kind: "importer", Status: Confirmed, Path: imp, Text: fmt.Sprintf("%s imports %s and was not changed", imp, f)})
		}
	}
	if len(subjects) > 0 {
		findings, err := retrieve.ContractEvidence(st, root, subjects)
		if err != nil {
			return rep, err
		}
		for _, f := range findings {
			switch f.Status {
			case retrieve.ConflictCandidate:
				add(Finding{Kind: "contract", Status: Conflict, Path: f.Path, Line: f.Line, Symbol: subjectName(f.Subject),
					Text: fmt.Sprintf("%s states %s but the code disagrees; Sidecar does not choose an authority", f.Path, f.Claim)})
			case retrieve.Documented:
				add(Finding{Kind: "doc", Status: Confirmed, Path: f.Path, Line: f.Line, Symbol: subjectName(f.Subject),
					Text: fmt.Sprintf("%s documents %s; update it if the behaviour changed", f.Path, subjectName(f.Subject))})
			}
		}
	}
	if n, err := st.CommitCount(); err == nil {
		rep.HistoryIndexed = n > 0
	}
	rep.Surface = surfaceOf(rep)
	for _, f := range surfaceTests(rep) {
		add(verifyHint(f))
	}
	return rep, nil
}

func isCallable(kind string) bool { return kind == "function" || kind == "method" }

func hasUpper(name string) bool {
	return name != "" && strings.ToUpper(name[:1]) == name[:1] && strings.ToLower(name[:1]) != name[:1]
}

func retrieveKind(kind string) bool {
	switch kind {
	case "function", "method", "constant", "route", "struct", "class":
		return true
	}
	return false
}

func subjectName(subject string) string {
	f := strings.Fields(subject)
	if len(f) >= 2 {
		return f[1]
	}
	return subject
}

func surfaceOf(rep Report) []string {
	set := map[string]bool{}
	for _, c := range rep.Changed {
		set[c] = true
	}
	for _, f := range rep.Findings {
		if f.Path != "" && f.Kind != "gap" {
			set[f.Path] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// surfaceTests lists the test files the findings name, not yet changed, for verification hints.
func surfaceTests(rep Report) []string {
	changed := map[string]bool{}
	for _, c := range rep.Changed {
		changed[c] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range rep.Findings {
		if f.Kind == "test" && !changed[f.Path] && !seen[f.Path] {
			seen[f.Path] = true
			out = append(out, f.Path)
		}
	}
	sort.Strings(out)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// verifyHint names an existing test and, for Go, a candidate command. Nothing is executed.
func verifyHint(testFile string) Finding {
	text := "existing test that names a changed symbol; run it with the project's test runner: " + testFile
	if strings.HasSuffix(testFile, "_test.go") {
		text = fmt.Sprintf("existing Go test names a changed symbol; candidate command: go test ./%s (not run by Sidecar)", path.Dir(testFile))
	}
	return Finding{Kind: "verify", Status: Candidate, Path: testFile, Text: text}
}
