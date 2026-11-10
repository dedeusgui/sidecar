package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

func indexedProject(t *testing.T) (string, *index.Index) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"scale.go":      "package scale\n\nfunc Settle(kg int) int { return applyTare(kg) }\n\nfunc applyTare(kg int) int { return kg - 10 }\n",
		"scale_test.go": "package scale\n\nimport \"testing\"\n\nfunc TestSettle(t *testing.T) { _ = Settle(100) }\n",
	}
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	return root, ix
}

func verifiedRegistry(t *testing.T, root string, ix *index.Index) (*contract.EvidenceRegistry[retrieve.Evidence], contract.EvidenceRef) {
	t.Helper()
	result, err := retrieve.Search(context.Background(), ix.Store(), "Settle", retrieve.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, evidence := range result.Evidence {
		if evidence.Hit.Name != "Settle" {
			continue
		}
		registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
		ref, _, err := registry.Add(contract.EvidenceRef{
			Path: evidence.Hit.Path, LineStart: evidence.Hit.Line, LineEnd: evidence.Hit.EndLine,
			Fingerprint: evidence.Hit.FileHash, Origin: "retrieve:index",
			Confidence: contract.ConfidenceVerified, Freshness: contract.FreshnessCurrent,
			CostEstimate: max(1, evidence.TokenEstimate),
			Proof:        contract.VerificationProof{Kind: contract.VerificationIndex, Reference: "symbol:" + evidence.ID},
		}, evidence)
		if err != nil {
			t.Fatal(err)
		}
		return registry, ref
	}
	t.Fatalf("Settle evidence not retrieved: %+v", result.Evidence)
	return nil, contract.EvidenceRef{}
}

func TestQuickExpandRegistersVerifiedRelationsWithinLimits(t *testing.T) {
	root, ix := indexedProject(t)
	registry, anchor := verifiedRegistry(t, root, ix)
	result := QuickExpand(context.Background(), ix.Store(), QuickRequest{
		Enabled: true, Root: root, Query: "Settle", Anchors: []contract.EvidenceRef{anchor},
		Limits: Limits{Timeout: time.Second, MaxFiles: 10, MaxTokens: 512},
	}, registry)
	if len(result.Explore.Files) > QuickMaxFiles {
		t.Fatalf("quick expansion returned %d files, cap is %d", len(result.Explore.Files), QuickMaxFiles)
	}
	if len(result.Evidence) == 0 {
		t.Fatalf("expected an indexed relation around Settle: %+v", result.Gaps)
	}
	totalTokens := 0
	foundCallee := false
	for _, ref := range result.Evidence {
		totalTokens += ref.CostEstimate
		if ref.CostEstimate <= 0 || ref.Fingerprint == "" || ref.Path == "" || ref.Origin == "" {
			t.Fatalf("incomplete evidence reference: %+v", ref)
		}
		if ref.Confidence == contract.ConfidenceVerified && !ref.Proof.Valid() {
			t.Fatalf("verified evidence has no proof: %+v", ref)
		}
		entry, ok := registry.Lookup(ref.Key)
		if !ok {
			t.Fatalf("returned evidence was not registered: %+v", ref)
		}
		foundCallee = foundCallee || entry.Value.Hit.Name == "applyTare"
	}
	if !foundCallee {
		t.Fatalf("quick expansion did not add the indexed callee: %+v", result.Evidence)
	}
	if totalTokens > 512 {
		t.Fatalf("token budget exceeded: %d", totalTokens)
	}
}

func TestQuickExpandAbsentDisabledAndStaleAnchorsFailOpen(t *testing.T) {
	root, ix := indexedProject(t)
	registry, anchor := verifiedRegistry(t, root, ix)
	before := registry.Len()

	if got := QuickExpand(context.Background(), ix.Store(), QuickRequest{Enabled: true, Root: root}, registry); !hasGap(got.Gaps, "no_anchor") || registry.Len() != before {
		t.Fatalf("missing anchors should be an empty no-op: %+v len=%d", got.Gaps, registry.Len())
	}
	if got := QuickExpand(context.Background(), ix.Store(), QuickRequest{Root: root, Anchors: []contract.EvidenceRef{anchor}}, registry); !hasGap(got.Gaps, "disabled") || registry.Len() != before {
		t.Fatalf("disabled expansion should preserve base evidence: %+v len=%d", got.Gaps, registry.Len())
	}
	stale := anchor
	stale.Freshness = contract.FreshnessStale
	got := QuickExpand(context.Background(), ix.Store(), QuickRequest{Enabled: true, Root: root, Query: "Settle", Anchors: []contract.EvidenceRef{stale}}, registry)
	if !hasGap(got.Gaps, "stale_anchor") || len(got.Evidence) != 0 || registry.Len() != before {
		t.Fatalf("stale anchor should fail open: %+v len=%d", got.Gaps, registry.Len())
	}
	got = QuickExpand(context.Background(), ix.Store(), QuickRequest{Enabled: true, Root: root, Query: "Settle",
		Anchors: []contract.EvidenceRef{anchor}, Limits: Limits{Timeout: time.Second, MaxTokens: 1}}, registry)
	if hasGap(got.Gaps, "stale_anchor") || registry.Len() != before {
		t.Fatalf("tight budget should degrade without being mislabeled stale: %+v len=%d", got.Gaps, registry.Len())
	}
	if err := os.WriteFile(filepath.Join(root, "scale.go"), []byte("package scale\n\nfunc Settle(kg int) int { return kg }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = QuickExpand(context.Background(), ix.Store(), QuickRequest{Enabled: true, Root: root, Query: "Settle", Anchors: []contract.EvidenceRef{anchor}}, registry)
	if !hasGap(got.Gaps, "stale_anchor") || len(got.Evidence) != 0 || registry.Len() != before {
		t.Fatalf("anchor whose source changed on disk should fail open: %+v len=%d", got.Gaps, registry.Len())
	}
}

func TestExploreTimeoutAndEmptyQueryReturnGapsWithoutErrors(t *testing.T) {
	root, ix := indexedProject(t)
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := DeepExplore(ctx, ix.Store(), DeepRequest{Root: root, Query: "Settle"}, registry)
	if !hasGap(got.Gaps, "timeout") || registry.Len() != 0 {
		t.Fatalf("timeout should return an empty gap: %+v", got)
	}
	empty := DeepExplore(context.Background(), ix.Store(), DeepRequest{Root: root}, registry)
	if !hasGap(empty.Gaps, "empty_query") || registry.Len() != 0 {
		t.Fatalf("empty query should return an empty gap: %+v", empty)
	}
	noMatch := DeepExplore(context.Background(), ix.Store(), DeepRequest{Root: root, Query: "qzxvplmno_not_a_repository_symbol"}, registry)
	if !hasGap(noMatch.Gaps, "empty_result") || len(noMatch.Evidence) != 0 || registry.Len() != 0 {
		t.Fatalf("no-match query should return an empty gap: %+v", noMatch)
	}
}

func TestDeepExploreIsDeterministicAndClampsFileBudget(t *testing.T) {
	root, ix := indexedProject(t)
	firstRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	secondRegistry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	req := DeepRequest{Root: root, Query: "Settle", Limits: Limits{Timeout: time.Second, MaxFiles: 1, MaxTokens: 500}}
	first := DeepExplore(context.Background(), ix.Store(), req, firstRegistry)
	second := DeepExplore(context.Background(), ix.Store(), req, secondRegistry)
	if len(first.Explore.Files) > 1 || len(second.Explore.Files) > 1 {
		t.Fatalf("max files not respected: first=%d second=%d", len(first.Explore.Files), len(second.Explore.Files))
	}
	if !reflect.DeepEqual(first.Evidence, second.Evidence) {
		t.Fatalf("same index and query produced different evidence:\n%+v\n%+v", first.Evidence, second.Evidence)
	}
	for _, ref := range first.Evidence {
		if ref.Confidence == contract.ConfidenceVerified && !ref.Proof.Valid() {
			t.Fatalf("verified evidence lacks proof: %+v", ref)
		}
	}
}

func TestDeepExploreEnforcesEstimatedTokenBudget(t *testing.T) {
	root, ix := indexedProject(t)
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	got := DeepExplore(context.Background(), ix.Store(), DeepRequest{
		Root: root, Query: "Settle", Limits: Limits{Timeout: time.Second, MaxFiles: 1, MaxTokens: 1},
	}, registry)
	if got.Explore.Budget.UsedChars > 4 {
		t.Fatalf("source character cap exceeded for one-token request: %+v", got.Explore.Budget)
	}
	totalTokens := 0
	for _, ref := range got.Evidence {
		totalTokens += ref.CostEstimate
	}
	if totalTokens > 1 {
		t.Fatalf("estimated token budget exceeded: %d", totalTokens)
	}
}

func TestUnverifiedRelationNeverBecomesVerifiedWithoutProof(t *testing.T) {
	root, ix := indexedProject(t)
	search, err := retrieve.Search(context.Background(), ix.Store(), "Settle", retrieve.Options{Root: root})
	if err != nil || len(search.Evidence) == 0 {
		t.Fatalf("retrieve anchor: evidence=%d err=%v", len(search.Evidence), err)
	}
	ev := search.Evidence[0]
	ev.Verdict = "INFERRED"
	ev.Source = "structural"
	ev.Relations = []string{"calls:unique_name"}
	ref := evidenceRef(ix.Store(), ev, true)
	if ref.Confidence != contract.ConfidenceCandidate || ref.Proof.Valid() {
		t.Fatalf("heuristic relation was upgraded: %+v", ref)
	}
}

func TestLimitsHaveHardModeCapsAndConcurrencyBound(t *testing.T) {
	quick := normalizedLimits(Limits{Timeout: time.Hour, MaxFiles: 100, MaxTokens: 100000},
		Limits{Timeout: QuickMaxDuration, MaxFiles: QuickMaxFiles, MaxTokens: QuickMaxTokens},
		Limits{Timeout: QuickMaxDuration, MaxFiles: QuickMaxFiles, MaxTokens: QuickMaxTokens})
	if quick.Timeout > QuickMaxDuration || quick.MaxFiles > QuickMaxFiles || quick.MaxTokens > QuickMaxTokens {
		t.Fatalf("quick limits were not clamped: %+v", quick)
	}
	if MaxConcurrent != 1 || cap(traversalSlot) != MaxConcurrent {
		t.Fatalf("unexpected concurrency bound: const=%d capacity=%d", MaxConcurrent, cap(traversalSlot))
	}
}

func TestConcurrentTraversalLimitTimesOutWithoutBlockingCaller(t *testing.T) {
	root, ix := indexedProject(t)
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	traversalSlot <- struct{}{}
	defer func() { <-traversalSlot }()
	started := time.Now()
	got := DeepExplore(context.Background(), ix.Store(), DeepRequest{
		Root: root, Query: "Settle", Limits: Limits{Timeout: 10 * time.Millisecond},
	}, registry)
	if time.Since(started) > 250*time.Millisecond || !hasGap(got.Gaps, "timeout") || registry.Len() != 0 {
		t.Fatalf("queued traversal did not fail open at its timeout: elapsed=%s result=%+v", time.Since(started), got)
	}
}

func hasGap(gaps []contract.Gap, kind string) bool {
	for _, gap := range gaps {
		if gap.Kind == kind {
			return true
		}
	}
	return false
}
