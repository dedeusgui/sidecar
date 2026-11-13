// Package service exposes bounded, read-only Explore APIs for library callers. It depends on the
// shared traversal core instead of the legacy CLI adapter, so the V3.5 engine can call it safely.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/core"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

const (
	// QuickMaxFiles is the hard ceiling on source files shown by QuickExpand.
	QuickMaxFiles = 2
	// QuickMaxTokens caps estimated evidence tokens added by QuickExpand.
	QuickMaxTokens = 512
	// QuickMaxDuration caps QuickExpand's cooperative wall-time budget.
	QuickMaxDuration = 500 * time.Millisecond
	// DeepMaxFiles is the hard ceiling on source files shown by DeepExplore.
	DeepMaxFiles = 6
	// DeepMaxTokens caps estimated evidence tokens added by DeepExplore.
	DeepMaxTokens = 4000
	// DeepMaxDuration caps DeepExplore's cooperative wall-time budget.
	DeepMaxDuration = 2 * time.Second

	// MaxConcurrent is the process-wide maximum number of active exploration traversals.
	MaxConcurrent   = 1
	maxQuickAnchors = QuickMaxFiles
)

var traversalSlot = make(chan struct{}, MaxConcurrent)

// Limits sets cooperative wall-time, file and estimated-token ceilings. Zero values select the
// mode defaults; values above the mode caps are clamped. The traversal remains serial.
type Limits struct {
	Timeout   time.Duration
	MaxFiles  int
	MaxTokens int
}

// QuickRequest describes a small expansion around evidence already present in the registry.
type QuickRequest struct {
	Enabled              bool
	Root                 string
	Query                string
	Anchors              []contract.EvidenceRef
	Limits               Limits
	AllowCandidateAnchor bool
}

// DeepRequest describes an explicit on-demand exploration using the shared retrieval and traversal.
type DeepRequest struct {
	Root   string
	Query  string
	Impact bool
	Limits Limits
}

// ExpansionResult contains the explore view, the canonical references added to the registry, and
// stable uncertainty codes. It carries no prompt text in Gap values.
type ExpansionResult struct {
	Explore   core.Result
	Evidence  []contract.EvidenceRef
	Gaps      []contract.Gap
	CallPaths []string
}

// QuickExpand expands only current, verified registry anchors. Missing, disabled, stale, timed-out
// or empty exploration returns a gap and no error so the caller can keep its base evidence.
func QuickExpand(ctx context.Context, st *store.Store, req QuickRequest, registry *contract.EvidenceRegistry[retrieve.Evidence]) ExpansionResult {
	limits := normalizedLimits(req.Limits, Limits{Timeout: QuickMaxDuration, MaxFiles: QuickMaxFiles, MaxTokens: QuickMaxTokens},
		Limits{Timeout: QuickMaxDuration, MaxFiles: QuickMaxFiles, MaxTokens: QuickMaxTokens})
	if !req.Enabled {
		return empty("disabled")
	}
	if len(req.Anchors) == 0 {
		return empty("no_anchor")
	}
	if st == nil {
		return empty("store_unavailable")
	}
	if !validRegistryRoot(req.Root, registry) {
		return empty("registry_scope_mismatch")
	}

	bounded, release, ok := begin(ctx, limits.Timeout)
	if !ok {
		return empty("timeout")
	}
	defer release()

	anchors, rejectedStale, timedOut := verifiedAnchors(bounded, st, registry, req.Anchors, min(maxQuickAnchors, limits.MaxFiles))
	if timedOut || bounded.Err() != nil {
		return empty("timeout")
	}
	if len(anchors) == 0 && req.AllowCandidateAnchor {
		var candStale bool
		anchors, candStale, timedOut = selectAnchors(bounded, st, registry, req.Anchors, min(maxQuickAnchors, limits.MaxFiles), false)
		rejectedStale = rejectedStale || candStale
		if timedOut || bounded.Err() != nil {
			return empty("timeout")
		}
	}
	if len(anchors) == 0 {
		if rejectedStale {
			return empty("stale_anchor")
		}
		return empty("no_verified_anchor")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		var names []string
		for _, anchor := range anchors {
			if anchor.Hit.Name != "" {
				names = append(names, anchor.Hit.Name)
			}
		}
		query = strings.Join(names, " ")
	}
	if query == "" {
		return empty("no_anchor")
	}

	result, err := core.Build(bounded, st, core.Options{Root: req.Root, Query: query,
		MaxFiles: limits.MaxFiles, MaxChars: limits.MaxTokens * 4}, core.BuildInput{
		Evidence: retrieve.Result{Evidence: anchors}, Relevance: "relevant", Class: "quick_expand",
	})
	if bounded.Err() != nil {
		return empty("timeout")
	}
	if err != nil {
		return empty("explore_unavailable")
	}
	for _, anchor := range anchors {
		found, stale := sourceState(result, anchor.Hit.Path)
		if stale {
			return empty("stale_anchor")
		}
		if !found {
			return empty("anchor_unavailable")
		}
	}
	return collect(bounded, st, result, anchors, registry, limits.MaxTokens, false)
}

