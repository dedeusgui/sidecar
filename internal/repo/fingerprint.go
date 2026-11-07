package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/dedeusgui/sidecar/internal/gitx"
)

// Fingerprint is what a repository observably contains. It is read from files only: no build
// script runs, no package is installed, and README text is never followed as an instruction.
type Fingerprint struct {
	Provenance  string          `json:"provenance"`     // "git ls-files" or "filesystem walk"
	Git         bool            `json:"git"`            // the root is a Git worktree
	Head        string          `json:"head,omitempty"` // HEAD commit, when Git has one
	Hash        string          `json:"hash"`           // invalidation key: HEAD plus manifest contents
	Monorepo    bool            `json:"monorepo"`       // several manifest roots or a workspace file
	Files       int             `json:"files"`
	Kinds       map[string]int  `json:"kinds"`
	Generated   int             `json:"generated"`
	Languages   []LanguageStat  `json:"languages"`
	Manifests   []Manifest      `json:"manifests"`
	Tests       TestConventions `json:"tests"`
	Entrypoints []string        `json:"entrypoints,omitempty"`
}

// LanguageStat counts the files of one language and says how Sidecar reads them.
type LanguageStat struct {
	Name    string `json:"name"`
	Support string `json:"support"` // ast_treesitter | text_rules | lexical_fallback
	Code    int    `json:"code"`
	Test    int    `json:"test"`
	Doc     int    `json:"doc"`
}

