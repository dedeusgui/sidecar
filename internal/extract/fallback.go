package extract

import (
	"regexp"
	"strings"
)

// Declarations must start a line, so comments and strings are not read as definitions.
// The name must be followed by a declaration delimiter: "struct Foo *make()" is a function
// returning a pointer, not a struct.
var fallbackDeclRe = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|fileprivate|open|static|final|export|pub|local|async|override|abstract|sealed|inline|case)\s+)*(func|fn|def|function|class|struct|interface|enum|trait|object|defmodule)\s+(?:(?:class|struct)\s+)?([A-Za-z_]\w*)\s*(?:[(<{:;\[=]|$|(?:do|extends|implements|where|with|final)\b)`)

var fallbackKinds = map[string]string{
	"func": "function", "fn": "function", "def": "function", "function": "function",
	"class": "class", "object": "class", "defmodule": "module", "trait": "trait",
	"struct": "struct", "interface": "interface", "enum": "enum",
}

var fallbackDefineRe = regexp.MustCompile(`^\s*#\s*define\s+([A-Za-z_]\w*)(.*)$`)

// fallbackLiteralRe matches a numeric or boolean literal, the only values kept for constants.
var fallbackLiteralRe = regexp.MustCompile(`^(?:-?\d[\w.]*|true|false)$`)

// fallbackConstRe requires a qualifier (const, let, static...) so that a bare assignment
// such as DEBUG=1 is not reported as a constant.
var fallbackConstRe = regexp.MustCompile(`^\s*(?:(?:export|public|private|protected|static|final|const|constexpr|let|val|local|readonly|pub|inline|extern)\s+)+(?:[A-Za-z_][\w<>*]*\s+)?([A-Za-z_]\w*)\s*(?::\s*[A-Za-z_][\w<>?]*\s*)?=\s*(-?\d[\w.]*|true|false)\s*;?\s*(?://.*)?$`)

// shell functions: "name() {" on one line. Without the brace, "name()" is a Lua call.
var fallbackShellFuncRe = regexp.MustCompile(`^\s*([A-Za-z_][\w.-]*)\s*\(\s*\)\s*\{`)

// ExtractFallback reads top-level-looking definitions and numeric or boolean constants from a
// language without a tree-sitter grammar. It is conservative: it reports only what a declaration
// line states, it does not guess block ends (EndLine is the declaration line), and it has no
// references or imports. The output depends only on src. path is reserved for diagnostics.
func ExtractFallback(path, lang string, src []byte) Result {
	var res Result
	off := 0
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimRight(raw, "\r")
		start := off
		off += len(raw) + 1
		sym, ok := fallbackSymbol(line)
		if !ok {
			continue
		}
		sym.Signature = truncate(strings.TrimSpace(line), 160)
		sym.Doc = "fallback:" + lang
		sym.Line, sym.EndLine, sym.Parent = i+1, i+1, -1
		sym.startByte, sym.endByte = uint32(start), uint32(start+len(line))
		res.Symbols = append(res.Symbols, sym)
	}
	return res
}

func fallbackSymbol(line string) (Symbol, bool) {
	if m := fallbackDeclRe.FindStringSubmatch(line); m != nil {
		return Symbol{Kind: fallbackKinds[m[1]], Name: m[2]}, true
	}
	if m := fallbackDefineRe.FindStringSubmatch(line); m != nil {
		if v := strings.TrimSpace(m[2]); fallbackLiteralRe.MatchString(v) {
			return Symbol{Kind: "constant", Name: m[1], Value: v}, true
		}
		return Symbol{Kind: "macro", Name: m[1]}, true
	}
	if m := fallbackConstRe.FindStringSubmatch(line); m != nil {
		return Symbol{Kind: "constant", Name: m[1], Value: m[2]}, true
	}
	if m := fallbackShellFuncRe.FindStringSubmatch(line); m != nil {
		return Symbol{Kind: "function", Name: m[1]}, true
	}
	return Symbol{}, false
}
