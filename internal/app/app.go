// Package app wires the pieces together for the hook, the background indexer and the
// diagnostic commands. It is the only package that knows the order of operations.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// SyncBudget bounds how long a prompt waits for the index. Work left over is resumed by
// a background indexer, so the first prompt in a large repository still gets an answer.
const SyncBudget = 1500 * time.Millisecond

// hookMaxFiles bounds the files one prompt may parse. Cold starts of large repositories
// therefore cost a prompt a fraction of a second, not minutes.
const hookMaxFiles = 300

// HookOptions configures one hook invocation. Spawn lets tests replace the background indexer.
type HookOptions struct {
	Agent      adapter.Agent
	Stdin      io.Reader
	Stdout     io.Writer
	Budget     time.Duration
	Spawn      func(root string, history bool) error
	LogPath    string
	StatePath  func(agent, session string) string
	ExecutedAt time.Time
}

// Hook answers one UserPromptSubmit. It never fails the agent: every error is logged,
// nothing is written to stdout, and the function returns 0.
func Hook(opts HookOptions) (exit int) {
	if opts.Budget <= 0 {
		opts.Budget = SyncBudget
	}
	if opts.Spawn == nil {
		opts.Spawn = SpawnIndexer
	}
	if opts.StatePath == nil {
		opts.StatePath = paths.SessionState
	}
	if opts.LogPath == "" {
		opts.LogPath = paths.HookLog()
	}
	start := time.Now()
	rec := logRecord{"at": start.UTC().Format(time.RFC3339), "agent": string(opts.Agent)}
	defer func() {
		if r := recover(); r != nil {
			rec["status"] = "panic"
			rec["error"] = fmt.Sprint(r)
			rec["stack"] = string(debug.Stack())
			exit = 0
		}
		rec["total_ms"] = ms(time.Since(start))
		writeLog(opts.LogPath, rec)
	}()

	in, err := adapter.Parse(opts.Agent, opts.Stdin)
	if err != nil {
		rec["status"] = "bad_input"
		rec["error"] = err.Error()
		return 0
	}
	rec["prompt_chars"] = utf8.RuneCountInString(in.Prompt)
	if skipPrompt(in.Prompt) {
		rec["status"] = "skipped"
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Budget+time.Second)
	defer cancel()
	block, err := answer(ctx, in, opts, rec)
	if err != nil {
		rec["status"] = "error"
		rec["error"] = err.Error()
		block = "" // fail open: the agent continues without context
	}
	if block == "" && rec["status"] == "" {
		rec["status"] = "no_evidence"
	}
	if _, err := opts.Stdout.Write(adapter.Format(opts.Agent, block)); err != nil {
		rec["status"] = "write_error"
		rec["error"] = err.Error()
	}
	return 0
}