// DeepExplore performs a wider on-demand search with the same read-only traversal as QuickExpand.
// Search errors and empty results become stable gaps rather than errors to hook callers.
func DeepExplore(ctx context.Context, st *store.Store, req DeepRequest, registry *contract.EvidenceRegistry[retrieve.Evidence]) ExpansionResult {
	limits := normalizedLimits(req.Limits, Limits{Timeout: time.Second, MaxFiles: DeepMaxFiles, MaxTokens: DeepMaxTokens},
		Limits{Timeout: DeepMaxDuration, MaxFiles: DeepMaxFiles, MaxTokens: DeepMaxTokens})
	if st == nil {
		return empty("store_unavailable")
	}
	if strings.TrimSpace(req.Query) == "" {
		return empty("empty_query")
	}
	if !validRegistryRoot(req.Root, registry) {
		return empty("registry_scope_mismatch")
	}

	bounded, release, ok := begin(ctx, limits.Timeout)
	if !ok {
		return empty("timeout")
	}
	defer release()

	ro := retrieve.Options{
		Root: req.Root, MaxItems: min(12, limits.MaxFiles*4), MaxPerFile: 4, Profile: "deep",
		EngineVersion: "v2", NoExactDominance: true, ExtraExactKeys: core.MemberKeys(req.Query),
	}
	if len(strings.Fields(req.Query)) <= 3 {
		for _, word := range strings.Fields(req.Query) {
			word = core.TrimQueryToken(word)
			if len(word) >= 3 && core.IsIdent(word) {
				ro.ExtraExactKeys = append(ro.ExtraExactKeys, word)
			}
		}
	}
	search, err := retrieve.Search(bounded, st, req.Query, ro)
	if bounded.Err() != nil {
		return empty("timeout")
	}
	if err != nil {
		return empty("explore_unavailable")
	}
	result, err := core.Build(bounded, st, core.Options{Root: req.Root, Query: req.Query, Impact: req.Impact,
		MaxFiles: limits.MaxFiles, MaxChars: limits.MaxTokens * 4}, core.BuildInput{
		Evidence: search, Relevance: "relevant", Class: "deep_explore", Intent: search.Trace.Intent.Kinds,
	})
	if bounded.Err() != nil {
		return empty("timeout")
	}
	if err != nil {
		return empty("explore_unavailable")
	}
	return collect(bounded, st, result, search.Evidence, registry, limits.MaxTokens, true)
}

func normalizedLimits(request, defaults, caps Limits) Limits {
	if request.Timeout <= 0 {
		request.Timeout = defaults.Timeout
	}
	if request.Timeout > caps.Timeout {
		request.Timeout = caps.Timeout
	}
	if request.MaxFiles <= 0 {
		request.MaxFiles = defaults.MaxFiles
	}
	request.MaxFiles = min(request.MaxFiles, caps.MaxFiles)
	if request.MaxTokens <= 0 {
		request.MaxTokens = defaults.MaxTokens
	}
	request.MaxTokens = min(request.MaxTokens, caps.MaxTokens)
	return request
}

