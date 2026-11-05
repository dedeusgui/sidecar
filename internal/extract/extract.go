// Package extract turns source files into symbols, references, imports and routes.
// Definitions and call references come from tree-sitter tags queries; constants,
// imports and routes come from small per-language rules. Nothing here executes code.
package extract

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Symbol is one named thing defined in a file. Parent indexes Result.Symbols, or is -1.
type Symbol struct {
	Kind      string
	Name      string
	Signature string
	Doc       string
	Value     string // literal value, only for numeric and boolean constants
	Line      int    // 1-based
	EndLine   int
	Parent    int

	startByte, endByte uint32
}

// Ref is a name used from inside a symbol (From ≥ 0) or from file scope (From = -1).
type Ref struct {
	From int
	Name string
	Kind string // call, type, ...
	Line int
}

// Result is everything extracted from one file.
type Result struct {
	Symbols []Symbol
	Refs    []Ref
	Imports []string

	refStarts []uint32 // byte offset of each Ref, used to scope it in finalize
}

// Extractor caches grammars and parsers. It is not safe for concurrent use; create one per worker.
type Extractor struct {
	grammars map[string]*grammar
}

type grammar struct {
	lang   *gts.Language
	parser *gts.Parser
	tagger *gts.Tagger
}

// New returns an Extractor with an empty cache.
func New() *Extractor {
	return &Extractor{grammars: map[string]*grammar{}}
}

// File extracts a file in the given grammar. lang "markdown" and "sql" use text rules;
// any other name must be a grammar known to gotreesitter.
func (e *Extractor) File(lang string, src []byte) (Result, error) {
	var res Result
	switch lang {
	case "markdown":
		res.Symbols = markdownSections(src)
	case "sql":
		res.Symbols = sqlObjects(src)
	default:
		g, err := e.grammar(lang)
		if err != nil {
			return res, err
		}
		res = g.extract(src)
		res.Symbols = append(res.Symbols, routes(src)...)
	}
	return finalize(res), nil
}

func (e *Extractor) grammar(name string) (*grammar, error) {
	if g, ok := e.grammars[name]; ok {
		return g, nil
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return nil, fmt.Errorf("no grammar for %q", name)
	}
	lang := entry.Language()
	g := &grammar{lang: lang, parser: gts.NewParser(lang)}
	if q := grammars.ResolveTagsQuery(*entry); q != "" {
		tg, err := gts.NewTagger(lang, q)
		if err != nil {
			return nil, fmt.Errorf("tags query for %q: %w", name, err)
		}
		g.tagger = tg
	}
	e.grammars[name] = g
	return g, nil
}

var containerKinds = map[string]bool{
	"class": true, "struct": true, "interface": true, "type": true, "enum": true,
	"module": true, "trait": true, "namespace": true,
}

// memberContainer are the kinds whose functions are methods.
var memberContainer = map[string]bool{"class": true, "struct": true, "interface": true, "trait": true}

var goReceiver = regexp.MustCompile(`^func\s*\(\s*\w*\s*\*?\s*(\w+)`)

// receiverType finds the Go type a method is declared on, from its receiver in the signature.
func receiverType(symbols []Symbol, signature string) int {
	m := goReceiver.FindStringSubmatch(signature)
	if m == nil {
		return -1
	}
	for j, c := range symbols {
		if containerKinds[c.Kind] && c.Name == m[1] {
			return j
		}
	}
	return -1
}

type rawRef struct {
	name, kind string
	start      uint32
	line       int
}

