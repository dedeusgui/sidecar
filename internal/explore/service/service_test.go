package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/core"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
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

func TestRelationEvidenceNeedsBothCurrentEndpointsForVerifiedStatus(t *testing.T) {
	_, ix := indexedProject(t)
	edge := core.Edge{
		From: core.Ref{Name: "Settle", Path: "scale.go", Line: 3},
		To:   core.Ref{Name: "TestSettle", Path: "scale_test.go", Line: 5},
		Kind: "calls", Status: "VERIFIED", Basis: "same_file",
	}
	get := func(current map[string]bool) []retrieve.Evidence {
		return relationEvidence(context.Background(), ix.Store(), edge, map[string][]store.SymbolHit{}, map[string]bool{}, current)
	}
	partial := get(map[string]bool{"scale.go": true})
	if len(partial) != 2 {
		t.Fatalf("partial relation endpoints = %d, want two indexed endpoints", len(partial))
	}
	for _, evidence := range partial {
		if evidence.Verdict == "VERIFIED" {
			t.Fatalf("one-current-endpoint relation retained VERIFIED: %+v", evidence)
		}
	}
	complete := get(map[string]bool{"scale.go": true, "scale_test.go": true})
	if len(complete) != 2 || complete[0].Verdict != "VERIFIED" || complete[1].Verdict != "VERIFIED" {
		t.Fatalf("current endpoints did not retain verified relation: %+v", complete)
	}
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
			Proof: contract.VerificationProof{Kind: contract.VerificationIndex,
				Reference: fmt.Sprintf("symbol:%d:%s", evidence.Hit.ID, evidence.Hit.FileHash)},
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

func TestQuickExpandAllowCandidateAnchorAndCallPaths(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"dispatch.rb": `class Router
  def dispatch!
    route!
  end

  def route!
    process_route
  end

  def process_route
    200
  end
end
`,
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o600); err != nil {
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
	st := ix.Store()
	syms, err := st.SymbolsInFile("dispatch.rb", 16)
	if err != nil {
		t.Fatal(err)
	}
	var dispatchSym store.SymbolHit
	for _, s := range syms {
		if s.Name == "dispatch!" {
			dispatchSym = s
		}
	}
	fileSym, ok, err := st.FileSymbol("dispatch.rb")
	if err != nil || !ok || dispatchSym.ID == 0 {
		t.Fatalf("missing symbols: ok=%v err=%v syms=%+v", ok, err, syms)
	}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: root})
	fileRef, _, err := registry.Add(contract.EvidenceRef{
		Path: fileSym.Path, LineStart: fileSym.Line, LineEnd: fileSym.EndLine,
		Fingerprint: fileSym.FileHash, Origin: "retrieve:lexical",
		Confidence: contract.ConfidenceCandidate, Freshness: contract.FreshnessCurrent, CostEstimate: 5,
	}, retrieve.Evidence{Hit: fileSym, Verdict: "INFERRED", Source: "lexical"})
	if err != nil {
		t.Fatal(err)
	}
	candRef, _, err := registry.Add(contract.EvidenceRef{
		Path: dispatchSym.Path, LineStart: dispatchSym.Line, LineEnd: dispatchSym.EndLine,
		Fingerprint: dispatchSym.FileHash, Origin: "retrieve:lexical",
		Confidence: contract.ConfidenceCandidate, Freshness: contract.FreshnessCurrent, CostEstimate: 10,
	}, retrieve.Evidence{Hit: dispatchSym, Verdict: "INFERRED", Source: "lexical", Matched: []string{"dispatch"}})
	if err != nil {
		t.Fatal(err)
	}

	// Default AllowCandidateAnchor=false preserves V3.5/V3.6 contract.
	disallowed := QuickExpand(context.Background(), st, QuickRequest{
		Enabled: true, Root: root, Query: "como funciona o roteamento?",
		Anchors: []contract.EvidenceRef{fileRef, candRef},
	}, registry)
	if !hasGap(disallowed.Gaps, "no_verified_anchor") || len(disallowed.Evidence) != 0 {
		t.Fatalf("AllowCandidateAnchor=false should reject candidate anchors: %+v", disallowed.Gaps)
	}

	// AllowCandidateAnchor=true skips file anchor and falls back to non-file INFERRED anchor.
	expanded := QuickExpand(context.Background(), st, QuickRequest{
		Enabled: true, Root: root, Query: "como funciona o roteamento?",
		Anchors:              []contract.EvidenceRef{fileRef, candRef},
		AllowCandidateAnchor: true,
		Limits:               Limits{Timeout: time.Second, MaxFiles: 2, MaxTokens: 512},
	}, registry)
	if len(expanded.Evidence) == 0 {
		t.Fatalf("AllowCandidateAnchor=true did not expand candidate anchor: gaps=%+v", expanded.Gaps)
	}
	var gotRoute, gotProcess bool
	for _, ref := range expanded.Evidence {
		entry, ok := registry.Lookup(ref.Key)
		if !ok {
			t.Fatalf("expanded ref missing from registry: %+v", ref)
		}
		if ref.Confidence == contract.ConfidenceVerified && !ref.Proof.Valid() {
			t.Fatalf("verified ref without proof: %+v", ref)
		}
		if entry.Value.Hit.Name == "route!" {
			gotRoute = true
		}
		if entry.Value.Hit.Name == "process_route" {
			gotProcess = true
		}
	}
	if !gotRoute || !gotProcess {
		t.Fatalf("call-chain neighbors missing from QuickExpand evidence: route!=%v process_route=%v ev=%+v", gotRoute, gotProcess, expanded.Evidence)
	}
	var hasCallPath bool
	for _, p := range expanded.CallPaths {
		if p == "Calls: dispatch! -> route! -> process_route" {
			hasCallPath = true
		}
	}
	if !hasCallPath {
		t.Fatalf("CallPaths missing expected chain: %v", expanded.CallPaths)
	}

	// Stale candidate anchor must still fail open as stale_anchor.
	staleCand := candRef
	staleCand.Freshness = contract.FreshnessStale
	staleRes := QuickExpand(context.Background(), st, QuickRequest{
		Enabled: true, Root: root, Query: "dispatch!",
		Anchors: []contract.EvidenceRef{staleCand}, AllowCandidateAnchor: true,
	}, registry)
	if !hasGap(staleRes.Gaps, "stale_anchor") {
		t.Fatalf("stale candidate anchor should fail open with stale_anchor: %+v", staleRes.Gaps)
	}
}
