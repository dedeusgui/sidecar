package retrieve

import (
	"context"
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dedeusgui/sidecar/internal/store"
)

func TestV37RarityDFAndSalienceSurfaced(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "InvoiceRenderer", Options{EngineVersion: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.TotalSymbols == 0 {
		t.Fatal("Trace.TotalSymbols not populated")
	}
	if len(res.Evidence) == 0 {
		t.Fatal("expected evidence for InvoiceRenderer")
	}
	ev := res.Evidence[0]
	if ev.MinSymbolDF <= 0 || ev.MaxSalience <= 0 || len(ev.MatchedDF) == 0 {
		t.Fatalf("expected MinSymbolDF, MaxSalience, MatchedDF populated, got: df=%d sal=%v matchedDF=%v",
			ev.MinSymbolDF, ev.MaxSalience, ev.MatchedDF)
	}
}

func TestV37RaritySupportKindMatching(t *testing.T) {
	hit := store.SymbolHit{
		Kind:      "table",
		Name:      "weighing_records",
		Path:      "sql/schema.sql",
		Signature: "(id INT)",
	}
	if hitTerms(hit)["table"] {
		t.Fatal("legacy hitTerms must not include Kind")
	}
	if !hitTermsFor(hit, true)["table"] {
		t.Fatal("hitTermsFor(true) must include Kind=table")
	}

	d, a := ownMatches(hit, []string{"tabela"}, false)
	if d != 0 || a != 0 {
		t.Fatalf("legacy ownMatches should not match table kind, got direct=%d alt=%d", d, a)
	}
	d, a = ownMatchesWithKind(hit, []string{"tabela"}, false, true)
	if a < 1 {
		t.Fatalf("ownMatchesWithKind should match tabela->table, got direct=%d alt=%d", d, a)
	}
}

func TestV37SplitMemberKeysAndExactLookup(t *testing.T) {
	legacy := identifierKeys("how does res.send() and Logger::info and route! work? see routes.go example.com")
	if len(legacy) != 0 {
		t.Fatalf("legacy identifierKeys changed: %v", legacy)
	}
	got := identifierKeysWithMembers("how does res.send() and Logger::info and route! and etag_matches? work? see routes.go example.com", true)
	for _, want := range []string{"send", "info", "route!", "etagmatches?"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing member/bang/predicate key %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{"go", "com"} {
		if slices.Contains(got, unwanted) {
			t.Fatalf("file/domain suffix %q should not be extracted: %v", unwanted, got)
		}
	}

	ix := buildIndexFromFiles(t, map[string]string{
		"http/response.js": "export const send = (body) => body;\n",
		"log/logger.rs":    "pub fn info(msg: &str) {}\n",
		"web/routes.rb":    "def etag_matches?\n  true\nend\n",
	})
	for _, q := range []struct {
		prompt string
		want   string
	}{
		{"how does res.send work?", "send"},
		{"how does Logger::info work?", "info"},
		{"when does etag_matches? return true?", "etag_matches?"},
	} {
		res, err := Search(context.Background(), ix.Store(), q.prompt, Options{
			EngineVersion:   "v2",
			SplitMemberKeys: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Evidence) == 0 || res.Evidence[0].Hit.Name != q.want || res.Evidence[0].Source != "exact" {
			t.Fatalf("prompt %q: got %+v, want exact %q", q.prompt, names(res.Evidence), q.want)
		}
	}
}

func TestV37TerseRareDomainMatchReachesBlockGate(t *testing.T) {
	ix := buildIndexFromFiles(t, map[string]string{
		"scale/weighing.go": "package scale\n\n// ConfirmWeighing validates gross tare.\nfunc ConfirmWeighing() {}\n",
		"scale/quota.go":    "package scale\n\n// HelperOne validates daily quota.\nfunc HelperOne() {}\n// HelperTwo checks daily quota.\nfunc HelperTwo() {}\n",
		"queue/dispatch.go": "package queue\n\nfunc Enqueue() {}\nfunc Dequeue() {}\nfunc Peek() {}\nfunc Drain() {}\nfunc Count() {}\nfunc Clear() {}\nfunc Pause() {}\nfunc Resume() {}\n",
	})
	// Prompt has 4 analyzed content terms with >=50% present in repo (tare on ConfirmWeighing, quota in comments);
	// ConfirmWeighing itself matches only 1 rare term ("tare") -> 0.45 base conf < MinBlockConfidence.
	prompt := "azul branco quota tare"
	legacy, err := Search(context.Background(), ix.Store(), prompt, Options{EngineVersion: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Evidence) != 0 {
		t.Fatalf("expected legacy V2 to abstain on 1-of-4 support, got %+v", names(legacy.Evidence))
	}

	tun := V37Tuning()
	v37, err := Search(context.Background(), ix.Store(), prompt, Options{
		EngineVersion: "v2",
		Tuning:        &tun,
		RaritySupport: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(v37.Evidence) == 0 || v37.Evidence[0].Hit.Name != "ConfirmWeighing" {
		t.Fatalf("expected ConfirmWeighing recovered by RaritySupport, got %+v", names(v37.Evidence))
	}
	if v37.Evidence[0].Confidence < 0.52 {
		t.Fatalf("expected confidence >= 0.52, got %.2f", v37.Evidence[0].Confidence)
	}

	// Single rare word on an off-topic multi-word prompt (<50% repo coverage) must stay abstained.
	offTopic, err := Search(context.Background(), ix.Store(), "azul verde amarelo laranja tare", Options{
		EngineVersion: "v2",
		Tuning:        &tun,
		RaritySupport: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offTopic.Evidence) != 0 {
		t.Fatalf("expected abstention on 1-of-5 off-topic prompt, got %+v", names(offTopic.Evidence))
	}

	// Absent anchor blocks rarity boost on terse prompts (mid-sentence proper noun Kafka).
	absentRes, err := Search(context.Background(), ix.Store(), "audit Kafka tare", Options{
		EngineVersion:  "v2",
		Tuning:         &tun,
		RaritySupport:  true,
		MaxAbsentTerms: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !absentRes.Trace.HasAbsentAnchor {
		t.Fatal("expected HasAbsentAnchor=true for Kafka")
	}
	if len(absentRes.Evidence) != 0 {
		t.Fatalf("expected abstention when HasAbsentAnchor is true, got %+v", names(absentRes.Evidence))
	}
}

func TestV37LongPromptSpecificityAndMaxAbsentTerms(t *testing.T) {
	c := &candidate{
		v2Own:         true,
		literal:       true,
		directMatches: 1,
		promptTerms:   8,
		coverage:      0.1,
		tun:           func() *Tuning { t := V37Tuning(); return &t }(),
		raritySupport: true,
	}
	if got := confidenceOf(c); got >= 0.5 {
		t.Fatalf("long prompt with support=1 and low coverage must stay below MinBlockConfidence, got %.2f", got)
	}
	c.coverage = 0.4
	// Even with coverage exemption, support=1 on 4-cap gives 0.45 without cap; with support=2:
	c.directMatches = 2
	if got := confidenceOf(c); math.Abs(got-0.6) > 1e-9 {
		t.Fatalf("support=2 should reach 0.6, got %.2f", got)
	}

	ix := buildIndex(t)
	longWithKafka := "precisamos investigar detalhadamente como o consumidor Kafka processa mensagens na fila de entrada durante picos operacionais diarios"
	// Default MaxAbsentTerms (10) skips absentAnchorTerms on >10 terms.
	legacy, err := Search(context.Background(), ix.Store(), longWithKafka, Options{EngineVersion: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Trace.HasAbsentAnchor {
		t.Fatal("default MaxAbsentTerms=0 should keep <=10 term cap")
	}

	v37, err := Search(context.Background(), ix.Store(), longWithKafka, Options{
		EngineVersion:  "v2",
		MaxAbsentTerms: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v37.Trace.HasAbsentAnchor {
		t.Fatal("MaxAbsentTerms=24 should detect absent anchor Kafka on 11..24 term prompt")
	}

	_ = filepath.Base("")
}
