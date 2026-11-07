package retrieve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/docprovider"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/repo"
)

// Enrichment adds evidence that the index does not hold: local dependency documentation, and
// opt-in notes about language servers. It is fail-open and never starts a process by itself.

// lspBudget bounds the language-server notes for one prompt; past it, items are sent without them.
const lspBudget = 150 * time.Millisecond

// minDepName keeps short dependency names, which are often ordinary words, from matching prompts.
const minDepName = 4

// exactVersionRe accepts a pinned release only; ranges such as ^1.2.3 or >=1 are not documented.
var exactVersionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

// docEcosystem maps a repo ecosystem to the name the documentation cache uses.
var docEcosystem = map[string]string{"python": "pypi"}

// docEvidence returns the local documentation of each pinned dependency that the prompt names.
// Only the local cache is read; a missing or invalid entry adds nothing. Score is the best score
// in the pool, so a named dependency competes with the top-ranked item rather than being valued at zero.
func docEvidence(ctx context.Context, fp *repo.Fingerprint, prompt string, score float64) []Evidence {
	if fp == nil {
		return nil
	}
	cache := docprovider.CacheProvider{Dir: filepath.Join(paths.Home(), "docs")}
	seen := map[string]bool{}
	var out []Evidence
	for _, m := range fp.Manifests {
		eco := m.Ecosystem
		if mapped, ok := docEcosystem[eco]; ok {
			eco = mapped
		}
		for _, d := range m.Dependencies {
			if len(d.Name) < minDepName || seen[d.Name] || !exactVersionRe.MatchString(d.Version) || !namedIn(prompt, d.Name) {
				continue
			}
			seen[d.Name] = true
			doc, err := cache.Lookup(ctx, docprovider.Dependency{Ecosystem: eco, Name: d.Name, Version: d.Version, Exact: true})
			if err != nil {
				continue
			}
			note := fmt.Sprintf("docs %s@%s (cache-local, source %s, sha256 %.12s)", doc.Library, doc.Version, doc.Source, doc.SHA256)
			out = append(out, Evidence{
				Verdict: "VERIFIED", Section: ctxbuild.Contracts, Source: "docprovider", Confidence: 0.8, Score: score,
				ID: "docprovider|" + eco + "|" + d.Name + "@" + d.Version, Note: note, TokenEstimate: ctxbuild.EstimateTokens(note),
			})
		}
	}
	return out
}

// namedIn reports whether name occurs in text as a whole token, ignoring case. Dependency names
// contain / . and -, so only letters, digits and underscores count as word characters.
func namedIn(text, name string) bool {
	t, n := strings.ToLower(text), strings.ToLower(name)
	for i := 0; ; {
		j := strings.Index(t[i:], n)
		if j < 0 {
			return false
		}
		j += i
		if (j == 0 || !isIdentByte(t[j-1])) && (j+len(n) == len(t) || !isIdentByte(t[j+len(n)])) {
			return true
		}
		i = j + 1
	}
}

// lspNotes marks each VERIFIED item whose file has a language server that can confirm it. It needs
// the user's opt-in (SIDECAR_LSP=1) and a root the user trusts; the caller's hooks decide the server.
// The pass shares one deadline, and an item keeps its note-free form when the deadline passes.
func lspNotes(ctx context.Context, opts Options, items []Evidence) {
	if os.Getenv("SIDECAR_LSP") != "1" || opts.Root == "" || opts.LSPTrusted == nil || opts.LSPServer == nil || !opts.LSPTrusted(opts.Root) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, lspBudget)
	defer cancel()
	for i := range items {
		if ctx.Err() != nil {
			return
		}
		if items[i].Verdict != "VERIFIED" || items[i].Note != "" || items[i].Hit.Path == "" {
			continue
		}
		if srv := opts.LSPServer(ctx, opts.Root, items[i].Hit.Path); srv != "" && ctx.Err() == nil {
			items[i].Relations = append(items[i].Relations, "lsp "+srv+" trusted")
		}
	}
}
