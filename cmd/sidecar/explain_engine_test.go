package main

import (
	"bytes"
	"strings"
	"testing"
)

// An unknown engine is a usage error, reported before any repository work starts.
func TestExplainRejectsUnknownEngine(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdExplain([]string{"--engine", "v9"}, strings.NewReader("prompt"), &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "v1, v2 ou v3") {
		t.Errorf("stderr = %q, want the engine choices", errOut.String())
	}
}

func TestExploreRequiresAQuery(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdExplore([]string{"--root", t.TempDir()}, &out, &errOut); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if out.Len() != 0 || !strings.Contains(errOut.String(), "sidecar explore --query") {
		t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}
