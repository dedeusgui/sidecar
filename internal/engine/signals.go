// Package engine is the opt-in Sidecar V3 decision layer (SIDECAR_ENGINE=v3). It reuses the V2
// retrieval pipeline (internal/retrieve) and the existing index; it adds no indexer, model, network
// call or embedding. It reads the prompt and the retrieved evidence, decides whether repository
// context is warranted at all (RelevanceGate) and, when it is, the smallest useful effort
// (EffortRouter): a lean answer, a task brief, an available on-demand exploration, a clarification
// or a verification hint. Every decision is deterministic and explained in Decision.
package engine

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// TaskClass is the effort class of a prompt.
type TaskClass string

const (
	ClassOffRepo        TaskClass = "off_repo"        // conversation, general knowledge or another subject
	ClassSimpleQuery    TaskClass = "simple_query"    // a question about one thing the repository names
	ClassLocate         TaskClass = "locate"          // where is X
	ClassDebug          TaskClass = "debug"           // an error, failure or stack trace
	ClassContractChange TaskClass = "contract_change" // an edit that may change a contract others rely on
	ClassMultiFile      TaskClass = "multi_file"      // an implementation that spans files or extends a pattern
	ClassEdit           TaskClass = "edit"            // a narrow edit with a clear target
)

// Locator is a concrete anchor written in the prompt.
type Locator struct {
	Kind     string `json:"kind"` // path | identifier | stack_frame | quoted_code
	Text     string `json:"text"`
	Verified bool   `json:"verified"` // the repository index resolves it
}

// TaskSignals is the deterministic reading of a prompt plus the retrieved evidence. It never stores
// the prompt: Locator texts are code anchors the user wrote, kept only in memory for explain output.
type TaskSignals struct {
	Intent         retrieve.Intent `json:"intent"`
	Edit           bool            `json:"edit"`           // asks to change something (negations and quotes removed)
	Conversational bool            `json:"conversational"` // greeting, thanks, emoji, meta request
	SoftwareTerms  []string        `json:"software_terms"` // software vocabulary present in the prompt
	RepoCue        bool            `json:"repo_cue"`       // the prompt points at this code ("where", "which handler", "this project")
	Locators       []Locator       `json:"locators,omitempty"`
	PromptTerms    int             `json:"prompt_terms"`   // analysed content terms
	Concepts       int             `json:"concepts"`       // terms present in the repository (directly or translated)
	PresentRatio   float64         `json:"present_ratio"`  // Concepts / PromptTerms
	BestSupport    int             `json:"best_support"`   // distinct concepts matched by the best evidence item
	TopSupport     int             `json:"top_support"`    // distinct concepts matched by the top 3 items together
	Coverage       float64         `json:"coverage"`       // TopSupport / Concepts
	TopConfidence  float64         `json:"top_confidence"` // confidence of the best item
	Verified       int             `json:"verified"`       // VERIFIED items
	Files          int             `json:"files"`          // distinct files among the evidence
	ContractRisk   bool            `json:"contract_risk"`  // edit touching a contract-shaped subject
	LongPrompt     bool            `json:"long_prompt"`
	Reasons        []string        `json:"reasons,omitempty"`
}

// softwareLexicon lists words that, in a prompt, point at software rather than the physical world.
// Folded (lowercase, no diacritics). Real-world homonyms that are common outside software ("settings",
// "export", "password", "format", "list", "store", "open") are deliberately absent: they are not
// evidence of a code question by themselves.
var softwareLexicon = buildLexicon(`
code codebase codigo repo repository repositorio function functions func funcao funcoes method methods metodo metodos
class classes classe struct interface interfaces module modules modulo pacote package packages file files arquivo arquivos
variable variavel variaveis constant constante parameter parameters parametro parametros argument arguments argumento
argumentos return retorna retorno enum field fields campo campos property propriedade object objeto instance instancia
constructor construtor callback closure handler handlers middleware middlewares route routes rota rotas router roteamento
routing endpoint endpoints api apis request requests requisicao requisicoes response responses resposta respostas header
headers cabecalho cabecalhos cookie cookies session sessao query queries consulta consultas sql database banco schema
migration migracao model controller controlador view template render component componente test tests teste testes
testing spec specs assert mock fixture bug bugs error errors erro erros exception excecao panic crash stacktrace traceback
log logs logger logging debug depurar depurando compile compilar build deploy config configuration configuracao cli
command comando flag flags terminal server servidor http https json xml yaml parse parser parsing serialize serializar
deserialize desserializar encode decode regex cache token jwt auth authentication autenticacao authorization hash
thread threads async await goroutine concurrency concorrencia mutex queue fila worker implement implementation implementar
implementacao refactor refatorar commit branch merge diff patch library biblioteca framework dependency dependencia
import importa runtime stdout stderr timeout retry gzip compression compressao cors csrf websocket graphql orm validation
validacao validator validador null nil undefined boolean string strings integer array slice dict buffer stream reader
writer bytes utf encoding decorator plugin adapter listener hook hooks formatter formatador serializer deserializer
invariant invariante contrato contract signature assinatura stub typescript javascript python golang rust ruby php java
kotlin csharp lambda regexp lexer tokenizer ast compiler compilador interpreter script scripts pipeline cron daemon
`)

