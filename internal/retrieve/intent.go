package retrieve

import (
	"math"
	"regexp"
	"strings"

	"github.com/dedeusgui/sidecar/internal/lexical"
)

// Intent is the deterministic reading of what a prompt asks for. Kinds may hold several labels,
// Confidence is the strength of the strongest one, and Signals name every rule that fired.
type Intent struct {
	Kinds      []string `json:"kinds"`
	Confidence float64  `json:"confidence"`
	Signals    []string `json:"signals,omitempty"`
}

// intentMinConfidence is the lowest confidence at which v2 routing changes the search. Below it
// the v1 pipeline runs unchanged.
const intentMinConfidence = 0.55

// intentOrder fixes the output order and the tie-break between kinds.
var intentOrder = []string{"locate", "debug", "impact", "extend", "test", "explain", "contract"}

type intentRule struct {
	kind, name string
	weight     float64
	re         *regexp.Regexp
}

// intentRules match the folded prompt (lowercase, no Latin diacritics). Weights express how
// specific a signal is: a named command or a stack frame is near-certain, a generic word is not.
var intentRules = []intentRule{
	rule("locate", "where", 0.6, `\b(onde|where)\b`),
	rule("locate", "find", 0.8, `\b(where (is|are|s)|which file|qual arquivo|em qual arquivo|em que arquivo|localiz\w*|locate|find|onde (fica|ficam|esta|estao)|encontrar (o|a|os|as|onde)|procurar)\b`),
	rule("locate", "definition", 0.7, `\b(definicao|definition|defined|definido|declarad\w*|declared)\b`),
	rule("locate", "path", 0.6, `[a-z0-9_.\-]+/[a-z0-9_./\-]+\.[a-z]{1,5}\b`),

	rule("debug", "error", 0.8, `\b(erro|erros|error|errors|exception|excecao|excecoes|falh\w*|fail|fails|failed|failing|failure|crash\w*|bug|bugs|panic|undefined|segfault|nao funciona|doesn'?t work|does not work|quebrou|quebrado|quebrada|traceback|stack ?trace)\b`),
	rule("debug", "stack-frame", 0.9, `at [\w.$<>]+\(|\.(go|py|ts|tsx|js|java|cs|rs|php|rb|kt|swift|c|cpp|h):\d+|file ".*", line \d+`),

	rule("impact", "callers", 0.85, `\b(quem (usa|chama|depende|referencia|invoca|consome|importa)|who (uses|calls|depends|imports|references|invokes)|callers?|references?|usages?|dependents?|dependentes?|dependencias? de)\b`),
	rule("impact", "change-effect", 0.85, `\b(impacto|impact|o que quebra|what breaks|break if|break when|quebra se|quebra quando|afeta|affects?|blast radius|se eu (mudar|alterar|remover|trocar|renomear)|if i (change|modify|remove|rename|alter))\b`),
	rule("impact", "used", 0.6, `\b(usad[oa]s?|used by|used in)\b`),

	rule("extend", "create-new", 0.8, `\b(como (criar|adicionar|implementar|estender|incluir)|how (do|to|can|should) (i |we )?(add|create|implement|extend|write|build)|(create|add|implement|extend|write) (a|an|another|new)\b|(criar|adicionar|implementar|estender) (um|uma|outr[oa]|novo|nova)\b|novo (endpoint|handler|comando|command|metodo|method|tipo|type|servico|service|job|worker)|nova (rota|feature|funcionalidade|funcao|tela|tabela|opcao)|outr[oa] (endpoint|handler|comando|command|rota))\b`),
	rule("extend", "same-pattern", 0.75, `\b(seguindo o padrao|mesmo padrao|mesma forma|como os outros|como o outro|analog\w*|similar (a|ao|to)|same pattern|like the existing|following the (pattern|convention)|padrao (existente|atual|do projeto))\b`),
	rule("extend", "generic-add", 0.5, `\b(adicion\w*|implement\w*|estend\w*|extend\w*|criar|create|add|new)\b`),

	rule("test", "test-words", 0.6, `\btest\w*|\bspecs?\b|\bcobertur\w*|\bcoverage\b|\bassert\w*|\bmock\w*|\bfixtures?\b|\b(junit|pytest|jest|vitest|mocha|xunit|nunit|rspec|phpunit|kotest)\b`),
	rule("test", "where-to-test", 0.85, `\b(onde (eu )?(valido|validar|testo|testar|cubro|cobrir)|where (do|can|should) (i|we) (test|validate)|how (do|to|can) (i |we )?test|como (testar|validar)|covered by|cobert[oa] por|quais testes|which tests|what tests|testes? (de|do|da|para|pra) )\b`),
	rule("test", "test-command", 0.9, `\b(go test|npm (run )?test|yarn( run)? test|pnpm( run)? test|pytest|mvn( \w+)? test|gradle( \w+)? test|gradlew test|cargo test|dotnet test|phpunit|rspec)\b`),

	rule("explain", "explain-request", 0.8, `\b(explique|explica|explicar|explain|walk me through|visao geral|overview|como funciona|how does|how do\b.*\bwork|how it works|how is .* (handled|done|implemented))\b`),
	rule("explain", "flow", 0.6, `\b(o que (faz|e|significa)|what does|what is|entender|entenda|understand|fluxo|flow|ciclo de vida|lifecycle|passo a passo|step by step|para que serve|por que existe|why does)\b`),

	rule("contract", "contract-words", 0.8, `\b(contrat\w*|contract\w*|invarian\w*|invariant\w*)\b`),
	rule("contract", "business-rule", 0.8, `\b(regras? de (negocio|dominio|validacao)|business rules?|domain rules?|validation rules?)\b`),
	rule("contract", "divergence", 0.8, `\b(diverg\w*|mismatch\w*|inconsisten\w*|desalinh\w*|desatualiz\w*|outdated|out of sync|contradi\w*|drift\w*|diferent\w* (do|da|dos|das|entre))\b`),
	rule("contract", "documented", 0.6, `\b(documentad\w*|documentacao|documentation|docs?|readme|especificad\w*|specified|especificacao|specification|spec)\b`),
	// "assinatura" alone usually means a subscription here, so only the code sense counts.
	rule("contract", "shape", 0.6, `\b(endpoints?|rotas?|routes?|assinatura (de|do|da) (funcao|metodo|classe)|signatures?|obrigatori\w*|mandatory|required|enums?|constantes?|constants?|status codes?|dtos?|schemas?|payloads?)\b`),
	rule("contract", "normative", 0.5, `\b(deve|devem|deveria|must|should|precisa ser|needs? to be)\b`),
}

