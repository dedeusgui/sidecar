package lsp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolate points every user-level input (trust file, Claude settings, opt-in, PATH) at
// temp folders, so the tests never read or write the real machine state.
func isolate(t *testing.T) (binDir string) {
	t.Helper()
	t.Setenv("SIDECAR_CONFIG_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("SIDECAR_LSP", "")
	binDir = t.TempDir()
	t.Setenv("PATH", binDir)
	return binDir
}

func writeFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func byLang(t *testing.T, root string) map[string]Server {
	t.Helper()
	rows, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Server{}
	for _, r := range rows {
		got[r.LanguageID] = r
	}
	return got
}

func TestDiscoverMonorepoMarkers(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	writeFiles(t, root, "go.mod", "frontend/package.json", "frontend/tsconfig.json", "services/api/pyproject.toml", "vendor/ignored/Cargo.toml")
	got := byLang(t, root)
	if len(got["go"].RepoMarkers) != 1 || len(got["typescript"].RepoMarkers) != 2 || got["rust"].Status != StatusNotApplicable {
		t.Fatalf("unexpected discovery: %#v", got)
	}
	for _, r := range got {
		if r.TrustedWorkspace {
			t.Fatalf("workspace implicitly trusted: %#v", r)
		}
	}
}

func TestDiscoverStatusLadder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the fake server")
	}
	binDir := isolate(t)
	script := "#!/bin/sh\necho 'golang.org/x/tools/gopls test-version'\n"
	if err := os.WriteFile(filepath.Join(binDir, "gopls"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFiles(t, root, "go.mod")

	if r := byLang(t, root)["go"]; r.Status != StatusDisabled || r.InstalledServerPath == "" || r.ServerVersion != "golang.org/x/tools/gopls test-version" {
		t.Fatalf("without opt-in: %+v", r)
	}
	t.Setenv("SIDECAR_LSP", "1")
	if r := byLang(t, root)["go"]; r.Status != StatusRequiresConsent {
		t.Fatalf("opt-in without trust: %+v", r)
	}
	if _, err := Trust(root); err != nil {
		t.Fatal(err)
	}
	if r := byLang(t, root)["go"]; r.Status != StatusAvailable || !r.TrustedWorkspace {
		t.Fatalf("opt-in and trust: %+v", r)
	}
	if err := os.Remove(filepath.Join(binDir, "gopls")); err != nil {
		t.Fatal(err)
	}
	if r := byLang(t, root)["go"]; r.Status != StatusMissing {
		t.Fatalf("server removed from PATH: %+v", r)
	}
}

func TestDiscoverReportsClaudeOfficialPluginCoexistence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the fake server")
	}
	binDir := isolate(t)
	if err := os.WriteFile(filepath.Join(binDir, "gopls"), []byte("#!/bin/sh\necho gopls-test\n"), 0755); err != nil {
		t.Fatal(err)
	}
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	settings := `{"enabledPlugins":{"gopls-lsp@claude-plugins-official":true}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFiles(t, root, "go.mod")
	r := byLang(t, root)["go"]
	if !r.ClaudePluginCoexists || !strings.Contains(r.Reason, "gopls-lsp") {
		t.Fatalf("official plugin not reported: %+v", r)
	}
}

func TestTrustIsPerCanonicalRoot(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	if Trusted(root) {
		t.Fatal("untrusted by default")
	}
	c, err := Trust(root)
	if err != nil {
		t.Fatal(err)
	}
	if !Trusted(root) || !Trusted(filepath.Join(root, ".")) {
		t.Fatalf("trusted root %q not recognised", c)
	}
	if Trusted(t.TempDir()) {
		t.Fatal("other root trusted")
	}
}
