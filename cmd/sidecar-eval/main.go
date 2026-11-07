// Command sidecar-eval measures retrieval quality and latency on the fixture repositories
// in bench/. It builds each fixture as a real Git repository (with co-change history and an
// uncommitted edit), indexes it, and runs every method on every case. Tuning and evaluation
// cases are reported separately; the evaluation split is never used to choose defaults.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

type gold struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type caseSpec struct {
	ID       string `json:"id"`
	Split    string `json:"split"`
	Repo     string `json:"repo"`
	Prompt   string `json:"prompt"`
	Negative bool   `json:"negative"`
	Relevant []gold `json:"relevant"`
}

type repoSpec struct {
	Fixture string     `json:"fixture"`
	History [][]string `json:"history"`
	Dirty   []string   `json:"dirty"`
}

type casesFile struct {
	Repos map[string]repoSpec `json:"repos"`
	Cases []caseSpec          `json:"cases"`
}

type callResult struct {
	hits      []store.SymbolHit
	tokens    int
	latencyMS float64
}

type runner func(st *store.Store, prompt string) (callResult, error)

type method struct {
	name string
	run  runner
}

type caseOutcome struct {
	Method    string   `json:"method"`
	Case      string   `json:"case"`
	Split     string   `json:"split"`
	Negative  bool     `json:"negative"`
	Top       []string `json:"top"`
	Found     int      `json:"gold_found_top10"`
	Gold      int      `json:"gold_total"`
	Hit5      bool     `json:"hit_at_5"`
	Recall10  float64  `json:"recall_at_10"`
	RR        float64  `json:"reciprocal_rank"`
	Precision float64  `json:"precision"`
	Empty     bool     `json:"empty"`
	LatencyMS float64  `json:"latency_ms"`
	Tokens    int      `json:"context_tokens"`
}

func main() {
	casesPath := flag.String("cases", "bench/cases.json", "cases file")
	outPath := flag.String("out", "", "write full results as JSON to this path")
	comparePath := flag.String("compare", "", "compare against a baseline results file written by --out")
	reportMD := flag.String("report-md", "", "write a Markdown report to this path (includes the comparison when --compare is set)")
	flag.Parse()

	raw, err := os.ReadFile(*casesPath)
	must(err)
	var cf casesFile
	must(json.Unmarshal(raw, &cf))
	base := filepath.Dir(filepath.Dir(*casesPath)) // fixtures are relative to the repo root

	tmp, err := os.MkdirTemp("", "sidecar-eval")
	must(err)
	defer func() { _ = os.RemoveAll(tmp) }()

	indexes := map[string]*index.Index{}
	for name, spec := range cf.Repos {
		root := filepath.Join(tmp, name)
		buildFixtureRepo(filepath.Join(base, spec.Fixture), root, spec)
		ix, _, err := index.Open(root, filepath.Join(tmp, name+".db"))
		must(err)
		rep, err := ix.Sync(context.Background(), index.Options{WithHistory: true})
		must(err)
		fmt.Fprintf(os.Stderr, "indexed %-5s files=%d parsed=%d complete=%v\n", name, rep.Files, rep.Parsed, rep.Complete)
		indexes[name] = ix
		defer func() { _ = ix.Close() }()
	}

	methods := []method{
		{"textual (substring)", textualRun},
		{"bm25 (fts5, raw terms)", retrieveRun(retrieve.Ablation{LexicalOnly: true, NoTranslation: true})},
		{"bm25 + traducao/normalizacao", retrieveRun(retrieve.Ablation{LexicalOnly: true})},
		{"lexical + grafo", retrieveRun(retrieve.Ablation{NoGit: true})},
		{"lexical + grafo + git", retrieveRun(retrieve.Ablation{})},
		{"v2.1 (intent router)", retrieveRunWith(retrieve.Options{EngineVersion: "v2"})},
	}

	var outcomes []caseOutcome
	for _, m := range methods {
		for _, c := range cf.Cases {
			ix := indexes[c.Repo]
			res, err := m.run(ix.Store(), c.Prompt)
			must(err)
			outcomes = append(outcomes, score(m.name, c, res))
		}
	}

	printTables(os.Stdout, methods, outcomes)
	var cmp *comparison
	if *comparePath != "" {
		base, err := loadBaseline(*comparePath)
		must(err)
		c := compareRuns(base, outcomes, methodNames(methods))
		cmp = &c
		printComparison(os.Stdout, c)
	}
	if *reportMD != "" {
		writeReportMD(*reportMD, methods, outcomes, cmp)
	}
	if *outPath != "" {
		report := map[string]any{"hardware": hardware(), "methods": methodNames(methods), "outcomes": outcomes}
		data, _ := json.MarshalIndent(report, "", "  ")
		must(os.WriteFile(*outPath, data, 0o644))
		fmt.Fprintln(os.Stderr, "wrote", *outPath)
	}
}