func rule(kind, name string, weight float64, pattern string) intentRule {
	return intentRule{kind: kind, name: name, weight: weight, re: regexp.MustCompile(pattern)}
}

// intentFamilies lists the structural relation families each intent may expand. Routing takes
// the union over the detected kinds; v1 expands every family.
var intentFamilies = map[string][]string{
	"locate":   {famParent, famMembers},
	"debug":    {famCallees, famParent, famTests, famHistory},
	"impact":   {famCallers, famTests, famHistory},
	"extend":   {famMembers, famParent, famTests},
	"test":     {famTests, famParent},
	"explain":  {famCallees, famMembers, famParent},
	"contract": {famTests, famMembers, famParent},
}

const (
	famMembers = "members"
	famParent  = "parent"
	famCallees = "callees"
	famCallers = "callers"
	famTests   = "tests"
	famHistory = "history"
)

// ClassifyIntent labels a prompt with the kinds of work it asks for. It applies fixed rules to
// the folded text (Portuguese and English), stack-frame shapes and code identifiers. Nothing is
// learned and no model runs, so the same prompt always yields the same answer.
func ClassifyIntent(prompt string) Intent {
	text := lexical.Fold(intentWindowOf(prompt))
	score := map[string]float64{}
	fired := map[string]int{}
	best := map[string]float64{}
	var signals []string
	fire := func(kind, name string, weight float64) {
		signals = append(signals, kind+":"+name)
		fired[kind]++
		best[kind] = math.Max(best[kind], weight)
	}
	for _, r := range intentRules {
		if r.re.MatchString(text) {
			fire(r.kind, r.name, r.weight)
		}
	}
	if hasCodeIdentifier(intentWindowOf(prompt)) {
		fire("locate", "symbol", 0.5)
	}

	top := 0.0
	var kinds []string
	for _, k := range intentOrder {
		if fired[k] == 0 {
			continue
		}
		// Each further distinct signal of the same kind adds a little certainty, capped below 1.
		s := math.Min(0.95, best[k]+0.1*float64(fired[k]-1))
		score[k] = s
		top = math.Max(top, s)
		if s >= 0.5 {
			kinds = append(kinds, k)
		}
	}
	// Stable order by strength keeps the fixed intent order between equally strong kinds.
	for i := 1; i < len(kinds); i++ {
		for j := i; j > 0 && score[kinds[j]] > score[kinds[j-1]]; j-- {
			kinds[j], kinds[j-1] = kinds[j-1], kinds[j]
		}
	}
	if len(kinds) == 0 {
		kinds = []string{"general"}
	}
	return Intent{Kinds: kinds, Confidence: math.Round(top*100) / 100, Signals: signals}
}

// intentWindow bounds the text the rules read from each end of a prompt. Intent cues sit at the
// start of a prompt or in a trailing stack trace; a pasted log in the middle costs time and adds
// no intent. Without the bound, a 240 KB paste took about 250 ms on every hook call.
const intentWindow = 2048

func intentWindowOf(prompt string) string {
	if len(prompt) <= 2*intentWindow {
		return prompt
	}
	return prompt[:intentWindow] + "\n" + prompt[len(prompt)-intentWindow:]
}

// hasCodeIdentifier reports whether the prompt names a code identifier: snake_case, a digit, or
// camelCase. Plain words and all-caps words are not identifiers here, unlike in exact matching.
func hasCodeIdentifier(prompt string) bool {
	for _, tok := range identRe.FindAllString(prompt, -1) {
		lower, inner := false, false
		for i, r := range tok {
			lower = lower || (r >= 'a' && r <= 'z')
			inner = inner || (i > 0 && r >= 'A' && r <= 'Z')
		}
		if strings.ContainsAny(tok, "_0123456789") || (lower && inner) {
			return true
		}
	}
	return false
}

// familiesFor returns the union of expansion families for the kinds. It returns nil, meaning
// every family, when no kind maps to one.
func familiesFor(kinds []string) map[string]bool {
	out := map[string]bool{}
	for _, k := range kinds {
		for _, f := range intentFamilies[k] {
			out[f] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
