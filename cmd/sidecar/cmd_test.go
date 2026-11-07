package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/app"
	"github.com/dedeusgui/sidecar/internal/paths"
)

// isolateHome keeps the hook log, the config and the trust file out of the real user folders.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("SIDECAR_HOME", t.TempDir())
	t.Setenv("SIDECAR_CONFIG_HOME", t.TempDir())
	t.Setenv("SIDECAR_LSP", "")
}

func TestSetupNeverInstallsWithoutConsent(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	// A non-terminal stdin without --yes must stop before any command runs.
	if code := run([]string{"lsp", "setup", "--language", "go"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "--yes") {
		t.Fatalf("no consent hint: %q", errOut.String())
	}
}

func TestSetupBlockedLanguageIsRefused(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"lsp", "setup", "--language", "php", "--yes"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "bloqueado") {
		t.Fatalf("plan does not say it is blocked: %q", out.String())
	}
}

func TestSetupDryRunShowsPinnedCommandAndRollback(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"lsp", "setup", "--language", "go", "--dry-run"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	for _, want := range []string{"go install golang.org/x/tools/gopls@v0.24.0", "origem:", "licença:", "rollback:", "nada foi alterado"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output misses %q:\n%s", want, out.String())
		}
	}
}

func TestSetupRejectsUnknownLanguage(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"lsp", "setup", "--language", "cobol", "--dry-run"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestActivityRejectsBadLimit(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"activity", "--limit", "0"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("limit 0: exit = %d, want 2", code)
	}
}

func TestActivityAndStatuslineWithoutLogExitZero(t *testing.T) {
	isolateHome(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"activity", "--json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("activity --json exit = %d: %s", code, errOut.String())
	}
	var rep map[string]any
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("activity output is not JSON: %v\n%s", err, out.String())
	}
	out.Reset()
	if code := run([]string{"statusline"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("statusline exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != "Sidecar: no match" {
		t.Fatalf("statusline without log = %q", out.String())
	}
}

func TestStatusLineReadsSessionFromStdinJSON(t *testing.T) {
	isolateHome(t)
	if err := os.MkdirAll(filepath.Dir(paths.HookLog()), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"status":"ok","injected_chars":9,"evidence":4,"total_ms":18,"session":"` + app.SessionKey("sess-1") + `"}` + "\n"
	if err := os.WriteFile(paths.HookLog(), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	in := strings.NewReader(`{"session_id":"sess-1","model":{"id":"x"}}`)
	if code := run([]string{"statusline"}, in, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out.String()) != "Sidecar: injected · 4 refs · 18ms" {
		t.Fatalf("statusline = %q", out.String())
	}
}

func TestStatusAndLSPStatusEmitJSON(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	if code := run([]string{"status", "--root", dir, "--json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("status --json exit = %d: %s", code, errOut.String())
	}
	var status map[string]any
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("status output is not JSON: %v", err)
	}
	out.Reset()
	if code := run([]string{"lsp", "status", "--root", dir, "--json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("lsp status --json exit = %d: %s", code, errOut.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 7 {
		t.Fatalf("lsp status rows: %d (%v)", len(rows), err)
	}
}
