package lexical

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed lexicon_pt_en.tsv
var embeddedLexiconTSV string

// phraseToEn maps multi-word PT expressions (stemmed, folded words) to EN terms.
var phraseToEn = map[string][]string{
	"nota fiscal": {"invoice"},
	"de novo":     {"retry"},
}

// translation tables built at init from embeddedLexiconTSV and optional user lexicon.
var (
	forward = map[string][]string{}
	reverse = map[string][]string{}
	phrases = map[string][]string{}
)

func init() {
	loadTSVLexicon(embeddedLexiconTSV)
	for phrase, ens := range phraseToEn {
		parts := strings.Fields(phrase)
		for i := range parts {
			parts[i] = Stem(Fold(parts[i]))
		}
		key := strings.Join(parts, " ")
		phrases[key] = append(phrases[key], ens...)
	}
	loadUserLexicon()
}

func loadTSVLexicon(tsv string) {
	for _, line := range strings.Split(tsv, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		src, targets, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		rawSrc := strings.TrimSpace(src)
		if strings.Contains(rawSrc, " ") {
			parts := strings.Fields(rawSrc)
			for i := range parts {
				parts[i] = Stem(Fold(parts[i]))
			}
			pkey := strings.Join(parts, " ")
			for _, tgt := range strings.Split(targets, ",") {
				if en := Stem(Fold(strings.TrimSpace(tgt))); en != "" {
					phrases[pkey] = append(phrases[pkey], en)
				}
			}
			continue
		}
		key := Stem(Fold(rawSrc))
		if key == "" {
			continue
		}
		for _, tgt := range strings.Split(targets, ",") {
			en := Stem(Fold(strings.TrimSpace(tgt)))
			if en == "" {
				continue
			}
			addUnique(forward, key, en)
			addUnique(reverse, en, key)
		}
	}
}

// loadUserLexicon merges an optional JSON map {"term": ["alt1", "alt2"]} from
// $SIDECAR_HOME/lexicon.json so users can extend domain terms without recompiling.
func loadUserLexicon() {
	home := os.Getenv("SIDECAR_HOME")
	if home == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(home, "lexicon.json"))
	if err != nil {
		return
	}
	var custom map[string][]string
	if json.Unmarshal(data, &custom) != nil {
		return
	}
	for src, ens := range custom {
		key := Stem(Fold(src))
		for _, en := range ens {
			enStem := Stem(Fold(en))
			addUnique(forward, key, enStem)
			addUnique(reverse, enStem, key)
		}
	}
}

func addUnique(m map[string][]string, key, value string) {
	for _, v := range m[key] {
		if v == value {
			return
		}
	}
	m[key] = append(m[key], value)
}

// Translations returns candidate cross-lingual and morphological stems for one word.
// It combines the embedded TSV lexicon, optional user overrides ($SIDECAR_HOME/lexicon.json),
// and algorithmic Greco-Latin cognate & derivational morphology transduction (CognateCandidates).
// Callers verify each returned candidate against the repository's FTS5 vocabulary (TermDocFreq > 0).
func Translations(word string) []string {
	key := Stem(Fold(word))
	var out []string
	out = append(out, forward[key]...)
	for _, v := range reverse[key] {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	for _, cog := range CognateCandidates(word) {
		if !slices.Contains(out, cog) {
			out = append(out, cog)
		}
	}
	return out
}

// ExpandPhrases returns translations for multi-word expressions found in text.
func ExpandPhrases(text string) []string {
	words := strings.FieldsFunc(Fold(text), isSeparator)
	for i := range words {
		words[i] = Stem(words[i])
	}
	var out []string
	for i := 0; i+1 < len(words); i++ {
		key := words[i] + " " + words[i+1]
		for _, en := range phrases[key] {
			if !slices.Contains(out, en) {
				out = append(out, en)
			}
		}
	}
	return out
}
