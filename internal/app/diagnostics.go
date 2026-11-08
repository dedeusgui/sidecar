package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/install"
	"github.com/dedeusgui/sidecar/internal/lsp"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// ExplainReport is the full trace of one prompt, for `sidecar explain`.
type ExplainReport struct {
	Root          string                    `json:"root"`
	Sync          SyncSummary               `json:"sync"`
	Terms         []string                  `json:"terms"`
	Phrases       []string                  `json:"phrases"`
	Translations  map[string][]string       `json:"translations"`
	Lexical       int                       `json:"lexical_candidates"`
	Exact         int                       `json:"exact_candidates"`
	Structural    int                       `json:"structural_candidates"`
	Budget        int                       `json:"budget_tokens"`
	TokenEstimate int                       `json:"token_estimate"`
	Intent        retrieve.Intent           `json:"intent"`
	Routed        bool                      `json:"intent_routed"`
	Fingerprint   *repo.Fingerprint         `json:"repo_fingerprint,omitempty"`
	TimingsMS     map[string]float64        `json:"timings_ms"`
	Candidates    []retrieve.CandidateTrace `json:"candidates"`
	Evidence      []EvidenceSummary         `json:"evidence"`
	Engine        string                    `json:"engine"`
	Contracts     []retrieve.Finding        `json:"contracts,omitempty"`
	Packing       *retrieve.PackTrace       `json:"packing,omitempty"`
	Context       string                    `json:"context"`
}

// engineOf names the pipeline that produced a result. Only the exact value "v2" selects v2.
func engineOf(opts retrieve.Options) string {
	if opts.EngineVersion == "v2" {
		return "v2"
	}
	return "v1"
}

// SyncSummary is the part of a sync report that matters to a user.
type SyncSummary struct {
	Files    int     `json:"files"`
	Parsed   int     `json:"parsed"`
	Complete bool    `json:"complete"`
	Head     string  `json:"head,omitempty"`
	MS       float64 `json:"ms"`
}

// EvidenceSummary is one retrieved item in readable form.
type EvidenceSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Path          string   `json:"path"`
	Lines         string   `json:"lines"`
	Verdict       string   `json:"verdict"`
	Section       string   `json:"section"`
	Source        string   `json:"source"`
	Confidence    float64  `json:"confidence"`
	TokenEstimate int      `json:"token_estimate"`
	Score         float64  `json:"score"`
	Relations     []string `json:"relations,omitempty"`
}

