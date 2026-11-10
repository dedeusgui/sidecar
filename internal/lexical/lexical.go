// Package lexical normalizes natural language and identifiers so that Portuguese
// prompts can match English identifiers without any model.
package lexical

import (
	"strings"
	"unicode"
)

// Fold lowercases s and removes Latin diacritics (ã→a, ç→c, ...).
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if base, ok := diacriticBase[r]; ok {
			b.WriteRune(base)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var diacriticBase = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y', 'ÿ': 'y',
}

// SplitIdentifier splits identifiers and paths into lowercase words.
// Handles camelCase, PascalCase, acronyms, snake_case, kebab-case and paths.
// Digits stay attached to the segment they follow (parseV2Config → parse, v2, config).
func SplitIdentifier(s string) []string {
	var words []string
	for _, piece := range splitOn(s, isSeparator) {
		words = append(words, splitCamel(piece)...)
	}
	for i := range words {
		words[i] = Fold(words[i])
	}
	if len(words) == 0 {
		return nil
	}
	return words
}

func isSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func splitOn(s string, sep func(rune) bool) []string {
	return strings.FieldsFunc(s, sep)
}

func splitCamel(s string) []string {
	runes := []rune(s)
	var out []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		boundary := false
		switch {
		case unicode.IsLower(prev) && unicode.IsUpper(cur):
			boundary = true
		case unicode.IsDigit(prev) && unicode.IsUpper(cur):
			boundary = true
		case unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
			boundary = true
		}
		if boundary {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

// Stem applies conservative plural stripping for Portuguese and English.
// Input is expected to be folded. Words are never stripped below three letters.
func Stem(w string) string {
	if len(w) <= 3 {
		return w
	}
	switch {
	case strings.HasSuffix(w, "mente") && len(w) > 8:
		return w[:len(w)-5] // manualmente → manual
	case strings.HasSuffix(w, "sses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "oes") && len(w) > 4:
		return w[:len(w)-3] + "ao"
	case strings.HasSuffix(w, "ens") && len(w) > 4:
		return w[:len(w)-3] + "em" // pesagens → pesagem, itens → item
	case strings.HasSuffix(w, "uses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") &&
		!strings.HasSuffix(w, "us") && !strings.HasSuffix(w, "is"):
		return w[:len(w)-1]
	}
	return w
}

// Analyze returns the index terms for free text: folded, split, stemmed,
// stopword-free and de-duplicated, in first-occurrence order.
func Analyze(text string) []string {
	var out []string
	seen := map[string]bool{}
	if !hasVoiceTranscription(text) {
		for _, word := range splitOn(text, isSeparator) {
			for _, part := range splitCamel(word) {
				folded := Fold(part)
				if len(folded) < 2 || stopwords[folded] {
					continue
				}
				stemmed := Stem(folded)
				if !seen[stemmed] {
					seen[stemmed] = true
					out = append(out, stemmed)
				}
			}
		}
		return out
	}
	var words []string
	for _, word := range splitOn(text, isSeparator) {
		words = append(words, splitCamel(word)...)
	}
	for _, part := range normalizeVoiceTranscriptions(words) {
		folded := Fold(part)
		if len(folded) < 2 || stopwords[folded] {
			continue
		}
		stemmed := Stem(folded)
		if !seen[stemmed] {
			seen[stemmed] = true
			out = append(out, stemmed)
		}
	}
	return out
}

// normalizeVoiceTranscriptions repairs only a few high-confidence multiword ASR confusions.
// Phrase-level matching keeps common words such as "cold", "byte" and "ripple" unchanged alone.
func normalizeVoiceTranscriptions(words []string) []string {
	out := words[:0]
	for i := 0; i < len(words); {
		if i+2 < len(words) && Fold(words[i]) == "byte" && Fold(words[i+1]) == "en" && Fold(words[i+2]) == "giin" {
			out = append(out, "engine")
			i += 3
			continue
		}
		if i+1 < len(words) && Fold(words[i]) == "cold" && Fold(words[i+1]) == "graph" {
			out = append(out, "code", "graph")
			i += 2
			continue
		}
		if i+1 < len(words) && Fold(words[i]) == "ader" && Fold(words[i+1]) == "ripple" {
			out = append(out, "aider")
			i += 2
			continue
		}
		out = append(out, words[i])
		i++
	}
	return out
}

func hasVoiceTranscription(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "byte en giin") || strings.Contains(text, "cold graph") || strings.Contains(text, "ader ripple")
}

var stopwords = toSet(
	// Portuguese
	"a", "o", "as", "os", "um", "uma", "uns", "umas", "de", "do", "da", "dos", "das",
	"em", "no", "na", "nos", "nas", "e", "ou", "que", "para", "pra", "por", "com", "sem",
	"se", "ao", "aos", "pelo", "pela", "pelos", "pelas", "num", "numa",
	"como", "mais", "menos", "isso", "isto", "este", "esta", "estao", "esse", "essa",
	"eu", "voce", "nao", "sim", "ser", "ter", "foi", "ja", "qual", "quais", "quando", "onde", "cade",
	"fica", "ficam", "sobre", "entre", "muito", "pode", "preciso", "quero", "deve", "ate", "somente",
	"apenas", "tambem", "ainda", "aqui", "ali", "la",
	// Spoken PT-BR filler from voice transcripts. Matched before stemming, so listed as spoken.
	"cara", "tipo", "assim", "entao", "acho", "acredito", "queria", "entender", "fazer", "coisa",
	"coisas", "alguma", "algum", "gente", "vamos", "olhando", "falando", "basicamente", "realmente",
	"interessante", "melhor", "questao", "entende", "sabe", "beleza", "certo", "bom", "dia", "tarde",
	"noite", "obrigado", "favor",
	// English
	"the", "an", "and", "or", "of", "to", "in", "on", "for", "with", "is", "are", "be",
	"it", "this", "that", "by", "at", "from", "we", "i", "you", "can", "should", "would",
	"how", "what", "when", "where", "why", "do", "does", "not", "so", "if", "then",
)

func toSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
