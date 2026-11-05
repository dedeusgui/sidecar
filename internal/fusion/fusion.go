// Package fusion combines heterogeneous rankings (lexical, structural, git) into one
// order using Reciprocal Rank Fusion, then diversifies it so one file cannot dominate.
package fusion

import "sort"

// Scored is an item with a fused score and the file it belongs to.
type Scored struct {
	Key   string
	File  string
	Score float64
}

// RRF returns sum(1/(k+rank)) per key over the given ranked lists. Ranks start at 1 and
// lists are ordered best first. Duplicate keys within one list count once, at their best rank.
func RRF(lists [][]string, k float64) map[string]float64 {
	scores := map[string]float64{}
	for _, list := range lists {
		seen := map[string]bool{}
		rank := 0
		for _, key := range list {
			if seen[key] {
				continue
			}
			seen[key] = true
			rank++
			scores[key] += 1.0 / (k + float64(rank))
		}
	}
	return scores
}

// Order returns keys sorted by descending score; ties are broken by key so results are reproducible.
func Order(scores map[string]float64) []string {
	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if scores[keys[i]] != scores[keys[j]] {
			return scores[keys[i]] > scores[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// Diversify keeps at most maxPerFile items per file and at most limit items overall.
// items must already be sorted best first; their relative order is preserved.
func Diversify(items []Scored, maxPerFile, limit int) []Scored {
	perFile := map[string]int{}
	var out []Scored
	for _, it := range items {
		if len(out) >= limit {
			break
		}
		if perFile[it.File] >= maxPerFile {
			continue
		}
		perFile[it.File]++
		out = append(out, it)
	}
	return out
}