// Explain runs the full pipeline for one prompt against a worktree, without the hook's deadline.
func Explain(root, prompt string, opts retrieve.Options) (ExplainReport, error) {
	ix, _, err := index.Open(root, paths.IndexDB(root))
	if err != nil {
		return ExplainReport{}, err
	}
	defer func() { _ = ix.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t := time.Now()
	sync, err := ix.Sync(ctx, index.Options{WithHistory: true})
	if err != nil {
		return ExplainReport{}, err
	}
	syncMS := ms(time.Since(t)) // taken before retrieval and fingerprinting, which are not sync
	opts.Root = root
	if opts.LSPTrusted == nil {
		opts.LSPTrusted = lsp.Trusted
	}
	if opts.LSPServer == nil {
		opts.LSPServer = lspServerForFile
	}
	res, err := retrieve.Search(ctx, ix.Store(), prompt, opts)
	if err != nil {
		return ExplainReport{}, err
	}
	if engineOf(opts) == "v2" {
		if _, err := attachDecisions(ix.Store(), root, prompt, &res, time.Now()); err != nil {
			return ExplainReport{}, err
		}
	}
	fpStart := time.Now()
	fp := repo.Discover(ctx, root)
	res.Trace.Fingerprint = &fp
	rep := ExplainReport{
		Root:          root,
		Engine:        engineOf(opts),
		Contracts:     res.Trace.Contracts,
		Packing:       res.Trace.Packing,
		Sync:          SyncSummary{Files: sync.Files, Parsed: sync.Parsed, Complete: sync.Complete, Head: sync.Head, MS: syncMS},
		Terms:         res.Trace.Terms,
		Phrases:       res.Trace.Phrases,
		Translations:  res.Trace.Translations,
		Lexical:       res.Trace.Lexical,
		Exact:         res.Trace.Exact,
		Structural:    res.Trace.Structural,
		Budget:        res.Trace.Budget,
		TokenEstimate: res.Trace.TokenEstimate,
		Intent:        res.Trace.Intent,
		Routed:        res.Trace.Routed,
		Fingerprint:   res.Trace.Fingerprint,
		TimingsMS:     map[string]float64{},
		Candidates:    res.Trace.Candidates,
		Context:       renderContext(res, opts),
	}
	for k, d := range res.Trace.Timings {
		rep.TimingsMS[k] = ms(d)
	}
	rep.TimingsMS["fingerprint"] = ms(time.Since(fpStart))
	for _, e := range res.Evidence {
		rep.Evidence = append(rep.Evidence, EvidenceSummary{
			ID: e.ID, Name: e.Hit.Name, Kind: e.Hit.Kind, Path: e.Hit.Path,
			Lines:   fmt.Sprintf("%d-%d", e.Hit.Line, e.Hit.EndLine),
			Verdict: e.Verdict, Section: e.Section, Source: e.Source,
			Confidence: e.Confidence, TokenEstimate: e.TokenEstimate,
			Score: e.Score, Relations: e.Relations,
		})
	}
	return rep, nil
}

// WriteExplain prints a report for people, or as JSON when asJSON is set.
func WriteExplain(w io.Writer, rep ExplainReport, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	_, _ = fmt.Fprintf(w, "Repositório: %s\n", rep.Root)
	_, _ = fmt.Fprintf(w, "Índice: %d arquivos (%d reprocessados), completo=%v\n\n", rep.Sync.Files, rep.Sync.Parsed, rep.Sync.Complete)
	_, _ = fmt.Fprintf(w, "Termos: %s\n", strings.Join(rep.Terms, ", "))
	keys := make([]string, 0, len(rep.Translations))
	for k := range rep.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "Tradução %s → %s\n", k, strings.Join(rep.Translations[k], ", "))
	}
	_, _ = fmt.Fprintf(w, "Candidatos: léxicos=%d exatos=%d estruturais=%d · orçamento=%d tokens\n", rep.Lexical, rep.Exact, rep.Structural, rep.Budget)
	_, _ = fmt.Fprintf(w, "Intenção: %s (confiança %.2f)\n", strings.Join(rep.Intent.Kinds, ", "), rep.Intent.Confidence)
	_, _ = fmt.Fprintf(w, "Motor: %s\n", rep.Engine)
	if pk := rep.Packing; pk != nil {
		_, _ = fmt.Fprintf(w, "Pacote: %d itens · %d/%d tokens · abstenção=%v %s\n", pk.Selected, pk.Used, pk.Budget, pk.Abstained, pk.Reason)
	}
	if fp := rep.Fingerprint; fp != nil {
		_, _ = fmt.Fprintf(w, "Repositório: git=%v monorepo=%v arquivos=%d manifests=%d linguagens=%d\n", fp.Git, fp.Monorepo, fp.Files, len(fp.Manifests), len(fp.Languages))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Evidências:")
	if len(rep.Evidence) == 0 {
		_, _ = fmt.Fprintln(w, "  (nenhuma — o prompt não gera contexto)")
	}
	for _, e := range rep.Evidence {
		_, _ = fmt.Fprintf(w, "  [%s] %s %s  %s:%s  seção=%s score=%.4f\n", e.Verdict, e.Kind, e.Name, e.Path, e.Lines, e.Section, e.Score)
		for _, r := range e.Relations {
			_, _ = fmt.Fprintf(w, "       ↳ %s\n", r)
		}
	}
	_, _ = fmt.Fprintln(w, "\nCandidatos considerados:")
	for _, c := range rep.Candidates {
		mark := " "
		if c.Kept {
			mark = "*"
		}
		_, _ = fmt.Fprintf(w, "  %s %-28s lex=%-3d cov=%.2f exact=%-5v %s — %s\n", mark, c.Name, c.Lexical, c.Coverage, c.Exact, c.Path, c.Reason)
	}
	_, _ = fmt.Fprintln(w, "\nTempos (ms):")
	tkeys := make([]string, 0, len(rep.TimingsMS))
	for k := range rep.TimingsMS {
		tkeys = append(tkeys, k)
	}
	sort.Strings(tkeys)
	for _, k := range tkeys {
		_, _ = fmt.Fprintf(w, "  %-12s %.3f\n", k, rep.TimingsMS[k])
	}
	_, _ = fmt.Fprintln(w, "\nContexto injetado:")
	if rep.Context == "" {
		_, _ = fmt.Fprintln(w, "  (vazio)")
	}
	_, _ = fmt.Fprintln(w, rep.Context)
	return nil
}

