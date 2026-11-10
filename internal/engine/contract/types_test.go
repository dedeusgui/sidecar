package contract

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func proofRef(id int64) string {
	return "symbol:" + strconv.FormatInt(id, 10) + ":" + strings.Repeat("a", 64)
}

func TestEvidenceRegistryRequiresProofForVerified(t *testing.T) {
	tests := []struct {
		name       string
		confidence EvidenceConfidence
		proof      VerificationProof
		want       EvidenceConfidence
	}{
		{name: "verified without proof demotes to candidate", confidence: ConfidenceVerified, want: ConfidenceCandidate},
		{name: "unsupported proof demotes to candidate", confidence: ConfidenceVerified, proof: VerificationProof{Kind: "guess", Reference: "symbol"}, want: ConfidenceCandidate},
		{name: "opaque index reference demotes to candidate", confidence: ConfidenceVerified, proof: VerificationProof{Kind: VerificationIndex, Reference: "opaque"}, want: ConfidenceCandidate},
		{name: "index resolution verifies", confidence: ConfidenceVerified, proof: VerificationProof{Kind: VerificationIndex, Reference: proofRef(1)}, want: ConfidenceVerified},
		{name: "candidate stays candidate", confidence: ConfidenceCandidate, proof: VerificationProof{Kind: VerificationAST, Reference: "ast:1:" + strings.Repeat("a", 64)}, want: ConfidenceCandidate},
		{name: "invalid label becomes unknown", confidence: "certain", want: ConfidenceUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewEvidenceRegistry[string](Scope{RepoRoot: "/repo", SessionID: "session-a"})
			ref, inserted, err := registry.Add(EvidenceRef{
				Path: "internal/x.go", Fingerprint: "sha256:abc", LineStart: 4, LineEnd: 8,
				Confidence: test.confidence, Proof: test.proof,
			}, "retrieved evidence")
			if err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			if !inserted {
				t.Fatal("Add() inserted = false, want true")
			}
			if ref.Confidence != test.want {
				t.Fatalf("confidence = %q, want %q", ref.Confidence, test.want)
			}
			if ref.Confidence == ConfidenceVerified && !ref.Proof.Valid() {
				t.Fatal("registry returned VERIFIED without valid proof")
			}
		})
	}
}

func TestEvidenceRegistryDeduplicatesCanonicalFacts(t *testing.T) {
	scope := Scope{RepoRoot: "/repo", WorktreeID: "worktree-a", SessionID: "session-a", WorktreeRevision: "rev-1"}
	registry := NewEvidenceRegistry[string](scope)
	first, inserted, err := registry.Add(EvidenceRef{
		Path: "internal/x.go", Fingerprint: "sha256:abc", LineStart: 4, LineEnd: 8,
		Origin: "ast", Confidence: ConfidenceCandidate, Freshness: FreshnessUnknown, CostEstimate: 20,
	}, "first")
	if err != nil || !inserted {
		t.Fatalf("first Add() = (%v, %v), want inserted without error", inserted, err)
	}
	second, inserted, err := registry.Add(EvidenceRef{
		Path: "internal/x.go", Fingerprint: "sha256:abc", LineStart: 4, LineEnd: 8,
		Origin: "index", Confidence: ConfidenceVerified,
		Proof:     VerificationProof{Kind: VerificationIndex, Reference: proofRef(1)},
		Freshness: FreshnessCurrent, CostEstimate: 12,
	}, "duplicate")
	if err != nil || inserted {
		t.Fatalf("duplicate Add() = (%v, %v), want existing identity without error", inserted, err)
	}
	if first.Key != second.Key || registry.Len() != 1 {
		t.Fatalf("dedup = keys %q/%q, Len()=%d; want one canonical key", first.Key, second.Key, registry.Len())
	}
	entry, ok := registry.Lookup(first.Key)
	if !ok {
		t.Fatal("Lookup() did not find canonical key")
	}
	if entry.Ref.Confidence != ConfidenceVerified || !entry.Ref.Proof.Valid() {
		t.Fatalf("merged confidence/proof = %q/%+v, want verified with proof", entry.Ref.Confidence, entry.Ref.Proof)
	}
	if entry.Ref.CostEstimate != 12 || entry.Ref.Freshness != FreshnessCurrent {
		t.Fatalf("merged metadata = cost %d freshness %q, want 12/current", entry.Ref.CostEstimate, entry.Ref.Freshness)
	}
	if entry.Value != "first" {
		t.Fatalf("duplicate replaced first value with %q", entry.Value)
	}
}

func TestScopeAndRegistryKeysAreIsolated(t *testing.T) {
	base := Scope{RepoRoot: "/repo", WorktreeID: "worktree-a", SessionID: "session-a", WorktreeRevision: "rev-1"}
	otherScopes := []Scope{
		{RepoRoot: "/repo", WorktreeID: "worktree-b", SessionID: "session-a", WorktreeRevision: "rev-1"},
		{RepoRoot: "/repo", WorktreeID: "worktree-a", SessionID: "session-b", WorktreeRevision: "rev-1"},
		{RepoRoot: "/repo", WorktreeID: "worktree-a", SessionID: "session-a", WorktreeRevision: "rev-2"},
	}
	for _, other := range otherScopes {
		if base.Equal(other) {
			t.Errorf("scope %+v unexpectedly equals %+v", base, other)
		}
		left, right := NewEvidenceRegistry[string](base), NewEvidenceRegistry[string](other)
		ref := EvidenceRef{Path: "x.go", Fingerprint: "same-content", LineStart: 1, LineEnd: 2}
		leftRef, _, err := left.Add(ref, "left")
		if err != nil {
			t.Fatal(err)
		}
		rightRef, _, err := right.Add(ref, "right")
		if err != nil {
			t.Fatal(err)
		}
		if leftRef.Key == rightRef.Key {
			t.Errorf("registry key %q leaked across scope %+v and %+v", leftRef.Key, base, other)
		}
	}
}

func TestRegistryRequiresStableIdentity(t *testing.T) {
	registry := NewEvidenceRegistry[string](Scope{})
	if _, _, err := registry.Add(EvidenceRef{Confidence: ConfidenceUnknown}, "value"); err == nil {
		t.Fatal("Add() without a key or path fingerprint succeeded")
	}
}

func TestTaskContextHasNoPromptField(t *testing.T) {
	typeOfContext := reflect.TypeOf(TaskContext[struct{}]{})
	if _, ok := typeOfContext.FieldByName("Prompt"); ok {
		t.Fatal("TaskContext must not store the raw prompt")
	}
}
