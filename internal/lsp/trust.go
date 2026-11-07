package lsp

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/dedeusgui/sidecar/internal/paths"
)

// Enabled is the global opt-in: language servers run only when SIDECAR_LSP=1.
func Enabled() bool { return os.Getenv("SIDECAR_LSP") == "1" }

// canonical is the absolute, symlink-free form of a folder, so roots compare exactly.
func canonical(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

type trustFile struct {
	Roots []string `json:"roots"`
}

func readTrust() ([]string, error) {
	data, err := os.ReadFile(paths.LSPTrust())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tf trustFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	return tf.Roots, nil
}

// Trusted reports whether the user trusted root with `sidecar lsp trust`. Trust is never
// inferred from repository content. Unreadable trust data fails closed.
func Trusted(root string) bool {
	c, err := canonical(root)
	if err != nil {
		return false
	}
	roots, err := readTrust()
	return err == nil && slices.Contains(roots, c)
}

// Trust records root as trusted and returns its canonical form.
func Trust(root string) (string, error) {
	c, err := canonical(root)
	if err != nil {
		return "", err
	}
	roots, err := readTrust()
	if err != nil {
		return "", err // never overwrite a trust file we cannot read
	}
	if slices.Contains(roots, c) {
		return c, nil
	}
	roots = append(roots, c)
	slices.Sort(roots)
	data, err := json.MarshalIndent(trustFile{Roots: roots}, "", "  ")
	if err != nil {
		return "", err
	}
	p := paths.LSPTrust()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return c, os.Rename(tmp, p)
}
