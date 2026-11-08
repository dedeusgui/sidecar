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

// gold is one relevant item. An empty Name is file-level gold: any symbol in Path counts.
type gold struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type caseSpec struct {
	ID       string `json:"id"`
	Split    string `json:"split"`
	Repo     string `json:"repo"`
	Category string `json:"category,omitempty"`
	Prompt   string `json:"prompt"`
	Negative bool   `json:"negative"`
	Relevant []gold `json:"relevant"`
	// Mechanical checks, used by scenario suites. Each applies only when set.
	ExpectConflict bool     `json:"expect_conflict,omitempty"` // a CONFLICT_CANDIDATE note reaches the output
	ExpectContract string   `json:"expect_contract,omitempty"` // a contract finding with this status exists
	MustNot        []string `json:"must_not,omitempty"`        // these paths must not appear in the top 10
	SeenRerun      bool     `json:"seen_rerun,omitempty"`
	// Clarification benchmark (--groom): the label is set by reading the repository before running anything.
	ExpectAsk   *bool  `json:"expect_ask,omitempty"`   // a careful engineer would interrupt the user
	ExpectState string `json:"expect_state,omitempty"` // the primary open-decision state, when asking      // a second pass with the first pass's evidence must not repeat gold
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
	notes     []string                  // conflict notes that reached the output
	contracts []string                  // contract statuses found, e.g. TEST_EXPECTED
	keys      []string                  // keys of the evidence returned, for the seen rerun
	cands     []retrieve.CandidateTrace // top candidates with their factors, for replay captures
}

// runner answers one prompt. seen, when set, reports evidence already delivered in the session.
type runner func(ix *index.Index, prompt string, seen ctxbuild.Seen) (callResult, error)

type method struct {
	name string
	run  runner
}

type caseOutcome struct {
	Method    string   `json:"method"`
	Case      string   `json:"case"`
	Split     string   `json:"split"`
	Category  string   `json:"category,omitempty"`
	Negative  bool     `json:"negative"`
	Top       []string `json:"top"`
	Found     int      `json:"gold_found_top10"`
	Gold      int      `json:"gold_total"`
	Hit1      bool     `json:"hit_at_1"`
	Hit5      bool     `json:"hit_at_5"`
	Recall10  float64  `json:"recall_at_10"`
	RR        float64  `json:"reciprocal_rank"`
	Precision float64  `json:"precision"`
	Empty     bool     `json:"empty"`
	LatencyMS float64  `json:"latency_ms"`
	Tokens    int      `json:"context_tokens"`
	Failed    []string `json:"failed_checks,omitempty"` // mechanical checks that did not hold
}

