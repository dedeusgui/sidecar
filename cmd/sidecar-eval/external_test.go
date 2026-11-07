package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo creates a clean repository with one commit and returns its path and SHA.
func gitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir, run("rev-parse", "HEAD")
}

// writeManifest writes a manifest for one repo named "r" and returns its path.
func writeManifest(t *testing.T, repoPath, commit, license, cases string) string {
	t.Helper()
	body := fmt.Sprintf(`{"repos":{"r":{"path":%q,"commit":%q,"source":"test fixture","license":%q}},"cases":[%s]}`,
		repoPath, commit, license, cases)
	p := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const positiveCase = `{"id":"c1","split":"arb","repo":"r","prompt":"onde fica Run","relevant":[{"path":"a.go"}]}`

func TestLoadExternalAcceptsPinnedCleanCheckout(t *testing.T) {
	dir, sha := gitRepo(t)
	m, err := loadExternal(writeManifest(t, dir, sha, "MIT", positiveCase))
	if err != nil {
		t.Fatalf("pinned clean checkout rejected: %v", err)
	}
	cases := casesOf(m)
	if len(cases) != 1 || cases[0].Negative || cases[0].Relevant[0].Name != "" {
		t.Fatalf("file-level positive case converted wrongly: %+v", cases)
	}
}

func TestNoGoldCasesBecomeNegatives(t *testing.T) {
	dir, sha := gitRepo(t)
	m, err := loadExternal(writeManifest(t, dir, sha, "MIT",
		`{"id":"n1","split":"arb","repo":"r","prompt":"bom dia","no_gold":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if cases := casesOf(m); !cases[0].Negative || len(cases[0].Relevant) != 0 {
		t.Fatalf("no_gold case did not become a negative: %+v", cases[0])
	}
}

func TestLoadExternalRejectsBrokenPins(t *testing.T) {
	dir, sha := gitRepo(t)
	cases := map[string]struct {
		manifest string
		want     string
	}{
		"wrong commit": {writeManifest(t, dir, strings.Repeat("0", 40), "MIT", positiveCase), "pinned"},
		"short sha":    {writeManifest(t, dir, sha[:12], "MIT", positiveCase), "40-character"},
		"no license":   {writeManifest(t, dir, sha, "", positiveCase), "license"},
		"unknown field": {writeManifest(t, dir, sha, "MIT",
			`{"id":"c1","split":"arb","repo":"r","prompt":"x","gold":[{"path":"a.go"}]}`), "gold"},
		"positive without gold": {writeManifest(t, dir, sha, "MIT",
			`{"id":"c1","split":"arb","repo":"r","prompt":"x"}`), "needs relevant"},
		"no_gold with gold": {writeManifest(t, dir, sha, "MIT",
			`{"id":"c1","split":"arb","repo":"r","prompt":"x","no_gold":true,"relevant":[{"path":"a.go"}]}`), "must not list"},
		"unknown repo": {writeManifest(t, dir, sha, "MIT",
			`{"id":"c1","split":"arb","repo":"zz","prompt":"x","relevant":[{"path":"a.go"}]}`), "unknown repo"},
		"repeated id": {writeManifest(t, dir, sha, "MIT", positiveCase+","+positiveCase), "repeated"},
	}
	for name, tc := range cases {
		_, err := loadExternal(tc.manifest)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}

func TestLoadExternalRejectsDirtyCheckout(t *testing.T) {
	dir, sha := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n// local edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadExternal(writeManifest(t, dir, sha, "MIT", positiveCase))
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty checkout accepted: %v", err)
	}
}