func begin(parent context.Context, timeout time.Duration) (context.Context, func(), bool) {
	if parent == nil {
		parent = context.Background()
	}
	if parent.Err() != nil {
		ctx, cancel := context.WithTimeout(parent, timeout)
		cancel()
		return ctx, func() {}, false
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	select {
	case traversalSlot <- struct{}{}:
		return ctx, func() { <-traversalSlot; cancel() }, true
	case <-ctx.Done():
		cancel()
		return ctx, func() {}, false
	}
}

func validRegistryRoot(root string, registry *contract.EvidenceRegistry[retrieve.Evidence]) bool {
	if root == "" || registry == nil {
		return false
	}
	return contract.NormalizeScope(registry.Scope()).RepoRoot == filepath.Clean(root)
}

func verifiedAnchors(ctx context.Context, st *store.Store, registry *contract.EvidenceRegistry[retrieve.Evidence], refs []contract.EvidenceRef, limit int) ([]retrieve.Evidence, bool, bool) {
	return selectAnchors(ctx, st, registry, refs, limit, true)
}

func selectAnchors(ctx context.Context, st *store.Store, registry *contract.EvidenceRegistry[retrieve.Evidence], refs []contract.EvidenceRef, limit int, requireVerified bool) ([]retrieve.Evidence, bool, bool) {
	anchors := make([]retrieve.Evidence, 0, min(limit, len(refs)))
	rejectedStale := false
	seen := map[string]bool{}
	type fileLookup struct {
		record store.FileRecord
		exists bool
		err    error
	}
	type symbolLookup struct {
		symbols []store.SymbolHit
		err     error
	}
	filesByPath := map[string]fileLookup{}
	symbolsByPath := map[string]symbolLookup{}
	for _, ref := range refs {
		if ctx.Err() != nil {
			return anchors, rejectedStale, true
		}
		if len(anchors) >= limit {
			break
		}
		if ref.Key == "" || seen[ref.Key] {
			continue
		}
		if requireVerified {
			if ref.Confidence != contract.ConfidenceVerified || !ref.Proof.Valid() {
				continue
			}
		} else if ref.Confidence != contract.ConfidenceVerified && ref.Confidence != contract.ConfidenceCandidate {
			continue
		}
		entry, ok := registry.Lookup(ref.Key)
		if !ok {
			continue
		}
		if requireVerified {
			if entry.Ref.Confidence != contract.ConfidenceVerified || !entry.Ref.Proof.Valid() || entry.Value.Verdict != "VERIFIED" {
				continue
			}
		} else {
			if entry.Ref.Confidence != contract.ConfidenceVerified && entry.Ref.Confidence != contract.ConfidenceCandidate {
				continue
			}
			if entry.Value.Verdict != "VERIFIED" && entry.Value.Verdict != "INFERRED" {
				continue
			}
			if entry.Value.Hit.Kind == "file" || entry.Value.Hit.Kind == "section" {
				continue
			}
		}
		if ref.Path != entry.Ref.Path || ref.Path != entry.Value.Hit.Path || ref.LineStart != entry.Ref.LineStart ||
			ref.LineEnd != entry.Ref.LineEnd || (requireVerified && ref.Proof != entry.Ref.Proof) {
			continue
		}
		if entry.Ref.Freshness != contract.FreshnessCurrent || ref.Freshness != contract.FreshnessCurrent {
			rejectedStale = true
			continue
		}
		hit := entry.Value.Hit
		if hit.Path == "" || hit.ID == 0 || hit.Line <= 0 || hit.FileHash == "" {
			continue
		}
		file, loaded := filesByPath[hit.Path]
		if !loaded {
			file.record, file.exists, file.err = st.FileRecordOf(hit.Path)
			filesByPath[hit.Path] = file
		}
		file = filesByPath[hit.Path]
		if file.err != nil || !file.exists || file.record.Hash != hit.FileHash || ref.Freshness == contract.FreshnessStale {
			rejectedStale = true
			continue
		}
		indexed := false
		symbols, loaded := symbolsByPath[hit.Path]
		if !loaded {
			symbols.symbols, symbols.err = st.SymbolsInFile(hit.Path, 1000)
			symbolsByPath[hit.Path] = symbols
		}
		symbols = symbolsByPath[hit.Path]
		if symbols.err != nil {
			rejectedStale = true
			continue
		}
		for _, symbol := range symbols.symbols {
			if symbol.ID == hit.ID && symbol.Name == hit.Name && symbol.Line == hit.Line && symbol.FileHash == hit.FileHash {
				indexed = true
				break
			}
		}
		if !indexed {
			rejectedStale = true
			continue
		}
		seen[ref.Key] = true
		anchors = append(anchors, entry.Value)
	}
	return anchors, rejectedStale, false
}

func collect(ctx context.Context, st *store.Store, explored core.Result, seeds []retrieve.Evidence,
	registry *contract.EvidenceRegistry[retrieve.Evidence], maxTokens int, includeSeeds bool) ExpansionResult {
	out := ExpansionResult{Explore: explored, Evidence: []contract.EvidenceRef{}, Gaps: []contract.Gap{}}
	if len(explored.Files) == 0 && len(explored.Relations) == 0 {
		out.Gaps = append(out.Gaps, contract.Gap{Kind: "empty_result"})
	}
	if len(explored.NotShown) > 0 || explored.Budget.UsedChars >= explored.Budget.MaxChars {
		out.Gaps = append(out.Gaps, contract.Gap{Kind: "explore_budget"})
	}
	for _, gap := range explored.Gaps {
		if strings.Contains(gap, "changed on disk") {
			out.Gaps = append(out.Gaps, contract.Gap{Kind: "stale_source"})
			break
		}
	}
	if ctx.Err() != nil {
		return empty("timeout")
	}
	currentPaths := make(map[string]bool, len(explored.Files))
	stalePaths := make(map[string]bool)
	testPaths := make(map[string]bool)
	for _, file := range explored.Files {
		if file.Role == "test" {
			testPaths[file.Path] = true
		}
		if file.Stale {
			stalePaths[file.Path] = true
			continue
		}
		if len(file.Snippets) > 0 {
			currentPaths[file.Path] = true
		}
	}
	out.CallPaths = FormatCallPaths(explored.Relations, testPaths)

	candidates := make([]retrieve.Evidence, 0, len(seeds)+len(explored.Relations))
	seedIDs := make(map[int64]bool, len(seeds))
	for _, seed := range seeds {
		if seed.Hit.ID != 0 {
			seedIDs[seed.Hit.ID] = true
		}
	}
	if includeSeeds {
		candidates = append(candidates, seeds...)
	}
	symbolCache := map[string][]store.SymbolHit{}
	loadedPaths := map[string]bool{}
	var priorityCandidates, secondaryCandidates []retrieve.Evidence
	coveredHits := map[int64]bool{}
	for _, edge := range explored.Relations {
		if ctx.Err() != nil {
			return empty("timeout")
		}
		for _, ev := range relationEvidence(ctx, st, edge, symbolCache, loadedPaths, currentPaths) {
			if ctx.Err() != nil {
				return empty("timeout")
			}
			if seedIDs[ev.Hit.ID] {
				continue
			}
			if edge.Kind == "calls" && currentPaths[ev.Hit.Path] {
				priorityCandidates = append(priorityCandidates, ev)
				coveredHits[ev.Hit.ID] = true
			} else {
				secondaryCandidates = append(secondaryCandidates, ev)
			}
		}
	}
	for _, file := range explored.Files {
		if file.Stale || len(file.Snippets) == 0 {
			continue
		}
		for _, sym := range file.Symbols {
			if sym.Source != "structural" {
				continue
			}
			hit, ok := lookupSymbol(st, file.Path, sym.Name, sym.Line, symbolCache, loadedPaths)
			if !ok || seedIDs[hit.ID] || coveredHits[hit.ID] {
				continue
			}
			coveredHits[hit.ID] = true
			priorityCandidates = append(priorityCandidates, retrieve.Evidence{
				Hit:           hit,
				Verdict:       "INFERRED",
				Section:       "RELATIONS",
				Source:        "structural",
				Confidence:    0.55,
				Relations:     []string{"member:" + sym.Reason},
				TokenEstimate: max(1, ctxbuild.EstimateTokens(fmt.Sprintf("%s %s (%s:%d)", sym.Kind, sym.Name, file.Path, sym.Line))),
			})
		}
	}
	candidates = append(candidates, priorityCandidates...)
	candidates = append(candidates, secondaryCandidates...)
	used := 0
	seenKeys := map[string]bool{}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return empty("timeout")
		}
		if candidate.Note != "" || candidate.Hit.Path == "" || stalePaths[candidate.Hit.Path] {
			continue
		}
		ref := evidenceRef(st, candidate, currentPaths[candidate.Hit.Path])
		if ref.CostEstimate <= 0 {
			continue
		}
		if used+ref.CostEstimate > maxTokens {
			out.Gaps = appendGapOnce(out.Gaps, "evidence_budget")
			continue
		}
		canonical, inserted, err := registry.Add(ref, candidate)
		if err != nil {
			out.Gaps = appendGapOnce(out.Gaps, "registry_rejected")
			continue
		}
		if !inserted || seenKeys[canonical.Key] {
			continue
		}
		seenKeys[canonical.Key] = true
		used += canonical.CostEstimate
		out.Evidence = append(out.Evidence, canonical)
	}
	if len(out.Evidence) == 0 && len(out.Gaps) == 0 {
		out.Gaps = append(out.Gaps, contract.Gap{Kind: "empty_result"})
	}
	return out
}

