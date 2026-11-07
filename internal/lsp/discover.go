// Package lsp discovers and talks to language servers. Discovery is deterministic and
// never runs project code; the client is opt-in (SIDECAR_LSP=1) and needs a trusted root.
package lsp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/repo"
)

// Status values, in the order a server moves through them. Only Discover and the probe
// set them; "ready" requires a completed initialize/initialized handshake.
const (
	StatusNotApplicable   = "not_applicable"
	StatusMissing         = "missing"
	StatusAvailable       = "available"
	StatusRequiresConsent = "requires_consent"
	StatusInitializing    = "initializing"
	StatusReady           = "ready"
	StatusFailed          = "failed"
	StatusDisabled        = "disabled"
)

// Server describes one language server and the repository evidence for it.
type Server struct {
	LanguageID           string   `json:"language_id"`
	RepoMarkers          []string `json:"repo_markers"`
	InstalledServerPath  string   `json:"installed_server_path,omitempty"`
	ServerVersion        string   `json:"server_version,omitempty"`
	Capabilities         []string `json:"capabilities"`
	TrustedWorkspace     bool     `json:"trusted_workspace"`
	Transport            string   `json:"transport"`
	Status               string   `json:"status"`
	Reason               string   `json:"reason"`
	ClaudePluginCoexists bool     `json:"claude_plugin_coexists"`
}

type spec struct {
	id       string
	plugin   string // official Claude Code code-intelligence plugin for the language
	markers  []string
	binaries []string
}

// Server binaries must speak LSP. The pyright CLI does not, so only pyright-langserver counts.
var specs = []spec{
	{"go", "gopls-lsp", []string{"go.mod"}, []string{"gopls"}},
	{"java", "jdtls-lsp", []string{"pom.xml", "build.gradle", "build.gradle.kts"}, []string{"jdtls"}},
	{"typescript", "typescript-lsp", []string{"package.json", "tsconfig.json"}, []string{"typescript-language-server"}},
	{"python", "pyright-lsp", []string{"pyproject.toml"}, []string{"pyright-langserver"}},
	{"rust", "rust-analyzer-lsp", []string{"Cargo.toml"}, []string{"rust-analyzer"}},
	{"csharp", "csharp-lsp", []string{"*.sln", "*.csproj"}, []string{"csharp-ls"}},
	{"php", "php-lsp", []string{"composer.json"}, []string{"intelephense"}},
}

// Discover walks the worktree without following symlinks or executing project files,
// and reports one row per supported language. Monorepos get one row per language with
// every marker found.
func Discover(root string) ([]Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	found := map[string]map[string]bool{}
	err = filepath.WalkDir(abs, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() && p != abs && repo.ExcludedDir(d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		for _, s := range specs {
			for _, marker := range s.markers {
				if match, _ := filepath.Match(marker, name); match {
					if found[s.id] == nil {
						found[s.id] = map[string]bool{}
					}
					rel, _ := filepath.Rel(abs, p)
					found[s.id][filepath.ToSlash(rel)] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	enabled, trusted, plugins := Enabled(), Trusted(abs), claudePlugins()
	var out []Server
	for _, s := range specs {
		markers := make([]string, 0, len(found[s.id]))
		for m := range found[s.id] {
			markers = append(markers, m)
		}
		sort.Strings(markers)
		row := Server{LanguageID: s.id, RepoMarkers: markers, Capabilities: []string{}, TrustedWorkspace: trusted, Transport: "stdio"}
		switch {
		case len(markers) == 0:
			row.Status = StatusNotApplicable
			row.Reason = "nenhum marcador de projeto encontrado"
		default:
			row.Status, row.Reason = StatusMissing, "nenhum servidor ("+strings.Join(s.binaries, ", ")+") no PATH"
			for _, bin := range s.binaries {
				path, lookErr := exec.LookPath(bin)
				if lookErr != nil {
					continue
				}
				row.InstalledServerPath = path
				row.ServerVersion = version(path)
				row.ClaudePluginCoexists = plugins[s.plugin+"@claude-plugins-official"]
				row.Status, row.Reason = statusFor(enabled, trusted)
				if row.ClaudePluginCoexists {
					row.Reason += "; plugin oficial " + s.plugin + " habilitado no Claude Code (o agente tem LSP próprio; o Sidecar usa processo separado)"
				}
				break
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func statusFor(enabled, trusted bool) (string, string) {
	switch {
	case !enabled:
		return StatusDisabled, "cliente LSP desligado: defina SIDECAR_LSP=1 para usar o servidor"
	case !trusted:
		return StatusRequiresConsent, "workspace não confiável: rode `sidecar lsp trust` na raiz do repositório"
	}
	return StatusAvailable, "servidor no PATH e workspace confiável; handshake não verificado (use `sidecar lsp doctor --probe`)"
}

// version returns the first line of the server's version output, or "" when none is printed.
// Each attempt is bounded, since a stuck binary must not hang discovery.
func version(path string) string {
	for _, arg := range []string{"--version", "version"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, path, arg).Output()
		cancel()
		if err != nil {
			continue
		}
		if line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); line != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// claudePlugins reads the enabledPlugins map from Claude Code settings (name@marketplace → bool).
func claudePlugins() map[string]bool {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".claude")
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return nil
	}
	var doc struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	return doc.EnabledPlugins
}
