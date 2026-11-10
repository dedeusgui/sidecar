package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/service"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/impact"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

type v35Output struct {
	Result   retrieve.Result
	Decision engine.Decision
	Plan     contract.OutputPlan
	Packed   ctxbuild.PackResult
}

// runV35Safe makes every V3.5-only failure an empty context. The hook remains fail-open,
// and explain can still report the retrieval trace without exposing an internal diagnostic.
func runV35Safe(ctx context.Context, st *store.Store, root, prompt string, opts retrieve.Options,
	agent adapter.Agent, sessionID, statePath string, now time.Time, deep bool) (out v35Output, err error) {
	defer func() {
		if recover() != nil {
			out = v35Output{}
			err = fmt.Errorf("v3.5 transaction failed")
		}
	}()
	return runV35(ctx, st, root, prompt, opts, agent, sessionID, statePath, now, deep)
}

func runV35(ctx context.Context, st *store.Store, root, prompt string, opts retrieve.Options,
	agent adapter.Agent, sessionID, statePath string, now time.Time, deep bool) (v35Output, error) {
	result, decision, err := engine.Run(ctx, st, prompt, engine.Options{Retrieve: opts})
	if err != nil {
		return v35Output{}, err
	}
	decision.Engine = engine.Version35
	if decision.Action == engine.ActionAbstain || ctx.Err() != nil {
		return v35Output{Result: result, Decision: decision}, nil
	}

	revision, err := gitx.Head(ctx, root)
	if err != nil {
		if ctx.Err() != nil {
			return v35Output{}, ctx.Err()
		}
		revision = "no-head"
	}
	scope := contract.NormalizeScope(contract.Scope{
		RepoRoot: root, WorktreeID: root, SessionID: sessionID, WorktreeRevision: revision,
	})
	registry := contract.NewEvidenceRegistry[retrieve.Evidence](scope)
	baseRefs, err := registerV35Evidence(st, root, result, registry)
	if err != nil {
		return v35Output{}, err
	}
	observability := v35Observability(root, agent, scope, now)
	phase := v35Phase(decision, observability)
	task := contract.TaskContext[engine.TaskSignals]{
		Scope: scope, QuerySignals: decision.Signals, Phase: phase,
		Observability: observability, ExploreCapability: contract.CapabilityUnverified,
		Evidence: append([]contract.EvidenceRef(nil), baseRefs...),
		Guidance: []contract.GuidanceProposal{}, Uncertainties: []contract.Gap{},
	}
	var proposals []ctxbuild.GuidanceProposal
	var groomState *GroomProposalResult

	// The decision action is the planner's request for bounded structural evidence. Hooks use
	// QuickExpand; explain is an explicit diagnostic surface and can use the wider shared traversal.
	if decision.Action == engine.ActionExploreAvailable || decision.Action == engine.ActionVerify {
		var expansion service.ExpansionResult
		if deep {
			expansion = service.DeepExplore(ctx, st, service.DeepRequest{
				Root: root, Query: prompt,
				Limits: service.Limits{Timeout: service.DeepMaxDuration, MaxFiles: service.DeepMaxFiles,
					MaxTokens: min(service.DeepMaxTokens, max(1, decision.Budget/3))},
			}, registry)
		} else {
			expansion = service.QuickExpand(ctx, st, service.QuickRequest{
				Enabled: true, Root: root, Query: prompt, Anchors: baseRefs,
				Limits: service.Limits{Timeout: service.QuickMaxDuration, MaxFiles: service.QuickMaxFiles,
					MaxTokens: min(service.QuickMaxTokens, max(1, decision.Budget/3))},
			}, registry)
		}
		task.Uncertainties = append(task.Uncertainties, expansion.Gaps...)
		items, keys := expansionItems(expansion, registry)
		if len(items) > 0 && len(keys) > 0 {
			proposal := contract.GuidanceProposal{Kind: ctxbuild.Code, EvidenceKeys: keys, Priority: 115,
				TokenEstimate: estimateItems(items), DeliveryReason: "explore.bounded_source_snippet", Phase: phase}
			task.Guidance = append(task.Guidance, proposal)
			proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: items})
		}
	}

	briefItems, hasBrief := brief.MicroBriefProposal(result)
	briefKeys := evidenceKeys(registryRefs(registry))
	if hasBrief && len(briefItems) > 0 && len(briefKeys) > 0 {
		proposal := contract.GuidanceProposal{Kind: ctxbuild.TaskBrief, EvidenceKeys: briefKeys, Priority: 80,
			TokenEstimate: estimateItems(briefItems), DeliveryReason: "brief.micro_orientation", Phase: phase}
		task.Guidance = append(task.Guidance, proposal)
		proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: briefItems})
	}

	if retrieve.FeatureEnabled("groom") {
		state := LoadGroomProposalState(scope)
		g, ok := ProposeGroomScoped(prompt, result, scope, phase, state,
			agent == adapter.Claude, registry, now)
		if g.Answered {
			if err := RecordGroomProposalAnswer(st, scope, g, now); err != nil {
				return v35Output{}, err
			}
			if err := SaveGroomProposalState(scope, g.State); err != nil {
				return v35Output{}, err
			}
		} else if ok {
			text := groom.Render(agent, g.Guidance.Decision)
			if text != "" {
				proposal := g.Guidance.Proposal
				item := ctxbuild.Item{Section: proposal.Kind, Key: "groom|" + strings.Join(proposal.EvidenceKeys, "|"),
					Fingerprint: textFingerprint(text), Text: text, Priority: proposal.Priority, Source: "groom",
					Role: "open_decision", Qualifier: "suggested"}
				task.Guidance = append(task.Guidance, proposal)
				proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: []ctxbuild.Item{item}})
				groomState = &g
			}
		}
	}

	if decision.Signals.Edit || decision.Action == engine.ActionVerify {
		if retrieve.FeatureEnabled("impact") {
			files, statusErr := uncommitted(ctx, root)
			if statusErr == nil && relevantChangedFiles(result, files) {
				report, analyzeErr := impact.Analyze(st, root, files)
				if analyzeErr != nil {
					return v35Output{}, analyzeErr
				}
				guidance, ok, proposeErr := ProposeImpact(ctx, root, report, scope, phase, true, registry)
				if proposeErr != nil {
					return v35Output{}, proposeErr
				}
				if ok {
					items := impactItems(guidance)
					if len(items) > 0 {
						proposal := canonicalV35Proposal(guidance.Proposal)
						for i := range items {
							items[i].Section = proposal.Kind
						}
						task.Guidance = append(task.Guidance, proposal)
						proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: items})
					}
				}
			}
		}
	}

	named := make([]string, 0, len(result.Evidence)*2)
	for _, evidence := range result.Evidence {
		named = append(named, evidence.Hit.Name, evidence.Hit.Path)
	}
	memory, err := ProposeMemory(st, scope, task.Evidence, named, now, registry)
	if err != nil {
		return v35Output{}, err
	}
	for _, guidance := range memory {
		proposal := canonicalV35Proposal(guidance.Proposal)
		item := memoryItem(guidance)
		item.Section = proposal.Kind
		task.Guidance = append(task.Guidance, proposal)
		proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: []ctxbuild.Item{item}})
	}

	if observability.ToolEvents == contract.ObservationSeen || observability.PreCompact == contract.ObservationSeen {
		for _, guidance := range ProposeObserverGuidance(scope, agent, observability, phase, now, registry) {
			proposal := canonicalV35Proposal(guidance.Proposal)
			item := sessionItem(guidance)
			item.Section = proposal.Kind
			task.Guidance = append(task.Guidance, proposal)
			proposals = append(proposals, ctxbuild.GuidanceProposal{Proposal: proposal, Items: []ctxbuild.Item{item}})
		}
	}

	if err := ctx.Err(); err != nil {
		return v35Output{}, err
	}
	task.Evidence = registryRefs(registry)
	planDecision := decision
	if len(planDecision.Included) > 1 {
		// Reserve room for the hybrid brief and a source window while preserving V3's top-ranked anchor.
		planDecision.Included = planDecision.Included[:1]
	}
	plan := (engine.Planner{}).Plan(engine.PlanInput{Task: task, Decision: planDecision,
		Evidence: registry, BudgetTokens: decision.Budget})
	snippetKeys := snippetKeysAndFingerprints(proposals)
	for i := range proposals {
		if proposals[i].Proposal.Kind != ctxbuild.TaskBrief {
			continue
		}
		for j := range proposals[i].Items {
			if fingerprint, ok := snippetKeys[proposals[i].Items[j].Key]; ok {
				proposals[i].Items[j].Fingerprint = fingerprint
			}
		}
	}
	packed := ctxbuild.Pack(plan, registry, func(entry contract.EvidenceEntry[retrieve.Evidence]) []ctxbuild.Item {
		value := entry.Value
		if _, hasSnippet := snippetKeys[value.Key()]; hasSnippet {
			return nil
		}
		return []ctxbuild.Item{{Section: value.Section, Key: value.Key(), Fingerprint: value.Fingerprint(),
			Text: retrieve.Describe(value), Source: value.Source, Role: value.Hit.Kind,
			Qualifier: evidenceQualifier(entry.Ref), Provenance: evidenceProvenance(entry.Ref)}}
	}, proposals, opts.Seen)
	if groomState != nil && containsSource(packed.Items, "groom") {
		if err := SaveGroomProposalState(scope, groomState.State); err != nil {
			return v35Output{}, err
		}
	}
	return v35Output{Result: result, Decision: decision, Plan: plan, Packed: packed}, nil
}

func registerV35Evidence(st *store.Store, root string, result retrieve.Result, registry *contract.EvidenceRegistry[retrieve.Evidence]) ([]contract.EvidenceRef, error) {
	refs := make([]contract.EvidenceRef, 0, len(result.Evidence))
	for _, evidence := range result.Evidence {
		ref := contract.EvidenceRef{Key: evidence.Key(), Fingerprint: evidence.Fingerprint(), Path: evidence.Hit.Path,
			LineStart: evidence.Hit.Line, LineEnd: evidence.Hit.EndLine, Origin: evidence.Source,
			Confidence: contract.ConfidenceCandidate, Freshness: contract.FreshnessUnknown,
			CostEstimate: max(1, evidence.TokenEstimate)}
		if ref.LineEnd < ref.LineStart {
			ref.LineEnd = ref.LineStart
		}
		if evidence.Verdict == "UNKNOWN" {
			ref.Confidence = contract.ConfidenceUnknown
		}
		if evidence.Verdict == "VERIFIED" && evidence.Hit.Path != "" && evidence.Hit.ID != 0 && evidence.Hit.Line > 0 {
			ref.Confidence = contract.ConfidenceVerified
			ref.Proof = contract.VerificationProof{Kind: contract.VerificationIndex, Reference: evidence.ID}
		}
		if evidence.Hit.Path == "" || evidence.Note != "" {
			ref.Freshness = contract.FreshnessCurrent
		} else {
			hash, exists := diskHash(root, evidence.Hit.Path)
			record, indexed, err := st.FileRecordOf(evidence.Hit.Path)
			if err != nil {
				return nil, err
			}
			if exists && indexed && hash == evidence.Hit.FileHash && record.Hash == evidence.Hit.FileHash {
				ref.Freshness = contract.FreshnessCurrent
			} else {
				ref.Freshness = contract.FreshnessStale
			}
		}
		canonical, _, err := registry.Add(ref, evidence)
		if err != nil {
			return nil, err
		}
		refs = append(refs, canonical)
	}
	return refs, nil
}

func v35Observability(root string, agent adapter.Agent, scope contract.Scope, now time.Time) contract.Observability {
	if scope.SessionID == "" {
		return contract.Observability{ToolEvents: contract.ObservationUnsupported, PreCompact: contract.ObservationUnsupported}
	}
	statePath := paths.ObserverState(root, string(agent), scope.SessionID)
	signals := brief.LoadSignals(statePath, scope.WorktreeRevision, now)
	if signals.ScopeFingerprint != brief.ScopeFingerprint(observerScope(root, scope.SessionID, scope.WorktreeRevision)) {
		return contract.Observability{ToolEvents: contract.ObservationUnsupported, PreCompact: contract.ObservationUnsupported}
	}
	out := contract.Observability{ToolEvents: contract.ObservationUnsupported, PreCompact: contract.ObservationUnsupported}
	if len(signals.Trace) > 0 {
		out.ToolEvents = contract.ObservationSeen
	}
	if signals.Compactions > 0 {
		out.PreCompact = contract.ObservationSeen
	}
	return out
}

func v35Phase(decision engine.Decision, observability contract.Observability) contract.TaskPhase {
	if observability.PreCompact == contract.ObservationSeen {
		return contract.PhaseResume
	}
	switch {
	case decision.Action == engine.ActionClarify:
		return contract.PhaseDecide
	case decision.Action == engine.ActionVerify || decision.Class == engine.ClassDebug:
		return contract.PhaseVerify
	case decision.Signals.Edit:
		return contract.PhaseEdit
	case decision.Action == engine.ActionExploreAvailable || decision.Class == engine.ClassLocate || decision.Class == engine.ClassSimpleQuery:
		return contract.PhaseExplore
	default:
		return contract.PhaseUnknown
	}
}

func expansionItems(expansion service.ExpansionResult, registry *contract.EvidenceRegistry[retrieve.Evidence]) ([]ctxbuild.Item, []string) {
	entries := registry.Snapshot()
	var items []ctxbuild.Item
	keySet := map[string]bool{}
	for _, file := range expansion.Explore.Files {
		if file.Stale || len(file.Snippets) == 0 {
			continue
		}
		snippet := file.Snippets[0]
		var match *contract.EvidenceEntry[retrieve.Evidence]
		for i := range entries {
			entry := &entries[i]
			if entry.Ref.Path != file.Path || entry.Ref.Freshness != contract.FreshnessCurrent {
				continue
			}
			if entry.Ref.LineStart > 0 && (entry.Ref.LineStart > snippet.End || entry.Ref.LineEnd > 0 && entry.Ref.LineEnd < snippet.Start) {
				continue
			}
			match = entry
			if entry.Ref.Confidence == contract.ConfidenceVerified {
				break
			}
		}
		if match == nil {
			continue
		}
		text := fmt.Sprintf("%s:%d-%d %s", file.Path, snippet.Start, snippet.End, snippet.Text)
		value := match.Value
		items = append(items, ctxbuild.Item{Section: ctxbuild.Code, Key: value.Key(), Fingerprint: textFingerprint(text),
			Text: text, Priority: 115, Source: "explore", Role: "snippet",
			Qualifier: evidenceQualifier(match.Ref), Provenance: fmt.Sprintf("%s:%d-%d", file.Path, snippet.Start, snippet.End)})
		keySet[match.Ref.Key] = true
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return items, keys
}

func evidenceKeys(refs []contract.EvidenceRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Key != "" && ref.Freshness != contract.FreshnessStale {
			keys = append(keys, ref.Key)
		}
	}
	sort.Strings(keys)
	return uniqueStrings(keys)
}

func registryRefs(registry *contract.EvidenceRegistry[retrieve.Evidence]) []contract.EvidenceRef {
	entries := registry.Snapshot()
	refs := make([]contract.EvidenceRef, 0, len(entries))
	for _, entry := range entries {
		refs = append(refs, entry.Ref)
	}
	return refs
}

func estimateItems(items []ctxbuild.Item) int {
	tokens := 8
	for _, item := range items {
		tokens += ctxbuild.EstimateTokens(item.Text) + 1
	}
	return tokens
}

func impactItems(guidance impact.Guidance) []ctxbuild.Item {
	items := make([]ctxbuild.Item, 0, len(guidance.Findings))
	for i, finding := range guidance.Findings {
		if i >= len(guidance.Evidence) {
			break
		}
		ref := guidance.Evidence[i]
		provenance := finding.Path
		if finding.Line > 0 {
			provenance = fmt.Sprintf("%s:%d", finding.Path, finding.Line)
		}
		items = append(items, ctxbuild.Item{Section: ctxbuild.ChangeImpact, Key: "impact|" + ref.Key,
			Fingerprint: textFingerprint(finding.Text), Text: fmt.Sprintf("[%s] %s", finding.Status, finding.Text),
			Priority: guidance.Proposal.Priority, Source: "impact", Role: finding.Kind,
			Qualifier: string(ref.Confidence), Provenance: provenance})
	}
	return items
}

func memoryItem(guidance MemoryGuidance) ctxbuild.Item {
	decision := guidance.Decision
	text := fmt.Sprintf("[explicit user decision] %s (%s): %s", decision.SymbolOrPath, decision.FilePath, decision.Summary)
	return ctxbuild.Item{Section: ctxbuild.Memory, Key: fmt.Sprintf("memory|%d|%s", decision.ID, decision.FileHash),
		Fingerprint: textFingerprint(text), Text: text, Priority: guidance.Proposal.Priority,
		Source: "memory", Role: "explicit_decision", Qualifier: "hash and TTL validated", Provenance: decision.FilePath}
}

func sessionItem(guidance brief.SessionGuidance) ctxbuild.Item {
	text := "Session observation was recorded."
	switch guidance.Proposal.DeliveryReason {
	case "session.observed_repeated_error":
		text = "A repeated test or tool error was observed in this session; recheck the relevant source before retrying."
	case "session.observed_repeated_navigation":
		text = "Repeated navigation was observed in this session; prioritize the relevant source evidence."
	case "session.observed_precompact":
		text = "The session compacted; re-establish evidence validated for the current repository snapshot."
	}
	key := "session"
	if len(guidance.Proposal.EvidenceKeys) > 0 {
		key += "|" + guidance.Proposal.EvidenceKeys[0]
	}
	return ctxbuild.Item{Section: ctxbuild.Session, Key: key, Fingerprint: textFingerprint(text), Text: text,
		Priority: guidance.Proposal.Priority, Source: "session", Role: "observed_signal",
		Qualifier: "observed", Provenance: guidance.Proposal.DeliveryReason}
}

func relevantChangedFiles(result retrieve.Result, changed []string) bool {
	set := make(map[string]bool, len(changed))
	for _, path := range changed {
		set[path] = true
	}
	for _, evidence := range result.Evidence {
		if evidence.Dirty || set[evidence.Hit.Path] {
			return true
		}
	}
	return false
}

func evidenceQualifier(ref contract.EvidenceRef) string {
	if ref.Confidence == contract.ConfidenceVerified && ref.Proof.Valid() {
		return string(contract.ConfidenceVerified)
	}
	return string(ref.Confidence)
}

func evidenceProvenance(ref contract.EvidenceRef) string {
	if ref.Path == "" {
		return ref.Origin
	}
	if ref.LineStart > 0 {
		return fmt.Sprintf("%s:%d-%d", ref.Path, ref.LineStart, max(ref.LineStart, ref.LineEnd))
	}
	return ref.Path
}

func snippetKeysAndFingerprints(proposals []ctxbuild.GuidanceProposal) map[string]string {
	out := map[string]string{}
	for _, proposal := range proposals {
		for _, item := range proposal.Items {
			if item.Source == "explore" {
				out[item.Key] = item.Fingerprint
			}
		}
	}
	return out
}

func containsSource(items []ctxbuild.Item, source string) bool {
	for _, item := range items {
		if item.Source == source {
			return true
		}
	}
	return false
}

func packedSourceEvidenceKeys(items []ctxbuild.Item) []string {
	seen := make(map[string]bool, len(items))
	keys := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Source {
		case "exact", "lexical", "contract", "structural", "git", "lsp", "docprovider", "explore":
		default:
			continue
		}
		if item.Key != "" && !seen[item.Key] {
			seen[item.Key] = true
			keys = append(keys, item.Key)
		}
	}
	return keys
}

func canonicalV35Proposal(proposal contract.GuidanceProposal) contract.GuidanceProposal {
	switch proposal.Kind {
	case "CHANGE_IMPACT":
		proposal.Kind = ctxbuild.ChangeImpact
	case "MEMORY_DECISION":
		proposal.Kind = ctxbuild.Memory
	case "SESSION_GUIDANCE":
		proposal.Kind = ctxbuild.Session
	}
	return proposal
}

func textFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum[:])
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	sort.Strings(values)
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
