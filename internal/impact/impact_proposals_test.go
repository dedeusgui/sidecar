package impact

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestProposeRequiresRelevantUncommittedChangeAndPreservesProvenance(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/work/repo", WorktreeID: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	report := Report{
		Changed: []string{"billing/charge.go"},
		Findings: []Finding{
			{Kind: "contract", Status: Conflict, Path: "docs/billing.md", Line: 7, Symbol: "Charge", Text: "documentation disagrees with code"},
			{Kind: "consumer", Status: Candidate, Path: "api/quote.go", Symbol: "Quote", Text: "possible consumer"},
			{Kind: "test", Status: Confirmed, Path: "billing/charge_test.go", Line: 22, Symbol: "Charge", Text: "test file references Charge and was not changed"},
		},
	}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	if _, ok := Propose(report, scope, contract.PhaseVerify, false, true, registry); ok {
		t.Fatal("committed/no-diff report produced impact guidance")
	}
	if _, ok := Propose(report, scope, contract.PhaseVerify, true, false, registry); ok {
		t.Fatal("unrelated report produced impact guidance")
	}

	guidance, ok := Propose(report, scope, contract.PhaseVerify, true, true, registry)
	if !ok {
		t.Fatal("eligible report did not produce a proposal")
	}
	if guidance.Proposal.Kind != "CHANGE_IMPACT" || guidance.Proposal.Phase != contract.PhaseVerify || guidance.Proposal.TokenEstimate <= 0 {
		t.Fatalf("proposal = %+v", guidance.Proposal)
	}
	if len(guidance.Findings) != 3 || len(guidance.Proposal.EvidenceKeys) != 3 {
		t.Fatalf("guidance lost findings or evidence links: %+v", guidance)
	}
	for _, ref := range guidance.Evidence {
		if _, ok := registry.Lookup(ref.Key); !ok {
			t.Fatalf("evidence reference %q was not added to the shared registry", ref.Key)
		}
		if ref.Confidence != contract.ConfidenceCandidate || ref.Freshness != contract.FreshnessUnknown || ref.Proof.Valid() {
			t.Fatalf("synthetic impact evidence was not downgraded without a current resolver proof: %+v", ref)
		}
	}
}

func TestProposeImpactUsesOnlyCurrentResolverProof(t *testing.T) {
	root := t.TempDir()
	body := []byte("package sample\n\nfunc Foo() {}\n")
	if err := os.WriteFile(filepath.Join(root, "foo.go"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	fileHash := hex.EncodeToString(sum[:])
	scope := contract.Scope{RepoRoot: root, WorktreeID: root, SessionID: "s1", WorktreeRevision: "head-1"}
	report := Report{Changed: []string{"foo.go"}, Findings: []Finding{{
		Kind: "consumer", Status: Confirmed, Path: "foo.go", Line: 3, Symbol: "Foo", Text: "confirmed consumer",
	}}}
	newRegistry := func(hash string) *contract.EvidenceRegistry[retrieve.Evidence] {
		registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
		evidence := retrieve.Evidence{ID: "actual-symbol", Hit: store.SymbolHit{
			ID: 7, Name: "Foo", Path: "foo.go", Line: 3, FileHash: hash,
		}, Verdict: "VERIFIED", Source: "exact"}
		_, _, err := registry.Add(contract.EvidenceRef{
			Path: "foo.go", Fingerprint: hash, LineStart: 3, LineEnd: 3,
			Confidence: contract.ConfidenceVerified, Freshness: contract.FreshnessCurrent,
			CostEstimate: 1, Proof: contract.VerificationProof{Kind: contract.VerificationIndex,
				Reference: "symbol:7:" + hash},
		}, evidence)
		if err != nil {
			t.Fatal(err)
		}
		return registry
	}
	stale, ok := Propose(report, scope, contract.PhaseVerify, true, true, newRegistry("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	if !ok || stale.Evidence[0].Confidence != contract.ConfidenceCandidate || stale.Evidence[0].Proof.Valid() {
		t.Fatalf("stale resolver hash was accepted as verified: %+v", stale.Evidence)
	}
	current, ok := Propose(report, scope, contract.PhaseVerify, true, true, newRegistry(fileHash))
	if !ok || current.Evidence[0].Confidence != contract.ConfidenceVerified || current.Evidence[0].Freshness != contract.FreshnessCurrent ||
		current.Evidence[0].Proof.Reference != "symbol:7:"+fileHash {
		t.Fatalf("current resolver proof was not preserved: %+v", current.Evidence)
	}
}

func TestProposeRequiresMatchingRegistryScope(t *testing.T) {
	scope := contract.Scope{RepoRoot: "/work/repo", SessionID: "s1", WorktreeRevision: "head-1"}
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](contract.Scope{RepoRoot: scope.RepoRoot, SessionID: "s2", WorktreeRevision: scope.WorktreeRevision})
	report := Report{Changed: []string{"a.go"}, Findings: []Finding{{Kind: "gap", Status: Unknown, Text: "no test found"}}}
	if _, ok := Propose(report, scope, contract.PhaseVerify, true, true, registry); ok {
		t.Fatal("proposal crossed into a different session registry")
	}
}
