package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// maxDecisionsInContext bounds how many recorded decisions one prompt receives.
const maxDecisionsInContext = 3

// attachDecisions refreshes the decisions of the worktree at root and adds to res the valid ones
// that concern its evidence or are named in the prompt. The refresh compares each decision's file
// hash, symbol and TTL with the code now, so an edited, removed or expired decision is never attached.
func attachDecisions(st *store.Store, root, prompt string, res *retrieve.Result, now time.Time) (int, error) {
	list, err := st.RefreshDecisions(root, now, func(rel string) (string, bool) { return diskHash(root, rel) })
	if err != nil {
		return 0, err
	}
	files, names := map[string]bool{}, map[string]bool{}
	for _, ev := range res.Evidence {
		files[ev.Hit.Path] = true
		names[ev.Hit.Name] = true
	}
	attached := 0
	// Newest first: the latest decision is the one most likely to still be wanted.
	for i := len(list) - 1; i >= 0 && attached < maxDecisionsInContext; i-- {
		d := list[i]
		if d.Status != store.DecisionValid {
			continue
		}
		if !files[d.FilePath] && (d.Symbol == "" || !names[d.Symbol]) && !strings.Contains(prompt, d.SymbolOrPath) {
			continue
		}
		hash := d.FileHash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		note := fmt.Sprintf("[DECISION] %s (hash=%s): %s", d.SymbolOrPath, hash, d.Summary)
		res.Evidence = append(res.Evidence, retrieve.Evidence{
			Hit:     store.SymbolHit{Kind: "decision", Name: d.SymbolOrPath, Path: d.FilePath, FileHash: d.FileHash},
			Verdict: "VERIFIED", Section: ctxbuild.Contracts, Source: "decision_memory", Confidence: 1,
			ID: fmt.Sprintf("decision|%d", d.ID), Note: note, TokenEstimate: ctxbuild.EstimateTokens(note),
		})
		attached++
	}
	return attached, nil
}

// DecisionTTL is how long a recorded decision stays valid unless the user sets another.
const DecisionTTL = 30 * 24 * time.Hour

// MemoryAdd records an explicit decision about a file or symbol of the worktree at root. The
// index is synced first, so the target and its hash reflect the current code.
func MemoryAdd(root, target, summary string, ttl time.Duration, now time.Time) (store.DecisionRecord, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return store.DecisionRecord{}, errors.New("--summary is required: write the decision in one line")
	}
	if ttl < time.Second {
		return store.DecisionRecord{}, errors.New("--ttl must be at least one second")
	}
	// Indexing a home directory or a filesystem root would walk all of it; the hook refuses
	// these folders for the same reason.
	if unsafeRoot(root) {
		return store.DecisionRecord{}, fmt.Errorf("%s is not a project folder; run this inside a repository", root)
	}
	if err := RunIndexer(root, false); err != nil {
		return store.DecisionRecord{}, fmt.Errorf("indexing before recording: %w", err)
	}
	st, ok, err := openIndexStore(root)
	if err != nil {
		return store.DecisionRecord{}, err
	}
	if !ok {
		return store.DecisionRecord{}, errors.New("no index for this worktree; nothing to attach the decision to")
	}
	defer func() { _ = st.Close() }()

	filePath, symbol, err := resolveTarget(st, root, target)
	if err != nil {
		return store.DecisionRecord{}, err
	}
	if repo.IsSensitive(filePath) {
		return store.DecisionRecord{}, fmt.Errorf("%s looks sensitive; no decision is recorded for it", filePath)
	}
	hash, _ := diskHash(root, filePath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	commit, _ := gitx.Head(ctx, root)
	// The main checkout's root identifies the repository; every worktree of it shares that.
	repoRoot := root
	if common, err := gitx.CommonDir(ctx, root); err == nil {
		repoRoot = filepath.Dir(common)
	}
	symbolOrPath := filePath
	if symbol != "" {
		symbolOrPath = symbol
	}
	rec := store.DecisionRecord{
		RepoRoot: repoRoot, Worktree: root, SymbolOrPath: symbolOrPath, FilePath: filePath, Symbol: symbol,
		CommitHash: commit, FileHash: hash, Summary: summary, Origin: "manual",
		CreatedAt: now.Unix(), TTLSeconds: int64(ttl / time.Second), Status: store.DecisionValid,
	}
	id, err := st.InsertDecision(rec)
	if err != nil {
		return store.DecisionRecord{}, err
	}
	rec.ID = id
	return rec, nil
}

// MemoryList refreshes the status of every decision of the worktree at root and returns them.
// A worktree that was never indexed has no decisions; nothing is created for it.
func MemoryList(root string, now time.Time) ([]store.DecisionRecord, error) {
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		return []store.DecisionRecord{}, err
	}
	defer func() { _ = st.Close() }()
	return st.RefreshDecisions(root, now, func(rel string) (string, bool) { return diskHash(root, rel) })
}

// MemoryPurge deletes the invalidated decisions of the worktree at root, or all of them when
// all is set. Statuses are refreshed first, so decisions that became invalid since the last
// listing are included.
func MemoryPurge(root string, all bool, now time.Time) (int64, error) {
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		return 0, err
	}
	defer func() { _ = st.Close() }()
	if _, err := st.RefreshDecisions(root, now, func(rel string) (string, bool) { return diskHash(root, rel) }); err != nil {
		return 0, err
	}
	return st.PurgeDecisions(root, all)
}

// openIndexStore opens the worktree's index without syncing or recovering it. ok is false
// when no index exists yet.
func openIndexStore(root string) (*store.Store, bool, error) {
	path := paths.IndexDB(root)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	st, err := store.Open(path)
	if err != nil {
		return nil, false, err
	}
	return st, true, nil
}

// resolveTarget finds what a decision is about: an indexed file path, or a symbol name that
// is defined in exactly one file.
func resolveTarget(st *store.Store, root, target string) (filePath, symbol string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errors.New("--target is required: a file path or a symbol name")
	}
	rel := filepath.ToSlash(filepath.Clean(relPath(root, target)))
	_, known, err := st.FileIDByPath(rel)
	if err != nil {
		return "", "", err
	}
	if known {
		return rel, "", nil
	}
	hits, err := st.ExactSymbols([]string{store.KeyOf(target)}, 50)
	if err != nil {
		return "", "", err
	}
	var matches []store.SymbolHit
	for _, h := range hits {
		if h.Name == target {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("%q is neither an indexed file nor a symbol", target)
	case 1:
		return matches[0].Path, matches[0].Name, nil
	}
	return "", "", fmt.Errorf("%q is defined in %d files; pass the file path as --target", target, len(matches))
}

// diskHash hashes a file the way the indexer does (SHA-256 of its readable content), so a
// decision made on indexed code compares equal to the file on disk. exists reports whether
// the file is there at all.
func diskHash(root, rel string) (hash string, exists bool) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Lstat(abs); err != nil {
		return "", false
	}
	data, reason := repo.ReadSource(abs, repo.MaxSourceBytes)
	if reason != "" {
		return "", true
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true
}
