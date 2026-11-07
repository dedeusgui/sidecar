package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dedeusgui/sidecar/internal/lsp"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// cmdLSP handles `sidecar lsp status|doctor|setup|trust`. None of these run from a hook:
// they are explicit user commands, and only setup/trust change anything, after consent.
func cmdLSP(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		lspUsage(errOut)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "status":
		return cmdLSPStatus(rest, out, errOut)
	case "doctor":
		return cmdLSPDoctor(rest, out, errOut)
	case "setup":
		return cmdLSPSetup(rest, in, out, errOut)
	case "trust":
		return cmdLSPTrust(rest, in, out, errOut)
	}
	lspUsage(errOut)
	return 2
}

func lspUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `sidecar lsp — servidores de linguagem (opt-in)

  sidecar lsp status [--root DIR] [--json]              descoberta: linguagens, servidores e estados
  sidecar lsp doctor [--root DIR] [--probe] [--json]    estado do cliente; --probe faz o handshake real
  sidecar lsp trust [--root DIR] [--yes]                autoriza iniciar servidores nesta worktree
  sidecar lsp setup --language LANG [--dry-run] [--yes] plano e instalação assistida (com consentimento)

O cliente só roda com SIDECAR_LSP=1 e worktree confiável.
`)
}

// lspRoot resolves the worktree root for --root, the same way the other commands do.
func lspRoot(flagValue string) (string, error) {
	abs, err := filepath.Abs(flagValue)
	if err != nil {
		return "", err
	}
	root, _ := repo.FindRoot(abs)
	if root == "" {
		root = abs
	}
	return root, nil
}