func relationEvidence(ctx context.Context, st *store.Store, edge core.Edge, symbolCache map[string][]store.SymbolHit, loadedPaths, currentPaths map[string]bool) []retrieve.Evidence {
	verdict, confidence := statusValues(edge.Status)
	if edge.Status == "VERIFIED" && (!verifiedRelationBasis(edge.Basis) ||
		!currentPaths[edge.From.Path] || !currentPaths[edge.To.Path]) {
		verdict, confidence = "INFERRED", 0.55
	}
	proofRelation := edge.Kind + ":" + edge.Basis + ":" + edge.From.Path + ":" + edge.From.Name + ":" +
		fmt.Sprint(edge.From.Line) + "->" + edge.To.Path + ":" + edge.To.Name + ":" + fmt.Sprint(edge.To.Line)
	var out []retrieve.Evidence
	for _, ref := range []core.Ref{edge.From, edge.To} {
		if ctx.Err() != nil {
			return out
		}
		if ref.Path == "" {
			continue
		}
		var hit store.SymbolHit
		if ref.Name != "" {
			var ok bool
			hit, ok = lookupSymbol(st, ref.Path, ref.Name, ref.Line, symbolCache, loadedPaths)
			if !ok {
				continue
			}
		} else {
			file, ok, err := st.FileSymbol(ref.Path)
			if err != nil || !ok {
				continue
			}
			hit = file
		}
		out = append(out, retrieve.Evidence{
			Hit: hit, Verdict: verdict, Section: "RELATIONS", Source: "structural", Confidence: confidence,
			Relations: []string{proofRelation}, TokenEstimate: ctxbuild.EstimateTokens(edgeText(edge)),
		})
	}
	return out
}