// Manifest is a build or dependency file with the main dependencies it declares.
type Manifest struct {
	Path         string       `json:"path"`
	Ecosystem    string       `json:"ecosystem"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// Dependency is a declared dependency with its version spec as written, when observable.
type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// TestConventions says where tests live and which test frameworks the manifests declare.
type TestConventions struct {
	Files      int      `json:"files"`
	Dirs       []string `json:"dirs,omitempty"` // test directory names, e.g. tests, __tests__
	Colocated  int      `json:"colocated"`      // test files outside any test directory
	Frameworks []string `json:"frameworks,omitempty"`
}

const (
	maxManifests     = 200
	maxManifestBytes = 256 * 1024
	maxDeps          = 40
	maxEntrypoints   = 50
	headBytes        = 512 // generator markers sit in the first lines of a file
)

var manifestNames = map[string]string{
	"go.mod": "go", "pom.xml": "maven", "build.gradle": "gradle", "build.gradle.kts": "gradle",
	"package.json": "npm", "tsconfig.json": "typescript", "Cargo.toml": "cargo",
	"pyproject.toml": "python", "requirements.txt": "python", "composer.json": "composer",
}

var workspaceFiles = toSet("go.work", "pnpm-workspace.yaml", "lerna.json", "nx.json", "turbo.json")

var testDirNames = toSet("test", "tests", "__tests__", "spec", "specs")

var testFrameworkNames = []string{"jest", "vitest", "mocha", "pytest", "junit", "xunit", "nunit",
	"mstest", "rspec", "phpunit", "kotest", "testify"}

// Discover reads the repository at root without running anything in it. It never fails:
// unreadable or sensitive files are skipped, so a partial fingerprint still reaches the caller.
func Discover(ctx context.Context, root string) Fingerprint {
	fp := Fingerprint{Provenance: "filesystem walk", Kinds: map[string]int{}}
	files, err := gitx.ListFiles(ctx, root)
	if err == nil {
		fp.Git, fp.Provenance = true, "git ls-files"
		if head, err := gitx.Head(ctx, root); err == nil {
			fp.Head = head
		}
	} else {
		files, _ = Walk(root)
	}

	type manifest struct{ path, eco string }
	var manifests []manifest
	langs := map[string]*LanguageStat{}
	testDirs := map[string]bool{}
	entrypoints := map[string]bool{}
	workspace := false
	goTests := 0
	for _, p := range files {
		if excludedPath(p) {
			continue
		}
		fp.Files++
		base := path.Base(p)
		lang, kind := LanguageOf(p)
		var head []byte
		if abs := filepath.Join(root, filepath.FromSlash(p)); (kind == "code" || kind == "test") && !IsSensitive(p) && Contained(root, abs) {
			head = headOf(abs)
		}
		if IsGenerated(p, head) {
			fp.Generated++
		}
		if kind != "" && kind != "binary" {
			fp.Kinds[kind]++
		}
		if lang != "" {
			st := langs[lang]
			if st == nil {
				st = &LanguageStat{Name: lang, Support: supportOf(lang)}
				langs[lang] = st
			}
			switch kind {
			case "code":
				st.Code++
			case "test":
				st.Test++
			case "doc":
				st.Doc++
			}
		}
		if kind == "test" {
			fp.Tests.Files++
			if dir := testDirOf(p); dir != "" {
				testDirs[dir] = true
			} else {
				fp.Tests.Colocated++
			}
			if lang == "go" {
				goTests++
			}
		}
		parts := strings.Split(p, "/")
		switch {
		case parts[0] == "cmd" && len(parts) >= 3:
			entrypoints["cmd/"+parts[1]] = true
		case parts[0] == "src" && len(parts) >= 2:
			entrypoints["src/"] = true
		case strings.HasPrefix(base, "main.") && kind == "code":
			entrypoints[p] = true
		}
		if eco := manifestEcosystem(p); eco != "" && len(manifests) < maxManifests {
			manifests = append(manifests, manifest{p, eco})
		}
		if !strings.Contains(p, "/") && workspaceFiles[base] {
			workspace = true
		}
	}

	slices.SortFunc(manifests, func(a, b manifest) int { return strings.Compare(a.path, b.path) })
	h := sha256.New()
	_, _ = io.WriteString(h, "v1\n"+fp.Head+"\n")
	roots := map[string]bool{}
	for _, m := range manifests {
		abs := filepath.Join(root, filepath.FromSlash(m.path))
		data, reason := ReadSource(abs, maxManifestBytes)
		if reason == "" && !Contained(root, abs) {
			data, reason = nil, "outside_root"
		}
		sum := sha256.Sum256(data)
		_, _ = io.WriteString(h, m.path+"\x00"+reason+"\x00"+hex.EncodeToString(sum[:])+"\n")
		man := Manifest{Path: m.path, Ecosystem: m.eco}
		if reason == "" {
			man.Dependencies = parseDeps(m.eco, path.Base(m.path), data)
		}
		fp.Manifests = append(fp.Manifests, man)
		if m.eco != "typescript" {
			roots[path.Dir(m.path)] = true
		}
	}
	fp.Hash = hex.EncodeToString(h.Sum(nil))
	fp.Monorepo = len(roots) > 1 || workspace

	for _, st := range langs {
		fp.Languages = append(fp.Languages, *st)
	}
	slices.SortFunc(fp.Languages, func(a, b LanguageStat) int {
		if ta, tb := a.Code+a.Test+a.Doc, b.Code+b.Test+b.Doc; ta != tb {
			return tb - ta
		}
		return strings.Compare(a.Name, b.Name)
	})
	fp.Tests.Dirs = sortedKeys(testDirs, 0)
	fp.Tests.Frameworks = frameworksOf(fp.Manifests, goTests > 0)
	fp.Entrypoints = sortedKeys(entrypoints, maxEntrypoints)
	return fp
}

// QuickHash names the state fp was read from without walking the tree: HEAD plus the size and
// mtime of each manifest fp recorded. ok is false outside Git, where a new manifest would not
// change HEAD and so would go unnoticed.
// shortcut: uncommitted new manifests and source files are seen only after the next commit or manifest edit, upgrade by hashing git status when stale fingerprints are reported.
func QuickHash(ctx context.Context, root string, fp Fingerprint) (string, bool) {
	if !fp.Git {
		return "", false
	}
	head, err := gitx.Head(ctx, root)
	if err != nil {
		return "", false
	}
	h := sha256.New()
	_, _ = io.WriteString(h, "v1\n"+head+"\n")
	for _, m := range fp.Manifests {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(m.Path)))
		if err != nil {
			_, _ = io.WriteString(h, m.Path+"\x00missing\n")
			continue
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", m.Path, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// excludedPath reports whether any directory in p is one the walk never enters.
func excludedPath(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if ExcludedDir(seg) {
			return true
		}
	}
	return false
}

// testDirOf returns the first directory of p that is a conventional test directory.
func testDirOf(p string) string {
	dirs := strings.Split(path.Dir(p), "/")
	for _, seg := range dirs {
		if testDirNames[seg] {
			return seg
		}
	}
	return ""
}

// supportOf says how Sidecar reads a language: tree-sitter grammar, text rules, or lexical text.
func supportOf(lang string) string {
	if lang == "markdown" || lang == "sql" {
		return "text_rules"
	}
	for _, g := range langByExt {
		if g == lang {
			return "ast_treesitter"
		}
	}
	return "lexical_fallback"
}

func manifestEcosystem(p string) string {
	base := path.Base(p)
	if eco, ok := manifestNames[base]; ok {
		return eco
	}
	switch path.Ext(base) {
	case ".sln":
		return "dotnet"
	case ".csproj":
		return "dotnet"
	}
	return ""
}

// headOf returns the first bytes of a regular file, or nil. Symlinks are not followed.
func headOf(abs string) []byte {
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, headBytes)
	n, _ := io.ReadFull(f, buf)
	return buf[:n]
}

// frameworksOf lists the test frameworks named by declared dependencies, plus go test when the
// repository has Go test files, which need no declaration.
func frameworksOf(manifests []Manifest, goTests bool) []string {
	found := map[string]bool{}
	for _, m := range manifests {
		for _, d := range m.Dependencies {
			name := strings.ToLower(d.Name)
			for _, fw := range testFrameworkNames {
				if strings.Contains(name, fw) {
					found[fw] = true
				}
			}
		}
	}
	if goTests {
		found["go test"] = true
	}
	return sortedKeys(found, 0)
}

// sortedKeys returns the keys of m in order, truncated to limit when limit > 0.
func sortedKeys[V any](m map[string]V, limit int) []string {
	keys := slices.Sorted(maps.Keys(m))
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}

// parseDeps reads the main dependencies of one manifest. Unparseable content yields none.
func parseDeps(eco, base string, data []byte) []Dependency {
	switch eco {
	case "go":
		return goDeps(data)
	case "npm":
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) != nil {
			return nil
		}
		return mapDeps(pkg.Dependencies, pkg.DevDependencies)
	case "composer":
		var pkg struct {
			Require    map[string]string `json:"require"`
			RequireDev map[string]string `json:"require-dev"`
		}
		if json.Unmarshal(data, &pkg) != nil {
			return nil
		}
		return mapDeps(pkg.Require, pkg.RequireDev)
	case "maven":
		var pom struct {
			Deps []struct {
				GroupID    string `xml:"groupId"`
				ArtifactID string `xml:"artifactId"`
				Version    string `xml:"version"`
			} `xml:"dependencies>dependency"`
		}
		if xml.Unmarshal(data, &pom) != nil {
			return nil
		}
		var out []Dependency
		for _, d := range pom.Deps {
			out = append(out, Dependency{Name: d.GroupID + ":" + d.ArtifactID, Version: d.Version})
		}
		return capDeps(out)
	case "gradle":
		var out []Dependency
		for _, m := range gradleDepRe.FindAllSubmatch(data, -1) {
			f := strings.Split(string(m[1]), ":")
			if len(f) < 2 {
				continue
			}
			d := Dependency{Name: f[0] + ":" + f[1]}
			if len(f) >= 3 {
				d.Version = f[2]
			}
			out = append(out, d)
		}
		return capDeps(out)
	case "cargo":
		return tomlDeps(data, map[string]bool{"dependencies": true, "dev-dependencies": true})
	case "python":
		if base == "requirements.txt" {
			return requirementDeps(data)
		}
		return pyprojectDeps(data)
	case "dotnet":
		if !strings.HasSuffix(base, ".csproj") {
			return nil
		}
		var out []Dependency
		for _, m := range csprojDepRe.FindAllSubmatch(data, -1) {
			out = append(out, Dependency{Name: string(m[1]), Version: string(m[2])})
		}
		return capDeps(out)
	}
	return nil
}

var (
	gradleDepRe   = regexp.MustCompile(`(?m)^\s*(?:implementation|api|compileOnly|runtimeOnly|testImplementation|testRuntimeOnly)\s*\(?\s*["']([^"']+)["']`)
	csprojDepRe   = regexp.MustCompile(`<PackageReference\s+Include="([^"]+)"(?:\s+Version="([^"]*)")?`)
	tomlSectionRe = regexp.MustCompile(`^\s*\[([^\]]+)\]`)
	tomlKeyRe     = regexp.MustCompile(`^\s*"?([A-Za-z0-9_.\-]+)"?\s*=\s*(.+?)\s*$`)
	tomlVersionRe = regexp.MustCompile(`version\s*=\s*"([^"]*)"`)
	projectDepsRe = regexp.MustCompile(`(?s)\[project\][^\[]*?dependencies\s*=\s*\[(.*?)\]`)
	quotedRe      = regexp.MustCompile(`"([^"]+)"`)
	pep508Re      = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)\s*(.*?)\s*(?:;.*)?$`)
	requirementRe = regexp.MustCompile(`^([A-Za-z0-9_.\-]+)\s*((?:==|>=|<=|~=|!=|>|<)[^\s;#]*)?`)
)

func goDeps(data []byte) []Dependency {
	var out []Dependency
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "require (" {
			inBlock = true
			continue
		}
		if inBlock && t == ")" {
			inBlock = false
			continue
		}
		if !inBlock {
			if !strings.HasPrefix(t, "require ") {
				continue
			}
			t = strings.TrimPrefix(t, "require ")
		}
		if strings.Contains(t, "// indirect") {
			continue
		}
		if f := strings.Fields(t); len(f) >= 2 {
			out = append(out, Dependency{Name: f[0], Version: f[1]})
		}
	}
	return capDeps(out)
}

