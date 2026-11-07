package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A plain folder goes through the full cycle: index, edit, incremental re-sync, removal.
func TestPlainFolderFullCycleIndexEditResyncRemove(t *testing.T) {
	root := t.TempDir()
	put(t, root, "pricing.py", "def compute_total_price():\n    return 1\n")
	put(t, root, "legacy.py", "def legacy_helper():\n    return 0\n")
	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("legacy helper", 10); len(hits) == 0 {
		t.Fatal("initial sync did not index legacy.py")
	}

	put(t, root, "pricing.py", "def compute_discounted_total():\n    return 2\n\ndef tax():\n    return 3\n")
	rep := mustSync(t, ix, Options{})
	if rep.Parsed != 1 {
		t.Fatalf("edit re-parsed %d files, want only pricing.py", rep.Parsed)
	}
	if hits, _ := ix.Store().SearchSymbols("discounted total", 10); countNamed(hits, "compute_discounted_total") != 1 {
		t.Fatalf("edited definition not indexed: %+v", hits)
	}
	if hits, _ := ix.Store().SearchSymbols("compute total price", 10); countNamed(hits, "compute_total_price") != 0 {
		t.Fatal("stale definition survived the edit")
	}

	if err := os.Remove(filepath.Join(root, "legacy.py")); err != nil {
		t.Fatal(err)
	}
	rep = mustSync(t, ix, Options{})
	if rep.Removed != 1 {
		t.Fatalf("removal reported Removed=%d, want 1", rep.Removed)
	}
	if hits, _ := ix.Store().SearchSymbols("legacy helper", 10); countNamed(hits, "legacy_helper") != 0 {
		t.Fatal("removed file still indexed")
	}
}

// Symlinks to files and directories outside the repository, and a two-link cycle, are skipped:
// the sync completes and no outside content is indexed.
func TestSymlinksOutsideRootAndCyclesAreSkipped(t *testing.T) {
	outside := t.TempDir()
	put(t, outside, "secret.py", "def leaked_secret_value():\n    return 42\n")
	root := t.TempDir()
	put(t, root, "app.py", "def serve_request():\n    return 1\n")
	mustSymlink(t, filepath.Join(outside, "secret.py"), filepath.Join(root, "passwd_link.py"))
	mustSymlink(t, outside, filepath.Join(root, "parent_link"))
	mustSymlink(t, "b", filepath.Join(root, "a"))
	mustSymlink(t, "a", filepath.Join(root, "b"))

	ix := openIndex(t, root)
	syncWithin(t, ix)
	if hits, _ := ix.Store().SearchSymbols("leaked secret value", 10); len(hits) != 0 {
		t.Fatalf("content behind a symlink was indexed: %+v", hits)
	}
	if hits, _ := ix.Store().SearchSymbols("serve request", 10); countNamed(hits, "serve_request") != 1 {
		t.Fatal("regular file next to the symlinks was not indexed")
	}
}

// In a Git worktree the tracked path ext/data.py still exists after ext/ becomes a symlink to a
// directory outside the repository. Its content must not be read through that link.
func TestTrackedPathThroughSymlinkedDirectoryDoesNotLeak(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "ext/data.py", "def in_repo_marker():\n    return 1\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	outside := t.TempDir()
	put(t, outside, "data.py", "def outside_secret_marker():\n    return 2\n")
	if err := os.RemoveAll(filepath.Join(root, "ext")); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, outside, filepath.Join(root, "ext"))

	ix := openIndex(t, root)
	syncWithin(t, ix)
	if hits, _ := ix.Store().SearchSymbols("outside secret marker", 10); len(hits) != 0 {
		t.Fatalf("content read through a symlinked directory: %+v", hits)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// syncWithin fails instead of hanging when a sync does not finish.
func syncWithin(t *testing.T, ix *Index) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rep, err := ix.Sync(ctx, Options{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !rep.Complete {
		t.Fatalf("Sync did not complete: %+v", rep)
	}
}