func answer(ctx context.Context, in adapter.Input, opts HookOptions, rec logRecord) (string, error) {
	cwd := in.CWD
	if cwd == "" {
		cwd = "."
	}
	root, _ := repo.FindRoot(cwd)
	// Check both the requested folder and the discovered worktree root. A project
	// located at the user's home must not cause an entire home directory to be indexed.
	if unsafeRoot(cwd) || unsafeRoot(root) {
		rec["status"] = "skipped_unsafe_root"
		return "", nil
	}
	rec["repo"] = repoLabel(root)

	t := time.Now()
	ix, rep, err := index.Open(root, paths.IndexDB(root))
	if err != nil {
		return "", err
	}
	defer func() { _ = ix.Close() }()
	rec["open_ms"] = ms(time.Since(t))
	if rep.Recovered {
		rec["index_recovered"] = true
	}

	t = time.Now()
	syncCtx, cancelSync := context.WithTimeout(ctx, opts.Budget)
	// A prompt parses a bounded batch; the rest is left to the background indexer.
	sync, err := ix.Sync(syncCtx, index.Options{MaxFiles: hookMaxFiles})
	cancelSync()
	if err != nil {
		return "", err
	}
	rec["sync_ms"] = ms(time.Since(t))
	rec["sync_parsed"] = sync.Parsed
	rec["sync_complete"] = sync.Complete
	rec["files"] = sync.Files
	if !sync.Complete || sync.HistoryStale {
		if err := maybeSpawn(opts, root, sync.HistoryStale); err != nil {
			rec["spawn_error"] = err.Error()
		}
	}

	t = time.Now()
	res, err := retrieve.Search(ctx, ix.Store(), in.Prompt, retrieve.Options{})
	if err != nil {
		return "", err
	}
	rec["retrieve_ms"] = ms(time.Since(t))
	rec["evidence"] = len(res.Evidence)
	rec["lexical"] = res.Trace.Lexical
	rec["exact"] = res.Trace.Exact

	t = time.Now()
	statePath := ""
	if in.SessionID != "" {
		statePath = opts.StatePath(string(opts.Agent), in.SessionID)
	}
	block := brief.RenderForSession(res, statePath)
	rec["render_ms"] = ms(time.Since(t))
	rec["injected_chars"] = len(block)
	rec["status"] = "ok"
	if !sync.Complete {
		rec["status"] = "partial"
	}
	if block == "" && rec["status"] == "ok" {
		rec["status"] = "no_evidence"
	}
	return block, nil
}

// unsafeRoot refuses to index folders that are not projects: the home directory, the
// filesystem root and temporary directories shared with other users.
func unsafeRoot(cwd string) bool {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return true
	}
	// filepath.Dir(root) == root detects POSIX '/', Windows drive roots (C:\\)
	// and UNC share roots without relying on a platform-specific separator.
	if filepath.Dir(abs) == abs {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil && abs == filepath.Clean(home) {
		return true
	}
	return false
}

func skipPrompt(p string) bool {
	p = strings.TrimSpace(p)
	return p == "" || strings.HasPrefix(p, "/") || utf8.RuneCountInString(p) < 3
}

// spawnCooldown limits background starts per worktree, so a failing indexer cannot be
// relaunched on every prompt.
const spawnCooldown = 20 * time.Second

// maybeSpawn starts a background indexer unless one is running or one was started recently.
func maybeSpawn(opts HookOptions, root string, history bool) error {
	lock := paths.IndexDB(root) + ".lock"
	if info, err := os.Stat(lock); err == nil && time.Since(info.ModTime()) < 10*time.Minute {
		return nil // an indexer is already working on this worktree
	}
	stamp := paths.IndexDB(root) + ".spawned"
	if info, err := os.Stat(stamp); err == nil && time.Since(info.ModTime()) < spawnCooldown {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(stamp), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(stamp, []byte(time.Now().Format(time.RFC3339)), 0o600); err != nil {
		return err
	}
	return opts.Spawn(root, history)
}

type logRecord map[string]any

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func repoLabel(root string) string {
	if len(root) > 0 {
		return filepath.Base(root)
	}
	return ""
}

// writeLog appends one JSON line. Prompt text is never logged. The log is capped by rotation.
func writeLog(path string, rec logRecord) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Rename(path, path+".1")
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// RunIndexer is the background entry point: it indexes one worktree to completion.
func RunIndexer(root string, history bool) error {
	release, ok, err := acquireLock(paths.IndexDB(root) + ".lock")
	if err != nil {
		return err
	}
	if !ok {
		return nil // another indexer is already working on this worktree
	}
	defer release()
	ix, _, err := index.Open(root, paths.IndexDB(root))
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	_, err = ix.Sync(ctx, index.Options{WithHistory: history})
	return err
}

// acquireLock takes a lock file, replacing one older than 10 minutes (a crashed run).
func acquireLock(path string) (release func(), ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, err
	}
	if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > 10*time.Minute {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()
	return func() { _ = os.Remove(path) }, true, nil
}
