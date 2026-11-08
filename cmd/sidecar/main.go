// Command sidecar adds repository context to Claude Code, Codex and Antigravity CLI prompts.
// Normal use is automatic after `sidecar install`; the other commands are for maintenance
// and debugging.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/app"
	"github.com/dedeusgui/sidecar/internal/install"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/progress"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
	"github.com/dedeusgui/sidecar/internal/studio"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		usage(errOut)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "hook":
		return cmdHook(rest, in, out, errOut)
	case "index":
		return cmdIndex(rest, errOut)
	case "install":
		return cmdInstall(rest, in, out, errOut)
	case "uninstall":
		return cmdUninstall(rest, out, errOut)
	case "status":
		return cmdStatus(rest, out, errOut)
	case "doctor":
		return cmdDoctor(out, errOut)
	case "explain":
		return cmdExplain(rest, in, out, errOut)
	case "impact":
		return cmdImpact(rest, out, errOut)
	case "progress":
		return cmdProgress(rest, in, out, errOut)
	case "studio":
		return cmdStudio(rest, out, errOut)
	case "observe":
		return cmdObserve(rest, in)
	case "memory":
		return cmdMemory(rest, out, errOut)
	case "feedback":
		return cmdFeedback(rest, in, out, errOut)
	case "lsp":
		return cmdLSP(rest, in, out, errOut)
	case "activity":
		return cmdActivity(rest, out, errOut)
	case "statusline":
		return cmdStatusline(in, out)
	case "version", "--version":
		_, _ = fmt.Fprintln(out, "sidecar", version)
		return 0
	case "help", "-h", "--help":
		usage(out)
		return 0
	}
	_, _ = fmt.Fprintf(errOut, "comando desconhecido: %q\n\n", cmd)
	usage(errOut)
	return 2
}

func usage(w io.Writer) {
	_, _ = fmt.Fprint(w, `sidecar — contexto de repositório para Claude Code, Codex e Antigravity CLI

Uso:
  sidecar install [--yes] [--agent claude|codex|antigravity]... [--statusline] [--observe-tools]
                                                      configura os hooks (com autorização)
  sidecar uninstall [--purge]                         remove apenas o que o Sidecar instalou
  sidecar status [--root DIR] [--json]                resumo do índice do repositório
  sidecar activity [--json] [--limit N]               últimas execuções do hook (sem prompt)
  sidecar statusline                                  linha de status (lê JSON do Claude Code)
  sidecar doctor                                      diagnóstico de instalação e ambiente
  sidecar explain [--root DIR] [--json] [--engine v1|v2]   mostra o rastro de um prompt (lê stdin)
  sidecar lsp status|doctor|trust|setup               servidores de linguagem (opt-in; veja sidecar lsp)
  sidecar memory list|add|purge [--root DIR] [--json] decisões técnicas verificáveis (opt-in)
                                                      add: --target ARQUIVO|SIMBOLO --summary TEXTO [--ttl 720h]
                                                      purge: remove as invalidadas (--all: todas)
  sidecar feedback record|list|purge [--root DIR] [--json]   feedback pós-tarefa (opt-in; docs/agent-feedback.md)
                                                      record: --helpful ID | --misleading ID (repetível) [--reason CODE]
                                                      [--confidence high|medium|low] [--completed=true|false]
                                                      [--additional-search=true|false] [--session ID]; sem IDs, lê JSON no stdin
                                                      purge: remove os obsoletos (--all: todos)
  sidecar observe --agent A --event PreToolUse|PostToolUse|PreCompact   registra eventos (silencioso)
  sidecar version

Uso normal é automático: depois de instalar, cada prompt enviado ao agente é analisado.
`)
}

type agentList []string

func (a *agentList) String() string { return strings.Join(*a, ",") }
func (a *agentList) Set(v string) error {
	if _, err := adapter.ParseAgent(v); err != nil {
		return err
	}
	*a = append(*a, v)
	return nil
}

