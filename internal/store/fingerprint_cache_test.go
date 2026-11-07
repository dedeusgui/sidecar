package store

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeRepoFile(t *testing.T, root, rel, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Overwriting the stored copy with a marker shows whether DiscoverCached walked the tree again.
func TestDiscoverCachedReusesStoredCopyUntilManifestChanges(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire example.com/old v1.0.0\n")
	writeRepoFile(t, root, "main.go", "package main\n")
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "init")
	st, _ := openTemp(t)
	ctx := context.Background()

	first := DiscoverCached(ctx, root, st)
	if !first.Git || len(first.Manifests) != 1 || first.Manifests[0].Dependencies[0].Name != "example.com/old" {
		t.Fatalf("cold call = %+v", first)
	}

	marked := first
	marked.Files = 999
	b, _ := json.Marshal(marked)
	if err := st.SetMeta(fingerprintKey, string(b)); err != nil {
		t.Fatal(err)
	}
	if hit := DiscoverCached(ctx, root, st); hit.Files != 999 {
		t.Fatalf("unchanged repository walked again: Files = %d, want the stored 999", hit.Files)
	}

	writeRepoFile(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire example.com/new v2.0.0\n")
	after := DiscoverCached(ctx, root, st)
	if after.Files == 999 || len(after.Manifests) != 1 || after.Manifests[0].Dependencies[0].Name != "example.com/new" {
		t.Fatalf("changed manifest was not re-read: %+v", after)
	}
}

// Outside Git nothing is stored, so every call reads the tree.
func TestDiscoverCachedIsDisabledOutsideGit(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "main.go", "package main\n")
	st, _ := openTemp(t)
	ctx := context.Background()

	first := DiscoverCached(ctx, root, st)
	if first.Git {
		t.Fatalf("expected a non-Git fingerprint, got %+v", first)
	}
	marked := first
	marked.Files = 999
	b, _ := json.Marshal(marked)
	_ = st.SetMeta(fingerprintKey, string(b))
	if again := DiscoverCached(ctx, root, st); again.Files == 999 {
		t.Fatal("non-Git copy was reused")
	}
}