func retrieveRun(ab retrieve.Ablation) runner {
	return retrieveRunWith(retrieve.Options{Ablate: ab})
}

// retrieveRunWith runs the retrieval pipeline with the given options.
func retrieveRunWith(opts retrieve.Options) runner {
	return func(st *store.Store, prompt string) (callResult, error) {
		t := time.Now()
		res, err := retrieve.Search(context.Background(), st, prompt, opts)
		if err != nil {
			return callResult{}, err
		}
		var hits []store.SymbolHit
		for _, e := range res.Evidence {
			hits = append(hits, e.Hit)
		}
		return callResult{hits: hits, tokens: ctxbuild.EstimateTokens(brief.Render(res, nil)),
			latencyMS: float64(time.Since(t).Microseconds()) / 1000}, nil
	}
}

// textualRun is the baseline a person would get from grep: raw prompt words (4+ letters,
// no translation, no stemming, no stopwords) matched as substrings of symbol names and paths.
func textualRun(st *store.Store, prompt string) (callResult, error) {
	t := time.Now()
	words := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var terms []string
	for _, w := range words {
		if len([]rune(w)) >= 4 {
			terms = append(terms, w)
		}
	}
	all, err := st.AllSymbols()
	if err != nil {
		return callResult{}, err
	}
	type scored struct {
		h     store.SymbolHit
		score int
	}
	var list []scored
	for _, h := range all {
		text := strings.ToLower(h.Name + " " + h.Path)
		n := 0
		for _, w := range terms {
			if strings.Contains(text, w) {
				n++
			}
		}
		if n > 0 {
			list = append(list, scored{h, n})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].h.ID < list[j].h.ID
	})
	var hits []store.SymbolHit
	var lines []string
	for i, s := range list {
		if i >= 10 {
			break
		}
		hits = append(hits, s.h)
		if i < 5 {
			lines = append(lines, fmt.Sprintf("- %s %s (%s:%d)", s.h.Kind, s.h.Name, s.h.Path, s.h.Line))
		}
	}
	return callResult{hits: hits, tokens: ctxbuild.EstimateTokens(strings.Join(lines, "\n")),
		latencyMS: float64(time.Since(t).Microseconds()) / 1000}, nil
}

