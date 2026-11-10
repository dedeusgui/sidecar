package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// ProposeImpact turns an already-computed impact report into a planner
// proposal only when its changed files still match the worktree's current
// uncommitted diff. It performs no indexing or second impact traversal.
func ProposeImpact(ctx context.Context, root string, report impact.Report, scope contract.Scope, phase contract.TaskPhase, taskRelevant bool, registry *contract.EvidenceRegistry[retrieve.Evidence]) (impact.Guidance, bool, error) {
	scope = contract.NormalizeScope(scope)
	if root == "" || scope.RepoRoot == "" {
		return impact.Guidance{}, false, fmt.Errorf("impact root does not match proposal scope")
	}
	normalizedRoot, err := filepath.Abs(root)
	if err != nil {
		return impact.Guidance{}, false, err
	}
	normalizedScopeRoot, err := filepath.Abs(scope.RepoRoot)
	if err != nil {
		return impact.Guidance{}, false, err
	}
	if filepath.Clean(normalizedRoot) != filepath.Clean(normalizedScopeRoot) {
		return impact.Guidance{}, false, fmt.Errorf("impact root does not match proposal scope")
	}
	root = normalizedRoot
	files, err := uncommitted(ctx, root)
	if err != nil {
		return impact.Guidance{}, false, err
	}
	changed := make(map[string]bool, len(files))
	for _, file := range files {
		changed[file] = true
	}
	if len(report.Changed) == 0 {
		return impact.Guidance{}, false, nil
	}
	for _, file := range report.Changed {
		if !changed[file] {
			return impact.Guidance{}, false, nil
		}
	}
	guidance, ok := impact.Propose(report, scope, phase, true, taskRelevant, registry)
	return guidance, ok, nil
}
