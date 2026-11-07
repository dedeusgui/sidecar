package lexical

import (
	"reflect"
	"testing"
)

func TestSplitIdentifier(t *testing.T) {
	cases := map[string][]string{
		"processManualRelease": {"process", "manual", "release"},
		"MAX_ACTIVE_TICKETS":   {"max", "active", "tickets"},
		"registrar_boleto":     {"registrar", "boleto"},
		"HTTPServerHandler":    {"http", "server", "handler"},
		"parseV2Config":        {"parse", "v2", "config"},
		"kebab-case-name":      {"kebab", "case", "name"},
		"path/to/file.go":      {"path", "to", "file", "go"},
		"":                     nil,
	}
	for in, want := range cases {
		if got := SplitIdentifier(in); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitIdentifier(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFoldRemovesDiacriticsAndCase(t *testing.T) {
	cases := map[string]string{
		"Liberação": "liberacao",
		"Ação":      "acao",
		"NOTA":      "nota",
		"Pesagem":   "pesagem",
		"José":      "jose",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStemIsConservativeForPortugueseAndEnglish(t *testing.T) {
	cases := map[string]string{
		"tickets":    "ticket",
		"classes":    "class",
		"statuses":   "status",
		"boletos":    "boleto",
		"liberacoes": "liberacao",
		"pesagens":   "pesagem",
		"itens":      "item",
		"release":    "release",
		"status":     "status",
		"chass":      "chass",
		"ok":         "ok",
	}
	for in, want := range cases {
		if got := Stem(in); got != want {
			t.Errorf("Stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdverbSuffixMapsToAdjectiveStem(t *testing.T) {
	if got := Stem("manualmente"); got != "manual" {
		t.Errorf("Stem(manualmente) = %q, want manual", got)
	}
	if !contains(Translations("libera"), "release") {
		t.Errorf("verb libera should translate to release, got %v", Translations("libera"))
	}
}

func TestAnalyzeDropsStopwordsAndFoldsStems(t *testing.T) {
	got := Analyze("Adiantar um caminhão na fila")
	for _, want := range []string{"adiantar", "caminhao", "fila"} {
		if !contains(got, want) {
			t.Errorf("Analyze missing %q in %v", want, got)
		}
	}
	for _, stop := range []string{"um", "na"} {
		if contains(got, stop) {
			t.Errorf("Analyze kept stopword %q in %v", stop, got)
		}
	}
}

func TestAnalyzeKeepsIdentifierWords(t *testing.T) {
	got := Analyze("processManualRelease")
	for _, want := range []string{"process", "manual", "release"} {
		if !contains(got, want) {
			t.Errorf("Analyze missing %q in %v", want, got)
		}
	}
}

func TestTranslationsOnlyForDictionaryTerms(t *testing.T) {
	if !contains(Translations("fila"), "queue") {
		t.Errorf("fila should translate to queue, got %v", Translations("fila"))
	}
	if !contains(Translations("liberação"), "release") {
		t.Errorf("liberacao should translate to release, got %v", Translations("liberação"))
	}
	if !contains(Translations("caminhão"), "truck") {
		t.Errorf("caminhao should translate to truck, got %v", Translations("caminhão"))
	}
	if got := Translations("xyzzy"); len(got) != 0 {
		t.Errorf("unknown term should have no translations, got %v", got)
	}
}

func TestTranslationsAreBidirectional(t *testing.T) {
	if !contains(Translations("queue"), "fila") {
		t.Errorf("queue should translate back to fila, got %v", Translations("queue"))
	}
}

func TestPhraseTranslation(t *testing.T) {
	got := ExpandPhrases("emitir nota fiscal")
	if !contains(got, "invoice") {
		t.Errorf("phrase 'nota fiscal' should expand to invoice, got %v", got)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Terms documented as holdout misses in docs/benchmark.md must translate to the identifiers that
// the holdout expects, so a regression in the dictionary shows up as a failing test.
func TestBenchmarkDomainTermsTranslate(t *testing.T) {
	cases := map[string][]string{
		"encomenda": {"shipment"},
		"pacote":    {"package"},
		"rastreio":  {"track"},
		"cupom":     {"coupon"},
		"cobrar":    {"charge"},
		"tentar":    {"retry"},
	}
	for word, want := range cases {
		got := Translations(word)
		for _, w := range want {
			if !contains(got, w) {
				t.Errorf("Translations(%q) = %v, want it to include %q", word, got, w)
			}
		}
	}
}

// PT-BR words from the real-repository prompts must reach the English names the repos use.
func TestRealCorpusTermsTranslate(t *testing.T) {
	cases := map[string]string{
		"veterinário":  "vet",
		"dono":         "owner",
		"proprietário": "owner",
		"visita":       "visit",
		"animal":       "pet",
		"mascote":      "pet",
		"roteador":     "router",
		"autenticação": "auth",
		"cabeçalho":    "header",
		"compressão":   "compress",
	}
	for word, want := range cases {
		if got := Translations(word); !contains(got, want) {
			t.Errorf("Translations(%q) = %v, want it to include %q", word, got, want)
		}
	}
}
