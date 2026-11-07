package repo

import "path/filepath"

// Contained reports whether abs, with every symlink in its path resolved, is root or lies under
// it. Lstat inspects only the last path component, so a tracked file under a directory that was
// replaced by a symlink passes Lstat while its content lies outside the repository.
func Contained(root, abs string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, resolved)
	return err == nil && filepath.IsLocal(rel)
}
