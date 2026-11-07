package lexical

import (
	"slices"
	"testing"
)

func TestCognateTransducerDerivesUnlistedDomainTerms(t *testing.T) {
	// None of these words are listed in lexicon_pt_en.tsv: they must be resolved
	// purely by algorithmic cognate transduction + code abbreviation rules in cognates.go.
	cases := []struct {
		input string
		want  string
	}{
		{"veterinário", "vet"},
		{"autenticação", "auth"},
		{"validador", "validate"},
		{"validador", "validator"},
		{"validation", "validate"},
		{"compressão", "compress"},
		{"especialidades", "specialty"},
		{"formulário", "form"},
		{"duplicado", "duplicate"},
		{"configuração", "config"},
		{"repositório", "repo"},
		{"diretório", "dir"},
		{"sincronização", "sync"},
		{"inicialização", "init"},
		{"transação", "tx"},
		{"serialização", "serialize"},
		{"paginação", "paginate"},
		{"controlador", "controller"},
		{"adaptador", "adapter"},
		{"processador", "processor"},
		{"formatador", "formatter"},
		{"observador", "observer"},
		{"indexador", "indexer"},
		{"exceção", "exception"},
		{"conexão", "connect"},
		{"permissão", "permission"},
		{"conversão", "convert"},
		{"Esquelite", "sqlite"},
		{"dinâmico", "dynamic"},
		{"assíncrona", "async"},
		{"assíncrona", "asynchronous"},
		{"serializa", "serialize"},
		{"deserializa", "deserialize"},
		{"configura", "config"},
		{"executa", "execute"},
		{"valida", "validate"},
		{"verifica", "verify"},
	}

	for _, tc := range cases {
		got := Translations(tc.input)
		wantStem := Stem(Fold(tc.want))
		if !slices.Contains(got, wantStem) && !slices.Contains(got, tc.want) {
			t.Errorf("Translations(%q) = %v; want %q (stem %q)", tc.input, got, tc.want, wantStem)
		}
	}
}
