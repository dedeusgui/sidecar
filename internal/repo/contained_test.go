package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainedRejectsPathsThatLeaveTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "dir", "ok.txt"), filepath.Join(outside, "secret.txt")} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leaf.txt")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, abs string
		want      bool
	}{
		{"regular file in root", filepath.Join(root, "dir", "ok.txt"), true},
		{"file under symlinked directory", filepath.Join(root, "linkdir", "secret.txt"), false},
		{"symlink leaf pointing outside", filepath.Join(root, "leaf.txt"), false},
		{"missing file", filepath.Join(root, "dir", "missing.txt"), false},
	}
	for _, c := range cases {
		if got := Contained(root, c.abs); got != c.want {
			t.Errorf("%s: Contained = %v, want %v", c.name, got, c.want)
		}
	}
}