func evidenceRef(st *store.Store, ev retrieve.Evidence, sourceCurrent bool) contract.EvidenceRef {
	hit := ev.Hit
	fingerprint := hit.FileHash
	if ev.Source == "structural" {
		sum := sha256.Sum256([]byte(hit.FileHash + "\x00" + strings.Join(ev.Relations, "\x00")))
		fingerprint = hex.EncodeToString(sum[:])
	}
	if fingerprint == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s", hit.Path, hit.Name, hit.Line, hit.Signature)))
		fingerprint = hex.EncodeToString(sum[:])
	}
	ref := contract.EvidenceRef{
		Path: hit.Path, LineStart: hit.Line, LineEnd: max(hit.Line, hit.EndLine), Fingerprint: fingerprint,
		Origin: "explore:" + ev.Source, Confidence: contract.ConfidenceUnknown,
		Freshness: contract.FreshnessUnknown, CostEstimate: estimatedCost(ev),
	}
	if hit.Path != "" && hit.FileHash != "" {
		if record, ok, err := st.FileRecordOf(hit.Path); err == nil && ok && record.Hash == hit.FileHash {
			ref.Freshness = contract.FreshnessUnknown
			if sourceCurrent {
				ref.Freshness = contract.FreshnessCurrent
			}
			if sourceCurrent && ev.Verdict == "VERIFIED" && hit.ID > 0 {
				ref.Confidence = contract.ConfidenceVerified
				ref.Proof = contract.VerificationProof{Kind: contract.VerificationIndex,
					Reference: fmt.Sprintf("symbol:%d:%s", hit.ID, hit.FileHash)}
			} else if ev.Verdict == "INFERRED" || ev.Verdict == "VERIFIED" {
				ref.Confidence = contract.ConfidenceCandidate
			}
		} else if ev.Verdict == "INFERRED" || ev.Verdict == "VERIFIED" {
			ref.Freshness = contract.FreshnessStale
			ref.Confidence = contract.ConfidenceCandidate
		}
	}
	if ev.Verdict == "VERIFIED" && ev.Source == "structural" && ev.Hit.ID > 0 && verifiedStructuralEvidence(ev) && ref.Freshness == contract.FreshnessCurrent {
		ref.Confidence = contract.ConfidenceVerified
		ref.Proof = contract.VerificationProof{Kind: contract.VerificationIndex,
			Reference: fmt.Sprintf("relation:%s:%d:%s", strings.Join(ev.Relations, ","), hit.ID, hit.FileHash)}
	}
	return ref
}

