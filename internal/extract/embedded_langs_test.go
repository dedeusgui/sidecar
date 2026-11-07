package extract

import "testing"

// Rust, PHP, Ruby and Kotlin ship embedded tree-sitter grammars, so they use the same
// tags path as the other languages. These cases pin names and exact declaration lines.
func TestEmbeddedGrammarsForRustPHPRubyKotlin(t *testing.T) {
	cases := []struct {
		lang, src string
		want      map[string]int // symbol name -> 1-based line
	}{
		{"rust", "pub struct Foo { x: i32 }\n\npub fn run(a: i32) -> i32 {\n    helper(a)\n}\n\nfn helper(a: i32) -> i32 { a }\n",
			map[string]int{"Foo": 1, "run": 3, "helper": 7}},
		{"php", "<?php\nclass Foo {\n    public function bar() { return baz(); }\n}\nfunction baz() { return 1; }\n",
			map[string]int{"Foo": 2, "bar": 3, "baz": 5}},
		{"ruby", "class Foo\n  def bar\n    baz\n  end\nend\n\ndef baz\n  1\nend\n",
			map[string]int{"Foo": 1, "bar": 2, "baz": 7}},
		{"kotlin", "package x\n\nclass Foo {\n    fun bar() = baz()\n}\n\nfun baz() = 1\n",
			map[string]int{"Foo": 3, "bar": 4, "baz": 7}},
	}
	for _, c := range cases {
		res, err := New().File(c.lang, []byte(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.lang, err)
		}
		got := map[string]int{}
		for _, s := range res.Symbols {
			if _, ok := c.want[s.Name]; ok {
				got[s.Name] = s.Line
			}
		}
		for name, line := range c.want {
			if got[name] != line {
				t.Errorf("%s: symbol %s at line %d, want %d (symbols: %+v)", c.lang, name, got[name], line, res.Symbols)
			}
		}
	}
}