func cmdHook(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(errOut)
	agentName := fs.String("agent", "", "claude, codex or antigravity")
	if err := fs.Parse(args); err != nil {
		return 0 // a malformed hook invocation must never block the agent
	}
	agent, err := adapter.ParseAgent(*agentName)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 0
	}
	return app.Hook(app.HookOptions{Agent: agent, Stdin: in, Stdout: out})
}

func cmdIndex(args []string, errOut io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", "", "worktree root to index")
	history := fs.Bool("history", false, "also ingest Git history")
	if err := fs.Parse(args); err != nil || *root == "" {
		_, _ = fmt.Fprintln(errOut, "uso interno: sidecar index --root DIR [--history]")
		return 2
	}
	if err := app.RunIndexer(*root, *history); err != nil {
		_, _ = fmt.Fprintln(errOut, "sidecar index:", err)
		return 1
	}
	return 0
}

func cmdInstall(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(errOut)
	yes := fs.Bool("yes", false, "confirm the changes without asking")
	statusline := fs.Bool("statusline", false, "also show Sidecar in the Claude Code status line (opt-in)")
	observeTools := fs.Bool("observe-tools", false, "also observe tool calls, metadata only (opt-in; Claude Code and Antigravity CLI)")
	var agents agentList
	fs.Var(&agents, "agent", "limit to an agent (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	env, err := install.DefaultEnv()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	opts := install.Options{Env: env, Agents: agents, Yes: *yes, Out: out, In: in, StatusLine: *statusline, ObserveTools: *observeTools}
	if !isTerminal(in) && !*yes {
		opts.In = nil
	}
	done, err := install.Install(opts)
	for _, d := range done {
		_, _ = fmt.Fprintln(out, "✓", d)
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if len(done) == 0 {
		return 0 // declined: install already reported that nothing changed
	}
	_, _ = fmt.Fprintln(out, "\nPronto. Abra um repositório e envie um prompt normalmente.")
	_, _ = fmt.Fprintln(out, "Verifique com: sidecar doctor")
	return 0
}

func cmdUninstall(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(errOut)
	purge := fs.Bool("purge", false, "also delete the index cache and logs")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	env, err := install.DefaultEnv()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	done, err := install.Uninstall(install.UninstallOptions{Env: env, Out: out})
	for _, d := range done {
		_, _ = fmt.Fprintln(out, "✓", d)
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if *purge {
		if err := os.RemoveAll(paths.Home()); err == nil {
			_, _ = fmt.Fprintln(out, "✓ cache removido:", paths.Home())
		}
		if err := os.RemoveAll(paths.ConfigHome()); err == nil {
			_, _ = fmt.Fprintln(out, "✓ configuração removida:", paths.ConfigHome())
		}
	}
	_, _ = fmt.Fprintln(out, "Sidecar desinstalado. Suas demais configurações foram preservadas.")
	return 0
}

func cmdStatus(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	asJSON := fs.Bool("json", false, "print the full summary as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	abs, _ := filepath.Abs(*root)
	rootDir, _ := repo.FindRoot(abs)
	info, err := app.Status(rootDir)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if *asJSON {
		return writeJSON(out, errOut, info)
	}
	for _, k := range []string{"root", "database", "files", "symbols", "commits", "dirty_files", "indexed_head", "last_sync", "index"} {
		if v, ok := info[k]; ok {
			_, _ = fmt.Fprintf(out, "%-14s %v\n", k+":", v)
		}
	}
	return 0
}

func cmdDoctor(out, errOut io.Writer) int {
	env, err := install.DefaultEnv()
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	cwd, _ := os.Getwd()
	failed := false
	for _, c := range app.Doctor(env, cwd) {
		icon := map[string]string{"ok": "✓", "warn": "!", "fail": "✗", "info": "·"}[c.Status]
		_, _ = fmt.Fprintf(out, "%s %-22s %s\n", icon, c.Name, c.Detail)
		if c.Status == "fail" {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

// defaultEngine is v2 only when SIDECAR_ENGINE asks for it; the product default stays v1.
func defaultEngine() string {
	if os.Getenv("SIDECAR_ENGINE") == "v2" {
		return "v2"
	}
	return "v1"
}

func cmdExplain(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	asJSON := fs.Bool("json", false, "print the trace as JSON")
	engine := fs.String("engine", defaultEngine(), "retrieval pipeline: v1 or v2 (default from SIDECAR_ENGINE)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *engine != "v1" && *engine != "v2" {
		_, _ = fmt.Fprintf(errOut, "--engine deve ser v1 ou v2, não %q\n", *engine)
		return 2
	}
	prompt, err := io.ReadAll(io.LimitReader(in, adapter.MaxInputBytes))
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro lendo stdin:", err)
		return 1
	}
	abs, _ := filepath.Abs(*root)
	rootDir, _ := repo.FindRoot(abs)
	opts := retrieve.Options{EngineVersion: *engine, Profile: os.Getenv("SIDECAR_PROFILE"),
		Adaptive: *engine == "v2" && retrieve.FeatureEnabled("adaptive")}
	rep, err := app.Explain(rootDir, strings.TrimSpace(string(prompt)), opts)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if err := app.WriteExplain(out, rep, *asJSON); err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	return 0
}

// cmdObserve is the hook entry for tool events. It is silent and never fails the agent.
func cmdObserve(args []string, in io.Reader) int {
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	agentName := fs.String("agent", "", "claude, codex or antigravity")
	event := fs.String("event", "", "PreToolUse, PostToolUse or PreCompact")
	if err := fs.Parse(args); err != nil {
		return 0
	}
	agent, err := adapter.ParseAgent(*agentName)
	if err != nil {
		return 0
	}
	return app.Observe(agent, *event, in)
}

func cmdMemory(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "uso: sidecar memory list|add|purge [--root DIR] [--json]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("memory "+sub, flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	asJSON := fs.Bool("json", false, "print as JSON")
	target := fs.String("target", "", "file path or symbol name (add)")
	summary := fs.String("summary", "", "the decision, in one line (add)")
	ttl := fs.Duration("ttl", app.DecisionTTL, "how long the decision stays valid (add)")
	all := fs.Bool("all", false, "purge every decision, not only invalidated ones (purge)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	abs, _ := filepath.Abs(*root)
	rootDir, _ := repo.FindRoot(abs)
	now := time.Now()
	var err error
	switch sub {
	case "list":
		var recs []store.DecisionRecord
		if recs, err = app.MemoryList(rootDir, now); err == nil {
			err = printDecisions(out, recs, *asJSON)
		}
	case "add":
		var rec store.DecisionRecord
		if rec, err = app.MemoryAdd(rootDir, *target, *summary, *ttl, now); err == nil {
			_, err = fmt.Fprintf(out, "✓ decisão %d registrada (%s)\n", rec.ID, rec.SymbolOrPath)
		}
	case "purge":
		var n int64
		if n, err = app.MemoryPurge(rootDir, *all, now); err == nil {
			_, err = fmt.Fprintf(out, "✓ %d decisão(ões) removida(s)\n", n)
		}
	default:
		_, _ = fmt.Fprintf(errOut, "subcomando desconhecido: %q (use list, add ou purge)\n", sub)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	return 0
}

func printDecisions(out io.Writer, recs []store.DecisionRecord, asJSON bool) error {
	if asJSON {
		data, err := json.MarshalIndent(recs, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	}
	if len(recs) == 0 {
		_, err := fmt.Fprintln(out, "nenhuma decisão registrada (use: sidecar memory add --target ARQUIVO --summary TEXTO)")
		return err
	}
	for _, d := range recs {
		if _, err := fmt.Fprintf(out, "%-4d %-12s %s  %s\n", d.ID, d.Status, d.SymbolOrPath, d.Summary); err != nil {
			return err
		}
	}
	return nil
}

// maxFeedbackBytes bounds a report read from stdin; the report is a few IDs and labels.
const maxFeedbackBytes = 64 << 10

// idList is a repeatable flag that collects evidence IDs.
type idList []string

func (l *idList) String() string { return strings.Join(*l, ",") }
func (l *idList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func cmdFeedback(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "uso: sidecar feedback record|list|purge [--root DIR] [--json]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("feedback "+sub, flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	asJSON := fs.Bool("json", false, "print as JSON")
	var helpful, misleading idList
	fs.Var(&helpful, "helpful", "evidence ID the agent found helpful (record; repeatable)")
	fs.Var(&misleading, "misleading", "evidence ID the agent found misleading (record; repeatable)")
	reason := fs.String("reason", "other", "exact_match, wrong_file, stale_context, missing_caller, noisy_neighbor or other (record)")
	confidence := fs.String("confidence", "medium", "high, medium or low (record)")
	completed := fs.Bool("completed", false, "the agent says the task was completed; this is a claim, not an oracle (record)")
	oracle := fs.String("oracle", "unknown", "outcome of a test or build run: passed, failed or unknown (record)")
	oracleProvenance := fs.String("oracle-provenance", "", "who ran the oracle: harness, human or agent_report; default agent_report when --oracle is known (record)")
	additional := fs.Bool("additional-search", false, "the agent needed another search (record)")
	session := fs.String("session", "", "agent session ID, to read the observer trace (record)")
	oracleArtifact := fs.String("oracle-artifact", "", "oracle artifact written by the harness; records an independent outcome (record)")
	harnessKey := fs.String("harness-key", "", "file with the harness signing key, kept outside the agent workspace (record with --oracle-artifact)")
	all := fs.Bool("all", false, "purge every record, not only stale ones (purge)")
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	abs, _ := filepath.Abs(*root)
	rootDir, _ := repo.FindRoot(abs)
	var err error
	switch sub {
	case "record":
		var report store.AgentFeedbackReport
		switch {
		case len(helpful) > 0 || len(misleading) > 0:
			report = store.AgentFeedbackReport{
				SchemaVersion: 1, SessionID: *session, TaskCompleted: *completed,
				OracleStatus: *oracle, OracleProvenance: *oracleProvenance,
				HelpfulEvidenceIDs: helpful, MisleadingEvidenceIDs: misleading,
				ReasonCode: *reason, Confidence: *confidence, AdditionalSearchNeeded: *additional,
			}
		case isTerminal(in):
			err = errors.New("pass --helpful or --misleading, or send the report as JSON on stdin")
		default:
			err = json.NewDecoder(io.LimitReader(in, maxFeedbackBytes)).Decode(&report)
		}
		if err != nil {
			break
		}
		var recs []store.FeedbackRecord
		if *oracleArtifact != "" {
			var art app.OracleArtifact
			var key []byte
			if art, key, err = readOracleArtifact(*oracleArtifact, *harnessKey); err == nil {
				recs, err = app.RecordFeedbackWithOracle(rootDir, report, art, key)
			}
		} else {
			recs, err = app.RecordFeedback(rootDir, report)
		}
		if err == nil {
			err = printFeedback(out, recs, *asJSON)
		}
	case "list":
		var recs []store.FeedbackRecord
		if recs, err = app.ListFeedback(rootDir); err == nil {
			err = printFeedback(out, recs, *asJSON)
		}
	case "purge":
		var n int64
		if n, err = app.PurgeFeedback(rootDir, *all); err == nil {
			_, err = fmt.Fprintf(out, "✓ %d registro(s) de feedback removido(s)\n", n)
		}
	default:
		_, _ = fmt.Fprintf(errOut, "subcomando desconhecido: %q (use record, list ou purge)\n", sub)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	return 0
}

func printFeedback(out io.Writer, recs []store.FeedbackRecord, asJSON bool) error {
	if asJSON {
		data, err := json.MarshalIndent(recs, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	}
	if len(recs) == 0 {
		_, err := fmt.Fprintln(out, "nenhum feedback registrado (use: sidecar feedback record --helpful ID | --misleading ID)")
		return err
	}
	for _, r := range recs {
		if _, err := fmt.Fprintf(out, "%-4d %-7s %+.3f %s\n", r.ID, r.Status, r.ReconciledWeight, r.EvidenceID); err != nil {
			return err
		}
	}
	return nil
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// readOracleArtifact reads an oracle artifact and the harness key that signed it. Both are read with a
// size limit, and unknown fields are refused: a misspelt field must not pass silently.
func readOracleArtifact(path, keyPath string) (app.OracleArtifact, []byte, error) {
	var art app.OracleArtifact
	if keyPath == "" {
		return art, nil, errors.New("--oracle-artifact needs --harness-key")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return art, nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return art, nil, err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(io.LimitReader(f, maxFeedbackBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&art); err != nil {
		return art, nil, fmt.Errorf("oracle artifact: %w", err)
	}
	return art, key, nil
}

// cmdProgress classifies a recorded tool-event sequence (JSON lines {"kind","key","err"}) into useful
// iteration, exploration and stagnation. It reads stdin only, runs nothing, and is how benchmark traces from
// real agent runs are scored with the same code the passive observer uses.
func cmdProgress(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("progress", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var events []progress.Event
	dec := json.NewDecoder(io.LimitReader(in, 8<<20))
	for dec.More() {
		var raw struct {
			Kind string `json:"kind"`
			Key  string `json:"key"`
			Err  string `json:"err"`
		}
		if err := dec.Decode(&raw); err != nil {
			_, _ = fmt.Fprintln(errOut, "erro lendo eventos:", err)
			return 1
		}
		events = append(events, progress.Event{Kind: raw.Kind, Key: raw.Key, Err: raw.Err})
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(progress.Analyze(events))
	return 0
}

// cmdImpact prints what the current change touches: unchanged consumers, tests that name the changed
// symbols, stated values the change may contradict, and gaps. It reads Git state and the index; it runs no
// repository code. Verification commands are candidates for the agent or the user to run.
func cmdImpact(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("impact", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	files := fs.String("files", "", "comma-separated changed paths (default: uncommitted changes)")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	abs, _ := filepath.Abs(*root)
	rootDir, _ := repo.FindRoot(abs)
	var list []string
	for _, f := range strings.Split(*files, ",") {
		if f = strings.TrimSpace(f); f != "" {
			list = append(list, f)
		}
	}
	rep, err := app.Impact(rootDir, list)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
		return 0
	}
	_, _ = fmt.Fprintf(out, "changed (%d): %s\n", len(rep.Changed), strings.Join(rep.Changed, ", "))
	for _, f := range rep.Findings {
		_, _ = fmt.Fprintf(out, "  [%s] %-8s %s\n", f.Status, f.Kind, f.Text)
	}
	if len(rep.Findings) == 0 {
		note := "  no consumer, importer, test or document in the index depends on this change"
		if !rep.HistoryIndexed {
			note += " (Git history is not indexed here, so co-change was not checked)"
		}
		_, _ = fmt.Fprintln(out, note)
	}
	if len(rep.Skipped) > 0 {
		_, _ = fmt.Fprintf(out, "skipped: %s\n", strings.Join(rep.Skipped, "; "))
	}
	if rep.Truncated {
		_, _ = fmt.Fprintln(out, "truncated: more related files exist than are listed")
	}
	return 0
}

// cmdStudio writes a self-contained HTML page from structured outputs: explain --json, lab runs and scorecards,
// replay, perf, clarification and impact reports. It reads the files given and writes one file; it starts no
// server, makes no request and is never injected into an agent prompt.
func cmdStudio(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("studio", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dest := fs.String("out", "sidecar-studio.html", "HTML file to write")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(errOut, "uso: sidecar studio [--out arquivo.html] entrada.json [entrada.jsonl ...]")
		return 2
	}
	var inputs []studio.Input
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			_, _ = fmt.Fprintln(errOut, "erro:", err)
			return 1
		}
		inputs = append(inputs, studio.Input{Name: filepath.Base(path), Raw: data})
	}
	page, err := studio.Build(inputs)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if err := os.WriteFile(*dest, page, 0o644); err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "wrote %s (%d bytes, %d input(s))\n", *dest, len(page), len(inputs))
	return 0
}