func (g *grammar) extract(src []byte) Result {
	tree, err := g.parser.Parse(src)
	if err != nil || tree == nil {
		return Result{}
	}
	defer tree.Release()

	var res Result
	var raws []rawRef
	if g.tagger != nil {
		lines := newLineIndex(src)
		for _, tag := range g.tagger.TagTree(tree) {
			name := strings.TrimSpace(tag.Name)
			switch {
			case strings.HasPrefix(tag.Kind, "definition.") && name != "":
				kind := strings.TrimPrefix(tag.Kind, "definition.")
				res.Symbols = append(res.Symbols, Symbol{
					Kind:      kind,
					Name:      name,
					Signature: signature(src, tag.Range.StartByte, tag.Range.EndByte),
					Doc:       lines.docAbove(int(tag.Range.StartPoint.Row)),
					Line:      int(tag.Range.StartPoint.Row) + 1,
					EndLine:   int(tag.Range.EndPoint.Row) + 1,
					Parent:    -1,
					startByte: tag.Range.StartByte,
					endByte:   tag.Range.EndByte,
				})
			case strings.HasPrefix(tag.Kind, "reference.") && name != "":
				raws = append(raws, rawRef{name: name, kind: strings.TrimPrefix(tag.Kind, "reference."),
					start: tag.Range.StartByte, line: int(tag.Range.StartPoint.Row) + 1})
			}
		}
		for _, r := range raws {
			res.Refs = append(res.Refs, Ref{From: -1, Name: r.name, Kind: r.kind, Line: r.line})
			res.refStarts = append(res.refStarts, r.start)
		}
	}

	walk(tree.RootNode(), func(n *gts.Node) {
		if sym, ok := g.constant(n, src); ok {
			res.Symbols = append(res.Symbols, sym)
		}
		if mod, ok := g.importModule(n, src); ok {
			res.Imports = append(res.Imports, mod)
		}
	})
	return res
}

