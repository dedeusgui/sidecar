package store

import (
	"fmt"
	"reflect"
	"testing"
)

// Symbol ids follow insertion order, which follows parallel indexing. The same files indexed
// in opposite order must still return the same sequence.
func TestQueryOrderIgnoresInsertionOrder(t *testing.T) {
	files := []struct {
		path string
		syms []SymbolRecord
	}{
		{"src/a.go", []SymbolRecord{
			{Kind: "func", Name: "Release", Line: 3, EndLine: 5, Parent: -1},
			{Kind: "func", Name: "Helper", Line: 9, EndLine: 9, Parent: -1},
		}},
		{"src/b.go", []SymbolRecord{{Kind: "func", Name: "Release", Line: 1, EndLine: 2, Parent: -1}}},
	}
	sequence := func(order []int) (all, exact []string) {
		s, _ := openTemp(t)
		for _, i := range order {
			if err := s.ReplaceFile(FileRecord{Path: files[i].path, Lang: "go", Kind: "code", Hash: "h"}, files[i].syms, nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		hits, err := s.AllSymbols()
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			all = append(all, fmt.Sprintf("%s:%d:%s:%s", h.Path, h.Line, h.Kind, h.Name))
		}
		exactHits, err := s.ExactSymbols([]string{KeyOf("Release")}, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range exactHits {
			exact = append(exact, fmt.Sprintf("%s:%d:%s:%s", h.Path, h.Line, h.Kind, h.Name))
		}
		return all, exact
	}
	allAB, exactAB := sequence([]int{0, 1})
	allBA, exactBA := sequence([]int{1, 0})
	if !reflect.DeepEqual(allAB, allBA) {
		t.Fatalf("AllSymbols depends on insertion order:\n%v\n%v", allAB, allBA)
	}
	if !reflect.DeepEqual(exactAB, exactBA) {
		t.Fatalf("ExactSymbols depends on insertion order:\n%v\n%v", exactAB, exactBA)
	}
	if len(allAB) != 3 || allAB[0] != "src/a.go:3:func:Release" {
		t.Fatalf("unexpected AllSymbols order: %v", allAB)
	}
}
