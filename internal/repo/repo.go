// Package repo discovers repository roots, classifies files and applies the read policy:
// sensitive files are never read, binaries and oversized files are never parsed, and
// symlinks are never followed.
package repo

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxSourceBytes is the largest file whose content is parsed.
const MaxSourceBytes = 512 * 1024

// MaxFiles bounds directory walks in non-git folders.
const MaxFiles = 50000

// FindRoot walks up from dir looking for a .git entry. Without one, dir itself is the root.
func FindRoot(dir string) (root string, isGit bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir, false
	}
	for cur := abs; ; {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, false
		}
		cur = parent
	}
}

var binaryExt = toSet(".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".zip", ".gz",
	".tar", ".jar", ".war", ".class", ".exe", ".dll", ".so", ".dylib", ".o", ".a", ".woff",
	".woff2", ".ttf", ".otf", ".mp3", ".mp4", ".mov", ".wasm", ".bin", ".db", ".sqlite")

var langByExt = map[string]string{
	".go": "go", ".py": "python", ".ts": "typescript", ".tsx": "tsx",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	".java": "java", ".cs": "c_sharp", ".sql": "sql", ".md": "markdown", ".markdown": "markdown",
}

var configExt = toSet(".yml", ".yaml", ".json", ".toml", ".xml", ".properties", ".gradle", ".ini", ".cfg", ".conf")

// discoveryLangByExt names languages that discovery recognises but the indexer does not parse.
// Classify deliberately ignores them: adding a grammar would change indexing and retrieval.
var discoveryLangByExt = map[string]string{
	".rs": "rust", ".php": "php", ".rb": "ruby", ".kt": "kotlin", ".swift": "swift",
	".c": "c", ".h": "c", ".cpp": "cpp", ".cc": "cpp", ".hpp": "cpp", ".sh": "shell",
}

// Classify returns the grammar name ("" when none), and the file kind:
// code, test, doc, config or binary.
func Classify(p string) (lang, kind string) {
	base := path.Base(p)
	ext := strings.ToLower(path.Ext(base))
	if binaryExt[ext] {
		return "", "binary"
	}
	if strings.EqualFold(base, "README") || strings.EqualFold(base, "CHANGELOG") {
		return "", "doc"
	}
	if configExt[ext] {
		return "", "config"
	}
	lang = langByExt[ext]
	if lang == "" {
		return "", ""
	}
	if lang == "markdown" {
		return lang, "doc"
	}
	if isTestPath(p, base, lang) {
		return lang, "test"
	}
	return lang, "code"
}

// LanguageOf names a file's language for repository discovery and returns its kind. It covers
// Classify's languages and discoveryLangByExt; lang is "" for files that are not source code.
func LanguageOf(p string) (lang, kind string) {
	lang, kind = Classify(p)
	if lang != "" || kind != "" {
		return lang, kind
	}
	base := path.Base(p)
	name := discoveryLangByExt[strings.ToLower(path.Ext(base))]
	switch {
	case name == "":
		return "", ""
	case isTestPath(p, base, name):
		return name, "test"
	}
	return name, "code"
}

var generatedNames = toSet("package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml",
	"go.sum", "Cargo.lock", "composer.lock", "poetry.lock", "Pipfile.lock", "Gemfile.lock", "gradle.lockfile")

var generatedSuffixes = []string{".pb.go", "_gen.go", "_pb2.py", ".min.js", ".min.css"}

// IsGenerated reports files people should not edit: lockfiles, minified bundles, codegen output
// and files whose leading bytes carry a generator marker. head is the file's first bytes, or nil.
func IsGenerated(p string, head []byte) bool {
	base := path.Base(p)
	if generatedNames[base] {
		return true
	}
	for _, s := range generatedSuffixes {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	text := string(head)
	return strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT") ||
		strings.Contains(text, "@generated")
}

func isTestPath(p, base, lang string) bool {
	lower := strings.ToLower(p)
	if strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "__tests__/") || strings.HasPrefix(lower, "test/") || strings.HasPrefix(lower, "tests/") {
		return true
	}
	switch lang {
	case "go":
		return strings.HasSuffix(base, "_test.go")
	case "python":
		return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
	case "java":
		return strings.HasSuffix(base, "Test.java") || strings.HasSuffix(base, "Tests.java") || strings.HasSuffix(base, "IT.java")
	case "c_sharp":
		return strings.HasSuffix(base, "Test.cs") || strings.HasSuffix(base, "Tests.cs")
	case "typescript", "tsx", "javascript":
		return strings.Contains(base, ".spec.") || strings.Contains(base, ".test.")
	}
	return false
}

var sensitiveNames = []string{".env", ".npmrc", ".pypirc", ".netrc", ".git-credentials",
	"credentials.json", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "service-account*.json"}

var sensitiveExt = toSet(".pem", ".key", ".p12", ".pfx", ".jks", ".kdbx", ".keystore")

// IsSensitive reports whether a path likely holds credentials. Such files are listed by
// name only and their content is never read.
func IsSensitive(p string) bool {
	base := strings.ToLower(path.Base(p))
	if sensitiveExt[path.Ext(base)] {
		return true
	}
	if strings.HasPrefix(base, ".env") {
		return true
	}
	for _, pat := range sensitiveNames {
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}

var excludedDirs = toSet(".git", "node_modules", "vendor", "dist", "build", "target", "out", "bin",
	"obj", "__pycache__", ".venv", "venv", ".gradle", ".idea", ".vs", "coverage", ".next", ".nuxt",
	".tox", ".mypy_cache", ".pytest_cache")

// Walk lists regular files under root for folders that are not git repositories.
// Symlinks, excluded directories and files past MaxFiles are skipped.
func Walk(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && excludedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= MaxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	return files, err
}

// ReadSource returns the content of a regular, non-binary file no larger than max.
// reason is "" on success, or one of: not_regular, too_large, binary, read_error.
func ReadSource(p string, max int64) (data []byte, reason string) {
	info, err := os.Lstat(p)
	if err != nil {
		return nil, "read_error"
	}
	if !info.Mode().IsRegular() {
		return nil, "not_regular"
	}
	if info.Size() > max {
		return nil, "too_large"
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, "read_error"
	}
	defer func() { _ = f.Close() }()
	data, err = io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, "read_error"
	}
	if int64(len(data)) > max {
		return nil, "too_large"
	}
	probe := data
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return nil, "binary"
	}
	return data, ""
}

// ExcludedDir reports whether a directory name is never walked.
func ExcludedDir(name string) bool { return excludedDirs[name] }

func toSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Kind returns the file kind for a path (code, test, doc, config, binary).
func Kind(p string) string {
	_, kind := Classify(p)
	return kind
}