func walk(n *gts.Node, visit func(*gts.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for i := 0; i < n.ChildCount(); i++ {
		walk(n.Child(i), visit)
	}
}

// finalize sorts symbols by span, computes parents and scopes references.
func finalize(res Result) Result {
	sort.SliceStable(res.Symbols, func(i, j int) bool {
		a, b := res.Symbols[i], res.Symbols[j]
		if a.startByte != b.startByte {
			return a.startByte < b.startByte
		}
		return a.endByte > b.endByte
	})
	for i := range res.Symbols {
		res.Symbols[i].Parent = innermost(res.Symbols, i, func(c Symbol) bool { return containerKinds[c.Kind] })
	}
	for i := range res.Symbols {
		s := &res.Symbols[i]
		if s.Parent >= 0 && s.Kind == "function" && memberContainer[res.Symbols[s.Parent].Kind] {
			s.Kind = "method" // Python and others tag every def; inside a class it is a method
		}
		if s.Kind == "method" && s.Parent < 0 {
			s.Parent = receiverType(res.Symbols, s.Signature)
		}
	}
	for i := range res.Refs {
		res.Refs[i].From = innermost(res.Symbols, -1, nil, res.refStarts[i])
	}
	res.refStarts = nil
	res.Imports = dedupe(res.Imports)
	return res
}

// innermost returns the index of the smallest symbol strictly enclosing symbols[i] (or
// the offset position when i < 0), restricted by accept when non-nil.
func innermost(symbols []Symbol, i int, accept func(Symbol) bool, pos ...uint32) int {
	var start, end uint32
	if i >= 0 {
		start, end = symbols[i].startByte, symbols[i].endByte
		if end == start {
			return -1
		}
	} else {
		start, end = pos[0], pos[0]
	}
	best := -1
	var bestSpan uint32
	for j, c := range symbols {
		if j == i || c.endByte <= c.startByte {
			continue
		}
		if accept != nil && !accept(c) {
			continue
		}
		if c.startByte <= start && end <= c.endByte && !(c.startByte == start && c.endByte == end) {
			span := c.endByte - c.startByte
			if best < 0 || span < bestSpan {
				best, bestSpan = j, span
			}
		}
	}
	return best
}

var literalValue = regexp.MustCompile(`^(?:-?\d[\d_]*(?:\.\d+)?|true|false|True|False)$`)
var upperConstName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// constant recognises a named constant in the grammar's own declaration shapes.
// Only numeric and boolean literal values are returned; string values are never stored.
func (g *grammar) constant(n *gts.Node, src []byte) (Symbol, bool) {
	typ := n.Type(g.lang)
	var name, value string
	switch typ {
	case "const_spec": // Go
		name = childText(n, g.lang, "identifier", src)
		if list := childOfType(n, g.lang, "expression_list"); list != nil && list.NamedChildCount() == 1 {
			value = list.NamedChild(0).Text(src)
		}
	case "assignment": // Python, module level, UPPER_CASE names
		if p := n.Parent(); p == nil || p.Type(g.lang) != "module" || n.NamedChildCount() < 2 {
			return Symbol{}, false
		}
		if first := n.NamedChild(0); first.Type(g.lang) == "identifier" && upperConstName.MatchString(first.Text(src)) {
			name, value = first.Text(src), n.NamedChild(1).Text(src)
		}
	case "variable_declarator": // TypeScript / JavaScript top-level const
		decl := n.Parent()
		if decl == nil || decl.Type(g.lang) != "lexical_declaration" || !strings.HasPrefix(strings.TrimSpace(decl.Text(src)), "const") {
			return Symbol{}, false
		}
		if n.NamedChildCount() == 2 && n.NamedChild(0).Type(g.lang) == "identifier" {
			name, value = n.NamedChild(0).Text(src), n.NamedChild(1).Text(src)
		}
	case "field_declaration": // Java static final, C# const or static readonly
		if !isConstField(n, g.lang, src) {
			return Symbol{}, false
		}
		decl := childOfType(n, g.lang, "variable_declaration")
		if decl == nil {
			decl = n
		}
		if vd := childOfType(decl, g.lang, "variable_declarator"); vd != nil && vd.NamedChildCount() == 2 {
			name, value = vd.NamedChild(0).Text(src), vd.NamedChild(1).Text(src)
		}
	default:
		return Symbol{}, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Symbol{}, false
	}
	if !literalValue.MatchString(value) {
		value = ""
	}
	return Symbol{
		Kind: "constant", Name: name, Value: value,
		Signature: signature(src, n.StartByte(), n.EndByte()),
		Line:      int(n.StartPoint().Row) + 1, EndLine: int(n.EndPoint().Row) + 1,
		Parent: -1, startByte: n.StartByte(), endByte: n.EndByte(),
	}, true
}

func isConstField(n *gts.Node, lang *gts.Language, src []byte) bool {
	text := n.Text(src)
	head := text
	if i := strings.IndexAny(text, "=;{"); i >= 0 {
		head = text[:i]
	}
	fields := strings.Fields(head)
	has := func(w string) bool {
		for _, f := range fields {
			if f == w {
				return true
			}
		}
		return false
	}
	switch n.Type(lang) {
	case "field_declaration":
		return (has("static") && has("final")) || has("const") || (has("static") && has("readonly"))
	}
	return false
}

// importModule returns the module named by an import-like node.
func (g *grammar) importModule(n *gts.Node, src []byte) (string, bool) {
	switch n.Type(g.lang) {
	case "import_spec": // Go
		if p := childOfType(n, g.lang, "interpreted_string_literal"); p != nil {
			return strings.Trim(p.Text(src), "\"`"), true
		}
	case "import_statement": // Python (import x) or TS/JS (import ... from 'x')
		if s := childOfType(n, g.lang, "string"); s != nil {
			return strings.Trim(s.Text(src), "'\""), true
		}
		if d := childOfType(n, g.lang, "dotted_name"); d != nil {
			return d.Text(src), true
		}
	case "import_from_statement": // Python
		if d := childOfType(n, g.lang, "dotted_name"); d != nil {
			return d.Text(src), true
		}
	case "import_declaration": // Java
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if t := c.Type(g.lang); t == "scoped_identifier" || t == "identifier" {
				return c.Text(src), true
			}
		}
	case "using_directive": // C#
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if t := c.Type(g.lang); t == "qualified_name" || t == "identifier" {
				return c.Text(src), true
			}
		}
	}
	return "", false
}

func childOfType(n *gts.Node, lang *gts.Language, typ string) *gts.Node {
	for i := 0; i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Type(lang) == typ {
			return c
		}
	}
	return nil
}

func childText(n *gts.Node, lang *gts.Language, typ string, src []byte) string {
	if c := childOfType(n, lang, typ); c != nil {
		return c.Text(src)
	}
	return ""
}

// signature is the first line of a declaration, without a trailing body opener.
func signature(src []byte, start, end uint32) string {
	if int(end) > len(src) || start >= end {
		return ""
	}
	text := string(src[start:end])
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "{"))
	return truncate(text, 200)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

type lineIndex struct{ lines []string }

func newLineIndex(src []byte) lineIndex {
	return lineIndex{lines: strings.Split(string(src), "\n")}
}

var commentPrefixes = []string{"//", "#", "/*", "*", "--", "\"\"\""}