func cmdLSPStatus(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("lsp status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	asJSON := fs.Bool("json", false, "print the rows as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rootDir, err := lspRoot(*root)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	rows, err := lsp.Discover(rootDir)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if *asJSON {
		return writeJSON(out, errOut, rows)
	}
	writeLSPRows(out, rows)
	return 0
}

func cmdLSPDoctor(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("lsp doctor", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	probe := fs.Bool("probe", false, "start each available server and run the handshake")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rootDir, err := lspRoot(*root)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	rows, err := lsp.Discover(rootDir)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if *probe {
		for i := range rows {
			rows[i] = lsp.Probe(context.Background(), rootDir, rows[i])
		}
	}
	report := map[string]any{
		"root": rootDir, "opt_in": lsp.Enabled(), "trusted": lsp.Trusted(rootDir),
		"probed": *probe, "servers": rows,
	}
	if *asJSON {
		return writeJSON(out, errOut, report)
	}
	_, _ = fmt.Fprintf(out, "Raiz: %s\nSIDECAR_LSP=1: %v · workspace confiável: %v · probe: %v\n\n", rootDir, lsp.Enabled(), lsp.Trusted(rootDir), *probe)
	writeLSPRows(out, rows)
	return 0
}

func cmdLSPSetup(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("lsp setup", flag.ContinueOnError)
	fs.SetOutput(errOut)
	language := fs.String("language", "", "language to set up: "+strings.Join(lsp.SetupLanguages(), ", "))
	dryRun := fs.Bool("dry-run", false, "print the plan and change nothing")
	yes := fs.Bool("yes", false, "confirm the plan without asking")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	plan, err := lsp.PlanFor(*language)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 2
	}
	writeSetupPlan(out, plan)
	if plan.Blocked != "" {
		_, _ = fmt.Fprintln(errOut, "bloqueado:", plan.Blocked)
		return 1
	}
	if *dryRun {
		_, _ = fmt.Fprintln(out, "\n--dry-run: nada foi alterado.")
		return 0
	}
	if !*yes {
		if !isTerminal(in) {
			_, _ = fmt.Fprintln(errOut, "erro: não é um terminal interativo; repita com --yes para confirmar o plano acima")
			return 1
		}
		_, _ = fmt.Fprint(out, "\nExecutar este plano? [s/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "s" && a != "sim" && a != "y" && a != "yes" {
			_, _ = fmt.Fprintln(out, "Nada foi alterado.")
			return 0
		}
	}
	if err := lsp.Install(context.Background(), plan, out, errOut); err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	if path, err := exec.LookPath(plan.Binary); err == nil {
		_, _ = fmt.Fprintf(out, "✓ %s encontrado em %s\n", plan.Binary, path)
		return 0
	}
	_, _ = fmt.Fprintf(errOut, "! %s foi instalado, mas não está no PATH atual; ajuste o PATH e rode `sidecar lsp status`.\n", plan.Binary)
	return 1
}

func cmdLSPTrust(args []string, in io.Reader, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("lsp trust", flag.ContinueOnError)
	fs.SetOutput(errOut)
	root := fs.String("root", ".", "folder inside the repository")
	yes := fs.Bool("yes", false, "trust the worktree without asking")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rootDir, err := lspRoot(*root)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	rows, err := lsp.Discover(rootDir)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	var names []string
	for _, r := range rows {
		if r.InstalledServerPath != "" {
			names = append(names, r.LanguageID+" ("+r.InstalledServerPath+")")
		}
	}
	if len(names) == 0 {
		_, _ = fmt.Fprintln(out, "Nenhum servidor LSP encontrado no PATH para esta worktree; nada a autorizar.")
		return 0
	}
	_, _ = fmt.Fprintf(out, "Autorizar o Sidecar a iniciar estes servidores nesta worktree (%s):\n  %s\n", rootDir, strings.Join(names, "\n  "))
	if !*yes {
		if !isTerminal(in) {
			_, _ = fmt.Fprintln(errOut, "erro: não é um terminal interativo; repita com --yes para confirmar")
			return 1
		}
		_, _ = fmt.Fprint(out, "Confirmar? [s/N] ")
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "s" && a != "sim" && a != "y" && a != "yes" {
			_, _ = fmt.Fprintln(out, "Nada foi alterado.")
			return 0
		}
	}
	canon, err := lsp.Trust(rootDir)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	_, _ = fmt.Fprintln(out, "✓ worktree confiável:", canon)
	return 0
}

func writeLSPRows(w io.Writer, rows []lsp.Server) {
	for _, r := range rows {
		_, _ = fmt.Fprintf(w, "%-11s %-17s %s\n", r.LanguageID, r.Status, r.Reason)
		if r.InstalledServerPath != "" {
			_, _ = fmt.Fprintf(w, "%-11s %-17s %s %s\n", "", "", r.InstalledServerPath, r.ServerVersion)
		}
	}
}

func writeSetupPlan(w io.Writer, p lsp.SetupPlan) {
	_, _ = fmt.Fprintf(w, "Plano de instalação: %s (%s)\n", p.Language, p.Binary)
	if p.Blocked != "" {
		_, _ = fmt.Fprintf(w, "  situação: bloqueado — %s\n", p.Blocked)
		return
	}
	state := "AUSENTE no PATH"
	if p.Ready() {
		state = "presente"
	}
	_, _ = fmt.Fprintf(w, "  pré-requisito: %s (%s)\n", p.Requires, state)
	_, _ = fmt.Fprintf(w, "  comando (sem shell): %s\n", strings.Join(p.Command, " "))
	_, _ = fmt.Fprintf(w, "  origem: %s\n  versão: %s\n  licença: %s\n", p.Origin, p.Version, p.License)
	_, _ = fmt.Fprintf(w, "  impacto no PATH: %s\n", p.PathImpact)
	_, _ = fmt.Fprintf(w, "  rollback: %s\n", strings.Join(p.Rollback, "; "))
}

// writeJSON prints v as indented JSON. A failure to encode is reported on errOut and returns 1.
func writeJSON(out, errOut io.Writer, v any) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_, _ = fmt.Fprintln(errOut, "erro:", err)
		return 1
	}
	return 0
}
