package extract

import (
	"fmt"
	"reflect"
	"testing"
)

// describe renders symbols as "line kind name [value]" so a test compares the whole list.
func describe(syms []Symbol) []string {
	var out []string
	for _, s := range syms {
		line := fmt.Sprintf("%d %s %s", s.Line, s.Kind, s.Name)
		if s.Value != "" {
			line += " = " + s.Value
		}
		out = append(out, line)
	}
	return out
}

func TestExtractFallbackFindsDeclarationsWithoutInventingSymbols(t *testing.T) {
	cases := []struct {
		name, lang, src string
		want            []string
	}{
		{"swift", "swift", `import Foundation

public final class Store: Base {
    static let maxRetries = 3
    func process(order: Order) -> Bool { return true }
}

struct Point { var x: Int }
enum Color { case red }
// func commented() {}
let timeoutEnabled = true
`, []string{
			"3 class Store",
			"4 constant maxRetries = 3",
			"5 function process",
			"8 struct Point",
			"9 enum Color",
			"11 constant timeoutEnabled = true",
		}},
		{"c", "c", `#include <stdio.h>
#define MAX_ACTIVE 25
#define FLAG_ON true
#define SQUARE(x) ((x) * (x))
struct Node { int value; };
enum class Mode { Fast, Slow };
static const int RETRY_LIMIT = 5;
int main(void) { return 0; }
// struct Ghost {
`, []string{
			"2 constant MAX_ACTIVE = 25",
			"3 constant FLAG_ON = true",
			"4 macro SQUARE",
			"5 struct Node",
			"6 enum Mode",
			"7 constant RETRY_LIMIT = 5",
		}},
		{"shell", "shell", `#!/bin/sh
readonly MAX_JOBS=4
DEBUG=1
deploy() {
  echo "deploying"
}
function rollback {
  echo back
}
# cleanup() {
main
`, []string{
			"2 constant MAX_JOBS = 4",
			"4 function deploy",
			"7 function rollback",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := ExtractFallback("x."+c.name, c.lang, []byte(c.src))
			if got := describe(res.Symbols); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("symbols =\n%v\nwant\n%v", got, c.want)
			}
			for _, s := range res.Symbols {
				if s.Doc != "fallback:"+c.lang || s.Parent != -1 || s.EndLine != s.Line {
					t.Fatalf("symbol %+v lacks fallback provenance or invents a block", s)
				}
			}
		})
	}
}

// Languages with no grammar still reach the fallback through File; unknown empty names still fail.
func TestFileUsesFallbackOnlyWhenNoGrammarExists(t *testing.T) {
	src := []byte("local function helper()\n  return 1\nend\n\nmain()\n")
	res, err := New().File("generic", src)
	if err != nil {
		t.Fatalf("generic: %v", err)
	}
	if got := describe(res.Symbols); !reflect.DeepEqual(got, []string{"1 function helper"}) {
		t.Fatalf("generic symbols = %v", got)
	}
	if _, err := New().File("", src); err == nil {
		t.Fatal("empty language should stay an error")
	}
	gores, err := New().File("go", []byte("package x\n\nfunc Run() {}\n"))
	if err != nil || len(gores.Symbols) == 0 || gores.Symbols[0].Doc == "fallback:go" {
		t.Fatalf("go should keep its grammar, got %+v (%v)", gores.Symbols, err)
	}
}
