// Package contract contains leaf contracts shared by the V3.5 engine and its
// consumers. It deliberately does not import retrieval or rendering packages.
package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// TaskPhase is the best-supported current phase of a task.
type TaskPhase string

const (
	PhaseExplore TaskPhase = "explore"
	PhaseDecide  TaskPhase = "decide"
	PhaseEdit    TaskPhase = "edit"
	PhaseVerify  TaskPhase = "verify"
	PhaseResume  TaskPhase = "resume"
	PhaseUnknown TaskPhase = "unknown"
)

// ObservationState records whether a client event was seen, not seen, or is
// unsupported. Unknown strings are treated as unsupported by consumers.
type ObservationState string

const (
	ObservationSeen        ObservationState = "seen"
	ObservationNotSeen     ObservationState = "not_seen"
	ObservationUnsupported ObservationState = "unsupported"
)

// Observability describes only events the client can reliably expose.
type Observability struct {
	ToolEvents ObservationState
	PreCompact ObservationState
}

// ObservationEvent identifies an event a proposal needs as evidence.
type ObservationEvent string

const (
	ObservationToolEvents ObservationEvent = "tool_events"
	ObservationPreCompact ObservationEvent = "pre_compact"
)

// CapabilityStatus reports whether an optional client capability is known to
// be usable. Empty and unknown values must be treated as unverified.
type CapabilityStatus string

const (
	CapabilityAvailable   CapabilityStatus = "available"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnverified  CapabilityStatus = "unverified"
)

// Scope binds in-memory state to one discovered worktree, revision and
// optional agent session. WorktreeID may be omitted for non-Git folders; in
// that case RepoRoot is the isolation key. No prompt or repository contents
// are stored in a Scope.
type Scope struct {
	RepoRoot         string
	WorktreeID       string
	SessionID        string
	WorktreeRevision string
}

// NormalizeScope cleans the root path and uses it as the worktree key when a
// caller has no Git worktree identity.
func NormalizeScope(scope Scope) Scope {
	if scope.RepoRoot != "" {
		scope.RepoRoot = filepath.Clean(scope.RepoRoot)
	}
	if scope.WorktreeID == "" {
		scope.WorktreeID = scope.RepoRoot
	}
	return scope
}

// Equal reports whether two scopes refer to the same worktree, revision and
// session. An absent session is distinct from every named session.
func (scope Scope) Equal(other Scope) bool {
	scope, other = NormalizeScope(scope), NormalizeScope(other)
	return scope == other
}

// TaskContext is an in-memory snapshot for one hook/task invocation. Signals
// is generic so the engine can reuse engine.TaskSignals without making this
// leaf package depend on engine. Raw prompts are intentionally not a field.
type TaskContext[Signals any] struct {
	Scope             Scope
	QuerySignals      Signals
	Phase             TaskPhase
	Observability     Observability
	ExploreCapability CapabilityStatus
	Evidence          []EvidenceRef
	Guidance          []GuidanceProposal
	Uncertainties     []Gap
}

// EvidenceConfidence is a confidence label for a source-backed evidence
// reference. VERIFIED requires a valid verification proof in the registry.
type EvidenceConfidence string

const (
	ConfidenceVerified  EvidenceConfidence = "VERIFIED"
	ConfidenceCandidate EvidenceConfidence = "CANDIDATE"
	ConfidenceUnknown   EvidenceConfidence = "UNKNOWN"
)

// EvidenceFreshness indicates whether the reference matches the current task
// snapshot.
type EvidenceFreshness string

const (
	FreshnessCurrent EvidenceFreshness = "current"
	FreshnessStale   EvidenceFreshness = "stale"
	FreshnessUnknown EvidenceFreshness = "unknown"
)

// VerificationKind is a resolver that can substantiate VERIFIED status.
type VerificationKind string

const (
	VerificationIndex VerificationKind = "index"
	VerificationAST   VerificationKind = "ast"
	VerificationLSP   VerificationKind = "lsp"
)

// VerificationProof identifies a concrete resolver result by stable index ID
// and the content hash of the source that was resolved.
type VerificationProof struct {
	Kind      VerificationKind
	Reference string
}

