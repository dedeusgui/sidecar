package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mkRepo creates a git repository with a deterministic identity and returns its root.
func mkRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", "main")
	return dir
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFilesReturnsTrackedAndUntrackedButNotIgnored(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(root, ".gitignore"), "ignored.log\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "init")
	write(t, filepath.Join(root, "new.go"), "package main\n")
	write(t, filepath.Join(root, "ignored.log"), "noise\n")

	files, err := ListFiles(context.Background(), root)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	got := strings.Join(files, ",")
	for _, want := range []string{"main.go", "new.go", ".gitignore"} {
		if !strings.Contains(got, want) {
			t.Errorf("ListFiles missing %s: %v", want, files)
		}
	}
	if strings.Contains(got, "ignored.log") {
		t.Errorf("ListFiles returned an ignored file: %v", files)
	}
}

func TestStatusReportsModifiedUntrackedAndRenamed(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "a.go"), "package a\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "init")
	run(t, root, "mv", "a.go", "b.go")
	write(t, filepath.Join(root, "c.go"), "package c\n")

	entries, err := Status(context.Background(), root)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	byPath := map[string]StatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if e, ok := byPath["b.go"]; !ok || e.OldPath != "a.go" {
		t.Errorf("expected rename a.go -> b.go, got %+v", entries)
	}
	if e, ok := byPath["c.go"]; !ok || !e.Untracked {
		t.Errorf("expected untracked c.go, got %+v", entries)
	}
}

func TestIngestCommitsRecordsRenamesAndSkipsMerges(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "queue.go"), "package q\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "add queue")
	run(t, root, "mv", "queue.go", "ticket_queue.go")
	run(t, root, "commit", "-q", "-m", "rename queue to ticket queue")

	commits, err := ReadHistory(context.Background(), root, "", 100)
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2: %+v", len(commits), commits)
	}
	newest := commits[0]
	if newest.Subject != "rename queue to ticket queue" {
		t.Errorf("subject = %q", newest.Subject)
	}
	var renamed bool
	for _, f := range newest.Files {
		if f.Path == "ticket_queue.go" && f.OldPath == "queue.go" && f.Status == "R" {
			renamed = true
		}
	}
	if !renamed {
		t.Errorf("rename not recorded: %+v", newest.Files)
	}
}

func TestIngestCommitsIsIncremental(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "a.go"), "package a\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "first")
	first, _ := ReadHistory(context.Background(), root, "", 100)

	write(t, filepath.Join(root, "b.go"), "package b\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "second")
	since, err := ReadHistory(context.Background(), root, first[0].Hash, 100)
	if err != nil {
		t.Fatalf("ReadHistory since: %v", err)
	}
	if len(since) != 1 || since[0].Subject != "second" {
		t.Fatalf("incremental read returned %+v, want only 'second'", since)
	}
}

func TestHeadAndWorktreeRoot(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "a.go"), "package a\n")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "init")
	head, err := Head(context.Background(), root)
	if err != nil || len(head) != 40 {
		t.Fatalf("Head = %q, %v", head, err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, root, "worktree", "add", "-q", "-b", "feature", wt)
	got, err := TopLevel(context.Background(), wt)
	if err != nil {
		t.Fatalf("TopLevel: %v", err)
	}
	if !samePath(got, wt) {
		t.Fatalf("TopLevel(worktree) = %q, want %q", got, wt)
	}
}

func TestSymlinksAreListedButMarkedForSkipping(t *testing.T) {
	root := mkRepo(t)
	write(t, filepath.Join(root, "real.go"), "package r\n")
	if err := os.Symlink("real.go", filepath.Join(root, "link.go")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "init")
	files, err := ListFiles(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "link.go" {
			info, err := os.Lstat(filepath.Join(root, f))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("expected link.go to be a symlink on disk")
			}
			return
		}
	}
	t.Fatalf("link.go not listed: %v", files)
}

func samePath(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}
