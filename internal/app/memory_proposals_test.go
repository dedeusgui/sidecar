package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestProposeMemoryRevalidatesHashesAndScopesSessionDecisions(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	now := time.Now()
	if _, err := MemoryAdd(root, "MaxActiveTickets", "limite escolhido explicitamente", DecisionTTL, now); err != nil {
		t.Fatal(err)
	}
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		t.Fatalf("open index store = %v, %v", ok, err)
	}
	defer func() { _ = st.Close() }()
	hash, exists := diskHash(root, "release/handler.go")
	if !exists || hash == "" {
		t.Fatal("fixture file hash is unavailable")
	}
	if _, err := st.InsertDecision(store.DecisionRecord{
		RepoRoot: root, Worktree: root, SymbolOrPath: "ManualReleaseHandler", FilePath: "release/handler.go",
		Symbol: "ManualReleaseHandler", FileHash: hash, Summary: "answer scoped to session s1", Origin: "user_answer",
		SessionID: "s1", CreatedAt: now.Unix(), TTLSeconds: int64(time.Hour / time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	ref := contract.EvidenceRef{Path: "release/handler.go", Key: "task:release"}
	scope := contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	proposals, err := ProposeMemory(st, scope, []contract.EvidenceRef{ref}, nil, now, registry)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("session s1 proposals = %+v, %v; want only its explicit answer", proposals, err)
	}
	if proposals[0].Decision.Origin != "user_answer" || proposals[0].Proposal.Kind != "MEMORY_DECISION" || proposals[0].Decision.SessionID != "s1" {
		t.Fatalf("proposal lost explicit-decision provenance: %+v", proposals[0])
	}
	if proposals[0].Evidence[0].Confidence != contract.ConfidenceCandidate || proposals[0].Decision.FileHash != hash || proposals[0].Evidence[0].Path != "release/handler.go" {
		t.Fatalf("user decision was promoted to a repository fact or lost its hash: evidence=%+v decision=%+v", proposals[0].Evidence[0], proposals[0].Decision)
	}

	otherScope := scope
	otherScope.SessionID = "s2"
	otherRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](otherScope)
	other, err := ProposeMemory(st, otherScope, []contract.EvidenceRef{ref}, nil, now, otherRegistry)
	if err != nil || len(other) != 0 {
		t.Fatalf("session s2 received s1's decision: %+v, %v", other, err)
	}

	limits := filepath.Join(root, "queue", "limits.go")
	if err := os.WriteFile(limits, []byte("package queue\n\nconst MaxActiveTickets = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	queueScope := scope
	queueScope.SessionID = "s3"
	queueRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](queueScope)
	queueEvidence := []contract.EvidenceRef{{Path: "queue/limits.go", Key: "task:limits"}}
	stale, err := ProposeMemory(st, queueScope, queueEvidence, []string{"MaxActiveTickets"}, now, queueRegistry)
	if err != nil || len(stale) != 0 {
		t.Fatalf("changed file retained old memory proposal: %+v, %v", stale, err)
	}
}
