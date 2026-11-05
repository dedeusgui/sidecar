// Package gitx reads Git state without modifying the repository. Commands run with
// optional locks disabled and fsmonitor off, so a repo-defined hook program is never
// executed by a read.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNoHead means the repository has no commits yet.
var ErrNoHead = errors.New("repository has no commits")

// StatusEntry is one working-tree change reported by git status.
type StatusEntry struct {
	Path      string
	OldPath   string // set for renames
	XY        string // two-letter status code, e.g. " M", "A.", "??"
	Untracked bool
	Deleted   bool
}

// FileChange is one path touched by a commit.
type FileChange struct {
	Status  string // A, M, D, R, C, T
	Path    string
	OldPath string // set for R and C
}

// Commit is compact history metadata: no diff content.
type Commit struct {
	Hash    string
	Time    int64
	Subject string
	Files   []FileChange
}

const gitTimeout = 20 * time.Second

func gitRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	full := append([]string{"-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// TopLevel returns the root of the worktree containing dir.
func TopLevel(ctx context.Context, dir string) (string, error) {
	out, err := gitRun(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Head returns the commit hash of HEAD, or ErrNoHead.
func Head(ctx context.Context, root string) (string, error) {
	out, err := gitRun(ctx, root, "rev-parse", "--verify", "-q", "HEAD")
	if err != nil {
		return "", ErrNoHead
	}
	return strings.TrimSpace(string(out)), nil
}

// ListFiles returns tracked files plus untracked files that are not ignored, relative to root.
func ListFiles(ctx context.Context, root string) ([]string, error) {
	out, err := gitRun(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			files = append(files, string(f))
		}
	}
	return files, nil
}

// Status lists working-tree changes (modified, added, deleted, renamed, untracked).
func Status(ctx context.Context, root string) ([]StatusEntry, error) {
	out, err := gitRun(ctx, root, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return nil, err
	}
	records := bytes.Split(out, []byte{0})
	var entries []StatusEntry
	for i := 0; i < len(records); i++ {
		rec := string(records[i])
		if rec == "" {
			continue
		}
		switch {
		case strings.HasPrefix(rec, "? "):
			entries = append(entries, StatusEntry{Path: rec[2:], XY: "??", Untracked: true})
		case strings.HasPrefix(rec, "1 "):
			fields := strings.SplitN(rec, " ", 9)
			if len(fields) == 9 {
				entries = append(entries, StatusEntry{Path: fields[8], XY: fields[1], Deleted: strings.Contains(fields[1], "D")})
			}
		case strings.HasPrefix(rec, "2 "):
			fields := strings.SplitN(rec, " ", 10)
			entry := StatusEntry{XY: fields[1]}
			if len(fields) == 10 {
				entry.Path = fields[9]
			}
			if i+1 < len(records) {
				i++
				entry.OldPath = string(records[i])
			}
			entries = append(entries, entry)
		case strings.HasPrefix(rec, "u "):
			fields := strings.SplitN(rec, " ", 11)
			if len(fields) == 11 {
				entries = append(entries, StatusEntry{Path: fields[10], XY: fields[1]})
			}
		}
	}
	return entries, nil
}

// ChangedBetween lists paths that differ between two commits (rename-aware).
func ChangedBetween(ctx context.Context, root, from, to string) ([]FileChange, error) {
	out, err := gitRun(ctx, root, "diff", "--name-status", "-M", "--no-renames=false", from, to)
	if err != nil {
		return nil, err
	}
	var changes []FileChange
	for _, line := range strings.Split(string(out), "\n") {
		if fc, ok := parseNameStatus(line); ok {
			changes = append(changes, fc)
		}
	}
	return changes, nil
}

// ReadHistory returns up to max non-merge commits, newest first. When since is set, only
// commits after since (since..HEAD) are returned, so ingestion is incremental.
func ReadHistory(ctx context.Context, root, since string, max int) ([]Commit, error) {
	args := []string{"log", "--no-merges", "-M", "--name-status",
		"--format=%x1e%H%x1f%ct%x1f%s", fmt.Sprintf("--max-count=%d", max)}
	if since != "" {
		args = append(args, since+"..HEAD")
	}
	out, err := gitRun(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, chunk := range strings.Split(string(out), "\x1e") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		lines := strings.Split(chunk, "\n")
		header := strings.SplitN(lines[0], "\x1f", 3)
		if len(header) != 3 {
			continue
		}
		var ts int64
		_, _ = fmt.Sscanf(header[1], "%d", &ts)
		c := Commit{Hash: header[0], Time: ts, Subject: header[2]}
		for _, line := range lines[1:] {
			if fc, ok := parseNameStatus(line); ok {
				c.Files = append(c.Files, fc)
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

// parseNameStatus parses "M\tpath", "R100\told\tnew" and similar lines.
func parseNameStatus(line string) (FileChange, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) < 2 || parts[0] == "" {
		return FileChange{}, false
	}
	code := parts[0][:1]
	switch code {
	case "R", "C":
		if len(parts) < 3 {
			return FileChange{}, false
		}
		return FileChange{Status: code, OldPath: parts[1], Path: parts[2]}, true
	default:
		return FileChange{Status: code, Path: parts[1]}, true
	}
}