// Check is one diagnostic result.
type Check struct {
	Name   string
	Status string // ok | warn | fail | info
	Detail string
}

// Doctor inspects the installation, the environment and the index for cwd.
func Doctor(env install.Env, cwd string) []Check {
	var checks []Check
	add := func(name, status, detail string) { checks = append(checks, Check{name, status, detail}) }

	if exe, err := os.Executable(); err == nil {
		add("binário", "ok", exe)
	} else {
		add("binário", "fail", err.Error())
	}
	if err := sqliteOK(); err != nil {
		add("sqlite+fts5", "fail", err.Error())
	} else {
		add("sqlite+fts5", "ok", "driver puro Go com FTS5")
	}
	if out, err := exec.Command("git", "--version").Output(); err != nil {
		add("git", "warn", "git não encontrado: histórico e status não serão usados")
	} else {
		add("git", "ok", strings.TrimSpace(string(out)))
	}
	if err := os.MkdirAll(paths.Home(), 0o700); err != nil {
		add("cache", "fail", err.Error())
	} else {
		add("cache", "ok", paths.Home())
	}

	claude := claudeHookCheck(env.ClaudeSettings)
	add("claude code", claude.Status, claude.Detail)
	codex := codexHookCheck(env)
	for _, c := range codex {
		add(c.Name, c.Status, c.Detail)
	}

	if root, isGit := repo.FindRoot(cwd); root != "" {
		db := paths.IndexDB(root)
		if _, err := os.Stat(db); err != nil {
			add("índice do repositório", "info", "ainda não criado; será criado no primeiro prompt")
		} else if st, err := store.Open(db); err != nil {
			add("índice do repositório", "fail", err.Error())
		} else {
			check, _ := st.IntegrityCheck()
			if check == "ok" {
				add("índice do repositório", "ok", fmt.Sprintf("integridade ok · git=%v · %s", isGit, db))
			} else {
				add("índice do repositório", "fail", "quick_check: "+check)
			}
			_ = st.Close()
		}
	}

	errs, p50, p95, n := hookLogSummary(paths.HookLog())
	if n == 0 {
		add("hook", "info", "nenhuma execução registrada ainda")
	} else {
		status := "ok"
		if errs > 0 {
			status = "warn"
		}
		add("hook", status, fmt.Sprintf("últimas %d execuções: p50=%.1fms p95=%.1fms, falhas=%d (ver %s)", n, p50, p95, errs, paths.HookLog()))
	}
	return checks
}

func sqliteOK() error {
	dir, err := os.MkdirTemp("", "sidecar-doctor")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	st, err := store.Open(filepath.Join(dir, "probe.db"))
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	_, err = st.SearchSymbols("probe", 1)
	return err
}

func claudeHookCheck(file string) Check {
	cmd, found := hookCommandIn(file, "claude")
	switch {
	case !found:
		return Check{"claude code", "warn", "hook não instalado — execute `sidecar install`"}
	case !binaryExists(cmd):
		return Check{"claude code", "fail", "o hook aponta para um binário inexistente — execute `sidecar install` de novo"}
	}
	return Check{"claude code", "ok", "hook UserPromptSubmit instalado em " + file}
}

