package retrieve

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/store"
)

// voicePrompt is a 200-word spoken PT-BR transcript. Filler and transcription noise surround one
// question about Enqueue, the same symbol the clean prompt asks for.
const voicePrompt = "Cara, tipo, então, deixa eu te falar uma coisa, né, eu acho que a gente precisa entender direitinho essa parte, sabe, tipo assim, beleza? " +
	"Bom dia, tudo bem? Olha, eu queria, assim, basicamente, fazer uma pergunta sobre o projeto, tá, mas é uma coisa meio longa, então vou falando aos poucos enquanto eu penso, ok? " +
	"Então, eh, o negócio é o seguinte: quando um caminhão chega, ele entra numa fila, né, e eu queria saber como é que o sistema faz isso, tipo, qual é a parte que coloca o veículo na fila de espera, sabe, o Enqueue, acho que é isso. " +
	"Tipo, realmente, é uma coisa interessante de olhar, a gente vem falando disso há um tempo e acredito que seja o melhor ponto pra começar, né. " +
	"Algum detalhe de cálculo não importa agora, vamos deixar pra depois, tá bom? Obrigado, cara, pela paciência de todo mundo na call hoje à tarde. " +
	"Ah, sei lá, eu nem sei se isso é importante, mas enfim, tá vendo, de qualquer jeito, gente. " +
	"Eu entendo que é meio confuso, mas, enfim, a questão é: como funciona o Enqueue da fila, entende? Beleza, é isso mesmo, valeu, tchau, bom trabalho pra gente, falou."

const voiceClean = "Como funciona o Enqueue da fila?"

// topHit names the best-ranked evidence as path:name, so two symbols with one name stay distinct.
func topHit(res Result) string {
	if len(res.Evidence) == 0 {
		return ""
	}
	h := res.Evidence[0].Hit
	return h.Path + ":" + h.Name
}

// fastest returns the best of several runs, which keeps scheduler noise out of the latency check.
func fastest(t *testing.T, st *store.Store, prompt string) (Result, time.Duration) {
	t.Helper()
	var best time.Duration
	var res Result
	for i := 0; i < 6; i++ {
		s := time.Now()
		r, err := Search(context.Background(), st, prompt, Options{})
		d := time.Since(s)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 || d < best {
			best, res = d, r
		}
	}
	return res, best
}

func TestVoicePromptMatchesCleanPromptTop1WithinLatency(t *testing.T) {
	ix := buildIndex(t)
	clean, _ := fastest(t, ix.Store(), voiceClean)
	voice, latency := fastest(t, ix.Store(), voicePrompt)
	if got, want := topHit(voice), topHit(clean); got != want {
		t.Fatalf("voice top-1 = %q, clean top-1 = %q (voice evidence %v)", got, want, names(voice.Evidence))
	}
	if !raceDetector && latency >= 15*time.Millisecond {
		t.Fatalf("voice retrieval took %v (timings=%v), want < 15ms", latency, voice.Trace.Timings)
	}
}

func TestVoiceGreetingAbstainsWithoutEvidence(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "Bom dia cara, tudo bem? Beleza, vamos conversar aqui rapidinho", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 0 {
		t.Fatalf("greeting injected %d evidence items: %v", len(res.Evidence), names(res.Evidence))
	}
}