// Valid reports whether the proof uses a resolver reference with a stable ID
// and a full source hash. Opaque labels cannot substantiate VERIFIED.
func (proof VerificationProof) Valid() bool {
	switch proof.Kind {
	case VerificationIndex:
		if strings.HasPrefix(proof.Reference, "symbol:") {
			parts := strings.Split(proof.Reference, ":")
			return len(parts) == 3 && positiveID(parts[1]) && validSourceHash(parts[2])
		}
		if strings.HasPrefix(proof.Reference, "relation:") {
			last := strings.LastIndexByte(proof.Reference, ':')
			if last <= len("relation:") || !validSourceHash(proof.Reference[last+1:]) {
				return false
			}
			previous := strings.LastIndexByte(proof.Reference[:last], ':')
			return previous > len("relation:") && positiveID(proof.Reference[previous+1:last])
		}
	case VerificationAST, VerificationLSP:
		prefix := string(proof.Kind) + ":"
		parts := strings.Split(proof.Reference, ":")
		return len(parts) == 3 && parts[0]+":" == prefix && positiveID(parts[1]) && validSourceHash(parts[2])
	default:
	}
	return false
}

func positiveID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0
}

func validSourceHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// EvidenceRef is safe metadata identifying a fact. Fingerprint should be a
// content hash when available; Key is the deterministic fallback identity.
// CostEstimate is required by the planner and is never inferred as zero.
type EvidenceRef struct {
	Key          string
	Fingerprint  string
	Path         string
	LineStart    int
	LineEnd      int
	Origin       string
	Confidence   EvidenceConfidence
	Freshness    EvidenceFreshness
	CostEstimate int
	Proof        VerificationProof
}

// Gap is a compact uncertainty code with links to supporting evidence. Kind
// should be a stable code rather than prompt or repository prose.
type Gap struct {
	Kind         string
	EvidenceKeys []string
}

// GuidanceProposal is an evidence-backed candidate for a single output
// section. DeliveryReason is a stable reason code, never prompt prose.
type GuidanceProposal struct {
	Kind                string
	EvidenceKeys        []string
	Priority            int
	TokenEstimate       int
	DeliveryReason      string
	Phase               TaskPhase
	RequiresObservation ObservationEvent
}

// SectionEvidence is the common allocation for selected repository evidence.
const SectionEvidence = "RELEVANT CODE"

// SectionAllocation is the portion of the global token/item budget assigned
// to one section. EvidenceKeys refer to registry entries; they contain no
// evidence content.
type SectionAllocation struct {
	Section      string
	BudgetTokens int
	MaxItems     int
	EvidenceKeys []string
}

// PlanReason explains a deterministic omission without carrying raw prompt or
// repository text.
type PlanReason struct {
	Code string
	Key  string
	Kind string
}

// OutputPlan is the dry-run allocation produced by the unified planner.
type OutputPlan struct {
	Scope             Scope
	Action            string
	Phase             TaskPhase
	BudgetTokens      int
	MaxItems          int
	Sections          []SectionAllocation
	NotDelivered      []PlanReason
	ExploreCapability CapabilityStatus
}

var errMissingEvidenceIdentity = errors.New("evidence reference needs a fingerprint/path or key")

// EvidenceEntry pairs the existing caller-owned evidence value with safe
// registry metadata. T is not copied into a second evidence model.
type EvidenceEntry[T any] struct {
	Ref   EvidenceRef
	Value T
}

// EvidenceRegistry deduplicates references within exactly one task scope. It
// stores no prompt, transcript, persistence handle or repository database.
type EvidenceRegistry[T any] struct {
	scope   Scope
	entries map[string]EvidenceEntry[T]
}

// NewEvidenceRegistry creates an empty registry bound to one worktree,
// revision and optional session.
func NewEvidenceRegistry[T any](scope Scope) *EvidenceRegistry[T] {
	return &EvidenceRegistry[T]{scope: NormalizeScope(scope), entries: make(map[string]EvidenceEntry[T])}
}

// Scope returns the normalized isolation scope of the registry.
func (registry *EvidenceRegistry[T]) Scope() Scope {
	if registry == nil {
		return Scope{}
	}
	return registry.scope
}

