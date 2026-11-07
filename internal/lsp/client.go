package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// ErrUnsupported means the server did not announce the capability the call needs.
var ErrUnsupported = errors.New("capacidade não anunciada pelo servidor")

const (
	maxFrame      = 16 << 20    // largest JSON-RPC body accepted from a server
	maxHeaderLine = 64 << 10    // largest header line accepted
	maxOpenBytes  = 512 << 10   // largest file sent with textDocument/didOpen
	shutdownGrace = time.Second // time a server gets to exit after `exit`
)

// Options bounds one server session. Zero values select the defaults.
type Options struct {
	RequestTimeout time.Duration // per request after initialize; default 5s
	StartTimeout   time.Duration // initialize handshake; default 15s
}

// Span is one location inside the worktree, normalised for retrieval. Path is relative to
// the worktree root and always slash-separated. Lines are 1-based.
type Span struct {
	Path    string
	Name    string
	Kind    string
	Line    int
	EndLine int
}

// Evidence turns a verified span into a retrieval item: Source "lsp", Verdict "VERIFIED".
func (s Span) Evidence() retrieve.Evidence {
	return retrieve.Evidence{
		Hit: store.SymbolHit{
			Path: s.Path, Name: s.Name, Kind: s.Kind, Line: s.Line, EndLine: s.EndLine,
		},
		Verdict: "VERIFIED",
		Section: ctxbuild.Code,
		Source:  "lsp",
	}
}

// Client is a JSON-RPC 2.0 session with one language server over stdio. It is safe for
// concurrent use by several goroutines.
type Client struct {
	root       string // canonical worktree root: every result must resolve inside it
	workspace  string // folder the server was initialised with (inside root)
	languageID string
	timeout    time.Duration

	cmd       *exec.Cmd
	stdin     io.WriteCloser
	writeMu   sync.Mutex
	nextID    atomic.Int64
	mu        sync.Mutex // guards pending and caps; never held across I/O
	pending   map[int64]chan rpcMessage
	openMu    sync.Mutex // serialises didOpen so each file is opened once, in order
	opened    map[string]bool
	caps      map[string]bool
	done      chan struct{} // closed when the reader stops
	exited    chan struct{} // closed when the process has been reaped
	doneOnce  sync.Once
	closeOnce sync.Once
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Start launches serverPath with the worktree root and workspace, then completes the
// initialize/initialized handshake. It refuses unless SIDECAR_LSP=1 and root is trusted.
func Start(ctx context.Context, root, workspace, languageID, serverPath string, opts Options) (*Client, error) {
	if !Enabled() {
		return nil, errors.New("cliente LSP desligado: defina SIDECAR_LSP=1")
	}
	canonRoot, err := canonical(root)
	if err != nil {
		return nil, err
	}
	if !Trusted(canonRoot) {
		return nil, errors.New("workspace não confiável: rode `sidecar lsp trust` antes")
	}
	canonWS, err := canonical(workspace)
	if err != nil {
		return nil, err
	}
	if rel, err := filepath.Rel(canonRoot, canonWS); err != nil || !filepath.IsLocal(rel) {
		return nil, errors.New("workspace fora da raiz confiável")
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 5 * time.Second
	}
	if opts.StartTimeout <= 0 {
		opts.StartTimeout = 15 * time.Second
	}

	cmd := exec.Command(serverPath, argsFor(filepath.Base(serverPath))...)
	cmd.Dir = canonWS
	setProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{
		root: canonRoot, workspace: canonWS, languageID: languageID, timeout: opts.RequestTimeout,
		cmd: cmd, stdin: stdin, pending: map[int64]chan rpcMessage{}, opened: map[string]bool{},
		caps: map[string]bool{}, done: make(chan struct{}), exited: make(chan struct{}),
	}
	go c.readLoop(bufio.NewReaderSize(stdout, maxHeaderLine))
	go func() {
		_ = cmd.Wait()
		close(c.exited)
	}()

	if err := c.handshake(ctx, opts.StartTimeout); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) handshake(ctx context.Context, timeout time.Duration) error {
	params := map[string]any{
		"processId":  os.Getpid(),
		"clientInfo": map[string]string{"name": "sidecar"},
		"rootUri":    fileURI(c.workspace),
		"workspaceFolders": []map[string]string{
			{"uri": fileURI(c.workspace), "name": filepath.Base(c.workspace)},
		},
		"capabilities": map[string]any{"textDocument": map[string]any{
			"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
			"definition":     map[string]any{},
			"references":     map[string]any{},
		}},
	}
	raw, err := c.request(ctx, timeout, "initialize", params)
	if err != nil {
		return err
	}
	var result struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("initialize: resposta inválida: %w", err)
	}
	c.mu.Lock()
	for _, name := range []string{"documentSymbolProvider", "definitionProvider", "referencesProvider"} {
		v := result.Capabilities[name]
		c.caps[name] = len(v) > 0 && string(v) != "null" && string(v) != "false"
	}
	c.mu.Unlock()
	return c.notify(ctx, "initialized", map[string]any{})
}