func estimatedCost(ev retrieve.Evidence) int {
	if ev.TokenEstimate > 0 {
		return ev.TokenEstimate
	}
	hit := ev.Hit
	return max(1, ctxbuild.EstimateTokens(fmt.Sprintf("%s %s (%s:%d-%d) %s",
		hit.Kind, hit.Name, hit.Path, hit.Line, max(hit.Line, hit.EndLine), hit.Signature)))
}

func sourceState(result core.Result, path string) (found, stale bool) {
	for _, file := range result.Files {
		if file.Path == path {
			return true, file.Stale
		}
	}
	return false, false
}

func verifiedStructuralEvidence(ev retrieve.Evidence) bool {
	for _, relation := range ev.Relations {
		if strings.HasPrefix(relation, "calls:same_file:") || strings.HasPrefix(relation, "calls:import:") ||
			strings.HasPrefix(relation, "imports:resolved_import:") {
			return true
		}
	}
	return false
}

func verifiedRelationBasis(basis string) bool {
	return basis == "same_file" || basis == "import" || basis == "resolved_import"
}

func statusValues(status string) (string, float64) {
	switch status {
	case "VERIFIED":
		return "VERIFIED", 0.85
	case "CANDIDATE":
		return "INFERRED", 0.55
	default:
		return "UNKNOWN", 0.2
	}
}

func edgeText(edge core.Edge) string {
	return fmt.Sprintf("%s %s:%d -> %s:%d %s", edge.Kind, edge.From.Path, edge.From.Line, edge.To.Path, edge.To.Line, edge.Basis)
}

func appendGapOnce(gaps []contract.Gap, kind string) []contract.Gap {
	for _, gap := range gaps {
		if gap.Kind == kind {
			return gaps
		}
	}
	return append(gaps, contract.Gap{Kind: kind})
}

func lookupSymbol(st *store.Store, path, name string, line int, symbolCache map[string][]store.SymbolHit, loadedPaths map[string]bool) (store.SymbolHit, bool) {
	if !loadedPaths[path] {
		symbols, err := st.SymbolsInFile(path, 1000)
		loadedPaths[path] = true
		if err == nil {
			symbolCache[path] = symbols
		}
	}
	for _, symbol := range symbolCache[path] {
		if symbol.Name == name && (line == 0 || symbol.Line == line) {
			return symbol, true
		}
	}
	return store.SymbolHit{}, false
}

