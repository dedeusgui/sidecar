package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeedbackCommandsRecordListPurge(t *testing.T) {
	t.Setenv("SIDECAR_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nfunc Release() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer

	// Recording needs an index; memory add creates one.
	if code := run([]string{"memory", "add", "--root", root, "--target", "a.go", "--summary", "x"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("index via memory add: exit %d: %s", code, errOut.String())
	}

	out.Reset()
	if code := run([]string{"feedback", "record", "--root", root, "--helpful", "a.go#function:Release@3", "--completed", "--confidence", "high", "--json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("record by flags: exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"evidence_id": "a.go#function:Release@3"`) {
		t.Fatalf("record JSON lacks the evidence ID: %s", out.String())
	}

	// Without evidence flags the report comes from stdin; a schema this build does not know is refused.
	out.Reset()
	report := `{"schema_version":1,"task_completed":true,"misleading_evidence_ids":["a.go"],"reason_code":"wrong_file","confidence":"low"}`
	if code := run([]string{"feedback", "record", "--root", root}, strings.NewReader(report), &out, &errOut); code != 0 {
		t.Fatalf("record from stdin: exit %d: %s", code, errOut.String())
	}
	if code := run([]string{"feedback", "record", "--root", root}, strings.NewReader(`{"schema_version":2}`), &out, &errOut); code != 1 {
		t.Fatalf("schema_version 2 accepted with exit %d", code)
	}

	out.Reset()
	if code := run([]string{"feedback", "list", "--root", root}, nil, &out, &errOut); code != 0 || strings.Count(out.String(), "valid") != 2 {
		t.Fatalf("list = exit %d, %q; want two valid records", code, out.String())
	}
	out.Reset()
	if code := run([]string{"feedback", "purge", "--root", root}, nil, &out, &errOut); code != 0 || !strings.Contains(out.String(), "0 registro") {
		t.Fatalf("purge of current records = %q; want none removed", out.String())
	}
	if code := run([]string{"feedback", "unknown", "--root", root}, nil, &out, &errOut); code != 2 {
		t.Fatalf("unknown subcommand exit %d, want 2", code)
	}
}