func TestVoicePhoneticTranscriptionsResolveOrAbstain(t *testing.T) {
	anchored := buildIndexFromFiles(t, map[string]string{
		"engine/engine.go": `package engine
// Engine owns local context assembly.
type Engine struct{}
`,
		"graph/code_graph.go": `package graph
// CodeGraph maps local code relations.
type CodeGraph struct{}
`,
		"aider/repo_map.go": `package aider
// Aider RepoMap gathers local repository context.
type AiderRepoMap struct{}
`,
	})
	for _, tc := range []struct{ prompt, path, name string }{
		{"byte en giin", "engine/engine.go", "Engine"},
		{"cold graph", "graph/code_graph.go", "CodeGraph"},
		{"Ader Ripple", "aider/repo_map.go", "AiderRepoMap"},
	} {
		res, err := Search(context.Background(), anchored.Store(), tc.prompt, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !hasHit(res.Evidence, tc.path, tc.name) {
			t.Errorf("transcription %q did not resolve to %s:%s; terms=%v translations=%v candidates=%+v evidence=%v", tc.prompt, tc.path, tc.name, res.Trace.Terms, res.Trace.Translations, res.Trace.Candidates, names(res.Evidence))
		}
	}

	decoys := buildIndexFromFiles(t, map[string]string{
		"buffers/byte_buffer.go": `package buffers
type ByteBuffer struct{}
type Ripple struct{}
type ColdStorage struct{}
`,
	})
	t.Cleanup(func() { _ = decoys.Close() })
	for _, prompt := range []string{"byte en giin", "cold graph", "Ader Ripple"} {
		res, err := Search(context.Background(), decoys.Store(), prompt, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Evidence) != 0 {
			t.Errorf("noise-only transcription %q injected unrelated evidence: %v", prompt, names(res.Evidence))
		}
	}
	spokenNoAnchor := "Cara, tipo, eu estava tentando explicar uma ideia sem apontar para função ou arquivo; byte en giin, cold graph e Ader Ripple foram só ruídos da transcrição, sabe?"
	res, err := Search(context.Background(), decoys.Store(), spokenNoAnchor, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 0 {
		t.Fatalf("spoken prompt without a repository anchor injected evidence: %v", names(res.Evidence))
	}
}

func TestFocalTermsPreserveNoisyTechnicalAnchorsInLongVoicePrompt(t *testing.T) {
	prompt := "Cara, tipo, deixa eu explicar um pouco mais sobre as coisas que eu estou tentando entender, sabe? " +
		"No fim, onde fica byte en giin, cold graph e Ader Ripple?"
	got := focalTerms(prompt)
	for _, want := range []string{"engine", "code", "graph", "aider"} {
		if !slices.Contains(got, want) {
			t.Errorf("focal terms %v omit normalized anchor %q", got, want)
		}
	}
}

// Feedback from an earlier Voice Mode session reorders the same two copies on the next voice query.
func TestVoiceFeedbackPromotesHelpfulAndDemotesMisleading(t *testing.T) {
	root, ix := dispatchFixture(t)
	st := ix.Store()
	const voiceDispatch = "Cara, tipo, então, eu queria entender, sabe, como o dispatch manda o ticket pra queue, tá? Beleza, valeu."
	base := dispatchPaths(t, st, root, voiceDispatch, true)
	if len(base) != 2 {
		t.Fatalf("want both Dispatch copies retrieved for the voice prompt, got %v", base)
	}
	misleading, helpful := base[0], base[1]
	recordDispatchFeedback(t, st, root, misleading, -1, store.ComputeReconciledWeight(-1, "high", store.TraceEdited, -1, true))
	recordDispatchFeedback(t, st, root, helpful, 1, store.ComputeReconciledWeight(1, "high", store.TraceEdited, 1, false))

	if on := dispatchPaths(t, st, root, voiceDispatch, false); !slices.Equal(on, []string{helpful, misleading}) {
		t.Fatalf("feedback did not reorder the copies for the voice prompt: got %v, want [%s %s]", on, helpful, misleading)
	}
}

// TestUserNaturalSpokenPortugueseVoicePrompt verifies that a spontaneous, unedited PT-BR
// Voice Mode transcript with conversational filler and self-corrections still distills
// the core technical intent and retrieves the exact target symbol in < 15ms.
func TestUserNaturalSpokenPortugueseVoicePrompt(t *testing.T) {
	ix := buildIndex(t)
	const naturalSpokenPrompt = "Certo, cara, é uma questão. Tu fez toda isso aí, tu rodou todos os benchmarks, " +
		"tu fez todas as implementations, mas enfim, uma questão. Eu gostaria que tivesse consolidado no repo " +
		"a questão de eu poder mandar um prompt pra ti em qualquer linguagem de português, como eu tô mandando " +
		"numa nova voz, falando assim, sabe? Mas me diz uma coisa: onde é que o sistema coloca o caminhão na fila " +
		"de espera quando ele chega no pátio, tipo o Enqueue da fila, entende?"

	clean, _ := fastest(t, ix.Store(), voiceClean)
	voice, latency := fastest(t, ix.Store(), naturalSpokenPrompt)
	if got, want := topHit(voice), topHit(clean); got != want {
		t.Fatalf("natural voice top-1 = %q, clean top-1 = %q (voice evidence %v)", got, want, names(voice.Evidence))
	}
	if !raceDetector && latency >= 15*time.Millisecond {
		t.Fatalf("natural voice retrieval took %v, want < 15ms", latency)
	}
}

func BenchmarkVoicePromptSearch(b *testing.B) {
	ix := buildIndex(b)
	queries := []struct{ name, prompt string }{
		{"clean", voiceClean},
		{"long_voice", voicePrompt},
		{"noisy_terms_without_code_anchor", "Cara, eu queria falar da byte en giin, cold graph e Ader Ripple, sabe, enfim, isso era só conversa."},
	}
	for _, q := range queries {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Search(context.Background(), ix.Store(), q.prompt, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkVoiceNormalization(b *testing.B) {
	queries := []struct{ name, prompt string }{
		{"clean", voiceClean},
		{"long_voice", voicePrompt},
		{"noisy_terms", "byte en giin cold graph Ader Ripple"},
	}
	for _, q := range queries {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				terms := lexical.Analyze(q.prompt)
				for _, term := range terms {
					_ = lexical.Translations(term)
				}
				_ = focalTerms(q.prompt)
			}
		})
	}
}