// Capabilities lists the providers the server announced, for example "definitionProvider".
func (c *Client) Capabilities() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for name, ok := range c.caps {
		if ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// DocumentSymbols lists the symbols declared in one file (rel is relative to the root).
func (c *Client) DocumentSymbols(ctx context.Context, rel string) ([]Span, error) {
	if err := c.requireCap("documentSymbolProvider"); err != nil {
		return nil, err
	}
	uri, err := c.open(ctx, rel)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, c.timeout, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	// Hierarchical DocumentSymbol items carry no URI: they belong to the requested file.
	return c.parseSymbols(raw, filepath.ToSlash(filepath.Clean(rel))), nil
}

// Definition returns the declarations of the symbol at a position. line is 0-based and
// character is in UTF-16 code units, as the LSP specification requires.
func (c *Client) Definition(ctx context.Context, rel string, line, character int) ([]Span, error) {
	if err := c.requireCap("definitionProvider"); err != nil {
		return nil, err
	}
	uri, err := c.open(ctx, rel)
	if err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, c.timeout, "textDocument/definition", position(uri, line, character))
	if err != nil {
		return nil, err
	}
	return c.parseLocations(raw, "definition"), nil
}

// References returns the uses of the symbol at a position, including its declaration.
func (c *Client) References(ctx context.Context, rel string, line, character int) ([]Span, error) {
	if err := c.requireCap("referencesProvider"); err != nil {
		return nil, err
	}
	uri, err := c.open(ctx, rel)
	if err != nil {
		return nil, err
	}
	params := position(uri, line, character)
	params["context"] = map[string]bool{"includeDeclaration": true}
	raw, err := c.request(ctx, c.timeout, "textDocument/references", params)
	if err != nil {
		return nil, err
	}
	return c.parseLocations(raw, "reference"), nil
}

// Close asks the server to shut down, then stops it and its children. It is idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		ctx := context.Background()
		_, _ = c.request(ctx, c.timeout, "shutdown", nil)
		_ = c.notify(ctx, "exit", nil)
		_ = c.stdin.Close()
		select {
		case <-c.exited:
		case <-time.After(shutdownGrace):
			killProcessTree(c.cmd)
			<-c.exited
		}
	})
	return nil
}

func (c *Client) requireCap(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.caps[name] {
		return ErrUnsupported
	}
	return nil
}

