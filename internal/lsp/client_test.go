package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain doubles the test binary as a fake language server when SIDECAR_LSP_MOCK is set.
// This keeps the contract tests free of extra fixture programs.
func TestMain(m *testing.M) {
	if mode := os.Getenv("SIDECAR_LSP_MOCK"); mode != "" {
		runMockServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func reply(id json.RawMessage, result any) {
	writeMock(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func replyErr(id json.RawMessage, message string) {
	writeMock(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32603, "message": message}})
}

func writeMock(msg map[string]any) {
	body, _ := json.Marshal(msg)
	_, _ = os.Stdout.Write([]byte("Content-Length: " + itoa(len(body)) + "\r\n\r\n"))
	_, _ = os.Stdout.Write(body)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func rng(sl, sc, el, ec int) map[string]any {
	return map[string]any{
		"start": map[string]int{"line": sl, "character": sc},
		"end":   map[string]int{"line": el, "character": ec},
	}
}

// runMockServer answers the requests Sidecar sends. Modes: ok, slow (navigation waits 2s),
// error (navigation fails), noprov (initialize omits documentSymbol and references).
func runMockServer(mode string) {
	r := bufio.NewReaderSize(os.Stdin, 64<<10)
	var rootURI string
	for {
		body, err := readFrame(r)
		if err != nil {
			return
		}
		var msg rpcMessage
		if json.Unmarshal(body, &msg) != nil || msg.Method == "" {
			continue
		}
		switch msg.Method {
		case "initialize":
			var p struct {
				RootURI string `json:"rootUri"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			rootURI = p.RootURI
			caps := map[string]any{"documentSymbolProvider": true, "definitionProvider": true, "referencesProvider": true}
			if mode == "noprov" {
				caps = map[string]any{"definitionProvider": true}
			}
			// A server request the client must answer, or a real server would stall here.
			writeMock(map[string]any{"jsonrpc": "2.0", "id": 900, "method": "workspace/configuration",
				"params": map[string]any{"items": []any{map[string]string{"section": "gopls"}}}})
			reply(msg.ID, map[string]any{"capabilities": caps})
		case "textDocument/documentSymbol", "textDocument/definition", "textDocument/references":
			if mode == "slow" {
				time.Sleep(2 * time.Second)
			}
			if mode == "error" {
				replyErr(msg.ID, "boom")
				continue
			}
			var p struct {
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			doc := p.TextDocument.URI
			outside := "file:///etc/hosts"
			link := rootURI + "/link.go" // symlink inside the worktree that points outside
			switch msg.Method {
			case "textDocument/documentSymbol":
				reply(msg.ID, []any{map[string]any{"name": "Run", "kind": 12, "range": rng(2, 0, 4, 1),
					"selectionRange": rng(2, 5, 2, 8),
					"children":       []any{map[string]any{"name": "inner", "kind": 12, "range": rng(3, 1, 3, 5)}}}})
			case "textDocument/definition":
				reply(msg.ID, []any{
					map[string]any{"targetUri": doc, "targetRange": rng(0, 0, 2, 1), "targetSelectionRange": rng(0, 5, 0, 8)},
					map[string]any{"targetUri": outside, "targetRange": rng(0, 0, 0, 1), "targetSelectionRange": rng(0, 0, 0, 1)},
				})
			case "textDocument/references":
				reply(msg.ID, []any{
					map[string]any{"uri": doc, "range": rng(5, 5, 5, 10)},
					map[string]any{"uri": outside, "range": rng(0, 0, 0, 1)},
					map[string]any{"uri": link, "range": rng(0, 0, 0, 1)},
				})
			}
		case "shutdown":
			reply(msg.ID, nil)
		case "exit":
			return
		}
	}
}

// mockClient starts this test binary as the server in the given mode, with root trusted.
func mockClient(t *testing.T, root, mode string, opts Options) (*Client, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIDECAR_LSP", "1")
	t.Setenv("SIDECAR_LSP_MOCK", mode)
	if _, err := Trust(root); err != nil {
		t.Fatal(err)
	}
	return Start(context.Background(), root, root, "go", exe, opts)
}

func worktree(t *testing.T) (root, outside string) {
	t.Helper()
	isolate(t)
	root = t.TempDir()
	outDir := t.TempDir()
	outside = filepath.Join(outDir, "secret.go")
	if err := os.WriteFile(outside, []byte("package secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, "go.mod")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc Run() {\n}\n\nfunc main() {\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return root, outside
}

func TestClientNavigationStaysInsideWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and POSIX fake server")
	}
	root, _ := worktree(t)
	c, err := mockClient(t, root, "ok", Options{RequestTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	caps := c.Capabilities()
	if !slices.Contains(caps, "documentSymbolProvider") || !slices.Contains(caps, "referencesProvider") {
		t.Fatalf("capabilities = %v", caps)
	}
	ctx := context.Background()
	syms, err := c.DocumentSymbols(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 2 || syms[0].Name != "Run" || syms[0].Line != 3 || syms[0].EndLine != 5 || syms[0].Path != "main.go" || syms[1].Name != "inner" || syms[1].Line != 4 {
		t.Fatalf("symbols = %+v", syms)
	}
	defs, err := c.Definition(ctx, "main.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || defs[0].Path != "main.go" || defs[0].Line != 1 {
		t.Fatalf("definitions must keep only the in-tree location, got %+v", defs)
	}
	refs, err := c.References(ctx, "main.go", 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Line != 6 {
		t.Fatalf("references must drop the outside and symlinked locations, got %+v", refs)
	}
	ev := refs[0].Evidence()
	if ev.Source != "lsp" || ev.Verdict != "VERIFIED" || ev.Hit.Path != "main.go" || ev.Hit.Line != 6 {
		t.Fatalf("evidence = %+v", ev)
	}
	// Requests for paths that leave the worktree are refused before the server sees them.
	if _, err := c.DocumentSymbols(ctx, "../outside.go"); err == nil {
		t.Fatal("traversal path accepted")
	}
	if _, err := c.DocumentSymbols(ctx, "link.go"); err == nil {
		t.Fatal("symlink to outside accepted")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.exited:
	default:
		t.Fatal("server process not reaped after Close")
	}
}

func TestClientRefusesWithoutOptInOrTrust(t *testing.T) {
	root, _ := worktree(t)
	exe, _ := os.Executable()
	if _, err := Start(context.Background(), root, root, "go", exe, Options{}); err == nil || !strings.Contains(err.Error(), "SIDECAR_LSP") {
		t.Fatalf("started without opt-in: %v", err)
	}
	t.Setenv("SIDECAR_LSP", "1")
	if _, err := Start(context.Background(), root, root, "go", exe, Options{}); err == nil || !strings.Contains(err.Error(), "não confiável") {
		t.Fatalf("started without trust: %v", err)
	}
}

func TestClientTimesOutOnSlowServerAndStillCloses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake server")
	}
	root, _ := worktree(t)
	c, err := mockClient(t, root, "slow", Options{RequestTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.DocumentSymbols(context.Background(), "main.go")
	if err == nil || !strings.Contains(err.Error(), "sem resposta") {
		t.Fatalf("slow server not timed out: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("timeout took %s, want about 200ms", elapsed)
	}
	closeStart := time.Now()
	_ = c.Close()
	if elapsed := time.Since(closeStart); elapsed > 4*time.Second {
		t.Fatalf("Close blocked for %s on a stuck server", elapsed)
	}
}

func TestClientReportsServerErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake server")
	}
	root, _ := worktree(t)
	c, err := mockClient(t, root, "error", Options{RequestTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.References(context.Background(), "main.go", 0, 0); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("server error not reported: %v", err)
	}
}

func TestClientUnannouncedCapabilityIsUnsupported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake server")
	}
	root, _ := worktree(t)
	c, err := mockClient(t, root, "noprov", Options{RequestTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	if _, err := c.DocumentSymbols(ctx, "main.go"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("documentSymbol: %v, want ErrUnsupported", err)
	}
	if _, err := c.References(ctx, "main.go", 0, 0); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("references: %v, want ErrUnsupported", err)
	}
	if defs, err := c.Definition(ctx, "main.go", 0, 0); err != nil || len(defs) != 1 {
		t.Fatalf("definition should still work: %v %+v", err, defs)
	}
}

func TestFileURIRoundTripsUnicodeAndSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX path form")
	}
	p := "/tmp/Projeto Ç/módulo#1/arquivo.go"
	got, err := pathFromURI(fileURI(p))
	if err != nil || got != p {
		t.Fatalf("round trip = %q (%v), want %q", got, err, p)
	}
	if _, err := pathFromURI("https://example.com/x.go"); err == nil {
		t.Fatal("non-file URI accepted")
	}
}

func TestReadFrameRejectsMalformedHeaders(t *testing.T) {
	cases := []string{
		"X-Other: 1\r\n\r\n{}",                   // no Content-Length
		"Content-Length: -4\r\n\r\n{}",           // negative length
		"Content-Length: 99999999999999\r\n\r\n", // beyond the frame limit
	}
	for _, in := range cases {
		if _, err := readFrame(bufio.NewReader(strings.NewReader(in))); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	ok := "content-length: 2\r\n\r\n{}"
	if body, err := readFrame(bufio.NewReader(strings.NewReader(ok))); err != nil || string(body) != "{}" {
		t.Fatalf("valid frame: %q %v", body, err)
	}
}

// TestGoplsEndToEnd runs the client against the real gopls on a throwaway module. It is
// skipped when gopls is not installed, so the regular test run stays hermetic.
func TestGoplsEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("E2E was only verified on Linux")
	}
	gopls, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not installed")
	}
	// PATH is left alone: gopls needs the go toolchain on it. Only the user config is isolated.
	t.Setenv("SIDECAR_CONFIG_HOME", t.TempDir())
	t.Setenv("SIDECAR_LSP", "1")
	root := t.TempDir()
	writeFiles(t, root, "go.mod")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/e2e\n\ngo 1.24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nfunc Helper() int { return 1 }\n\nfunc main() {\n\t_ = Helper()\n}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Trust(root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	c, err := Start(ctx, root, root, "go", gopls, Options{RequestTimeout: 60 * time.Second, StartTimeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	syms, err := c.DocumentSymbols(ctx, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, s := range syms {
		names[s.Name] = s.Line
	}
	if names["Helper"] != 3 || names["main"] != 5 {
		t.Fatalf("gopls symbols = %+v", syms)
	}
	defs, err := c.Definition(ctx, "main.go", 5, 5) // the Helper call on line 6
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || defs[0].Path != "main.go" || defs[0].Line != 3 {
		t.Fatalf("gopls definition = %+v", defs)
	}
	refs, err := c.References(ctx, "main.go", 2, 5) // the Helper declaration
	if err != nil {
		t.Fatal(err)
	}
	lines := map[int]bool{}
	for _, r := range refs {
		lines[r.Line] = true
	}
	if !lines[3] || !lines[6] {
		t.Fatalf("gopls references = %+v", refs)
	}
}