func main() {
	casesPath := flag.String("cases", "bench/cases.json", "cases file")
	externalPath := flag.String("external-cases", "", "pinned external manifest (repos checked out at their commit); replaces --cases")
	agenticDir := flag.String("agentic-report", "", "print the agentic A/B report for the run records in this directory and exit")
	agenticManifest := flag.String("agentic-manifest", "bench/agentic/manifest.jsonl", "task manifest used by --agentic-report")
	outPath := flag.String("out", "", "write full results as JSON to this path")
	replayPath := flag.String("replay", "", "offline replay: run the V2 pipeline over versioned tuning variants (config file) and compare against the active one")
	replayCheck := flag.String("replay-check", "", "verify the active tuning in this config equals the compiled defaults, then exit")
	impactHistory := flag.String("impact-history", "", "evaluate change-impact analysis on the Git history of this repository (needs history)")
	impactN := flag.Int("impact-commits", 40, "commits to use with --impact-history")
	groomFlag := flag.Bool("groom", false, "clarification benchmark: run the V2 pipeline and the ask/assume policy over cases with expect_ask labels")
	includeHoldout := flag.Bool("include-holdout", false, "with --replay: also run the holdout split (a final report; never used to choose variants)")
	perfRuns := flag.Int("perf", 0, "measure cold/warm latency per pipeline stage with this many warm runs per case (p50/p95/p99)")
	comparePath := flag.String("compare", "", "compare against a baseline results file written by --out")
	reportMD := flag.String("report-md", "", "write a Markdown report to this path (includes the comparison when --compare is set)")
	flag.Parse()

	if *agenticDir != "" {
		if *reportMD == "" {
			must(printAgenticReport(os.Stdout, *agenticDir, *agenticManifest))
			return
		}
		f, err := os.Create(*reportMD)
		must(err)
		must(printAgenticReport(f, *agenticDir, *agenticManifest))
		must(f.Close())
		fmt.Fprintf(os.Stderr, "wrote %s\n", *reportMD)
		return
	}

	if *replayCheck != "" {
		must(checkActiveTuning(*replayCheck))
		fmt.Println("active tuning matches compiled defaults")
		return
	}

	if *impactHistory != "" {
		must(runImpactHistory(*impactHistory, *impactN, *outPath))
		return
	}

	tmp, err := os.MkdirTemp("", "sidecar-eval")
	must(err)
	defer func() { _ = os.RemoveAll(tmp) }()

	var cf casesFile
	var indexes map[string]*index.Index
	if *externalPath != "" {
		cf, indexes = openExternal(*externalPath, tmp)
	} else {
		raw, err := os.ReadFile(*casesPath)
		must(err)
		must(json.Unmarshal(raw, &cf))
		base := filepath.Dir(filepath.Dir(*casesPath)) // fixtures are relative to the repo root
		indexes = buildFixtures(cf, base, tmp)
	}
	defer func() {
		for _, ix := range indexes {
			_ = ix.Close()
		}
	}()

	if *groomFlag {
		must(runGroomEval(cf, indexes, *outPath))
		return
	}
	if *replayPath != "" {
		must(runReplay(*replayPath, cf, indexes, *outPath, *includeHoldout))
		return
	}
	if *perfRuns > 0 {
		must(runPerf(cf, indexes, *perfRuns, *outPath))
		return
	}

	methods := []method{
		{"textual (substring)", textualRun},
		{"bm25 (fts5, raw terms)", retrieveRun(retrieve.Ablation{LexicalOnly: true, NoTranslation: true})},
		{"bm25 + traducao/normalizacao", retrieveRun(retrieve.Ablation{LexicalOnly: true})},
		{"lexical + grafo", retrieveRun(retrieve.Ablation{NoGit: true})},
		{"lexical + grafo + git", retrieveRun(retrieve.Ablation{})},
		{"v2.1 (intent router)", retrieveRunWith(retrieve.Options{IntentRouting: true})},
		{"v2 (intent + contract + packer)", retrieveRunWith(retrieve.Options{EngineVersion: "v2"})},
		{"v2 adaptive (policy)", retrieveRunWith(retrieve.Options{EngineVersion: "v2", Adaptive: true})},
		{"v2 + task brief", retrieveRunRender(retrieve.Options{EngineVersion: "v2"}, true)},
		{"v2 sem intent", retrieveRunWith(retrieve.Options{EngineVersion: "v2", Ablate: retrieve.Ablation{NoIntent: true}})},
		{"v2 sem contract", retrieveRunWith(retrieve.Options{EngineVersion: "v2", Ablate: retrieve.Ablation{NoContract: true}})},
		{"v2 sem packer", retrieveRunWith(retrieve.Options{EngineVersion: "v2", Ablate: retrieve.Ablation{NoPacker: true}})},
	}

	var outcomes []caseOutcome
	for _, m := range methods {
		for _, c := range cf.Cases {
			ix := indexes[c.Repo]
			res, err := m.run(ix, c.Prompt, nil)
			must(err)
			o := score(m.name, c, res)
			if c.SeenRerun {
				rerun, err := m.run(ix, c.Prompt, seenSet(res.keys))
				must(err)
				o.Failed = append(o.Failed, seenFailures(c, res, rerun)...)
			}
			outcomes = append(outcomes, o)
		}
	}

	printTables(os.Stdout, methods, outcomes)
	printScenarios(os.Stdout, methodNames(methods), outcomes)
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
func retrieveRunWith(opts retrieve.Options) runner { return retrieveRunRender(opts, false) }

// retrieveRunRender is retrieveRunWith that can render the Task Evidence Brief instead of the standard
// block, so the token cost of the brief is measured on the same retrieval.
func retrieveRunRender(opts retrieve.Options, taskBrief bool) runner {
	return func(ix *index.Index, prompt string, seen ctxbuild.Seen) (callResult, error) {
		t := time.Now()
		o := opts
		o.Root = ix.Root()
		o.Seen = seen
		res, err := retrieve.Search(context.Background(), ix.Store(), prompt, o)
		if err != nil {
			return callResult{}, err
		}
		// A conflict note is a second item about the same definition; the hit list counts each once.
		var hits []store.SymbolHit
		var notes, keys []string
		dedup := map[string]bool{}
		for _, e := range res.Evidence {
			key := fmt.Sprintf("%s|%s|%s|%d", e.Hit.Path, e.Hit.Kind, e.Hit.Name, e.Hit.Line)
			if !dedup[key] {
				dedup[key] = true
				hits = append(hits, e.Hit)
			}
			if e.Note != "" {
				notes = append(notes, e.Note)
			}
			keys = append(keys, e.Key())
		}
		var contracts []string
		for _, f := range res.Trace.Contracts {
			contracts = append(contracts, string(f.Status))
		}
		rendered := brief.Render(res, nil)
		if taskBrief {
			rendered, _ = brief.RenderTask(res, nil)
		}
		return callResult{hits: hits, tokens: ctxbuild.EstimateTokens(rendered),
			latencyMS: float64(time.Since(t).Microseconds()) / 1000,
			notes:     notes, contracts: contracts, keys: keys, cands: res.Trace.Candidates[:min(len(res.Trace.Candidates), 10)]}, nil
	}
}

// textualRun is the baseline a person would get from grep: raw prompt words (4+ letters,
// no translation, no stemming, no stopwords) matched as substrings of symbol names and paths.
func textualRun(ix *index.Index, prompt string, _ ctxbuild.Seen) (callResult, error) {
	t := time.Now()
	words := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var terms []string
	for _, w := range words {
		if len([]rune(w)) >= 4 {
			terms = append(terms, w)
		}
	}
	all, err := ix.Store().AllSymbols()
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
	o := caseOutcome{Method: name, Case: c.ID, Split: c.Split, Category: c.Category, Negative: c.Negative, Gold: len(c.Relevant),
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
			if g.matches(res.hits[i]) {
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
			if g.matches(res.hits[i]) {
				o.Hit5 = true
				if i == 0 {
					o.Hit1 = true
				}
			}
		}
	}
	if firstRank > 0 {
		o.RR = 1 / float64(firstRank)
	}
	if limit > 0 {
		o.Precision = float64(relevantInList) / float64(limit)
	}
	o.Failed = checkFailures(c, res, o)
	return o
}

// matches reports whether a hit is this gold item. File-level gold matches any symbol in the file.
func (g gold) matches(h store.SymbolHit) bool {
	return h.Path == g.Path && (g.Name == "" || h.Name == g.Name)
}

func printTables(w io.Writer, methods []method, outs []caseOutcome) {
	for _, split := range splitsOf(outs) {
		_, _ = fmt.Fprintf(w, "\n### Split: %s\n\n", split)
		_, _ = fmt.Fprintln(w, "| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |")
		_, _ = fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|")
		for _, m := range methods {
			s := summarize(m.name, split, outs)
			_, _ = fmt.Fprintf(w, "| %s | %.1f%% | %.1f%% | %.1f%% | %.3f | %.1f%% | %.1f%% | %.1f%% | %.2f | %.2f | %.0f |\n",
				m.name, s.Hit1, s.Hit5, s.Recall10, s.MRR, s.Precision, s.Empty, s.FalsePos, s.P50, s.P95, s.Tokens)
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
	printScenarios(f, methodNames(methods), outs)
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