// Add registers a value and returns its canonical reference. Duplicate facts
// share one key. A VERIFIED label without a valid resolver proof is demoted.
// The bool is true only when a new canonical identity was inserted.
func (registry *EvidenceRegistry[T]) Add(ref EvidenceRef, value T) (EvidenceRef, bool, error) {
	if registry == nil {
		return EvidenceRef{}, false, errors.New("nil evidence registry")
	}
	key, err := canonicalEvidenceKey(registry.scope, ref)
	if err != nil {
		return EvidenceRef{}, false, err
	}
	ref.Key = key
	ref.Confidence = normalizeConfidence(ref.Confidence, ref.Proof)
	if ref.Freshness == "" {
		ref.Freshness = FreshnessUnknown
	}
	if old, ok := registry.entries[key]; ok {
		if confidenceRank(ref.Confidence) > confidenceRank(old.Ref.Confidence) {
			old.Ref.Confidence = ref.Confidence
			old.Ref.Proof = ref.Proof
			if ref.Origin != "" {
				old.Ref.Origin = ref.Origin
			}
		}
		if ref.CostEstimate > 0 && (old.Ref.CostEstimate <= 0 || ref.CostEstimate < old.Ref.CostEstimate) {
			old.Ref.CostEstimate = ref.CostEstimate
		}
		if old.Ref.Freshness != FreshnessCurrent && ref.Freshness == FreshnessCurrent {
			old.Ref.Freshness = FreshnessCurrent
		}
		registry.entries[key] = old
		return old.Ref, false, nil
	}
	registry.entries[key] = EvidenceEntry[T]{Ref: ref, Value: value}
	return ref, true, nil
}

// Lookup returns a registry entry by canonical key.
func (registry *EvidenceRegistry[T]) Lookup(key string) (EvidenceEntry[T], bool) {
	if registry == nil {
		var zero EvidenceEntry[T]
		return zero, false
	}
	entry, ok := registry.entries[key]
	return entry, ok
}

// Snapshot returns entries sorted by canonical key for deterministic planning.
func (registry *EvidenceRegistry[T]) Snapshot() []EvidenceEntry[T] {
	if registry == nil {
		return []EvidenceEntry[T]{}
	}
	keys := make([]string, 0, len(registry.entries))
	for key := range registry.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]EvidenceEntry[T], 0, len(keys))
	for _, key := range keys {
		entries = append(entries, registry.entries[key])
	}
	return entries
}

// Len returns the number of canonical evidence entries.
func (registry *EvidenceRegistry[T]) Len() int {
	if registry == nil {
		return 0
	}
	return len(registry.entries)
}

func canonicalEvidenceKey(scope Scope, ref EvidenceRef) (string, error) {
	if ref.Path != "" && ref.Fingerprint != "" {
		path := filepath.Clean(ref.Path)
		identity := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%s",
			scope.RepoRoot, scope.WorktreeID, scope.SessionID, scope.WorktreeRevision,
			path, ref.LineStart, ref.LineEnd, ref.Fingerprint)
		return digestKey(identity), nil
	}
	if strings.TrimSpace(ref.Key) != "" {
		return digestKey(scope.RepoRoot + "\x00" + scope.WorktreeID + "\x00" + scope.SessionID + "\x00" + scope.WorktreeRevision + "\x00key\x00" + ref.Key), nil
	}
	return "", errMissingEvidenceIdentity
}

func digestKey(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("ev:%x", sum[:])
}

func normalizeConfidence(confidence EvidenceConfidence, proof VerificationProof) EvidenceConfidence {
	switch confidence {
	case ConfidenceVerified:
		if proof.Valid() {
			return ConfidenceVerified
		}
		return ConfidenceCandidate
	case ConfidenceCandidate, ConfidenceUnknown:
		return confidence
	default:
		return ConfidenceUnknown
	}
}

func confidenceRank(confidence EvidenceConfidence) int {
	switch confidence {
	case ConfidenceVerified:
		return 3
	case ConfidenceCandidate:
		return 2
	default:
		return 1
	}
}