// docAbove collects the contiguous comment block directly above a 0-based row.
func (l lineIndex) docAbove(row int) string {
	var block []string
	for r := row - 1; r >= 0 && r < len(l.lines); r-- {
		t := strings.TrimSpace(l.lines[r])
		if t == "" || !hasAnyPrefix(t, commentPrefixes) {
			break
		}
		block = append([]string{strings.TrimSpace(strings.TrimLeft(t, "/#*-"))}, block...)
		if len(block) >= 8 {
			break
		}
	}
	return truncate(strings.Join(block, " "), 400)
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

type routePattern struct {
	re     *regexp.Regexp
	method func(m []string) string
}

var routePatterns = []routePattern{
	{regexp.MustCompile(`@(Get|Post|Put|Delete|Patch|Request)Mapping\(\s*(?:(?:value|path)\s*=\s*)?"([^"]+)"`),
		func(m []string) string {
			if m[1] == "Request" {
				return "ANY"
			}
			return strings.ToUpper(m[1])
		}},
	{regexp.MustCompile(`\[Http(Get|Post|Put|Delete|Patch)\(\s*"([^"]+)"`),
		func(m []string) string { return strings.ToUpper(m[1]) }},
	{regexp.MustCompile("\\b(?:app|router)\\.(get|post|put|delete|patch)\\(\\s*['\"`]([^'\"`]+)['\"`]"),
		func(m []string) string { return strings.ToUpper(m[1]) }},
	{regexp.MustCompile(`@\w+\.(get|post|put|delete|patch|route)\(\s*['"]([^'"]+)['"]`),
		func(m []string) string { return strings.ToUpper(m[1]) }},
	{regexp.MustCompile(`\bHandleFunc\(\s*"(?:([A-Z]+) )?([^"]+)"`),
		func(m []string) string {
			if m[1] == "" {
				return "ANY"
			}
			return m[1]
		}},
}

// routes finds HTTP endpoints declared with common framework idioms.
func routes(src []byte) []Symbol {
	var out []Symbol
	for _, p := range routePatterns {
		for _, loc := range p.re.FindAllSubmatchIndex(src, -1) {
			m := make([]string, len(loc)/2)
			for i := range m {
				if loc[2*i] >= 0 {
					m[i] = string(src[loc[2*i]:loc[2*i+1]])
				}
			}
			path := m[len(m)-1]
			name := p.method(m) + " " + path
			line := 1 + strings.Count(string(src[:loc[0]]), "\n")
			out = append(out, Symbol{
				Kind: "route", Name: name, Signature: truncate(m[0], 160),
				Line: line, EndLine: line, Parent: -1,
				startByte: uint32(loc[0]), endByte: uint32(loc[0]),
			})
		}
	}
	return out
}

var sqlObjectRe = regexp.MustCompile(`(?i)\bcreate\s+(?:or\s+replace\s+)?(table|view)\s+(?:if\s+not\s+exists\s+)?("?[A-Za-z_][\w$]*"?(?:\."?[A-Za-z_][\w$]*"?)?)`)

// sqlObjects lists tables and views created by a SQL script.
func sqlObjects(src []byte) []Symbol {
	var out []Symbol
	for _, loc := range sqlObjectRe.FindAllSubmatchIndex(src, -1) {
		kind := strings.ToLower(string(src[loc[2]:loc[3]]))
		name := strings.ReplaceAll(string(src[loc[4]:loc[5]]), "\"", "")
		line := 1 + strings.Count(string(src[:loc[0]]), "\n")
		out = append(out, Symbol{Kind: kind, Name: name, Signature: truncate(string(src[loc[0]:loc[1]]), 160),
			Line: line, EndLine: line, Parent: -1, startByte: uint32(loc[0]), endByte: uint32(loc[0])})
	}
	return out
}

// markdownSections lists headings; a section ends where the next heading starts.
func markdownSections(src []byte) []Symbol {
	lines := strings.Split(string(src), "\n")
	var out []Symbol
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "#") {
			continue
		}
		name := strings.TrimSpace(strings.TrimLeft(t, "#"))
		if name == "" {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "#") {
				end = j
				break
			}
		}
		out = append(out, Symbol{Kind: "section", Name: name, Signature: truncate(t, 160),
			Line: i + 1, EndLine: end, Parent: -1})
	}
	return out
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
