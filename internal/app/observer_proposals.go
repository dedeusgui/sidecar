package app

import (
	"os"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// ProposeObserverGuidance reads the existing hashed, bounded observer state
// and turns only positively observed events into proposals. Unknown coverage
// is unsupported/unobserved and therefore produces no observation-dependent
// guidance. This API is read-only and never injects on tool events.
func ProposeObserverGuidance(scope contract.Scope, agent adapter.Agent, observability contract.Observability, phase contract.TaskPhase, now time.Time, registry *contract.EvidenceRegistry[retrieve.Evidence]) []brief.SessionGuidance {
	scope = contract.NormalizeScope(scope)
	if scope.RepoRoot == "" || scope.SessionID == "" || scope.WorktreeRevision == "" || registry == nil || !scope.Equal(registry.Scope()) {
		return nil
	}
	statePath := paths.ObserverState(scope.RepoRoot, string(agent), scope.SessionID)
	if _, err := os.Stat(statePath); err != nil {
		return nil
	}
	signals := brief.LoadSignals(statePath, scope.WorktreeRevision, now)
	storedScope := observerScope(scope.RepoRoot, scope.SessionID, scope.WorktreeRevision)
	if signals.ScopeFingerprint != brief.ScopeFingerprint(storedScope) {
		return nil
	}
	// The persisted observer path is bound to root/session/agent. Rebind its
	// validated signal snapshot to the richer engine scope for registry keys.
	signals.ScopeFingerprint = brief.ScopeFingerprint(scope)
	return brief.ProposeSessionGuidance(scope, signals, observability, phase, now, registry)
}
