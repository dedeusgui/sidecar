// Command sidecar adds repository context to Claude Code, Codex and Antigravity CLI prompts.
// Normal use is automatic after `sidecar install`; the other commands are for maintenance
// and debugging.
package main

import (
	"encoding/json"
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
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
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
	case "observe":
		return cmdObserve(rest, in)
	case "memory":
		return cmdMemory(rest, out, errOut)
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
  sidecar install [--yes] [--agent claude|codex|antigravity]... [--statusline]
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
  sidecar observe --agent A --event PreToolUse|PostToolUse   registra eventos de ferramenta (silencioso)
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
	opts := install.Options{Env: env, Agents: agents, Yes: *yes, Out: out, In: in, StatusLine: *statusline}
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
	opts := retrieve.Options{EngineVersion: *engine, Profile: os.Getenv("SIDECAR_PROFILE")}
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
	event := fs.String("event", "", "PreToolUse or PostToolUse")
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

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
