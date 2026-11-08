package groom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dedeusgui/sidecar/internal/lexical"
)

// Pending is the question Sidecar suggested and has not seen answered. It lives in the per-session state
// next to the evidence memory, holds no prompt text and expires after a few turns.
type Pending struct {
	Decision string   `json:"decision"`
	Options  []Option `json:"options"`
	Path     string   `json:"path"` // primary evidence file the answer is bound to
	Turn     int      `json:"turn"`
}

// State is the grooming memory of one session.
type SessionState struct {
	Asked   int      `json:"asked"`
	Turn    int      `json:"turn"`
	Pending *Pending `json:"pending,omitempty"`
}

// PendingTurns is how many prompts a pending question waits for its answer.
const PendingTurns = 3

func LoadState(path string) SessionState {
	var s SessionState
	if path == "" {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil {
		return SessionState{}
	}
	return s
}

func SaveState(path string, s SessionState) {
	if path == "" {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	// Temporary file + rename: two hooks of one session must never leave a torn file that loads as "nothing asked".
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

var negation = regexp.MustCompile(`\b(nao|nunca|sem|jamais|not|never|no|without|don'?t|dont|except|exceto)\b`)

var orWord = regexp.MustCompile(`\b(ou|or|talvez|maybe)\b`)

var letterAnswer = regexp.MustCompile(`^\s*(?:(?:opcao|option|letra|alternativa|resposta|answer)\s*[:\-]?\s*)?\(?([a-d])\)?[\s.,!)]*(?:pode ser|please|por favor|pfv)?\s*$`)

// MatchAnswer returns the option the prompt unambiguously selects, or -1. It accepts the option letter
// ("B", "opção b") or the exact option label. A long prompt, a prompt that selects no option or one that
// names several is not an answer: nothing is recorded and the task continues unchanged.
func MatchAnswer(prompt string, opts []Option) int {
	text := lexical.Fold(strings.TrimSpace(prompt))
	if text == "" || len(strings.Fields(text)) > 8 || len(opts) == 0 {
		return -1
	}
	if orWord.MatchString(text) {
		return -1 // "A or B" is not a choice
	}
	if m := letterAnswer.FindStringSubmatch(text); m != nil {
		if i := int(m[1][0] - 'a'); i >= 0 && i < len(opts) {
			return i
		}
		return -1
	}
	if negation.MatchString(text) {
		return -1 // "do not use X" is not a choice of X
	}
	hit := -1
	for i, o := range opts {
		l := lexical.Fold(o.Label)
		if l == "" {
			continue
		}
		// Whole words only: the label "user" must not match "username".
		if regexp.MustCompile(`(^|[^a-z0-9_])` + regexp.QuoteMeta(l) + `($|[^a-z0-9_])`).MatchString(text) {
			if hit >= 0 {
				return -1
			}
			hit = i
		}
	}
	return hit
}