// tomlDeps reads "name = version" and "name = { version = ... }" lines inside the given tables.
func tomlDeps(data []byte, tables map[string]bool) []Dependency {
	var out []Dependency
	in := false
	for _, line := range strings.Split(string(data), "\n") {
		if m := tomlSectionRe.FindStringSubmatch(line); m != nil {
			in = tables[m[1]]
			continue
		}
		m := tomlKeyRe.FindStringSubmatch(line)
		if !in || m == nil {
			continue
		}
		d := Dependency{Name: m[1]}
		if strings.HasPrefix(m[2], `"`) {
			d.Version = strings.Trim(m[2], `"`)
		} else if vm := tomlVersionRe.FindStringSubmatch(m[2]); vm != nil {
			d.Version = vm[1]
		}
		out = append(out, d)
	}
	return capDeps(out)
}

// pyprojectDeps reads PEP 508 strings of [project] dependencies and Poetry tables.
func pyprojectDeps(data []byte) []Dependency {
	out := tomlDeps(data, map[string]bool{"tool.poetry.dependencies": true, "tool.poetry.dev-dependencies": true})
	if m := projectDepsRe.FindSubmatch(data); m != nil {
		for _, q := range quotedRe.FindAllSubmatch(m[1], -1) {
			if d := pep508Re.FindSubmatch(q[1]); d != nil {
				out = append(out, Dependency{Name: string(d[1]), Version: string(d[2])})
			}
		}
	}
	return capDeps(out)
}

func requirementDeps(data []byte) []Dependency {
	var out []Dependency
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || t[0] == '#' || t[0] == '-' {
			continue
		}
		if m := requirementRe.FindStringSubmatch(t); m != nil {
			out = append(out, Dependency{Name: m[1], Version: m[2]})
		}
	}
	return capDeps(out)
}

// mapDeps merges declared dependency maps into a name-sorted list.
func mapDeps(sets ...map[string]string) []Dependency {
	merged := map[string]string{}
	for _, m := range sets {
		for k, v := range m {
			merged[k] = v
		}
	}
	var out []Dependency
	for _, k := range sortedKeys(merged, 0) {
		out = append(out, Dependency{Name: k, Version: merged[k]})
	}
	return capDeps(out)
}

func capDeps(deps []Dependency) []Dependency {
	if len(deps) > maxDeps {
		return deps[:maxDeps]
	}
	return deps
}
