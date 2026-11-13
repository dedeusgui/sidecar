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
	name   string
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
		if lang != "" && grammars.DetectLanguageByName(lang) == nil {
			// No grammar: conservative line rules instead of an error, so the file still yields definitions.
			res.Symbols = ExtractFallback("", lang, src).Symbols
		} else {
			g, err := e.grammar(lang)
			if err != nil {
				return res, err
			}
			res = g.extract(src)
		}
		res.Symbols = append(res.Symbols, routes(lang, src)...)
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
	g := &grammar{name: name, lang: lang, parser: gts.NewParser(lang)}
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
	lines := newLineIndex(src)
	if g.tagger != nil {
		for _, tag := range g.tagger.TagTree(tree) {
			name := strings.TrimSpace(tag.Name)
			switch {
			case strings.HasPrefix(tag.Kind, "definition.") && name != "":
				kind := strings.TrimPrefix(tag.Kind, "definition.")
				res.Symbols = append(res.Symbols, Symbol{
					Kind:      kind,
					Name:      name,
					Signature: signature(src, tag.Range.StartByte, tag.Range.EndByte),
					Doc:       lines.docFor(int(tag.Range.StartPoint.Row), int(tag.Range.EndPoint.Row)),
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

	defined := map[int]map[string]bool{}
	for _, sym := range res.Symbols {
		if defined[sym.Line] == nil {
			defined[sym.Line] = map[string]bool{}
		}
		defined[sym.Line][sym.Name] = true
	}
	type refKey struct {
		name, kind string
		line       int
		start      uint32
	}
	seenRefs := map[refKey]bool{}
	for _, r := range res.Refs {
		seenRefs[refKey{name: r.Name, kind: r.Kind, line: r.Line}] = true
	}
	walk(tree.RootNode(), func(n *gts.Node) {
		if sym, ok := g.assignedFunction(n, src); ok && !defined[sym.Line][sym.Name] {
			sym.Doc = lines.docAbove(sym.Line - 1)
			res.Symbols = append(res.Symbols, sym)
		}
		if sym, ok := g.constant(n, src); ok {
			sym.Doc = lines.docAbove(sym.Line - 1)
			res.Symbols = append(res.Symbols, sym)
		}
		for _, mod := range g.importModules(n, src) {
			res.Imports = append(res.Imports, mod)
		}
		for _, r := range g.extraRefs(n, src) {
			k := refKey{name: r.name, kind: r.kind, line: r.line, start: r.start}
			if r.name == "" || seenRefs[k] {
				continue
			}
			seenRefs[k] = true
			res.Refs = append(res.Refs, Ref{From: -1, Name: r.name, Kind: r.kind, Line: r.line})
			res.refStarts = append(res.refStarts, r.start)
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
		if c.startByte <= start && end <= c.endByte && (c.startByte != start || c.endByte != end) {
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
// jsFunctionValues are the JavaScript/TypeScript node types of a function value.
var jsFunctionValues = map[string]bool{"function_expression": true, "function": true, "arrow_function": true,
	"generator_function": true}

// assignedFunction extracts a JavaScript/TypeScript function assigned to a name or member, the common
// definition style of CommonJS code: `res.send = function send(body) {…}`, `exports.x = () => {…}` and
// `Foo.prototype.bar = function () {…}`. The tags query does not report these (gap L03), so the
// definitions of Express-style APIs were missing from the index.
func (g *grammar) assignedFunction(n *gts.Node, src []byte) (Symbol, bool) {
	if n.Type(g.lang) == "variable_declarator" && (g.name == "javascript" || g.name == "typescript" || g.name == "tsx") {
		nameNode := n.ChildByFieldName("name", g.lang)
		val := n.ChildByFieldName("value", g.lang)
		if nameNode == nil || val == nil || nameNode.Type(g.lang) != "identifier" || val.Type(g.lang) != "assignment_expression" {
			return Symbol{}, false
		}
		for val != nil && val.Type(g.lang) == "assignment_expression" {
			val = val.ChildByFieldName("right", g.lang)
		}
		if val == nil || !jsFunctionValues[val.Type(g.lang)] {
			return Symbol{}, false
		}
		name := strings.TrimSpace(nameNode.Text(src))
		if name == "" || name == "exports" || name == "module" {
			return Symbol{}, false
		}
		return Symbol{
			Kind: "function", Name: name,
			Signature: signature(src, n.StartByte(), n.EndByte()),
			Line:      int(n.StartPoint().Row) + 1, EndLine: int(n.EndPoint().Row) + 1,
			Parent: -1, startByte: n.StartByte(), endByte: n.EndByte(),
		}, true
	}
	if n.Type(g.lang) != "assignment_expression" {
		return Symbol{}, false
	}
	left, right := n.ChildByFieldName("left", g.lang), n.ChildByFieldName("right", g.lang)
	if left == nil || right == nil || !jsFunctionValues[right.Type(g.lang)] {
		return Symbol{}, false
	}
	kind, name := "function", ""
	switch left.Type(g.lang) {
	case "identifier":
		name = left.Text(src)
	case "member_expression":
		if prop := left.ChildByFieldName("property", g.lang); prop != nil {
			name = prop.Text(src)
		}
		if obj := left.ChildByFieldName("object", g.lang); obj != nil && strings.HasSuffix(obj.Text(src), ".prototype") {
			kind = "method"
		}
	}
	if name == "exports" || name == "module" || name == "" {
		// module.exports = function createApplication() {…}: the function's own name is the definition.
		if own := right.ChildByFieldName("name", g.lang); own != nil {
			name = own.Text(src)
		} else {
			return Symbol{}, false
		}
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\n[]()") {
		return Symbol{}, false
	}
	return Symbol{
		Kind: kind, Name: name,
		Signature: signature(src, n.StartByte(), n.EndByte()),
		Line:      int(n.StartPoint().Row) + 1, EndLine: int(n.EndPoint().Row) + 1,
		Parent: -1, startByte: n.StartByte(), endByte: n.EndByte(),
	}, true
}

func (g *grammar) constant(n *gts.Node, src []byte) (Symbol, bool) {
	typ := n.Type(g.lang)
	var name, value string
	switch typ {
	case "const_spec": // Go
		name = childText(n, g.lang, "identifier", src)
		if list := childOfType(n, g.lang, "expression_list"); list != nil && list.NamedChildCount() == 1 {
			value = list.NamedChild(0).Text(src)
		}
	case "const_item": // Rust
		name = childText(n, g.lang, "identifier", src)
		if n.NamedChildCount() >= 2 {
			value = n.NamedChild(n.NamedChildCount() - 1).Text(src)
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
	case "const_declaration": // PHP
		if g.name != "php" {
			return Symbol{}, false
		}
		if elem := childOfType(n, g.lang, "const_element"); elem != nil && elem.NamedChildCount() >= 2 {
			name, value = elem.NamedChild(0).Text(src), elem.NamedChild(1).Text(src)
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

// importModules returns zero or more module names referenced by an import-like node.
func (g *grammar) importModules(n *gts.Node, src []byte) []string {
	if g.name == "php" && n.Type(g.lang) == "namespace_use_declaration" {
		return g.phpUseModules(n, src)
	}
	if mod, ok := g.importModule(n, src); ok {
		return []string{mod}
	}
	return nil
}

func (g *grammar) phpUseModules(n *gts.Node, src []byte) []string {
	var out []string
	prefix := ""
	if ns := childOfType(n, g.lang, "namespace_name"); ns != nil {
		prefix = strings.TrimPrefix(strings.TrimSpace(ns.Text(src)), `\`)
	}
	appendClause := func(clause *gts.Node, base string) {
		var target string
		if q := childOfType(clause, g.lang, "qualified_name"); q != nil {
			target = q.Text(src)
		} else if nm := childOfType(clause, g.lang, "name"); nm != nil {
			target = nm.Text(src)
		}
		target = strings.TrimPrefix(strings.TrimSpace(target), `\`)
		if target == "" {
			return
		}
		if base != "" {
			target = base + `\` + target
		}
		out = append(out, target)
	}
	if grp := childOfType(n, g.lang, "namespace_use_group"); grp != nil {
		for i := 0; i < grp.NamedChildCount(); i++ {
			if c := grp.NamedChild(i); c.Type(g.lang) == "namespace_use_clause" {
				appendClause(c, prefix)
			}
		}
		return out
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Type(g.lang) == "namespace_use_clause" {
			appendClause(c, "")
		}
	}
	return out
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
	case "call_expression": // JS/TS CommonJS require('...')
		if g.name == "javascript" || g.name == "typescript" || g.name == "tsx" {
			fn := n.ChildByFieldName("function", g.lang)
			args := n.ChildByFieldName("arguments", g.lang)
			if fn != nil && fn.Type(g.lang) == "identifier" && fn.Text(src) == "require" && args != nil && args.NamedChildCount() >= 1 {
				if arg := args.NamedChild(0); arg.Type(g.lang) == "string" {
					if mod := strings.Trim(arg.Text(src), "'\""); mod != "" {
						return mod, true
					}
				}
			}
		}
	case "call", "method_call": // Ruby require / require_relative
		if g.name == "ruby" {
			mn := n.ChildByFieldName("method", g.lang)
			if mn == nil {
				return "", false
			}
			mname := mn.Text(src)
			if mname != "require" && mname != "require_relative" {
				return "", false
			}
			args := n.ChildByFieldName("arguments", g.lang)
			if args != nil && args.NamedChildCount() >= 1 {
				arg := args.NamedChild(0)
				if arg.Type(g.lang) == "string" && childOfType(arg, g.lang, "interpolation") == nil {
					if mod := strings.Trim(arg.Text(src), "'\""); mod != "" {
						if mname == "require_relative" && !strings.HasPrefix(mod, ".") {
							mod = "./" + mod
						}
						return mod, true
					}
				}
			}
		}
	}
	return "", false
}

// extraRefs supplements tree-sitter tags queries where a grammar omits @reference.call
// or places decorator callables outside the decorated definition span.
func (g *grammar) extraRefs(n *gts.Node, src []byte) []rawRef {
	switch g.name {
	case "php":
		return g.phpRefs(n, src)
	case "ruby":
		return g.rubyRefs(n, src)
	case "python":
		return g.pythonDecoratorRefs(n, src)
	}
	return nil
}

func (g *grammar) phpRefs(n *gts.Node, src []byte) []rawRef {
	line := int(n.StartPoint().Row) + 1
	start := n.StartByte()
	switch n.Type(g.lang) {
	case "function_call_expression":
		fn := n.ChildByFieldName("function", g.lang)
		if name := phpSymbolName(fn, g.lang, src); name != "" {
			return []rawRef{{name: name, kind: "call", start: start, line: line}}
		}
	case "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression":
		mn := n.ChildByFieldName("name", g.lang)
		if mn != nil && mn.Type(g.lang) == "name" {
			return []rawRef{{name: strings.TrimSpace(mn.Text(src)), kind: "call", start: start, line: line}}
		}
	case "object_creation_expression":
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if name := phpSymbolName(c, g.lang, src); name != "" {
				return []rawRef{{name: name, kind: "call", start: start, line: line}}
			}
		}
	case "base_clause", "class_interface_clause":
		var out []rawRef
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if name := phpSymbolName(c, g.lang, src); name != "" {
				out = append(out, rawRef{name: name, kind: "type", start: start, line: line})
			}
		}
		return out
	}
	return nil
}

func phpSymbolName(n *gts.Node, lang *gts.Language, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Type(lang) {
	case "name":
		return strings.TrimSpace(n.Text(src))
	case "qualified_name":
		var last string
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Type(lang) == "name" {
				last = strings.TrimSpace(c.Text(src))
			}
		}
		return last
	}
	return ""
}

func (g *grammar) rubyRefs(n *gts.Node, src []byte) []rawRef {
	typ := n.Type(g.lang)
	if typ == "identifier" {
		p := n.Parent()
		if p == nil {
			return nil
		}
		pt := p.Type(g.lang)
		if pt != "body_statement" && pt != "then" && pt != "else" && pt != "return" {
			return nil
		}
		name := strings.TrimSpace(n.Text(src))
		if len(name) < 3 || name == "self" || name == "nil" || name == "true" || name == "false" || name == "super" || name == "yield" {
			return nil
		}
		return []rawRef{{name: name, kind: "call", start: n.StartByte(), line: int(n.StartPoint().Row) + 1}}
	}
	if typ != "call" && typ != "method_call" {
		return nil
	}
	if p := n.Parent(); p != nil && (p.Type(g.lang) == "method" || p.Type(g.lang) == "singleton_method") {
		if declName := p.ChildByFieldName("name", g.lang); declName != nil && declName == n {
			return nil
		}
	}
	mn := n.ChildByFieldName("method", g.lang)
	if mn == nil {
		mn = childOfType(n, g.lang, "identifier")
	}
	if mn == nil {
		return nil
	}
	name := strings.TrimSpace(mn.Text(src))
	if name == "" || name == "require" || name == "require_relative" {
		return nil
	}
	return []rawRef{{name: name, kind: "call", start: n.StartByte(), line: int(n.StartPoint().Row) + 1}}
}

func (g *grammar) pythonDecoratorRefs(n *gts.Node, src []byte) []rawRef {
	if n.Type(g.lang) != "decorated_definition" {
		return nil
	}
	def := n.ChildByFieldName("definition", g.lang)
	if def == nil || def.EndByte() <= def.StartByte() {
		return nil
	}
	refStart := def.StartByte()
	var out []rawRef
	for i := 0; i < n.NamedChildCount(); i++ {
		dec := n.NamedChild(i)
		if dec.Type(g.lang) != "decorator" || dec.NamedChildCount() == 0 {
			continue
		}
		line := int(dec.StartPoint().Row) + 1
		expr := dec.NamedChild(0)
		if expr.Type(g.lang) == "call" {
			if isPythonRouteDecorator(expr, g.lang, src) {
				continue
			}
			expr = expr.ChildByFieldName("function", g.lang)
			if expr == nil {
				continue
			}
		}
		switch expr.Type(g.lang) {
		case "identifier":
			if name := strings.TrimSpace(expr.Text(src)); name != "" {
				out = append(out, rawRef{name: name, kind: "call", start: refStart, line: line})
			}
		case "attribute":
			if obj := expr.ChildByFieldName("object", g.lang); obj != nil && obj.Type(g.lang) == "identifier" {
				if name := strings.TrimSpace(obj.Text(src)); name != "" && name != "self" && name != "cls" {
					out = append(out, rawRef{name: name, kind: "call", start: refStart, line: line})
				}
			}
			if attr := expr.ChildByFieldName("attribute", g.lang); attr != nil {
				if name := strings.TrimSpace(attr.Text(src)); name != "" {
					out = append(out, rawRef{name: name, kind: "call", start: refStart, line: line})
				}
			}
		}
	}
	return out
}

func isPythonRouteDecorator(callNode *gts.Node, lang *gts.Language, src []byte) bool {
	fn := callNode.ChildByFieldName("function", lang)
	args := callNode.ChildByFieldName("arguments", lang)
	if fn == nil || fn.Type(lang) != "attribute" || args == nil || childOfType(args, lang, "string") == nil {
		return false
	}
	attr := fn.ChildByFieldName("attribute", lang)
	if attr == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(attr.Text(src))) {
	case "get", "post", "put", "delete", "patch", "route":
		return true
	}
	return false
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

// docFor returns docAbove when present, or a Python triple-quoted docstring at the start of the body.
func (l lineIndex) docFor(row, endRow int) string {
	if d := l.docAbove(row); d != "" {
		return d
	}
	return l.docstringBelow(row, endRow)
}

// docstringBelow extracts a Python-style """...""" docstring immediately following a def/class header.
func (l lineIndex) docstringBelow(row, endRow int) string {
	limit := min(endRow, min(len(l.lines)-1, row+12))
	for r := row + 1; r <= limit; r++ {
		t := strings.TrimSpace(l.lines[r])
		if t == "" {
			continue
		}
		quote := ""
		if strings.HasPrefix(t, `"""`) {
			quote = `"""`
		} else if strings.HasPrefix(t, `'''`) {
			quote = `'''`
		} else {
			if strings.HasSuffix(t, ":") || strings.HasSuffix(t, ",") || strings.HasSuffix(t, "(") {
				continue
			}
			return ""
		}
		rest := strings.TrimPrefix(t, quote)
		if end := strings.Index(rest, quote); end >= 0 {
			return truncate(strings.TrimSpace(rest[:end]), 400)
		}
		var block []string
		if s := strings.TrimSpace(rest); s != "" {
			block = append(block, s)
		}
		for j := r + 1; j <= endRow && j < len(l.lines) && len(block) < 8; j++ {
			line := strings.TrimSpace(l.lines[j])
			if end := strings.Index(line, quote); end >= 0 {
				if s := strings.TrimSpace(line[:end]); s != "" {
					block = append(block, s)
				}
				break
			}
			if line != "" {
				block = append(block, line)
			}
		}
		return truncate(strings.Join(block, " "), 400)
	}
	return ""
}

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

var rubyRoutePattern = routePattern{
	re:     regexp.MustCompile(`(?m)^\s*(get|post|put|delete|patch|before|after)\s*(?:\(\s*|\s+)['"](/[^'"]*)['"]`),
	method: func(m []string) string { return strings.ToUpper(m[1]) },
}

// routes finds HTTP endpoints declared with common framework idioms.
func routes(lang string, src []byte) []Symbol {
	patterns := routePatterns
	if lang == "ruby" {
		patterns = append(append([]routePattern(nil), routePatterns...), rubyRoutePattern)
	}
	var out []Symbol
	for _, p := range patterns {
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
