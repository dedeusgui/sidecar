package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
)

// SetupPlan is what `sidecar lsp setup` shows before it changes anything. Commands are
// argv lists run without a shell; nothing is downloaded from a URL, and nothing runs from a hook.
type SetupPlan struct {
	Language   string
	Binary     string
	Requires   string   // tool that must already be on PATH
	Command    []string // exact argv, pinned to the version below
	Origin     string
	Version    string
	License    string
	PathImpact string
	Rollback   []string
	Blocked    string // non-empty: this build refuses to install the language, and says why
}

// setupPlans holds only pinned, official commands that were checked on this machine. A
// language without a verified pin or a redistributable license is blocked, not guessed.
var setupPlans = map[string]SetupPlan{
	"go": {
		Language: "go", Binary: "gopls", Requires: "go",
		Command:    []string{"go", "install", "golang.org/x/tools/gopls@v0.24.0"},
		Origin:     "proxy.golang.org, módulo golang.org/x/tools/gopls",
		Version:    "v0.24.0",
		License:    "BSD-3-Clause (golang.org/x/tools; conferir o LICENSE do módulo)",
		PathImpact: "binário vai para GOBIN (ou GOPATH/bin); precisa estar no PATH",
		Rollback:   []string{"remova o arquivo gopls de GOBIN (ou GOPATH/bin)"},
	},
	"typescript": {
		Language: "typescript", Binary: "typescript-language-server", Requires: "npm",
		Command:    []string{"npm", "install", "-g", "typescript-language-server@6.0.3", "typescript@7.0.2"},
		Origin:     "registry.npmjs.org",
		Version:    "typescript-language-server@6.0.3, typescript@7.0.2",
		License:    "Apache-2.0 (typescript-language-server e typescript)",
		PathImpact: "binário vai para o prefixo global do npm (npm prefix -g)/bin",
		Rollback:   []string{"npm uninstall -g typescript-language-server typescript"},
	},
	"python": {
		Language: "python", Binary: "pyright-langserver", Requires: "npm",
		Command:    []string{"npm", "install", "-g", "pyright@1.1.414"},
		Origin:     "registry.npmjs.org",
		Version:    "pyright@1.1.414",
		License:    "MIT",
		PathImpact: "binário vai para o prefixo global do npm (npm prefix -g)/bin",
		Rollback:   []string{"npm uninstall -g pyright"},
	},
	"rust": {
		Language: "rust", Binary: "rust-analyzer", Requires: "rustup",
		Command:    []string{"rustup", "component", "add", "rust-analyzer"},
		Origin:     "distribuição oficial do rustup (static.rust-lang.org)",
		Version:    "a versão do toolchain Rust ativo (sem pin próprio)",
		License:    "MIT OR Apache-2.0",
		PathImpact: "o proxy rust-analyzer fica em ~/.cargo/bin",
		Rollback:   []string{"rustup component remove rust-analyzer"},
	},
	"csharp": {
		Language: "csharp", Binary: "csharp-ls",
		Blocked: "nenhuma versão de csharp-ls foi fixada e verificada neste build; instale manualmente com `dotnet tool install --global csharp-ls --version <versão>`",
	},
	"java": {
		Language: "java", Binary: "jdtls",
		Blocked: "jdtls não tem gerenciador de pacotes oficial confiável neste build; siga o guia do Eclipse JDT LS",
	},
	"php": {
		Language: "php", Binary: "intelephense",
		Blocked: "intelephense não é distribuível pelo Sidecar (licença proprietária); revise a licença e instale manualmente",
	},
}

// SetupLanguages lists the languages `sidecar lsp setup` knows, sorted.
func SetupLanguages() []string {
	out := make([]string, 0, len(setupPlans))
	for k := range setupPlans {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// PlanFor returns the setup plan for a language.
func PlanFor(language string) (SetupPlan, error) {
	p, ok := setupPlans[language]
	if !ok {
		return SetupPlan{}, fmt.Errorf("linguagem sem plano de instalação: %q (use: %s)", language, strings.Join(SetupLanguages(), ", "))
	}
	return p, nil
}

// Ready reports whether the plan's prerequisite tool is on PATH.
func (p SetupPlan) Ready() bool {
	if p.Requires == "" {
		return false
	}
	_, err := exec.LookPath(p.Requires)
	return err == nil
}

// Install runs the plan's argv after consent has been given. It refuses blocked plans and
// missing prerequisites. The output of the package manager goes to out and errOut.
func Install(ctx context.Context, p SetupPlan, out, errOut io.Writer) error {
	if p.Blocked != "" {
		return errors.New(p.Blocked)
	}
	if !p.Ready() {
		return fmt.Errorf("pré-requisito %q ausente no PATH", p.Requires)
	}
	cmd := exec.CommandContext(ctx, p.Command[0], p.Command[1:]...)
	cmd.Stdout, cmd.Stderr = out, errOut
	return cmd.Run()
}
