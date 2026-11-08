package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// Impact analyses the change at root. With no files it uses the uncommitted changes of the worktree
// (deleted files are not analysed: there is no code left to relate). It syncs the index first, so the
// answer reflects the working tree. Nothing in the repository is executed.
func Impact(root string, files []string) (impact.Report, error) {
	ix, _, err := index.Open(root, paths.IndexDB(root))
	if err != nil {
		return impact.Report{}, err
	}
	defer func() { _ = ix.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := ix.Sync(ctx, index.Options{WithHistory: true}); err != nil {
		return impact.Report{}, err
	}
	if len(files) == 0 {
		files, err = uncommitted(ctx, root)
		if err != nil {
			return impact.Report{}, err
		}
	}
	return impact.Analyze(ix.Store(), root, files)
}

func uncommitted(ctx context.Context, root string) ([]string, error) {
	_, entries, err := gitx.Status(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("not a Git worktree or git unavailable (pass --files): %w", err)
	}
	var out []string
	for _, e := range entries {
		if !e.Deleted {
			out = append(out, e.Path)
		}
	}
	return out, nil
}

// changeCheckNote renders the report as one compact note for the prompt hook (feature "impact"), or ""
// when the change has nothing the agent is likely to miss. Only unchanged consumers, conflicts and gaps are
// worth a line; tests already changed and CONFIRMED docs add nothing new.
func changeCheckNote(rep impact.Report) (string, string) {
	var parts []string
	count := map[string]int{}
	for _, f := range rep.Findings {
		switch {
		case f.Kind == "contract" && f.Status == impact.Conflict:
			parts = append(parts, fmt.Sprintf("[CONFLICT] %s", f.Text))
		case (f.Kind == "consumer" || f.Kind == "importer") && count["consumer"] < 3:
			count["consumer"]++
			parts = append(parts, fmt.Sprintf("[%s] %s", f.Status, f.Text))
		case f.Kind == "test" && strings.Contains(f.Text, "was not changed") && count["test"] < 2:
			count["test"]++
			parts = append(parts, fmt.Sprintf("[%s] %s", f.Status, f.Text))
		case f.Kind == "gap" && count["gap"] < 1:
			count["gap"]++
			parts = append(parts, fmt.Sprintf("[%s] %s", f.Status, f.Text))
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	const lead = "Change check on uncommitted edits (index; verify): "
	// The block keeps an item to 300 runes: drop the least important lines rather than truncate mid-line.
	text := lead + strings.Join(parts, " | ")
	for utf8.RuneCountInString(text) > 295 && len(parts) > 1 {
		parts = parts[:len(parts)-1]
		text = lead + strings.Join(parts, " | ")
	}
	if r := []rune(text); len(r) > 295 {
		text = string(r[:294]) + "…"
	}
	return text, strings.Join(rep.Changed, ",")
}

// applyImpact adds the change check (feature "impact") for the uncommitted edits of the worktree. Every
// failure is returned for the log; the prompt keeps its evidence. The note's ID is a hash of its text, so
// the session memory sends an unchanged check once and a changed one again.
func applyImpact(ctx context.Context, st *store.Store, root string, res *retrieve.Result, rec map[string]any) error {
	files, err := uncommitted(ctx, root)
	if err != nil || len(files) == 0 {
		return nil // not a Git worktree, or nothing edited: nothing to check
	}
	rep, err := impact.Analyze(st, root, files)
	if err != nil {
		return err
	}
	text, _ := changeCheckNote(rep)
	rec["impact_files"] = len(rep.Changed)
	rec["impact_findings"] = len(rep.Findings)
	if text == "" {
		return nil
	}
	fp := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
	res.Evidence = append(res.Evidence, retrieve.Evidence{
		Hit:     store.SymbolHit{Kind: "change_check", Name: "impact", FileHash: fp},
		Verdict: "INFERRED", Section: ctxbuild.Concerns, Source: "impact", Confidence: 1,
		ID: "impact|" + fp, Note: text, TokenEstimate: ctxbuild.EstimateTokens(text),
	})
	res.Trace.Budget += ctxbuild.EstimateTokens(text) + 8
	return nil
}