func score(name string, c caseSpec, res callResult) caseOutcome {
	o := caseOutcome{Method: name, Case: c.ID, Split: c.Split, Negative: c.Negative, Gold: len(c.Relevant),
		LatencyMS: res.latencyMS, Tokens: res.tokens, Empty: len(res.hits) == 0}
	for i, h := range res.hits {
		if i < 10 {
			o.Top = append(o.Top, h.Kind+":"+h.Name+"@"+h.Path)
		}
	}
	limit := min(len(res.hits), 10)
	matched := map[int]bool{}
	relevantInList := 0
	firstRank := 0
	for i := 0; i < limit; i++ {
		isGold := false
		for gi, g := range c.Relevant {
			if res.hits[i].Path == g.Path && res.hits[i].Name == g.Name {
				isGold = true
				matched[gi] = true
				if firstRank == 0 {
					firstRank = i + 1
				}
			}
		}
		if isGold {
			relevantInList++
		}
	}
	o.Found = len(matched)
	if len(c.Relevant) > 0 {
		o.Recall10 = float64(o.Found) / float64(len(c.Relevant))
	}
	for i := 0; i < min(len(res.hits), 5); i++ {
		for _, g := range c.Relevant {
			if res.hits[i].Path == g.Path && res.hits[i].Name == g.Name {
				o.Hit5 = true
			}
		}
	}
	if firstRank > 0 {
		o.RR = 1 / float64(firstRank)
	}
	if limit > 0 {
		o.Precision = float64(relevantInList) / float64(limit)
	}
	return o
}

func printTables(w io.Writer, methods []method, outs []caseOutcome) {
	for _, split := range splits {
		_, _ = fmt.Fprintf(w, "\n### Split: %s\n\n", split)
		_, _ = fmt.Fprintln(w, "| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |")
		_, _ = fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
		for _, m := range methods {
			s := summarize(m.name, split, outs)
			_, _ = fmt.Fprintf(w, "| %s | %.1f%% | %.1f%% | %.3f | %.1f%% | %.1f%% | %.1f%% | %.2f | %.2f | %.0f |\n",
				m.name, s.Hit5, s.Recall10, s.MRR, s.Precision, s.Empty, s.FalsePos, s.P50, s.P95, s.Tokens)
		}
	}
}

// writeReportMD saves the split tables and, when a baseline was given, the comparison.
func writeReportMD(path string, methods []method, outs []caseOutcome, cmp *comparison) {
	f, err := os.Create(path)
	must(err)
	h := hardware()
	_, _ = fmt.Fprintf(f, "# Relatório sidecar-eval\n\nAmbiente: %s · %s · %s\n", hardwareField(h, "date"), hardwareField(h, "go"), hardwareField(h, "cpu_model"))
	printTables(f, methods, outs)
	if cmp != nil {
		printComparison(f, *cmp)
	}
	must(f.Close())
	fmt.Fprintln(os.Stderr, "wrote", path)
}

func methodNames(ms []method) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.name)
	}
	return out
}

func hardware() map[string]any {
	info := map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH, "cpus": runtime.NumCPU(), "go": runtime.Version(), "date": time.Now().UTC().Format(time.RFC3339)}
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				info["cpu_model"] = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				break
			}
		}
	}
	return info
}

// buildFixtureRepo copies a fixture into a fresh Git repository with reproducible history.
func buildFixtureRepo(src, dst string, spec repoSpec) {
	must(os.MkdirAll(dst, 0o755))
	must(exec.Command("cp", "-r", src+"/.", dst).Run())
	git := func(args ...string) {
		full := append([]string{"-c", "user.name=bench", "-c", "user.email=bench@example.com", "-c", "commit.gpgsign=false"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Dir = dst
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic(fmt.Sprintf("git %v: %v\n%s", args, err, out))
		}
	}
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "initial import")
	for i, pair := range spec.History {
		for _, rel := range pair {
			f, err := os.OpenFile(filepath.Join(dst, rel), os.O_APPEND|os.O_WRONLY, 0o644)
			must(err)
			_, _ = fmt.Fprintf(f, "\n// history touch %d\n", i)
			_ = f.Close()
		}
		git("add", ".")
		git("commit", "-q", "-m", fmt.Sprintf("touch pair %d", i))
	}
	for _, rel := range spec.Dirty {
		f, err := os.OpenFile(filepath.Join(dst, rel), os.O_APPEND|os.O_WRONLY, 0o644)
		must(err)
		_, _ = fmt.Fprintf(f, "\n// uncommitted local change\n")
		_ = f.Close()
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "sidecar-eval:", err)
		os.Exit(1)
	}
}
