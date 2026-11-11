package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore/service"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// v36EvidenceItem renders one registry entry with a single trust label and a single location:
// the registry qualifier (VERIFIED only with a proof) and the source range, both in the suffix
// the packer keeps when it truncates. V3.5 printed the retrieval verdict and the location inside
// the text as well, which could read "[VERIFIED] ... [CANDIDATE]" on one line.
func v36EvidenceItem(entry contract.EvidenceEntry[retrieve.Evidence]) ctxbuild.Item {
	value := entry.Value
	return ctxbuild.Item{Section: value.Section, Key: value.Key(), Fingerprint: value.Fingerprint(),
		Text: retrieve.DescribeBare(value), Source: value.Source, Role: value.Hit.Kind,
		Qualifier: evidenceQualifier(entry.Ref), Provenance: evidenceProvenance(entry.Ref)}
}

// v36Choices keeps the ranked selection whole, minus entries a source snippet already shows or
// the session already received: planning them would spend budget on items the packer skips.
func v36Choices(choices []engine.Choice, registry *contract.EvidenceRegistry[retrieve.Evidence], snippetKeys map[string]string, seen ctxbuild.Seen) []engine.Choice {
	skip := map[string]bool{}
	for _, entry := range registry.Snapshot() {
		key := entry.Value.Key()
		if _, hasSnippet := snippetKeys[key]; hasSnippet || seen != nil && seen(key, entry.Value.Fingerprint()) {
			skip[entry.Value.ID] = true
		}
	}
	out := make([]engine.Choice, 0, len(choices))
	for _, choice := range choices {
		if !skip[choice.ID] {
			out = append(out, choice)
		}
	}
	return out
}

// v36Plan only suppresses the fallback for snippet proposals the planner actually allocated.
// A rejected proposal cannot stand in for evidence, even when it was generated successfully.
func v36Plan(input engine.PlanInput, choices []engine.Choice, proposals []ctxbuild.GuidanceProposal, seen ctxbuild.Seen) (contract.OutputPlan, map[string]string) {
	proposed := snippetKeysAndFingerprints(proposals)
	input.Decision.Included = v36Choices(choices, input.Evidence, proposed, seen)
	first := (engine.Planner{}).Plan(input)
	allocated := make(map[string]string)
	for _, proposal := range proposals {
		for _, section := range first.Sections {
			if section.Section != proposal.Proposal.Kind || len(section.EvidenceKeys) != len(proposal.Proposal.EvidenceKeys) {
				continue
			}
			match := true
			for _, key := range proposal.Proposal.EvidenceKeys {
				found := false
				for _, allocatedKey := range section.EvidenceKeys {
					if key == allocatedKey {
						found = true
						break
					}
				}
				if !found {
					match = false
					break
				}
			}
			if match {
				for _, item := range proposal.Items {
					if item.Source == "explore" {
						allocated[item.Key] = item.Fingerprint
					}
				}
				break
			}
		}
	}
	input.Decision.Included = v36Choices(choices, input.Evidence, allocated, seen)
	return (engine.Planner{}).Plan(input), allocated
}

// minSnippetLines is the least non-blank source lines worth a snippet; a lone heading or brace
// repeats what the evidence line already says.
const minSnippetLines = 2

// v36ExpansionItems turns exploration snippets into multi-line source items in exploration order.
// Unlike the V3.5 form (numbered lines collapsed into one 300-rune line), every source line is
// rendered, a snippet contained in another one of the same file is dropped, and trailing snippets
// are cut until the whole proposal fits maxTokens, so one oversized snippet cannot void the rest.
func v36ExpansionItems(expansion service.ExpansionResult, registry *contract.EvidenceRegistry[retrieve.Evidence], maxTokens int) ([]ctxbuild.Item, []string) {
	entries := registry.Snapshot()
	type span struct {
		path       string
		start, end int
	}
	var spans []span
	for _, file := range expansion.Explore.Files {
		if file.Stale {
			continue
		}
		for _, snippet := range file.Snippets {
			spans = append(spans, span{file.Path, snippet.Start, snippet.End})
		}
	}
	var items []ctxbuild.Item
	used := map[string]bool{}
	tokens := 0
	for _, file := range expansion.Explore.Files {
		if file.Stale {
			continue
		}
		for _, snippet := range file.Snippets {
			contained := false
			for _, other := range spans {
				if other.path == file.Path && other.start <= snippet.Start && snippet.End <= other.end &&
					other.end-other.start > snippet.End-snippet.Start {
					contained = true
					break
				}
			}
			var body []string
			for _, line := range strings.Split(snippet.Text, "\n") {
				if _, source, _ := strings.Cut(line, "\t"); strings.TrimSpace(source) != "" {
					body = append(body, line)
				}
			}
			if contained || len(body) < minSnippetLines {
				continue
			}
			var match *contract.EvidenceEntry[retrieve.Evidence]
			for i := range entries {
				entry := &entries[i]
				if entry.Ref.Path != file.Path || entry.Ref.Freshness != contract.FreshnessCurrent || used[entry.Ref.Key] ||
					entry.Ref.LineStart > 0 && (entry.Ref.LineStart > snippet.End || entry.Ref.LineEnd > 0 && entry.Ref.LineEnd < snippet.Start) {
					continue
				}
				// Prefer the entry the snippet starts closest to: it names what the lines show.
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
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return items, keys
}