// FormatCallPaths builds compact, deterministic call chains formatted as "Calls: A -> B -> C"
// from explore call relations, omitting test callers when production paths exist.
func FormatCallPaths(edges []core.Edge, testPaths map[string]bool) []string {
	type callLink struct {
		fromID, toID string
	}
	var callEdges []core.Edge
	var nonTestEdges []core.Edge
	for _, e := range edges {
		if e.Kind != "calls" || e.From.Name == "" || e.To.Name == "" {
			continue
		}
		callEdges = append(callEdges, e)
		if !isTestRef(e.From, testPaths) && !isTestRef(e.To, testPaths) {
			nonTestEdges = append(nonTestEdges, e)
		}
	}
	if len(nonTestEdges) > 0 {
		callEdges = nonTestEdges
	}
	if len(callEdges) == 0 {
		return nil
	}
	refID := func(r core.Ref) string {
		return r.Path + "#" + r.Name + "#" + strconv.Itoa(r.Line)
	}
	adj := map[string][]string{}
	inDegree := map[string]int{}
	displayName := map[string]string{}
	seenNode := map[string]bool{}
	seenLink := map[callLink]bool{}
	var order []string
	addNode := func(r core.Ref) string {
		id := refID(r)
		if !seenNode[id] {
			seenNode[id] = true
			displayName[id] = r.Name
			order = append(order, id)
		}
		return id
	}
	for _, e := range callEdges {
		fromID := addNode(e.From)
		toID := addNode(e.To)
		if fromID == toID {
			continue
		}
		link := callLink{fromID, toID}
		if seenLink[link] {
			continue
		}
		seenLink[link] = true
		adj[fromID] = append(adj[fromID], toID)
		inDegree[toID]++
	}
	const maxPaths = 6
	const maxDepth = 5
	var out []string
	seenPath := map[string]bool{}
	var dfs func(cur string, path []string, visiting map[string]bool)
	dfs = func(cur string, path []string, visiting map[string]bool) {
		if len(out) >= maxPaths {
			return
		}
		nexts := adj[cur]
		progressed := false
		if len(path) < maxDepth {
			for _, nxt := range nexts {
				if visiting[nxt] {
					continue
				}
				progressed = true
				visiting[nxt] = true
				dfs(nxt, append(path, displayName[nxt]), visiting)
				delete(visiting, nxt)
			}
		}
		if !progressed && len(path) >= 2 {
			formatted := "Calls: " + strings.Join(path, " -> ")
			if !seenPath[formatted] {
				seenPath[formatted] = true
				out = append(out, formatted)
			}
		}
	}
	for _, id := range order {
		if inDegree[id] == 0 && len(adj[id]) > 0 {
			visiting := map[string]bool{id: true}
			dfs(id, []string{displayName[id]}, visiting)
		}
	}
	if len(out) == 0 {
		for _, id := range order {
			if len(adj[id]) > 0 {
				visiting := map[string]bool{id: true}
				dfs(id, []string{displayName[id]}, visiting)
			}
		}
	}
	return out
}

func isTestRef(r core.Ref, testPaths map[string]bool) bool {
	if testPaths[r.Path] {
		return true
	}
	low := strings.ToLower(r.Path)
	return strings.Contains(low, "_test.") || strings.Contains(low, "/test/") || strings.Contains(low, "/tests/")
}

func empty(kind string) ExpansionResult {
	return ExpansionResult{
		Explore: core.Result{Schema: core.Schema, EntryPoints: []core.Ref{}, Files: []core.File{}, Relations: []core.Edge{},
			Consumers: []core.Ref{}, Dependencies: []core.Ref{}, Tests: []core.Ref{}, Gaps: []string{}},
		Evidence: []contract.EvidenceRef{}, Gaps: []contract.Gap{{Kind: kind}},
	}
}