func codexHookCheck(env install.Env) []Check {
	cmd, found := hookCommandIn(env.CodexHooks, "codex")
	if !found {
		return []Check{{"codex", "info", "hook não instalado (ou Codex não detectado)"}}
	}
	out := []Check{{"codex", "ok", "hook UserPromptSubmit em " + env.CodexHooks}}
	if !binaryExists(cmd) {
		out[0] = Check{"codex", "fail", "o hook aponta para um binário inexistente — execute `sidecar install` de novo"}
	}
	if data, err := os.ReadFile(env.CodexConfig); err == nil {
		text := string(data)
		if strings.Contains(text, "codex_hooks = false") || strings.Contains(text, "hooks = false") {
			out = append(out, Check{"codex hooks", "fail", "a funcionalidade hooks está desativada em config.toml"})
		}
		if strings.Contains(text, "hooks.json:user_prompt_submit") && strings.Contains(text, "trusted_hash") {
			out = append(out, Check{"codex aprovação", "ok", "hook aprovado na revisão do Codex (verificação aproximada)"})
		} else {
			out = append(out, Check{"codex aprovação", "warn", "abra o Codex e aprove o hook na revisão de hooks (sem aprovação ele não executa)"})
		}
	}
	return out
}

// hookCommandIn returns the Sidecar hook command recorded in a hooks file, if any.
func hookCommandIn(file, agent string) (string, bool) {
	obj, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	var doc struct {
		Hooks struct {
			UserPromptSubmit []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"UserPromptSubmit"`
		} `json:"hooks"`
	}
	if json.Unmarshal(obj, &doc) != nil {
		return "", false
	}
	for _, g := range doc.Hooks.UserPromptSubmit {
		for _, h := range g.Hooks {
			if install.IsOurs(h.Command, agent) {
				return h.Command, true
			}
		}
	}
	return "", false
}

func binaryExists(command string) bool {
	bin := strings.Trim(strings.TrimSuffix(command, " hook --agent claude"), `"`)
	bin = strings.Trim(strings.TrimSuffix(bin, " hook --agent codex"), `"`)
	_, err := os.Stat(bin)
	return err == nil
}

// hookLogSummary reads recent hook runs: failures and latency percentiles.
func hookLogSummary(path string) (errs int, p50, p95 float64, n int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0
	}
	defer func() { _ = f.Close() }()
	var totals []float64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > 200 {
			lines = lines[1:]
		}
	}
	for _, l := range lines {
		var rec map[string]any
		if json.Unmarshal([]byte(l), &rec) != nil {
			continue
		}
		n++
		switch rec["status"] {
		case "error", "panic", "bad_input", "write_error":
			errs++
		}
		if v, ok := rec["total_ms"].(float64); ok {
			totals = append(totals, v)
		}
	}
	if len(totals) > 0 {
		sort.Float64s(totals)
		p50 = totals[len(totals)/2]
		p95 = totals[min(len(totals)-1, len(totals)*95/100)]
	}
	return errs, p50, p95, n
}

// Status summarizes the index for cwd.
func Status(root string) (map[string]any, error) {
	db := paths.IndexDB(root)
	if _, err := os.Stat(db); err != nil {
		return map[string]any{"root": root, "index": "not created yet"}, nil
	}
	st, err := store.Open(db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = st.Close() }()
	files, _ := st.FileRecords()
	symbols, _ := st.SymbolCount()
	commits, _ := st.CommitCount()
	dirty, _ := st.DirtyPaths()
	lastSync, _ := st.GetMeta("last_sync")
	head, _ := st.GetMeta("indexed_head")
	return map[string]any{
		"root": root, "files": len(files), "symbols": symbols, "commits": commits,
		"dirty_files": len(dirty), "indexed_head": head, "last_sync": lastSync, "database": db,
	}, nil
}

// SupportedAgents lists agent names for help text.
func SupportedAgents() []string { return []string{string(adapter.Claude), string(adapter.Codex)} }

// renderContext mirrors what the hook would inject: the Task Evidence Brief when the feature is on and
// applies, otherwise the standard block.
func renderContext(res retrieve.Result, opts retrieve.Options) string {
	if opts.EngineVersion == "v2" && retrieve.FeatureEnabled("brief") {
		text, _ := brief.RenderTask(res, nil)
		return text
	}
	return brief.Render(res, nil)
}
