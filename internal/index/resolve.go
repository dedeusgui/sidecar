package index

import (
	"context"
	"path"
	"strings"

	"github.com/dedeusgui/sidecar/internal/store"
)

// resolveImports maps import strings to repository files when the mapping is unambiguous:
// relative TS/JS, Ruby and Python paths, and Java/PHP classes by package/namespace path.
// Anything else (external packages, Go modules, C# namespaces) stays unresolved and is never guessed.
func (ix *Index) resolveImports(ctx context.Context) error {
	pending, err := ix.st.PendingImports()
	if err != nil {
		return err
	}
	for _, p := range pending {
		if ctx.Err() != nil {
			return nil
		}
		target, ok, err := ix.findTarget(p)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := ix.st.SetImportTarget(p.ID, target); err != nil {
			return err
		}
	}
	return nil
}

func (ix *Index) findTarget(p store.PendingImport) (int64, bool, error) {
	switch p.Lang {
	case "typescript", "tsx", "javascript":
		if !strings.HasPrefix(p.Module, ".") {
			return 0, false, nil
		}
		base := path.Join(path.Dir(p.Path), p.Module)
		for _, cand := range []string{base, base + ".ts", base + ".tsx", base + ".js", base + ".jsx",
			base + "/index.ts", base + "/index.tsx", base + "/index.js"} {
			if id, ok, err := ix.st.FileIDByPath(cand); err != nil || ok {
				return id, ok, err
			}
		}
	case "python":
		return ix.pythonTarget(p)
	case "java":
		return ix.suffixTarget(strings.Split(p.Module, "."), ".java")
	case "php":
		mod := strings.TrimPrefix(p.Module, `\`)
		return ix.suffixTarget(strings.Split(mod, `\`), ".php")
	case "ruby":
		return ix.rubyTarget(p)
	}
	return 0, false, nil
}

func (ix *Index) suffixTarget(segs []string, ext string) (int64, bool, error) {
	for drop := 0; drop <= 1 && len(segs)-drop > 1; drop++ {
		suffix := strings.Join(segs[:len(segs)-drop], "/") + ext
		ids, err := ix.st.FileIDsWithSuffix(suffix)
		if err != nil {
			return 0, false, err
		}
		if len(ids) == 1 {
			return ids[0], true, nil
		}
	}
	return 0, false, nil
}

func (ix *Index) rubyTarget(p store.PendingImport) (int64, bool, error) {
	mod := strings.TrimSpace(p.Module)
	if mod == "" {
		return 0, false, nil
	}
	if strings.HasPrefix(mod, ".") {
		base := path.Join(path.Dir(p.Path), mod)
		cands := []string{base + ".rb", base}
		if strings.HasSuffix(base, ".rb") {
			cands = []string{base}
		}
		for _, cand := range cands {
			if id, ok, err := ix.st.FileIDByPath(cand); err != nil || ok {
				return id, ok, err
			}
		}
		return 0, false, nil
	}
	suffix := mod
	if !strings.HasSuffix(suffix, ".rb") {
		suffix += ".rb"
	}
	ids, err := ix.st.FileIDsWithSuffix(suffix)
	if err != nil {
		return 0, false, err
	}
	if len(ids) == 1 {
		return ids[0], true, nil
	}
	return 0, false, nil
}

func (ix *Index) pythonTarget(p store.PendingImport) (int64, bool, error) {
	mod := p.Module
	dots := len(mod) - len(strings.TrimLeft(mod, "."))
	rest := strings.ReplaceAll(mod[dots:], ".", "/")
	var bases []string
	if dots > 0 {
		dir := path.Dir(p.Path)
		for i := 1; i < dots; i++ {
			dir = path.Dir(dir)
		}
		bases = []string{path.Join(dir, rest)}
	} else {
		if rest == "" {
			return 0, false, nil
		}
		bases = []string{rest}
	}
	for _, base := range bases {
		for _, cand := range []string{base + ".py", base + "/__init__.py"} {
			if dots > 0 {
				if id, ok, err := ix.st.FileIDByPath(cand); err != nil || ok {
					return id, ok, err
				}
				continue
			}
			ids, err := ix.st.FileIDsWithSuffix(cand)
			if err != nil {
				return 0, false, err
			}
			if len(ids) == 1 {
				return ids[0], true, nil
			}
		}
	}
	return 0, false, nil
}

// ingest history is in index.go; this file only owns import resolution.
