package retrieve

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/store"
)

// ContractStatus says what a link between a code definition and another source establishes.
type ContractStatus string

const (
	Implemented       ContractStatus = "IMPLEMENTED"        // defined in code; no test or doc names it
	TestExpected      ContractStatus = "TEST_EXPECTED"      // a test file references the definition by name
	Documented        ContractStatus = "DOCUMENTED"         // a doc names the definition; any value or route it states agrees
	Inferred          ContractStatus = "INFERRED"           // a code-like name appears in prose only, or a route path without method
	Unverified        ContractStatus = "UNVERIFIED"         // a value is stated but cannot be attributed to the definition
	ConflictCandidate ContractStatus = "CONFLICT_CANDIDATE" // an explicit value or HTTP method disagrees with the code
)

// conflictHead opens every conflict note. The tool reports both sides and never picks one.
const conflictHead = "CONFLICT_CANDIDATE: verificar intenção do usuário antes de mudar contrato (fontes divergem)"

// Finding is one checked link between a code definition and a test, doc or docstring.
type Finding struct {
	Subject string         `json:"subject"`           // kind name path:line of the code definition
	Status  ContractStatus `json:"status"`            // how the link was established
	Source  string         `json:"source"`            // code | test | doc | docstring
	Path    string         `json:"path,omitempty"`    // the other source's file
	Line    int            `json:"line,omitempty"`    // 1-based; 0 when the file was not read
	Claim   string         `json:"claim,omitempty"`   // the value or route the other source states
	Heading string         `json:"heading,omitempty"` // enclosing Markdown heading
	id      string         // ID of the code definition, to attach the finding to its evidence
	hit     store.SymbolHit
}

// contractKinds are the definitions that can carry a contract.
var contractKinds = map[string]bool{"function": true, "method": true, "constant": true, "route": true, "struct": true, "class": true}

const (
	maxSubjects  = 12  // code definitions checked per prompt, in ranked order
	maxTests     = 3   // test files reported per definition
	maxOwners    = 5   // docstrings checked per definition
	maxDocFiles  = 300 // Markdown files scanned per prompt
	maxTestFiles = 300 // test files scanned per prompt, for constants only
	maxConflicts = 5   // conflict notes per prompt; further conflicts are dropped, not summarised
	maxDocBytes  = 256 << 10
)

// docMarkers tie a number to a constant in prose. Other wording is ambiguous and is not compared.
var docMarkers = []string{"=", " é ", " is ", "padrão", "default", "valor", "value", "igual a", "equals"}

// testMarkers are the assertion forms that state an expected literal.
var testMarkers = []string{"==", "===", "toBe(", "toEqual(", "Equal(", "Equals(", "assertEqual", "assert_equal"}

// comparisonMarkers move a number away from the name: inequality or arithmetic.
var comparisonMarkers = []string{"!=", "<", ">", "≤", "≥", "+", " - ", "%"}

var (
	literalRe  = regexp.MustCompile(`\b(?:\d[\d_]*(?:\.\d+)?|true|false|True|False)\b`)
	codeSpanRe = regexp.MustCompile("`([^`]*)`")
	routeRe    = regexp.MustCompile("(?:^|[\\s`'\"(])(?:(GET|POST|PUT|DELETE|PATCH|ANY)\\s+)?(/[A-Za-z0-9_./:{}*+%~-]*)")
)

