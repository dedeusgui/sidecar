package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/app"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/index"
)

// The delivery scorecard measures what the agent receives, not what retrieval ranked: every case
// goes through the real hook entry point (app.Hook) with a Claude-shaped payload and the bytes
// written to stdout are scored. It runs no model and no network call. Gold presence in the
// delivered block is a delivery diagnostic; it is neither Hit@K nor an agent outcome.

type deliveryRow struct {
	Engine        string   `json:"engine"`
	Case          string   `json:"case"`
	Split         string   `json:"split"`
	Category      string   `json:"category,omitempty"`
	Negative      bool     `json:"negative"`
	Tokens        int      `json:"context_tokens"`
	Sections      []string `json:"sections,omitempty"`
	Bullets       int      `json:"bullets"`
	GoldDelivered bool     `json:"gold_path_delivered"`
	GoldSymbol    bool     `json:"gold_symbol_delivered"`
	GoldLine      int      `json:"gold_first_line"` // 1-based content line of the first gold path; 0 = absent
	BriefOnly     bool     `json:"orientation_only"`
	LabelConflict int      `json:"trust_label_conflicts"`
	Deterministic bool     `json:"deterministic"`
	LatencyMS     float64  `json:"latency_ms"`
	ContextSHA    string   `json:"context_sha256"`    // hash only: retrieved source is not stored in reports
	Context       string   `json:"context,omitempty"` // only with --delivery-contexts, for local diagnosis
}

type deliverySummary struct {
	Engine          string  `json:"engine"`
	Stratum         string  `json:"stratum"`
	Cases           int     `json:"cases"`
	GoldDelivered   int     `json:"gold_path_delivered"`
	GoldSymbol      int     `json:"gold_symbol_delivered"`
	GoldTop3Lines   int     `json:"gold_in_first_3_lines"`
	NonEmpty        int     `json:"non_empty"`
	BriefOnly       int     `json:"orientation_only"`
	LabelConflicts  int     `json:"trust_label_conflicts"`
	NonDeterminstic int     `json:"non_deterministic"`
	MeanTokens      float64 `json:"mean_tokens"`
	TokensPerGold   float64 `json:"tokens_per_gold_delivered"` // all tokens spent in the stratum / gold deliveries
	P50MS           float64 `json:"p50_ms"`
	P95MS           float64 `json:"p95_ms"`
}

var (
	trustLabelRe  = regexp.MustCompile(`\[(VERIFIED|CANDIDATE|INFERRED|UNKNOWN)\]`)
	sourceSection = map[string]bool{ctxbuild.Code: true, ctxbuild.Contracts: true, ctxbuild.ContractsAndTests: true,
		ctxbuild.Relations: true, ctxbuild.V35Relations: true}
)

