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
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/explore"
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
	Decision      *engine.Decision          `json:"v3_decision,omitempty"` // v3 only
	V35Plan       *contract.OutputPlan      `json:"v35_plan,omitempty"`
	V35Tokens     int                       `json:"v35_tokens,omitempty"`
}

// engineOf names the pipeline that produced a result. Unknown values keep the v1 compatibility default.
func engineOf(opts retrieve.Options) string {
	switch opts.EngineVersion {
	case "v2", engine.Version, engine.Version35:
		return opts.EngineVersion
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
	var res retrieve.Result
	var decision *engine.Decision
	var unified *v35Output
	if engineOf(opts) == engine.Version35 {
		r, err := runV35Safe(ctx, ix.Store(), root, prompt, opts, adapter.Agent(""), "", "", time.Now(), true)
		if err == nil {
			res, unified = r.Result, &r
			decision = &r.Decision
		}
	} else if engineOf(opts) == engine.Version {
		r, d, err := engine.Run(ctx, ix.Store(), prompt, engine.Options{Retrieve: opts})
		if err != nil {
			return ExplainReport{}, err
		}
		res, decision = r, &d
	} else {
		r, err := retrieve.Search(ctx, ix.Store(), prompt, opts)
		if err != nil {
			return ExplainReport{}, err
		}
		res = r
	}
	if engineOf(opts) != engine.Version35 && (engineOf(opts) == "v2" || decision != nil && decision.Action != engine.ActionAbstain) {
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
		Decision:      decision,
	}
	if unified != nil {
		rep.Context = unified.Packed.Text
		rep.V35Plan = &unified.Plan
		rep.V35Tokens = unified.Packed.EstimatedTokens
	} else if decision != nil {
		rep.Context = engine.Render(res, *decision, nil)
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
	if d := rep.Decision; d != nil {
		version := "V3"
		if rep.Engine == engine.Version35 {
			version = "V3.5"
		}
		_, _ = fmt.Fprintf(w, "Decisão %s: %s · classe %s · relevância %s · orçamento %d tokens · %d itens\n", version, d.Action, d.Class, d.Relevance, d.Budget, len(d.Included))
		for _, r := range append(append([]string{}, d.AbstainReasons...), d.LowConfidence...) {
			_, _ = fmt.Fprintf(w, "  motivo: %s\n", r)
		}
	}
	if rep.V35Plan != nil {
		_, _ = fmt.Fprintf(w, "Plano V3.5: %d tokens alocados · %d tokens empacotados · %d seções\n", rep.V35Plan.BudgetTokens, rep.V35Tokens, len(rep.V35Plan.Sections))
	}
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

type ClientCapabilities struct {
	Client      string                    `json:"client"`
	Detected    bool                      `json:"detected"`
	PromptEvent string                    `json:"prompt_event,omitempty"`
	Prompt      contract.CapabilityStatus `json:"prompt"`
	ToolEvents  contract.CapabilityStatus `json:"tool_events"`
	PreCompact  contract.CapabilityStatus `json:"pre_compact"`
	Explore     contract.CapabilityStatus `json:"explore"`
}

func engineReadiness() map[string]any {
	selected := configuredEngine()
	active := selected == engine.Version35
	activation := "export SIDECAR_ENGINE=v3.5"
	deactivation := "unset SIDECAR_ENGINE (default: v1; applies on the next hook)"
	detail := "motor atual: " + selected + "; V3.5 inativa (opt-in); ativar: " + activation + "; desativar: " + deactivation
	if active {
		detail = "motor atual: v3.5; rollback imediato: " + deactivation
	}
	env, err := install.DefaultEnv()
	var detected []string
	if err == nil {
		detected = install.Detect(env)
	}
	clients := clientCapabilityMatrix(detected)
	return map[string]any{
		"engine": selected, "v35_active": active, "engine_detail": detail,
		"engine_activation": activation, "engine_deactivation": deactivation,
		"client_capabilities": clients,
	}
}

func configuredEngine() string {
	switch os.Getenv("SIDECAR_ENGINE") {
	case "v2", engine.Version, engine.Version35:
		return os.Getenv("SIDECAR_ENGINE")
	default:
		return "v1"
	}
}

func clientCapabilityMatrix(detected []string) []ClientCapabilities {
	present := make(map[string]bool, len(detected))
	for _, name := range detected {
		present[name] = true
	}
	clients := []adapter.Agent{adapter.Claude, adapter.Codex, adapter.Antigravity}
	result := make([]ClientCapabilities, 0, len(clients))
	for _, client := range clients {
		capabilities := adapter.CapabilitiesFor(client)
		result = append(result, ClientCapabilities{
			Client: string(client), Detected: present[string(client)], PromptEvent: capabilities.PromptEvent,
			Prompt: capabilities.PromptCapability, ToolEvents: capabilities.ToolEventsCapability,
			PreCompact: capabilities.PreCompactCapability, Explore: capabilities.ExploreCapability,
		})
	}
	return result
}

func clientCapabilityDetail(env install.Env) string {
	clients := clientCapabilityMatrix(install.Detect(env))
	parts := make([]string, 0, len(clients))
	for _, client := range clients {
		name := client.Client
		if !client.Detected {
			name += " (não detectado)"
		}
		parts = append(parts, fmt.Sprintf("%s: prompt=%s, tools=%s, PreCompact=%s, explore=%s",
			name, client.Prompt, client.ToolEvents, client.PreCompact, client.Explore))
	}
	return strings.Join(parts, "; ")
}

// Doctor inspects the installation, the environment and the index for cwd.
func Doctor(env install.Env, cwd string) []Check {
	var checks []Check
	add := func(name, status, detail string) { checks = append(checks, Check{name, status, detail}) }
	add("motor", "info", engineReadiness()["engine_detail"].(string))
	add("matriz de clientes", "info", clientCapabilityDetail(env))

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
	info := engineReadiness()
	info["root"] = root
	db := paths.IndexDB(root)
	if _, err := os.Stat(db); err != nil {
		info["index"] = "not created yet"
		return info, nil
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
	for key, value := range map[string]any{
		"root": root, "files": len(files), "symbols": symbols, "commits": commits,
		"dirty_files": len(dirty), "indexed_head": head, "last_sync": lastSync, "database": db,
	} {
		info[key] = value
	}
	return info, nil
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

// Explore opens the index of root, syncs it within a bound and answers one structural query.
// It writes nothing but the index cache outside the repository.
func Explore(root, query string, maxFiles, maxChars int, withImpact bool) (explore.Result, error) {
	ix, _, err := index.Open(root, paths.IndexDB(root))
	if err != nil {
		return explore.Result{}, err
	}
	defer func() { _ = ix.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncCtx, cancelSync := context.WithTimeout(ctx, 60*time.Second)
	sync, err := ix.Sync(syncCtx, index.Options{WithHistory: true})
	cancelSync()
	if err != nil {
		return explore.Result{}, err
	}
	res, err := explore.Run(ctx, ix.Store(), explore.Options{Root: root, Query: query, MaxFiles: maxFiles, MaxChars: maxChars,
		Impact: withImpact, Retrieve: retrieve.Options{LSPTrusted: lsp.Trusted, LSPServer: lspServerForFile}})
	if err != nil {
		return res, err
	}
	if !sync.Complete {
		res.Gaps = append(res.Gaps, "the index sync did not finish within its time bound; recently changed files may be missing")
	}
	return res, nil
}