func buildLexicon(words string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(words) {
		out[w] = true
	}
	return out
}

// repoCue matches phrasing that points at the code being worked on rather than the world: a
// location question, a request about "this" project, or a named code element.
var repoCue = regexp.MustCompile(`\b(onde|where|cade|cadê|em que (arquivo|lugar|parte)|em qual (arquivo|lugar|parte)|which (file|function|class|method|module|route|handler|middleware|controller|type|component|part)|qual (arquivo|funcao|classe|metodo|rota|handler|middleware|controller|controlador|modulo|tipo|componente|parte do codigo)|esse projeto|este projeto|nesse projeto|neste projeto|this (project|repo|repository|codebase|code)|in the (code|repo|codebase)|no (codigo|projeto|repositorio)|nesse codigo|neste codigo|implementad[oa]|implemented|defined|definid[oa]|how does (it|this)|como (esse|este) projeto)\b`)

// conversational matches whole prompts that are social or meta, never a task on the repository.
var conversational = regexp.MustCompile(`^(oi|ola|hello|hi|hey|bom dia|boa tarde|boa noite|good (morning|afternoon|evening)|obrigad[oa]|valeu|thanks|thank you|thx|ok|okay|beleza|blz|show|massa|top|perfeito|great|nice|cool|kkk+|haha+|rs+|tudo (bem|certo|bom)|ready to start|vamos (la|comecar)|era so isso|that'?s all)\b`)

// metaRequest matches requests that ask the assistant for writing or conversation work unrelated to the
// repository. They are only conversational when no software term or locator is present.
var metaRequest = regexp.MustCompile(`^(resum[ea]|summari[sz]e|traduz\w*|translate|write a (poem|haiku|song|story|letter)|escreva um (poema|conto|texto|email)|me (conta|indica|recomenda)|recommend|tell me a joke|conte uma piada)\b`)

// preserveCue marks an edit that must keep something others rely on: compatibility, a signature, a format.
var preserveCue = regexp.MustCompile(`\b(keep|keeping|preserv\w*|without (changing|breaking)|backward\w*|compatib\w*|sem (alterar|mudar|quebrar)|mantend\w*|manter|mantenha|assinatura|signature|public api|api publica|breaking|contrat\w*|contract\w*|invarian\w*)\b`)

