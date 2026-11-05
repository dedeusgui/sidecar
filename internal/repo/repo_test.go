package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRootWalksUpToGitDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, isGit := FindRoot(deep)
	if got != root || !isGit {
		t.Fatalf("FindRoot = %q, %v; want %q, true", got, isGit, root)
	}
}

func TestFindRootWithoutGitUsesDirectory(t *testing.T) {
	dir := t.TempDir()
	got, isGit := FindRoot(dir)
	if got != dir || isGit {
		t.Fatalf("FindRoot = %q, %v; want %q, false", got, isGit, dir)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		path, lang, kind string
	}{
		{"internal/queue.go", "go", "code"},
		{"internal/queue_test.go", "go", "test"},
		{"app/service.py", "python", "code"},
		{"tests/test_service.py", "python", "test"},
		{"src/Service.java", "java", "code"},
		{"src/ServiceTest.java", "java", "test"},
		{"web/app.tsx", "tsx", "code"},
		{"web/app.spec.ts", "typescript", "test"},
		{"Billing/Invoice.cs", "c_sharp", "code"},
		{"db/schema.sql", "sql", "code"},
		{"docs/guia.md", "markdown", "doc"},
		{"image.png", "", "binary"},
		{"README", "", "doc"},
	}
	for _, c := range cases {
		lang, kind := Classify(c.path)
		if lang != c.lang || kind != c.kind {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", c.path, lang, kind, c.lang, c.kind)
		}
	}
}

func TestSensitivePathsAreNeverContentIndexed(t *testing.T) {
	for _, p := range []string{".env", "config/.env.production", "certs/server.pem", "keys/id_rsa", "deploy/secrets.key", ".npmrc"} {
		if !IsSensitive(p) {
			t.Errorf("IsSensitive(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"internal/envelope.go", "docs/keys.md", "src/secretary.go"} {
		if IsSensitive(p) {
			t.Errorf("IsSensitive(%q) = true, want false", p)
		}
	}
}

func TestReadSourceSkipsBinaryAndLargeFiles(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "x.go")
	if err := os.WriteFile(bin, []byte("package x\x00\x01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, reason := ReadSource(bin, MaxSourceBytes); reason != "binary" {
		t.Fatalf("binary file: reason = %q, want binary", reason)
	}
	big := filepath.Join(dir, "big.go")
	if err := os.WriteFile(big, make([]byte, MaxSourceBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, reason := ReadSource(big, MaxSourceBytes); reason != "too_large" {
		t.Fatalf("large file: reason = %q, want too_large", reason)
	}
}