// open sends textDocument/didOpen once per file. The path must resolve inside the root,
// so a symlink or ".." cannot make the server read files outside the worktree.
func (c *Client) open(ctx context.Context, rel string) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("caminho fora da worktree: %q", rel)
	}
	abs, err := filepath.EvalSymlinks(filepath.Join(c.root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if _, ok := relInRoot(c.root, abs); !ok {
		return "", fmt.Errorf("caminho fora da worktree: %q", rel)
	}
	uri := fileURI(abs)
	c.openMu.Lock()
	defer c.openMu.Unlock()
	if c.opened[uri] {
		return uri, nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.Size() > maxOpenBytes {
		return "", fmt.Errorf("arquivo grande demais para o cliente LSP: %q", rel)
	}
	text, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	err = c.notify(ctx, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": c.languageID, "version": 1, "text": string(text),
	}})
	if err != nil {
		return "", err
	}
	c.opened[uri] = true
	return uri, nil
}

// parseSymbols accepts DocumentSymbol (hierarchical, belongs to rel) and SymbolInformation
// (flat, carries its own location). Each symbol is checked against the worktree like any span.
func (c *Client) parseSymbols(raw json.RawMessage, rel string) []Span {
	var items []symbolItem
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []Span
	var walk func(it symbolItem)
	walk = func(it symbolItem) {
		r, path, ok := it.Range, rel, true
		if it.Location != nil {
			r = &it.Location.Range
			path, ok = c.spanPath(it.Location.URI)
		}
		if ok && r != nil && it.Name != "" {
			out = append(out, Span{Path: path, Name: it.Name, Kind: symbolKind(it.Kind), Line: r.Start.Line + 1, EndLine: r.End.Line + 1})
		}
		var kids []symbolItem
		if json.Unmarshal(it.Children, &kids) == nil {
			for _, k := range kids {
				walk(k)
			}
		}
	}
	for _, it := range items {
		walk(it)
	}
	return out
}

type symbolItem struct {
	Name     string          `json:"name"`
	Kind     int             `json:"kind"`
	Range    *lspRange       `json:"range"`
	Location *lspLocation    `json:"location"`
	Children json.RawMessage `json:"children"`
}

// parseLocations accepts Location, Location[], LocationLink[] or null.
func (c *Client) parseLocations(raw json.RawMessage, kind string) []Span {
	var list []json.RawMessage
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &list)
	} else if len(raw) > 0 && raw[0] == '{' {
		list = []json.RawMessage{raw}
	}
	var out []Span
	for _, item := range list {
		var loc struct {
			URI       string    `json:"uri"`
			Range     lspRange  `json:"range"`
			TargetURI string    `json:"targetUri"`
			TargetRng *lspRange `json:"targetSelectionRange"`
		}
		if json.Unmarshal(item, &loc) != nil {
			continue
		}
		uri, r := loc.URI, loc.Range
		if loc.TargetURI != "" {
			uri = loc.TargetURI
			if loc.TargetRng != nil {
				r = *loc.TargetRng
			}
		}
		path, ok := c.spanPath(uri)
		if !ok {
			continue // outside the worktree, or not a file: dropped, never injected
		}
		out = append(out, Span{Path: path, Kind: kind, Line: r.Start.Line + 1, EndLine: r.End.Line + 1})
	}
	return out
}

// spanPath maps a server URI to a worktree-relative path, or rejects it. Symlinks are
// resolved first, so a link inside the worktree that points outside it is refused.
func (c *Client) spanPath(uri string) (string, bool) {
	abs, err := pathFromURI(uri)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	return relInRoot(c.root, real)
}

// relInRoot returns the slash-separated path of target inside root, or false when target is
// outside root (including through "..") or is root itself.
func relInRoot(root, target string) (string, bool) {
	rel, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

type lspRange struct {
	Start lspPos `json:"start"`
	End   lspPos `json:"end"`
}

type lspPos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

func position(uri string, line, character int) map[string]any {
	return map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": character},
	}
}

// fileURI builds a file:// URI for an absolute path, percent-encoding it.
func fileURI(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive paths: C:/x → /C:/x
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// pathFromURI decodes a file:// URI into an OS path. Other schemes are refused.
func pathFromURI(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return "", errors.New("uri não é file local")
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p), nil
}