// hardNegatives expands the development and validation splits of a negatives file over every
// repository. The holdout split is never read into cases.
func hardNegatives(path string, repos []string) ([]caseSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Splits map[string][]string `json:"splits"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	var out []caseSpec
	for _, split := range []string{"dev", "validation"} {
		for i, prompt := range f.Splits[split] {
			for _, repo := range repos {
				out = append(out, caseSpec{ID: fmt.Sprintf("HN-%s-%d@%s", split, i+1, repo), Split: "hard-negative-" + split,
					Repo: repo, Category: "hard_negative", Prompt: prompt, Negative: true})
			}
		}
	}
	return out, nil
}

func contextHash(context string) string {
	sum := sha256.Sum256([]byte(context))
	return hex.EncodeToString(sum[:])
}

func containsDeliveryPath(line, path string) bool {
	if path == "" {
		return false
	}
	pattern := `(^|[^[:alnum:]_./-])` + regexp.QuoteMeta(path) + `($|[^[:alnum:]_./-])`
	return regexp.MustCompile(pattern).MatchString(line)
}

func containsDeliverySymbol(line, name string) bool {
	if name == "" {
		return false
	}
	pattern := `(^|[^[:alnum:]_])` + regexp.QuoteMeta(name) + `($|[^[:alnum:]_])`
	return regexp.MustCompile(pattern).MatchString(line)
}

// hookContext runs one prompt through the hook and returns the injected block.
func hookContext(root, prompt string) (string, float64, error) {
	payload, _ := json.Marshal(map[string]string{"session_id": "", "cwd": root, "prompt": prompt, "hook_event_name": "UserPromptSubmit"})
	var out bytes.Buffer
	t := time.Now()
	app.Hook(app.HookOptions{Agent: adapter.Claude, Stdin: bytes.NewReader(payload), Stdout: &out,
		Budget: 30 * time.Second, Spawn: func(string, bool) error { return nil }})
	elapsed := float64(time.Since(t).Microseconds()) / 1000
	if out.Len() == 0 {
		return "", elapsed, nil
	}
	var parsed struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return "", elapsed, fmt.Errorf("hook stdout is not the Claude contract: %w", err)
	}
	return parsed.HookSpecificOutput.AdditionalContext, elapsed, nil
}

func scoreDelivery(engineName string, c caseSpec, context string) deliveryRow {
	row := deliveryRow{Engine: engineName, Case: c.ID, Split: c.Split, Category: c.Category, Negative: c.Negative,
		Tokens: ctxbuild.EstimateTokens(context), ContextSHA: contextHash(context)}
	hasSource := false
	contentLine := 0
	for _, line := range strings.Split(context, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "Repository evidence") {
			continue
		}
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, " ") && trimmed == strings.ToUpper(trimmed) {
			row.Sections = append(row.Sections, trimmed)
			hasSource = hasSource || sourceSection[trimmed]
			continue
		}
		contentLine++
		if strings.HasPrefix(line, "- ") {
			row.Bullets++
			labels := map[string]bool{}
			for _, m := range trustLabelRe.FindAllStringSubmatch(line, -1) {
				labels[m[1]] = true
			}
			if len(labels) > 1 {
				row.LabelConflict++
			}
		}
		for _, g := range c.Relevant {
			if containsDeliveryPath(line, g.Path) {
				row.GoldDelivered = true
				if containsDeliverySymbol(line, g.Name) {
					row.GoldSymbol = true
				}
				if row.GoldLine == 0 {
					row.GoldLine = contentLine
				}
			}
		}
	}
	row.BriefOnly = context != "" && !hasSource
	return row
}

func summarizeDelivery(rows []deliveryRow) []deliverySummary {
	type key struct{ engine, stratum string }
	groups := map[key][]deliveryRow{}
	var order []key
	add := func(k key, r deliveryRow) {
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	for _, r := range rows {
		stratum := r.Category
		if stratum == "" {
			stratum = r.Split
		}
		if r.Negative {
			if stratum != "hard_negative" {
				stratum = "negative"
			}
		} else {
			add(key{r.Engine, "ALL POSITIVES"}, r)
		}
		add(key{r.Engine, stratum}, r)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].stratum != order[j].stratum {
			return order[i].stratum < order[j].stratum
		}
		return order[i].engine < order[j].engine
	})
	var out []deliverySummary
	for _, k := range order {
		s := deliverySummary{Engine: k.engine, Stratum: k.stratum, Cases: len(groups[k])}
		var lat []float64
		tokens := 0
		for _, r := range groups[k] {
			tokens += r.Tokens
			lat = append(lat, r.LatencyMS)
			if r.GoldDelivered {
				s.GoldDelivered++
			}
			if r.GoldSymbol {
				s.GoldSymbol++
			}
			if r.GoldLine > 0 && r.GoldLine <= 3 {
				s.GoldTop3Lines++
			}
			if r.Tokens > 0 {
				s.NonEmpty++
			}
			if r.BriefOnly {
				s.BriefOnly++
			}
			s.LabelConflicts += r.LabelConflict
			if !r.Deterministic {
				s.NonDeterminstic++
			}
		}
		s.MeanTokens = float64(tokens) / float64(len(groups[k]))
		if s.GoldDelivered > 0 {
			s.TokensPerGold = float64(tokens) / float64(s.GoldDelivered)
		}
		sort.Float64s(lat)
		s.P50MS, s.P95MS = lat[len(lat)/2], lat[min(len(lat)-1, len(lat)*95/100)]
		out = append(out, s)
	}
	return out
}

func writeDeliveryTable(w io.Writer, summaries []deliverySummary) {
	_, _ = fmt.Fprintln(w, "| Estrato | Motor | Casos | Gold (caminho) entregue | Gold (símbolo) | Gold nas 3 primeiras linhas | Não vazio | Só orientação | Rótulos conflitantes | Tokens médios | Tokens/gold | P50 ms | P95 ms |")
	_, _ = fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, s := range summaries {
		_, _ = fmt.Fprintf(w, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %.0f | %.0f | %.1f | %.1f |\n", s.Stratum, s.Engine, s.Cases,
			s.GoldDelivered, s.GoldSymbol, s.GoldTop3Lines, s.NonEmpty, s.BriefOnly, s.LabelConflicts, s.MeanTokens, s.TokensPerGold, s.P50MS, s.P95MS)
	}
}

// runDelivery indexes every repository once into an isolated Sidecar home and scores the hook
// output of each engine. Each case runs twice without a session; differing bytes are reported.
func runDelivery(cases []caseSpec, indexes map[string]*index.Index, engines []string, tmp, outPath, reportMD string, keepContexts bool) error {
	for _, name := range engines {
		switch name {
		case "v1", "v2", "v3", "v3.5", "v3.6":
		default:
			return fmt.Errorf("unknown delivery engine %q", name)
		}
	}
	if err := os.Setenv("SIDECAR_HOME", filepath.Join(tmp, "delivery-home")); err != nil {
		return err
	}
	_ = os.Unsetenv("SIDECAR_FEATURES")
	_ = os.Unsetenv("SIDECAR_PROFILE")
	for name, ix := range indexes {
		if err := app.RunIndexer(ix.Root(), true); err != nil {
			return fmt.Errorf("index %s: %w", name, err)
		}
	}
	var rows []deliveryRow
	for _, engineName := range engines {
		if err := os.Setenv("SIDECAR_ENGINE", engineName); err != nil {
			return err
		}
		for _, c := range cases {
			root := indexes[c.Repo].Root()
			context, elapsed, err := hookContext(root, c.Prompt)
			if err != nil {
				return fmt.Errorf("%s %s: %w", engineName, c.ID, err)
			}
			again, _, err := hookContext(root, c.Prompt)
			if err != nil {
				return fmt.Errorf("%s %s: %w", engineName, c.ID, err)
			}
			row := scoreDelivery(engineName, c, context)
			row.LatencyMS, row.Deterministic = elapsed, context == again
			if keepContexts {
				row.Context = context
			}
			rows = append(rows, row)
		}
	}
	summaries := summarizeDelivery(rows)
	writeDeliveryTable(os.Stdout, summaries)
	if reportMD != "" {
		var b bytes.Buffer
		_, _ = fmt.Fprintf(&b, "# Scorecard de entrega (hook real, offline, sem modelo)\n\nAmbiente: %s · %v\n\nPresença de gold no bloco entregue é diagnóstico de entrega; não é Hit@K nem resultado de agente. Latência é de execução única nesta máquina, não é benchmark controlado.\n\n", time.Now().UTC().Format(time.RFC3339), hardware()["cpu_model"])
		writeDeliveryTable(&b, summaries)
		if err := os.WriteFile(reportMD, b.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if outPath != "" {
		data, _ := json.MarshalIndent(map[string]any{"hardware": hardware(), "engines": engines, "summary": summaries, "rows": rows}, "", "  ")
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "wrote", outPath)
	}
	return nil
}
