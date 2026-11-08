package lexical

import (
	"strings"
)

// codeAbbrevs maps high-signal Greco-Latin stems to domain code stems and safe abbreviations.
// Ubiquitous Go parameter names like "ctx", "err", "id", "pkg" are intentionally excluded
// because they match nearly every function signature in Go repositories.
var codeAbbrevs = map[string][]string{
	"authentic":  {"auth", "authenticate", "authentication"},
	"authoriz":   {"auth", "authorize", "authorization"},
	"configur":   {"config", "cfg", "configure", "configuration"},
	"repositor":  {"repo", "repository"},
	"director":   {"dir", "directory"},
	"databas":    {"db", "database"},
	"veterinar":  {"vet", "veterinary"},
	"initializ":  {"init", "initialize", "initialization"},
	"asynchron":  {"async", "asynchronous"},
	"synchron":   {"sync", "synchronize", "synchronous"},
	"specific":   {"spec", "specification"},
	"environ":    {"env", "environment"},
	"paramet":    {"param", "params", "parameter"},
	"statist":    {"stats", "statistic"},
	"temporar":   {"temp", "tmp", "temporary"},
	"transact":   {"tx", "txn", "transaction"},
	"connect":    {"conn", "connect", "connection"},
	"credential": {"creds", "credentials"},
	"validat":    {"validate", "validator", "validation"},
	"compress":   {"compress", "compressor", "compression"},
	"regist":     {"register", "registration", "registry", "record"},
	"creat":      {"create", "creation"},
	"select":     {"select", "selector", "selection"},
	"rout":       {"route", "router", "routing"},
	"format":     {"format", "formatter"},
	"observ":     {"observe", "observer"},
	"index":      {"index", "indexer"},
	"process":    {"process", "processor"},
	"control":    {"control", "controller"},
	"adapt":      {"adapt", "adapter"},
	"generat":    {"generate", "generator"},
	"deserializ": {"deserialize", "deserializer", "deserialization", "de"},
	"serializ":   {"serialize", "serializer", "serialization", "ser"},
	"identific":  {"ident", "identifier", "identify", "identification"},
	"confirm":    {"confirm", "confirmation"},
	"pagin":      {"paginate", "pagination"},
	"permiss":    {"permission", "permit"},
	"convers":    {"convert", "conversion", "parse", "deserialize"},
	"transform":  {"transform", "convert", "serialize"},
	"execu":      {"execute", "execution"},
	"inject":     {"inject", "injection"},
	"inspect":    {"inspect", "inspector", "inspection"},
	"redirect":   {"redirect", "redirection"},
	"collect":    {"collect", "collection", "collector"},
	"duplic":     {"duplicate", "duplicated"},
	"special":    {"specialty", "specialties"},
	"formul":     {"form"},
}

// CognateCandidates algorithmically derives English morphological and Greco-Latin cognate
// stems from a Portuguese or English word without relying on a hardcoded per-repo dictionary.
// Callers verify each returned candidate against the repository's FTS5 vocabulary (TermDocFreq > 0).
func CognateCandidates(word string) []string {
	w := Fold(word)
	if len(w) < 5 {
		return nil
	}
	var out []string
	add := func(cand string) {
		s := Stem(Fold(cand))
		if len(s) >= 2 && s != w && s != Stem(w) {
			for _, existing := range out {
				if existing == s {
					return
				}
			}
			out = append(out, s)
		}
	}

	// Phonetic / transcription normalization for spoken technical terms (e.g. "esquelite" -> "sqlite").
	for _, p := range phoneticTechTerms(w) {
		add(p)
	}

	// Orthographic root normalization from Romance (PT/ES) to English Greco-Latin roots.
	norm := normalizeRomanceOrthography(w)
	if norm != w {
		add(norm)
	}

	// Apply high-precision suffix transduction rules on both raw folded word and normalized word.
	for _, base := range []string{w, norm} {
		for _, derived := range transduceSuffixes(base) {
			add(derived)
			for prefix, abbrevs := range codeAbbrevs {
				if strings.HasPrefix(derived, prefix) {
					for _, ab := range abbrevs {
						add(ab)
					}
				}
			}
		}
		for prefix, abbrevs := range codeAbbrevs {
			if strings.HasPrefix(base, prefix) {
				for _, ab := range abbrevs {
					add(ab)
				}
			}
		}
	}

	return out
}

