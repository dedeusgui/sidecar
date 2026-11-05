package lexical

import (
	"slices"
	"strings"
)

// ptToEn is an offline, contextual-but-conservative PT-BR → EN vocabulary.
// Each entry lists candidate translations; callers must prefer candidates that
// really occur in the repository (see retrieve). Keys are written in plain form
// and normalized with Fold+Stem at load time.
var ptToEn = map[string][]string{
	"fila":        {"queue"},
	"liberacao":   {"release"},
	"caminhao":    {"truck", "vehicle"},
	"veiculo":     {"vehicle"},
	"pesagem":     {"weighing", "weigh"},
	"fornecedor":  {"supplier", "vendor"},
	"cobranca":    {"billing", "charge"},
	"baixa":       {"settlement", "payment"},
	"boleto":      {"slip", "bankslip"},
	"adiantar":    {"prioritize", "expedite", "advance"},
	"automatica":  {"automatic", "auto"},
	"automatico":  {"automatic", "auto"},
	"manual":      {"manual"},
	"usuario":     {"user"},
	"pedido":      {"order"},
	"entrega":     {"delivery"},
	"estoque":     {"stock", "inventory"},
	"cadastro":    {"registration", "register"},
	"cliente":     {"customer", "client"},
	"pagamento":   {"payment"},
	"recibo":      {"receipt"},
	"imposto":     {"tax"},
	"relatorio":   {"report"},
	"senha":       {"password"},
	"chave":       {"key"},
	"erro":        {"error"},
	"falha":       {"failure", "fail"},
	"consulta":    {"query", "lookup"},
	"agendamento": {"schedule", "scheduling"},
	"prazo":       {"deadline"},
	"notificacao": {"notification"},
	"tarefa":      {"task", "job"},
	"cancelar":    {"cancel"},
	"ticket":      {"ticket"},
	"limite":      {"limit", "max"},
	"ativo":       {"active"},
	"ativa":       {"active"},
}

// phraseToEn maps multi-word PT expressions (stemmed, folded words) to EN terms.
var phraseToEn = map[string][]string{
	"nota fiscal": {"invoice"},
}

// translation tables built once at init: PT→EN (forward) and EN→PT (reverse).
var (
	forward = map[string][]string{}
	reverse = map[string][]string{}
	phrases = map[string][]string{}
)

func init() {
	for pt, ens := range ptToEn {
		key := Stem(Fold(pt))
		for _, en := range ens {
			addUnique(forward, key, Stem(Fold(en)))
			addUnique(reverse, Stem(Fold(en)), key)
		}
	}
	for phrase, ens := range phraseToEn {
		parts := strings.Fields(phrase)
		for i := range parts {
			parts[i] = Stem(Fold(parts[i]))
		}
		key := strings.Join(parts, " ")
		phrases[key] = append(phrases[key], ens...)
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

// Translations returns candidate English terms for one word, in both directions.
// It returns nil for words without dictionary entries.
func Translations(word string) []string {
	key := Stem(Fold(word))
	var out []string
	out = append(out, forward[key]...)
	for _, v := range reverse[key] {
		if !slices.Contains(out, v) {
			out = append(out, v)
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