// ContractEvidence links each code definition to the tests, Markdown sections and docstrings
// that name it, and classifies every link. Only deterministic checks run: name references from
// the index, and literal values or HTTP routes stated explicitly on one line. Root is read for
// test files and Markdown; an empty root uses the index alone. A disagreement is returned as
// CONFLICT_CANDIDATE with both locations; neither side is chosen.
func ContractEvidence(st *store.Store, root string, subjects []store.SymbolHit) ([]Finding, error) {
	if len(subjects) > maxSubjects {
		subjects = subjects[:maxSubjects]
	}
	if len(subjects) == 0 {
		return nil, nil
	}
	textTests, err := testPathsForValues(st, root, subjects)
	if err != nil {
		return nil, err
	}
	sets := make([]linkSet, len(subjects))
	for i, s := range subjects {
		tests, err := testLinks(st, root, s, textTests)
		if err != nil {
			return nil, err
		}
		for _, f := range tests {
			sets[i].add(f.Path, f)
		}
	}

	docs, err := docPaths(st, root)
	if err != nil {
		return nil, err
	}
	for _, p := range docs {
		heading := ""
		for n, line := range readLines(root, p) {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
				heading = strings.TrimSpace(strings.TrimLeft(t, "#"))
			}
			for i, s := range subjects {
				status, claim := mention(line, s, docMarkers)
				if status == "" {
					continue
				}
				f := finding(s, status, "doc", p, n+1)
				f.Claim, f.Heading = claim, heading
				sets[i].add(p, f)
			}
		}
	}

	for i, s := range subjects {
		owners, err := st.DocstringOwners(s.Name, maxOwners)
		if err != nil {
			return nil, err
		}
		for _, o := range owners {
			status, claim := mention(o.Doc, s, docMarkers)
			// A definition's own comment is not corroboration; only a contradiction in it is reported.
			if status == "" || (o.ID == s.ID && status != ConflictCandidate) {
				continue
			}
			f := finding(s, status, "docstring", o.Path, o.Line)
			f.Claim = claim
			sets[i].add(fmt.Sprintf("%s:%d", o.Path, o.Line), f)
		}
	}

	var out []Finding
	conflicts := 0
	for i, s := range subjects {
		list := sets[i].list
		if len(list) == 0 {
			list = []Finding{finding(s, Implemented, "code", s.Path, s.Line)}
		}
		for _, f := range list {
			if f.Status == ConflictCandidate {
				if conflicts == maxConflicts {
					continue
				}
				conflicts++
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// testPathsForValues lists the test files to scan by text. Only constants need it: the index
// records calls and types, not a constant used as an argument. It is empty without a root.
func testPathsForValues(st *store.Store, root string, subjects []store.SymbolHit) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	for _, s := range subjects {
		if s.Kind == "constant" {
			return st.TestPaths(maxTestFiles)
		}
	}
	return nil, nil
}

// testLinks reports the test files that name the definition: those the index links by
// reference, and for a constant those whose text names it. For a constant with a literal value,
// a test line that asserts a different literal is a conflict instead of a link.
func testLinks(st *store.Store, root string, s store.SymbolHit, textTests []string) ([]Finding, error) {
	if s.Kind == "route" {
		return nil, nil
	}
	refs, err := st.TestFilesReferencing(s.Name, maxTests)
	if err != nil {
		return nil, err
	}
	referenced := map[string]bool{}
	var paths []string
	for _, t := range refs {
		referenced[t.Path] = true
		paths = append(paths, t.Path)
	}
	if s.Kind == "constant" {
		for _, p := range textTests {
			if !referenced[p] {
				paths = append(paths, p)
			}
		}
	}
	var out []Finding
	for _, p := range paths {
		f := finding(s, TestExpected, "test", p, 0)
		named := false
		for i, line := range readLines(root, p) {
			if !hasWord(line, s.Name) {
				continue
			}
			named = true
			if f.Line == 0 {
				f.Line = i + 1
			}
			if s.Kind == "constant" && s.Value != "" {
				if v, ok := valueClaim(line, s.Name, testMarkers); ok && !sameValue(v, s.Value) {
					f.Status, f.Line, f.Claim = ConflictCandidate, i+1, v
					break
				}
			}
		}
		if named || referenced[p] {
			out = append(out, f)
		}
	}
	return out, nil
}

// mention classifies what one text (a doc line or a docstring) says about a code definition.
// It returns "" when the text does not name the definition. A name inside backticks is an
// explicit reference; a code-like name in prose is a weaker one; a plain word is not a link.
func mention(text string, s store.SymbolHit, markers []string) (ContractStatus, string) {
	if s.Kind == "route" {
		return routeMention(text, s)
	}
	if !hasWord(text, s.Name) {
		return "", ""
	}
	explicit := inCodeSpan(text, s.Name)
	if !explicit && !codeLike(s.Name) {
		return "", ""
	}
	if s.Kind == "constant" && literalRe.MatchString(text) {
		if s.Value == "" {
			return Unverified, "" // the code value is not a literal, so the claim cannot be compared
		}
		v, ok := valueClaim(text, s.Name, markers)
		switch {
		case !ok:
			return Unverified, ""
		case sameValue(v, s.Value):
			return Documented, v
		default:
			return ConflictCandidate, v
		}
	}
	if explicit {
		return Documented, ""
	}
	return Inferred, ""
}

// routeMention compares the HTTP method stated for the route's path with the code's method.
func routeMention(text string, s store.SymbolHit) (ContractStatus, string) {
	method, path, _ := strings.Cut(s.Name, " ")
	claimed := map[string]bool{}
	for _, m := range routeRe.FindAllStringSubmatch(text, -1) {
		if normalizePath(m[2]) == normalizePath(path) {
			claimed[m[1]] = true
		}
	}
	if len(claimed) == 0 {
		return "", ""
	}
	if len(claimed) > 1 {
		return Unverified, "" // two methods for one path in one line: not attributable
	}
	for m := range claimed {
		if m == "" {
			return Inferred, path
		}
		claim := m + " " + path
		if m == method || m == "ANY" || method == "ANY" {
			return Documented, claim
		}
		return ConflictCandidate, claim
	}
	return "", ""
}

// valueClaim returns the single literal that text states for name. It needs an explicit
// marker, exactly one literal, no other code-like identifier and no comparison: otherwise
// the number may belong to a different definition, and ok is false.
func valueClaim(text, name string, markers []string) (string, bool) {
	if !containsAny(text, markers) || containsAny(text, comparisonMarkers) {
		return "", false
	}
	lits := literalRe.FindAllString(text, -1)
	if len(lits) != 1 {
		return "", false
	}
	for _, tok := range identRe.FindAllString(text, -1) {
		if tok != name && codeLike(tok) && !isMarkerWord(tok, markers) {
			return "", false
		}
	}
	return lits[0], true
}

// isMarkerWord reports whether tok is the marker itself, such as the assertion helper assertEqual.
func isMarkerWord(tok string, markers []string) bool {
	for _, m := range markers {
		if strings.TrimSuffix(m, "(") == tok {
			return true
		}
	}
	return false
}

// sameValue compares literals numerically when both are numbers, otherwise case-insensitively.
func sameValue(a, b string) bool {
	x, errA := strconv.ParseFloat(strings.ReplaceAll(a, "_", ""), 64)
	y, errB := strconv.ParseFloat(strings.ReplaceAll(b, "_", ""), 64)
	if errA == nil && errB == nil {
		return x == y
	}
	return strings.EqualFold(a, b)
}

// subjectsOf returns the ranked code definitions that carry a contract.
func subjectsOf(pool []Evidence) []store.SymbolHit {
	var out []store.SymbolHit
	for _, ev := range pool {
		if contractKinds[ev.Hit.Kind] && repo.Kind(ev.Hit.Path) == "code" {
			out = append(out, ev.Hit)
		}
	}
	return out
}

// annotate records each non-conflict finding on its code item as a relation, at most two per
// item, and returns one concern item per conflict. The concerns are never ranked above the code.
func annotate(pool []Evidence, findings []Finding) []Evidence {
	at := make(map[string]int, len(pool))
	for i, ev := range pool {
		at[ev.ID] = i
	}
	notes := map[int][]string{}
	var concerns []Evidence
	for _, f := range findings {
		if f.Status == ConflictCandidate {
			concerns = append(concerns, conflictEvidence(f))
			continue
		}
		i, ok := at[f.id]
		if !ok || f.Status == Implemented || len(notes[i]) == 2 {
			continue
		}
		notes[i] = append(notes[i], relationOf(f))
	}
	for i := range pool {
		if len(notes[i]) > 0 {
			pool[i].Relations = append(notes[i], pool[i].Relations...)
		}
	}
	return concerns
}

// relationOf is the one-line label a finding leaves on its code item.
func relationOf(f Finding) string {
	if f.Line > 0 {
		return fmt.Sprintf("%s %s:%d [%s]", f.Source, f.Path, f.Line, f.Status)
	}
	return fmt.Sprintf("%s %s [%s]", f.Source, f.Path, f.Status)
}

func conflictEvidence(f Finding) Evidence {
	note := conflictNote(f)
	return Evidence{
		Hit: f.hit, Verdict: "VERIFIED", Section: ctxbuild.Concerns, Source: "contract",
		Confidence: 0.8, ID: fmt.Sprintf("conflict|%s|%s:%d", f.id, f.Path, f.Line),
		Note: note, TokenEstimate: ctxbuild.EstimateTokens(note),
	}
}

// conflictNote states both sides with their exact locations and the rule that forbids choosing one.
func conflictNote(f Finding) string {
	s := f.hit
	code := s.Kind + " " + s.Name
	if s.Kind == "constant" {
		code += " = " + s.Value
	}
	if s.Kind == "route" {
		code = "route " + s.Name
	}
	src := map[string]string{"doc": "doc", "docstring": "docstring", "test": "teste"}[f.Source]
	where := fmt.Sprintf("%s:%d", f.Path, f.Line)
	if f.Heading != "" {
		where += " (" + f.Heading + ")"
	}
	return fmt.Sprintf("%s · código: %s (%s:%d) · %s: %s diz %s", conflictHead, code, s.Path, s.Line, src, where, f.Claim)
}

// finding starts a link from a code definition; the caller fills in the other side's claim.
func finding(s store.SymbolHit, status ContractStatus, source, path string, line int) Finding {
	return Finding{
		Subject: fmt.Sprintf("%s %s %s:%d", s.Kind, s.Name, s.Path, s.Line),
		Status:  status, Source: source, Path: path, Line: line,
		id: idOf(s), hit: s,
	}
}

// linkSet keeps one finding per source: the strongest, and the earliest among equals.
type linkSet struct {
	list []Finding
	at   map[string]int
}

func (ls *linkSet) add(key string, f Finding) {
	if ls.at == nil {
		ls.at = map[string]int{}
	}
	i, ok := ls.at[key]
	if !ok {
		ls.at[key] = len(ls.list)
		ls.list = append(ls.list, f)
		return
	}
	if rank(f.Status) > rank(ls.list[i].Status) {
		ls.list[i] = f
	}
}

func rank(s ContractStatus) int {
	switch s {
	case ConflictCandidate:
		return 4
	case Documented:
		return 3
	case TestExpected, Unverified:
		return 2
	}
	return 1
}

// docPaths lists the indexed Markdown files, or none when the repository root is unknown.
func docPaths(st *store.Store, root string) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	return st.DocPaths(maxDocFiles)
}

// readLines returns the lines of an indexed file, or nil when it is missing, binary, too large
// or outside the root. Paths come from the index; the check keeps a bad one inside the root.
func readLines(root, rel string) []string {
	if root == "" || !filepath.IsLocal(rel) {
		return nil
	}
	data, reason := repo.ReadSource(filepath.Join(root, rel), maxDocBytes)
	if reason != "" {
		return nil
	}
	return strings.Split(string(data), "\n")
}

// hasWord reports whether w occurs in s delimited by non-identifier bytes.
func hasWord(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		j += i
		if (j == 0 || !isIdentByte(s[j-1])) && (j+len(w) == len(s) || !isIdentByte(s[j+len(w)])) {
			return true
		}
		i = j + 1
	}
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func inCodeSpan(text, name string) bool {
	for _, m := range codeSpanRe.FindAllStringSubmatch(text, -1) {
		if hasWord(m[1], name) {
			return true
		}
	}
	return false
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// normalizePath treats a trailing slash as insignificant, except for the root path.
func normalizePath(p string) string {
	if len(p) > 1 {
		return strings.TrimSuffix(p, "/")
	}
	return p
}
