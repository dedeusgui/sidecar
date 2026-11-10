package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func TestProposeImpactRequiresTheReportStillMatchUncommittedFiles(t *testing.T) {
	root := t.TempDir()
	for path, body := range map[string]string{
		"go.mod": "module example.com/impact\n\ngo 1.24\n",
		"a.go":   "package impact\n\nfunc Changed() {}\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, root, "init", "-q")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package impact\n\nfunc Changed() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scope := contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	report := impact.Report{Changed: []string{"a.go"}, Findings: []impact.Finding{{Kind: "consumer", Status: impact.Candidate, Path: "b.go", Text: "candidate consumer"}}}
	guidance, ok, err := ProposeImpact(context.Background(), root, report, scope, contract.PhaseVerify, true, registry)
	if err != nil || !ok || guidance.Proposal.Kind != "CHANGE_IMPACT" {
		t.Fatalf("uncommitted report proposal = %+v, ok=%v, err=%v", guidance, ok, err)
	}

	gitIn(t, root, "checkout", "--", "a.go")
	cleanRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	if _, ok, err := ProposeImpact(context.Background(), root, report, scope, contract.PhaseVerify, true, cleanRegistry); err != nil || ok {
		t.Fatalf("clean worktree retained an impact proposal: ok=%v err=%v", ok, err)
	}
}

func TestProposeImpactRejectsDifferentRootBeforeReadingDiff(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	scope := contract.Scope{RepoRoot: filepath.Join(root, "other"), WorktreeID: filepath.Join(root, "other"), SessionID: "s1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	_, ok, err := ProposeImpact(context.Background(), root, impact.Report{Changed: []string{"a.go"}}, scope, contract.PhaseVerify, true, registry)
	if err == nil || ok {
		t.Fatalf("cross-root report = ok %v err %v; want rejection before the diff read", ok, err)
	}
}
