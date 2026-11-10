package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/progress"
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
	item := memoryItem(proposals[0])
	rendered := ctxbuild.Memory + "\n- " + item.Text + " [" + item.Qualifier + "] (source: " + item.Provenance + ")\n"
	if proposals[0].Proposal.TokenEstimate < ctxbuild.EstimateTokens(rendered) {
		t.Fatalf("memory estimate %d omits rendered heading/bullet/metadata cost %d", proposals[0].Proposal.TokenEstimate, ctxbuild.EstimateTokens(rendered))
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

func TestMemoryImpactAndSessionProvidersReachPlannerPackOutput(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	now := time.Now()
	if _, err := MemoryAdd(root, "ManualReleaseHandler", "keep the queue order stable", DecisionTTL, now); err != nil {
		t.Fatal(err)
	}
	st, ok, err := openIndexStore(root)
	if err != nil || !ok {
		t.Fatalf("open index store = %v, %v", ok, err)
	}
	defer func() { _ = st.Close() }()
	scope := contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	baseRef, _, err := registry.Add(contract.EvidenceRef{
		Key: "task:release", Path: "release/handler.go", Fingerprint: "source-current",
		Freshness: contract.FreshnessCurrent, Confidence: contract.ConfidenceCandidate, CostEstimate: 1,
	}, retrieve.Evidence{ID: "task:release", Hit: store.SymbolHit{Path: "release/handler.go", Name: "ManualReleaseHandler"}})
	if err != nil {
		t.Fatal(err)
	}
	memory, err := ProposeMemory(st, scope, []contract.EvidenceRef{baseRef}, nil, now, registry)
	if err != nil || len(memory) != 1 {
		t.Fatalf("memory proposals = %+v, err %v", memory, err)
	}
	impactGuidance, ok := impact.Propose(impact.Report{
		Changed: []string{"release/handler.go"},
		Findings: []impact.Finding{{Kind: "consumer", Status: impact.Candidate, Path: "release/handler.go",
			Symbol: "ManualReleaseHandler", Text: "possible consumer of the changed handler"}},
	}, scope, contract.PhaseEdit, true, true, registry)
	if !ok {
		t.Fatal("impact provider returned no proposal")
	}
	signals := brief.NewSignals(scope.WorktreeRevision, now)
	signals.ScopeFingerprint = brief.ScopeFingerprint(scope)
	signals.Trace = []progress.Event{{Kind: "read", Key: "release/handler.go"}, {Kind: "read", Key: "release/handler.go"}}
	session := brief.ProposeSessionGuidance(scope, signals, contract.Observability{ToolEvents: contract.ObservationSeen},
		contract.PhaseEdit, now, registry)
	if len(session) != 1 {
		t.Fatalf("session proposals = %+v, want repeated-read guidance", session)
	}
	task := contract.TaskContext[engine.TaskSignals]{
		Scope: scope, Phase: contract.PhaseEdit,
		Observability: contract.Observability{ToolEvents: contract.ObservationSeen},
		Evidence:      registryRefs(registry),
		Guidance:      []contract.GuidanceProposal{memory[0].Proposal, impactGuidance.Proposal, session[0].Proposal},
	}
	decision := engine.Decision{Action: engine.ActionBrief, Budget: 300, MaxItems: 3}
	plan := (engine.Planner{}).Plan(engine.PlanInput{Task: task, Decision: decision, Evidence: registry, BudgetTokens: 300})
	packed := ctxbuild.Pack(plan, registry, nil, []ctxbuild.GuidanceProposal{
		{Proposal: memory[0].Proposal, Items: []ctxbuild.Item{memoryItem(memory[0])}},
		{Proposal: impactGuidance.Proposal, Items: impactItems(impactGuidance)},
		{Proposal: session[0].Proposal, Items: []ctxbuild.Item{sessionItem(session[0])}},
	}, nil)
	for _, want := range []string{ctxbuild.Memory, ctxbuild.ChangeImpact, ctxbuild.Session} {
		if !strings.Contains(packed.Text, want) {
			t.Errorf("final packed output missing %q (plan %+v):\n%s", want, plan, packed.Text)
		}
	}
	if !strings.Contains(packed.Text, "keep the queue order stable") || !strings.Contains(packed.Text, "possible consumer") || !strings.Contains(packed.Text, "Repeated navigation") {
		t.Fatalf("provider payload is absent from final packed block:\n%s", packed.Text)
	}
}
