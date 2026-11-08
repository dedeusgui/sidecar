package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
)

// abstentionFixture is a small repository whose vocabulary overlaps everyday words. A voice prompt
// constant and a doc comment contain "natural"; a timeout function contains "time"; a release handler
// is the only real answer to the positive prompts.
func abstentionFixture(t *testing.T) *index.Index {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"voice/prompt.go":    "package voice\n\n// naturalSpokenPrompt is a transcript with natural language and filler words.\nconst naturalSpokenPrompt = \"Cara, tipo, então, eu acho\"\n",
		"middleware/time.go": "package middleware\n\n// Timeout cancels a request after the deadline.\nfunc Timeout(seconds int) int { return seconds }\n",
		"release/handler.go": "package release\n\n// Release sends the queued tickets to the dispatcher.\nfunc Release(ticket string) string { return ticket }\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	return ix
}

// v2Evidence returns the IDs the V2 pipeline would inject for a prompt; an empty list is an abstention.
func v2Evidence(t *testing.T, ix *index.Index, root, prompt string) []string {
	t.Helper()
	res, err := Search(context.Background(), ix.Store(), prompt, Options{EngineVersion: "v2", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ev := range res.Evidence {
		ids = append(ids, ev.ID)
	}
	return ids
}

// Negatives: prompts about something the repository does not discuss. Each contains a word that also
// exists in code (natural, time, cards), so a lexical match alone must not justify an injection.
func TestV2AbstainsOnGenericWordsWithoutEvidence(t *testing.T) {
	ix := abstentionFixture(t)
	root := ix.Root()
	for _, prompt := range []string{
		"como faço pão de fermentação natural em casa?",           // the case from the pilot: 'natural' alone
		"qual o melhor horário para plantar tomate na primavera?", // 'horário' translates to 'time'
		"how do I change the oil filter on a 2010 honda civic?",
		"me indica um filme de comédia leve pra assistir hoje",
	} {
		if ids := v2Evidence(t, ix, root, prompt); len(ids) != 0 {
			t.Errorf("prompt %q injected %v; a generic word must not be enough evidence", prompt, ids)
		}
	}
}

// Positives must survive the stricter rule: a specific prompt about the repository still gets its
// evidence, and an exact identifier is always retrieved.
func TestV2KeepsSpecificAndExactEvidence(t *testing.T) {
	ix := abstentionFixture(t)
	root := ix.Root()
	if ids := v2Evidence(t, ix, root, "how does release send the queued tickets?"); !containsPrefix(ids, "release/handler.go") {
		t.Errorf("specific prompt lost its evidence: %v", ids)
	}
	if ids := v2Evidence(t, ix, root, "onde está naturalSpokenPrompt?"); !containsPrefix(ids, "voice/prompt.go") {
		t.Errorf("exact identifier was not retrieved: %v", ids)
	}
}

func containsPrefix(ids []string, path string) bool {
	for _, id := range ids {
		if len(id) >= len(path) && id[:len(path)] == path {
			return true
		}
	}
	return false
}
