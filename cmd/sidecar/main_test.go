package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryCommandsListAddPurge(t *testing.T) {
	t.Setenv("SIDECAR_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nfunc Release() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer

	if code := run([]string{"memory", "list", "--root", root, "--json"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("list on a fresh worktree: exit %d: %s", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Fatalf("empty list JSON = %q, want []", got)
	}

	out.Reset()
	if code := run([]string{"memory", "add", "--root", root, "--target", "a.go", "--summary", "usar Release"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("add: exit %d: %s", code, errOut.String())
	}
	out.Reset()
	if code := run([]string{"memory", "list", "--root", root}, nil, &out, &errOut); code != 0 {
		t.Fatalf("list: exit %d", code)
	}
	if !strings.Contains(out.String(), "valid") || !strings.Contains(out.String(), "usar Release") {
		t.Fatalf("list output = %q", out.String())
	}

	out.Reset()
	if code := run([]string{"memory", "purge", "--root", root}, nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "0 decis") {
		t.Fatalf("purge invalidated = %q (exit %d); a valid decision must survive", out.String(), code)
	}
	if code := run([]string{"memory", "unknown", "--root", root}, nil, &out, &errOut); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
}

func TestObserveIsSilentEvenWhenMisconfigured(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"observe", "--agent", "gemini", "--event", "PreToolUse"}, strings.NewReader("{}"), &out, &errOut)
	if code != 0 || out.Len() != 0 {
		t.Fatalf("observe wrote stdout %q (exit %d)", out.String(), code)
	}
}
