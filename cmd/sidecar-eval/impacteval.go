package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// historyOutcome is one held-out file of one real commit.
type historyOutcome struct {
	Commit      string  `json:"commit"`
	Held        string  `json:"held_out"`
	Changed     int     `json:"changed_files"`
	Hit         bool    `json:"flagged"`       // the held-out file is named by a consumer/importer finding
	CoHit       bool    `json:"cochange_hit"`  // named by a co-change finding (history before the commit)
	BothHit     bool    `json:"both_hit"`      // named by either signal
	FlaggedBoth int     `json:"flagged_both"`  // code files flagged by either signal
	HitAny      bool    `json:"flagged_any"`   // ... or by any finding (tests, docs)
	FlaggedCode int     `json:"flagged_code"`  // code files flagged (excluding the changed ones)
	SameDirHit  bool    `json:"same_dir_hit"`  // baseline: the held-out file shares a directory with a changed file
	SameDirSize int     `json:"same_dir_size"` // baseline: code files in those directories
	MS          float64 `json:"analyze_ms"`
}

// runImpactHistory evaluates the change-impact analysis on real history. For a commit that edited several code
// files, all but one file play "the edits so far"; the question is whether the analysis names the file that the
// real author edited next. No labels are written by us: the ground truth is what the maintainers changed together.
// The baseline is the directory heuristic. Merge commits, renames, deletions, test-only and 1-file commits are skipped.
func runImpactHistory(repoDir string, maxCommits int, outPath string) error {
	out, err := exec.Command("git", "-C", repoDir, "rev-list", "--no-merges", "-n", "600", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("git rev-list: %w", err)
	}
	commits := strings.Fields(string(out))
	var outcomes []historyOutcome
	used := 0
	for _, c := range commits {
		if used >= maxCommits {
			break
		}
		files, ok := codeEdits(repoDir, c)
		if !ok {
			continue
		}
		o, err := historyCase(repoDir, c, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", c[:8], err)
			continue
		}
		outcomes = append(outcomes, o...)
		used++
	}
	var hit, coHit, bothHit, hitAny, base, n int
	var flagged, flaggedBoth, baseSize []float64
	for _, o := range outcomes {
		n++
		if o.Hit {
			hit++
		}
		if o.HitAny {
			hitAny++
		}
		if o.CoHit {
			coHit++
		}
		if o.BothHit {
			bothHit++
		}
		flaggedBoth = append(flaggedBoth, float64(o.FlaggedBoth))
		if o.SameDirHit {
			base++
		}
		flagged = append(flagged, float64(o.FlaggedCode))
		baseSize = append(baseSize, float64(o.SameDirSize))
	}
	fmt.Printf("impact history (%s): %d commits, %d held-out files\n", filepath.Base(repoDir), used, n)
	fmt.Printf("  structural (callers/importers): %d/%d (%.0f%%), code files flagged median %.0f\n", hit, n, pct(hit, n), percentile(flagged, 50))
	fmt.Printf("  co-change history only:        %d/%d (%.0f%%)\n", coHit, n, pct(coHit, n))
	fmt.Printf("  structural + co-change:        %d/%d (%.0f%%), code files flagged median %.0f\n", bothHit, n, pct(bothHit, n), percentile(flaggedBoth, 50))
	fmt.Printf("  baseline same-directory: %d/%d (%.0f%%), code files in those directories median %.0f\n", base, n, pct(base, n), percentile(baseSize, 50))
	if outPath != "" {
		data, _ := json.MarshalIndent(map[string]any{"schema": "impact-history/1", "repo": filepath.Base(repoDir), "commits": used,
			"held_out": n, "structural": hit, "cochange": coHit, "both": bothHit, "flagged_any": hitAny, "baseline_same_dir": base, "outcomes": outcomes}, "", "  ")
		return os.WriteFile(outPath, data, 0o644)
	}
	return nil
}

// codeEdits returns the modified, non-test, non-generated code files of a commit, or ok=false when the commit is
// not a clean multi-file code edit (2 to 6 files, nothing added, deleted or renamed among the code files).
func codeEdits(repoDir, commit string) ([]string, bool) {
	out, err := exec.Command("git", "-C", repoDir, "diff-tree", "--no-commit-id", "--name-status", "-r", "-M", commit).Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		p := f[len(f)-1]
		if _, kind := repo.Classify(p); kind != "code" || repo.IsGenerated(p, nil) {
			continue
		}
		if f[0] != "M" {
			return nil, false // an added, deleted or renamed code file changes what "next edit" means
		}
		files = append(files, p)
	}
	return files, len(files) >= 2 && len(files) <= 6
}

func historyCase(repoDir, commit string, files []string) ([]historyOutcome, error) {
	tmp, err := os.MkdirTemp("", "impact-hist")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	tree := filepath.Join(tmp, "tree")
	// A worktree at the parent keeps the real history up to that point, so the co-change signal sees only
	// what was known before the commit (never the commit being predicted).
	if out, err := exec.Command("git", "-C", repoDir, "worktree", "add", "--detach", "-f", tree, commit+"^").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worktree: %v: %s", err, out)
	}
	defer func() {
		_ = exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", tree).Run()
	}()
	ix, _, err := index.Open(tree, filepath.Join(tmp, "index.db"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = ix.Close() }()
	if _, err := ix.Sync(context.Background(), index.Options{WithHistory: true}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	var outs []historyOutcome
	for i, held := range files {
		if i >= 3 {
			break
		}
		var changed []string
		for _, f := range files {
			if f != held {
				changed = append(changed, f)
			}
		}
		t := timeNow()
		rep, err := impact.Analyze(ix.Store(), tree, changed)
		if err != nil {
			return nil, err
		}
		o := historyOutcome{Commit: commit[:10], Held: held, Changed: len(changed), MS: sinceMS(t)}
		changedSet := map[string]bool{}
		for _, c := range changed {
			changedSet[c] = true
		}
		structural, cochange := map[string]bool{}, map[string]bool{}
		for _, f := range rep.Findings {
			if f.Path == held {
				o.HitAny = true
			}
			switch {
			case (f.Kind == "consumer" || f.Kind == "importer") && !changedSet[f.Path]:
				structural[f.Path] = true
			case f.Kind == "cochange":
				cochange[f.Path] = true
			}
		}
		o.Hit = structural[held]
		o.CoHit = cochange[held]
		o.FlaggedCode = len(structural)
		both := map[string]bool{}
		for p := range structural {
			both[p] = true
		}
		for p := range cochange {
			both[p] = true
		}
		o.FlaggedBoth = len(both)
		o.BothHit = both[held]
		dirs := map[string]bool{}
		for _, c := range changed {
			dirs[path.Dir(c)] = true
		}
		o.SameDirHit = dirs[path.Dir(held)]
		all, _ := ix.Store().AllSymbols()
		seen := map[string]bool{}
		for _, h := range all {
			if dirs[path.Dir(h.Path)] && !seen[h.Path] && !changedSet[h.Path] {
				if _, kind := repo.Classify(h.Path); kind == "code" {
					seen[h.Path] = true
				}
			}
		}
		o.SameDirSize = len(seen)
		outs = append(outs, o)
	}
	return outs, nil
}

func timeNow() time.Time { return time.Now() }

func sinceMS(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }
