package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/service"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

const v37MaxSnippetLines = 24

// v37ExpansionItems adapts exploration snippets and synthesized call paths for V3.7 under maxTokens.
// Oversized snippets are compacted around their head lines instead of dropping the whole snippet so
// multi-hop flow chains fit within the hook budget.
func v37ExpansionItems(expansion service.ExpansionResult, registry *contract.EvidenceRegistry[retrieve.Evidence], maxTokens int) ([]ctxbuild.Item, []string) {
	items, keys := v36ExpansionItems(expansion, registry, maxTokens)
	if len(expansion.Explore.Files) > 0 {
		// V3.6 skips a snippet that does not fit whole. Offer those snippets in compacted form with the
		// budget V3.6 left over, instead of only compacting when V3.6 delivered nothing at all.
		spent := 0
		for _, it := range items {
			spent += ctxbuild.PackItemTokens(it)
		}
		taken := make(map[string]bool, len(keys))
		for _, key := range keys {
			taken[key] = true
		}
		more, moreKeys := compactExpansionSnippets(expansion, registry, maxTokens-spent, taken)
		items = append(items, more...)
		keys = append(keys, moreKeys...)
		sort.Strings(keys)
		keys = uniqueStrings(keys)
	}
	if len(expansion.CallPaths) > 0 {
		anchorKeys := keys
		if len(anchorKeys) == 0 {
			for _, ref := range expansion.Evidence {
				if ref.Key != "" && ref.Freshness != contract.FreshnessStale {
					anchorKeys = append(anchorKeys, ref.Key)
				}
			}
		}
		if len(anchorKeys) > 0 {
			used := ctxbuild.SectionFrameTokens(ctxbuild.Code, 1)
			for _, it := range items {
				used += ctxbuild.PackItemTokens(it)
			}
			pathText := sanitizeTrustLabels(strings.Join(expansion.CallPaths[:min(2, len(expansion.CallPaths))], " ; "))
			callItem := ctxbuild.Item{
				Section:     ctxbuild.Code,
				Key:         "explore|callpath",
				Fingerprint: textFingerprint(pathText),
				Text:        pathText,
				Priority:    112,
				Source:      "explore",
				Role:        "callpath",
				Qualifier:   string(contract.ConfidenceCandidate),
				Provenance:  "callgraph",
			}
			if used+ctxbuild.PackItemTokens(callItem) <= maxTokens {
				items = append(items, callItem)
				keys = uniqueStrings(append([]string(nil), anchorKeys...))
			}
		}
	}
	return items, keys
}

var v37EmbeddedLabelReplacer = strings.NewReplacer(
	"[VERIFIED]", "(verified)",
	"[CANDIDATE]", "(candidate)",
	"[INFERRED]", "(inferred)",
	"[UNKNOWN]", "(unknown)",
)

func sanitizeTrustLabels(s string) string {
	return v37EmbeddedLabelReplacer.Replace(s)
}

// v37EvidenceItem renders one registry entry with a single authoritative trust label suffix:
// relation tags like "docstring path [INFERRED]" are normalized so a single bullet never emits
// conflicting bracketed confidence badges alongside the packer's [VERIFIED]/[CANDIDATE] suffix.
func v37EvidenceItem(entry contract.EvidenceEntry[retrieve.Evidence]) ctxbuild.Item {
	value := entry.Value
	if len(value.Relations) > 0 {
		rels := make([]string, len(value.Relations))
		for i, r := range value.Relations {
			rels[i] = sanitizeTrustLabels(r)
		}
		value.Relations = rels
	}
	return ctxbuild.Item{
		Section:     value.Section,
		Key:         value.Key(),
		Fingerprint: value.Fingerprint(),
		Text:        sanitizeTrustLabels(retrieve.DescribeBare(value)),
		Source:      value.Source,
		Role:        value.Hit.Kind,
		Qualifier:   evidenceQualifier(entry.Ref),
		Provenance:  evidenceProvenance(entry.Ref),
	}
}

// compactExpansionSnippets offers the head of each snippet whose evidence key is not already in taken,
// within maxTokens. A non-positive budget yields nothing.
func compactExpansionSnippets(expansion service.ExpansionResult, registry *contract.EvidenceRegistry[retrieve.Evidence], maxTokens int, taken map[string]bool) ([]ctxbuild.Item, []string) {
	entries := registry.Snapshot()
	var items []ctxbuild.Item
	used := make(map[string]bool, len(taken))
	for key := range taken {
		used[key] = true
	}
	tokens := 0
	for _, file := range expansion.Explore.Files {
		if file.Stale {
			continue
		}
		for _, snippet := range file.Snippets {
			var body []string
			for _, line := range strings.Split(snippet.Text, "\n") {
				if _, source, _ := strings.Cut(line, "\t"); strings.TrimSpace(source) != "" {
					body = append(body, line)
					if len(body) >= v37MaxSnippetLines {
						break
					}
				}
			}
			if len(body) < minSnippetLines {
				continue
			}
			var match *contract.EvidenceEntry[retrieve.Evidence]
			for i := range entries {
				entry := &entries[i]
				if entry.Ref.Path != file.Path || entry.Ref.Freshness != contract.FreshnessCurrent || used[entry.Ref.Key] ||
					entry.Ref.LineStart > 0 && (entry.Ref.LineStart > snippet.End || entry.Ref.LineEnd > 0 && entry.Ref.LineEnd < snippet.Start) {
					continue
				}
				if match == nil || entry.Ref.LineStart >= snippet.Start && (match.Ref.LineStart < snippet.Start || entry.Ref.LineStart < match.Ref.LineStart) {
					match = entry
				}
			}
			if match == nil {
				continue
			}
			provenance := fmt.Sprintf("%s:%d-%d", file.Path, snippet.Start, snippet.End)
			item := ctxbuild.Item{Section: ctxbuild.Code, Key: match.Value.Key(), Text: "source",
				Fingerprint: textFingerprint(provenance + "\x00" + snippet.Text), Priority: 115, Source: "explore", Role: "snippet",
				Qualifier: evidenceQualifier(match.Ref), Provenance: provenance, Body: body}
			if h := match.Value.Hit; h.Kind != "file" && h.Name != "" {
				item.Text = h.Kind + " " + h.Name
			}
			cost := ctxbuild.PackItemTokens(item)
			if tokens+cost+ctxbuild.SectionFrameTokens(ctxbuild.Code, 1) > maxTokens {
				continue
			}
			tokens += cost
			used[match.Ref.Key] = true
			items = append(items, item)
		}
	}
	keys := make([]string, 0, len(used))
	for key := range used {
		if !taken[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return items, keys
}

// v37PrepareProposals eliminates Planner<->PackWith discrepancies before planning:
// 1. drops items already filtered by seen so the planner does not spend budget on dead proposals,
// 2. drops redundant TaskBrief "entrypoint" items when an explore snippet already covers that key,
// 3. drops TaskBrief "test" orientation items when CONTRACTS ranked evidence already names a test symbol,
// 4. prices every remaining proposal with exact rendered PackItemTokens.
func v37PrepareProposals(proposals []ctxbuild.GuidanceProposal, choices []engine.Choice, snippetKeys map[string]string, seen ctxbuild.Seen) ([]ctxbuild.GuidanceProposal, []contract.GuidanceProposal) {
	// Only a contract that names a test file makes the TaskBrief test pointer redundant.
	hasContractTest := false
	for _, c := range choices {
		if c.Source == "contract" && repo.Kind(c.Path) == "test" {
			hasContractTest = true
			break
		}
	}
	out := make([]ctxbuild.GuidanceProposal, 0, len(proposals))
	metas := make([]contract.GuidanceProposal, 0, len(proposals))
	for _, p := range proposals {
		kept := make([]ctxbuild.Item, 0, len(p.Items))
		for _, item := range p.Items {
			if seen != nil && seen(item.Key, item.Fingerprint) {
				continue
			}
			if p.Proposal.Kind == ctxbuild.TaskBrief {
				if item.Role == "entrypoint" {
					if _, covered := snippetKeys[item.Key]; covered {
						continue
					}
				}
				if item.Role == "test" && hasContractTest {
					continue
				}
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			continue
		}
		estimate := ctxbuild.SectionFrameTokens(p.Proposal.Kind, len(kept))
		for _, item := range kept {
			estimate += ctxbuild.PackItemTokens(item)
		}
		p.Items = kept
		p.Proposal.Items = len(kept)
		p.Proposal.TokenEstimate = estimate
		p.Proposal.EvidenceKeys = uniqueStrings(append([]string(nil), p.Proposal.EvidenceKeys...))
		out = append(out, p)
		metas = append(metas, p.Proposal)
	}
	return out, metas
}