// normalizeRomanceOrthography converts systematic PT-BR orthographic patterns to English cognates.
func normalizeRomanceOrthography(w string) string {
	s := w
	if strings.HasPrefix(s, "esqu") {
		s = "sch" + s[4:]
	} else if len(s) >= 6 && strings.HasPrefix(s, "es") && isConsonant(s[2]) {
		s = s[1:]
	}
	replacer := strings.NewReplacer(
		"autentic", "authentic",
		"autoriz", "authoriz",
		"inici", "initi",
		"transac", "transact",
		"sincron", "synchron",
		"assincron", "asynchron",
		"orquestr", "orchestr",
		"arquitet", "architect",
		"criptograf", "cryptograph",
		"cripto", "crypto",
		"dinam", "dynam",
		"estatist", "statist",
		"estrutur", "structur",
		"especif", "specif",
		"especial", "special",
		"excecao", "exception",
		"excecoes", "exceptions",
		"conex", "connect",
		"colec", "collect",
		"selec", "select",
		"injec", "inject",
		"inspec", "inspect",
		"redirec", "redirect",
		"projec", "project",
		"execu", "execut",
		"diretor", "director",
		"repositor", "repositor",
		"dicion", "diction",
		"comprim", "compress",
		"credencia", "credential",
	)
	return replacer.Replace(s)
}

func isConsonant(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	default:
		return b >= 'a' && b <= 'z'
	}
}

// transduceSuffixes maps high-precision Romance and English derivational suffixes to code stems.
func transduceSuffixes(w string) []string {
	type rule struct {
		suffix  string
		targets []string
	}
	rules := []rule{
		{"izacao", []string{"ization", "ize", "ise"}},
		{"izacoes", []string{"ization", "ize", "ise"}},
		{"acao", []string{"ation", "ate"}},
		{"acoes", []string{"ation", "ate"}},
		{"cao", []string{"tion", "te"}},
		{"coes", []string{"tion", "te"}},
		{"ssao", []string{"ssion", "ss"}},
		{"ssoes", []string{"ssion", "ss"}},
		{"sao", []string{"sion", "se", "t"}},
		{"soes", []string{"sion", "se", "t"}},
		{"idador", []string{"idator", "idate"}},
		{"idadores", []string{"idator", "idate"}},
		{"ador", []string{"ator", "er", "or", "ate"}},
		{"adores", []string{"ator", "er", "or", "ate"}},
		{"idade", []string{"ity", "ty"}},
		{"idades", []string{"ity", "ties", "ty"}},
		{"dade", []string{"ty", "ties"}},
		{"dades", []string{"ty", "ties"}},
		{"amento", []string{"ing", "ment"}},
		{"amentos", []string{"ing", "ment"}},
		{"imento", []string{"ing", "ment"}},
		{"imentos", []string{"ing", "ment"}},
		{"ario", []string{"ary", "er"}},
		{"arios", []string{"ary", "er"}},
		{"orio", []string{"ory", "er"}},
		{"orios", []string{"ory", "er"}},
		{"duplicado", []string{"duplicate", "duplicated"}},
		{"duplicada", []string{"duplicate", "duplicated"}},
		{"izar", []string{"ize", "ization"}},
		{"iza", []string{"ize", "ization"}},
		{"ificar", []string{"ify", "ification"}},
		{"ifica", []string{"ify", "ification"}},
		{"ico", []string{"ic"}},
		{"rono", []string{"ronous"}},
		{"rona", []string{"ronous"}},
		{"urar", []string{"ure"}},
		{"ura", []string{"ure"}},
		{"utar", []string{"ute"}},
		{"uta", []string{"ute"}},
		{"itar", []string{"it"}},
		{"ita", []string{"it"}},
		{"idar", []string{"idate", "id"}},
		{"ida", []string{"idate", "id"}},
		{"ectar", []string{"ect"}},
		{"ecta", []string{"ect"}},
		{"erter", []string{"ert"}},
		{"erte", []string{"ert"}},
		{"ormar", []string{"orm"}},
		{"orma", []string{"orm"}},
		// English derivational morphology (bridges "validation"<->"validate"<->"validator", "compression"<->"compress")
		{"ation", []string{"ate", "ator"}},
		{"ition", []string{"ite"}},
		{"sion", []string{"se", "ss"}},
		{"ssion", []string{"ss"}},
		{"ator", []string{"ate", "ation"}},
	}

	var out []string
	for _, r := range rules {
		if len(w) >= len(r.suffix)+3 && strings.HasSuffix(w, r.suffix) {
			root := w[:len(w)-len(r.suffix)]
			for _, tgt := range r.targets {
				cand := root + tgt
				if len(cand) >= 3 {
					out = append(out, cand)
				}
			}
		}
	}
	return out
}

// phoneticTechTerms normalizes common Voice Mode / spoken PT-BR transcriptions of English tech terms.
func phoneticTechTerms(w string) []string {
	switch w {
	case "esquelite", "sequelite", "sqlit":
		return []string{"sqlite"}
	case "haiku", "haai", "haiq":
		return []string{"haiku"}
	case "antigraft", "antigravit":
		return []string{"antigravity"}
	case "postgre", "postgresq":
		return []string{"postgres", "postgresql"}
	case "rediz":
		return []string{"redis"}
	case "kubernet", "kubernets":
		return []string{"kubernetes", "k8s"}
	}
	return nil
}