// symbolKind maps LSP SymbolKind numbers to names; unknown numbers become "symbol".
func symbolKind(k int) string {
	names := []string{"", "file", "module", "namespace", "package", "class", "method", "property", "field",
		"constructor", "enum", "interface", "function", "variable", "constant", "string", "number", "boolean",
		"array", "object", "key", "null", "enumMember", "struct", "event", "operator", "typeParameter"}
	if k > 0 && k < len(names) {
		return names[k]
	}
	return "symbol"
}

// argsFor returns the flags that make a server speak LSP on stdio. Servers not listed here
// use stdio by default.
func argsFor(bin string) []string {
	switch strings.TrimSuffix(bin, ".exe") {
	case "typescript-language-server", "pyright-langserver", "intelephense":
		return []string{"--stdio"}
	}
	return nil
}

// request sends a call and waits for its response, bounded by timeout and ctx.
func (c *Client) request(ctx context.Context, timeout time.Duration, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	msg := rpcMessage{JSONRPC: "2.0", ID: json.RawMessage(strconv.FormatInt(id, 10)), Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		msg.Params = raw
	}
	if err := c.send(ctx, msg, timeout); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-timer.C:
		return nil, fmt.Errorf("%s: sem resposta em %s", method, timeout)
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("%s: servidor encerrou", method)
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	msg := rpcMessage{JSONRPC: "2.0", Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		msg.Params = raw
	}
	return c.send(ctx, msg, c.timeout)
}

// send writes one frame. The write runs in its own goroutine so a server that stops reading
// cannot block the caller past timeout; Close kills the process and unblocks the write.
func (c *Client) send(ctx context.Context, msg rpcMessage, timeout time.Duration) error {
	errc := make(chan error, 1)
	go func() { errc <- c.writeFrame(msg) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-errc:
		return err
	case <-timer.C:
		return fmt.Errorf("envio sem resposta do servidor em %s", timeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) writeFrame(msg rpcMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

// readLoop routes responses to their waiting callers and answers the server's own requests,
// so a server that waits for the client cannot stall.
func (c *Client) readLoop(r *bufio.Reader) {
	defer c.doneOnce.Do(func() { close(c.done) })
	for {
		body, err := readFrame(r)
		if err != nil {
			return
		}
		var msg rpcMessage
		if json.Unmarshal(body, &msg) != nil {
			continue
		}
		switch {
		case msg.Method != "" && len(msg.ID) > 0:
			_ = c.writeFrame(rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: serverRequestResult(msg)})
		case msg.Method == "" && len(msg.ID) > 0:
			var id int64
			if json.Unmarshal(msg.ID, &id) != nil {
				continue
			}
			c.mu.Lock()
			ch, ok := c.pending[id]
			c.mu.Unlock()
			if ok {
				select {
				case ch <- msg: // buffered: the waiting caller reads exactly one response
				default: // duplicate response for the same id: drop it, never block the reader
				}
			}
		}
	}
}

// serverRequestResult answers the requests servers send during a session. Configuration
// asks get one null per item, meaning "use defaults".
func serverRequestResult(msg rpcMessage) json.RawMessage {
	if msg.Method == "workspace/configuration" {
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			out := make([]any, len(p.Items))
			raw, _ := json.Marshal(out)
			return raw
		}
	}
	return json.RawMessage("null")
}

// readFrame reads one Content-Length framed JSON-RPC body. Header lines and bodies are bounded.
func readFrame(r *bufio.Reader) ([]byte, error) {
	n := -1
	for headers := 0; ; headers++ {
		if headers > 32 {
			return nil, errors.New("cabeçalhos LSP demais")
		}
		line, err := r.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		text := strings.TrimRight(string(line), "\r\n")
		if text == "" {
			break
		}
		name, value, ok := strings.Cut(text, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			v, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || v < 0 || v > maxFrame {
				return nil, errors.New("Content-Length inválido")
			}
			n = v
		}
	}
	if n < 0 {
		return nil, errors.New("mensagem sem Content-Length")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