var (
	pathRe       = regexp.MustCompile(`(?:^|[\s"'(\x60])((?:[\w.\-]+/)+[\w.\-]+\.[A-Za-z]{1,5}|[\w\-]+\.(?:go|py|ts|tsx|js|jsx|rb|php|java|kt|cs|rs|swift|c|cc|cpp|h|hpp|scala|ex|exs|md|json|yaml|yml|toml))\b`)
	frameRe      = regexp.MustCompile(`(?:\.(?:go|py|ts|tsx|js|java|cs|rs|php|rb|kt|swift|c|cpp|h):\d+)|(?:File "[^"]+", line \d+)|(?:at [\w.$<>]+\()`)
	backtickRe   = regexp.MustCompile("`([^`\n]{2,80})`")
	identTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:[.:#][A-Za-z_][A-Za-z0-9_]*)*[?!]?`)
	camelAcronym = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	camelWord    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// readSignals derives the prompt-only signals. It runs before retrieval and costs microseconds.
func readSignals(prompt string) TaskSignals {
	s := TaskSignals{Intent: retrieve.ClassifyIntent(prompt), Edit: retrieve.IsEditRequest(prompt)}
	folded := lexical.Fold(prompt)
	trimmed := strings.TrimSpace(folded)
	s.LongPrompt = utf8.RuneCountInString(prompt) > 280
	for _, w := range strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if softwareLexicon[w] && !contains(s.SoftwareTerms, w) {
			s.SoftwareTerms = append(s.SoftwareTerms, w)
		}
	}
	s.RepoCue = repoCue.MatchString(folded)
	s.ContractRisk = s.Edit && preserveCue.MatchString(folded)
	s.Locators = locatorsOf(prompt)
	letters := 0
	for _, r := range prompt {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	short := len(strings.Fields(trimmed)) <= 8
	switch {
	case letters == 0:
		s.Conversational = true
		s.Reasons = append(s.Reasons, "no words: emoji or punctuation only")
	case short && conversational.MatchString(trimmed) && len(s.SoftwareTerms) == 0 && len(s.Locators) == 0:
		s.Conversational = true
		s.Reasons = append(s.Reasons, "social or acknowledgement message")
	case metaRequest.MatchString(trimmed) && len(s.SoftwareTerms) == 0 && len(s.Locators) == 0 && !s.RepoCue:
		s.Conversational = true
		s.Reasons = append(s.Reasons, "writing or conversation request without a code subject")
	}
	return s
}

func locatorsOf(prompt string) []Locator {
	var out []Locator
	seen := map[string]bool{}
	add := func(kind, text string) {
		text = strings.TrimSpace(text)
		if text == "" || seen[kind+text] || len(out) >= 12 {
			return
		}
		seen[kind+text] = true
		out = append(out, Locator{Kind: kind, Text: text})
	}
	for _, m := range pathRe.FindAllStringSubmatch(prompt, -1) {
		add("path", m[1])
	}
	for _, m := range frameRe.FindAllString(prompt, 4) {
		add("stack_frame", m)
	}
	for _, m := range backtickRe.FindAllStringSubmatch(prompt, -1) {
		add("quoted_code", m[1])
	}
	for _, tok := range identTokenRe.FindAllString(prompt, -1) {
		if codeIdentifier(tok) {
			add("identifier", tok)
		}
	}
	return out
}

// codeIdentifier keeps tokens shaped like code: snake_case, camelCase, PascalCase with an inner capital,
// dotted member access (res.redirect) or a Ruby-style predicate (etag_matches?). Plain words are not.
func codeIdentifier(tok string) bool {
	core := strings.TrimRight(tok, "?!")
	if len(core) < 3 {
		return false
	}
	if strings.ContainsAny(core, ".:#") {
		parts := strings.FieldsFunc(core, func(r rune) bool { return r == '.' || r == ':' || r == '#' })
		return len(parts) >= 2 && len(parts[0]) >= 2 && len(parts[len(parts)-1]) >= 2 && !strings.HasSuffix(strings.ToLower(core), ".com")
	}
	if strings.Contains(core, "_") && strings.Trim(core, "_") != "" {
		return true
	}
	for i, r := range core {
		if i > 0 && unicode.IsUpper(r) {
			prev, _ := utf8.DecodeLastRuneInString(core[:i])
			if unicode.IsLower(prev) {
				return true
			}
		}
	}
	return strings.ContainsAny(core, "0123456789") && unicode.IsLetter(rune(core[0])) && strings.IndexFunc(core, unicode.IsLetter) >= 0 && len(core) >= 4 && strings.IndexFunc(core, unicode.IsLower) >= 0
}

// hasCodeAnchor is the stricter V3.5 delivery gate. A documentation heading or a generic
// lexical coincidence is not enough: there must be a retrieved code symbol plus a repository
// cue, verified locator, or a prompt term that names that symbol.
func hasCodeAnchor(s TaskSignals, res retrieve.Result, translated bool) bool {
	var symbols []string
	for _, evidence := range res.Evidence {
		if evidence.Hit.Name == "" || evidence.Hit.Kind == "file" || evidence.Hit.Kind == "section" {
			continue
		}
		symbols = append(symbols, evidence.Hit.Name)
	}
	if len(symbols) == 0 {
		return false
	}
	for _, locator := range s.Locators {
		if locator.Verified {
			return true
		}
	}
	if s.RepoCue {
		return true
	}
	terms := make(map[string]bool, len(res.Trace.Terms))
	for _, term := range res.Trace.Terms {
		if len(term) >= 3 && !genericAnchorTerms[term] {
			terms[term] = true
		}
	}
	if translated {
		for term, translations := range res.Trace.Translations {
			if genericAnchorTerms[term] {
				continue
			}
			for _, translation := range translations {
				if len(translation) >= 3 && !genericAnchorTerms[translation] {
					terms[translation] = true
				}
			}
		}
	}
	for _, name := range symbols {
		for _, term := range symbolWords(name) {
			if terms[term] {
				return true
			}
		}
	}
	return false
}

var genericAnchorTerms = buildLexicon(`
a an and are as at be by can could do does for from had has have he her here hers him his how i if in into is it its may me might my of on or our ours she should so than that the their them then there these they this those through to too up us was we were what when where which who why will with would you your
ainda algum alguma alguns algumas assim bem cada coisa como da das de dela delas dele deles depois do dos e ela elas ele eles em enquanto entre era essa esse essas esses esta este estas estes eu faz fazer fez fica ficar foi foram fosse garantia hoje isso isto já la mais mas me meu minha meus minhas muito nada nele nela nem no nos nossa nossas nosso nossos novo o os outra outras outro outros para pela pelas pelo pelos pequeno pode por porque qual quando que quem será seu seus sem sempre sua suas também tanto toda todas todo todos tu um uma umas uns vai vais vão voce voces você vocês
`)

func symbolWords(name string) []string {
	name = camelAcronym.ReplaceAllString(name, "$1 $2")
	name = camelWord.ReplaceAllString(name, "$1 $2")
	folded := lexical.Fold(name)
	return strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
