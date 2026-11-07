// Package paths decides where Sidecar keeps its own files. Nothing is ever written
// inside a repository.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Home is the cache root: $SIDECAR_HOME, or the user cache directory's sidecar folder.
func Home() string {
	if v := os.Getenv("SIDECAR_HOME"); v != "" {
		return v
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "sidecar")
	}
	return filepath.Join(os.TempDir(), "sidecar")
}

// ConfigHome holds installation state: $SIDECAR_CONFIG_HOME, or the user config folder.
func ConfigHome() string {
	if v := os.Getenv("SIDECAR_CONFIG_HOME"); v != "" {
		return v
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "sidecar")
	}
	return filepath.Join(os.TempDir(), "sidecar-config")
}

// IndexDB is the index database for one worktree root. Each worktree gets its own
// database, so worktrees with different contents never share mutable state.
func IndexDB(worktreeRoot string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(worktreeRoot)))
	return filepath.Join(Home(), "repos", hex.EncodeToString(sum[:])[:24], "index.db")
}

// HookLog is the append-only diagnostic log written by the hook.
func HookLog() string { return filepath.Join(Home(), "logs", "hook.jsonl") }

// SessionState remembers which evidence was already injected in an agent session.
func SessionState(agent, sessionID string) string {
	sum := sha256.Sum256([]byte(agent + "\x00" + sessionID))
	return filepath.Join(Home(), "sessions", hex.EncodeToString(sum[:])[:32]+".json")
}

// ObserverState holds the session signals of one agent session in one worktree. The
// worktree is part of the key, so sessions in different worktrees never share state.
func ObserverState(worktreeRoot, agent, sessionID string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(worktreeRoot) + "\x00" + agent + "\x00" + sessionID))
	return filepath.Join(Home(), "sessions", hex.EncodeToString(sum[:])[:32]+".signals.json")
}

// Manifest records exactly which files and entries the installer changed.
func Manifest() string { return filepath.Join(ConfigHome(), "install.json") }

// LSPTrust lists the worktree roots the user explicitly allowed to start language servers.
func LSPTrust() string { return filepath.Join(ConfigHome(), "lsp-trusted-roots.json") }
