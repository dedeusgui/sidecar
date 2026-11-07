// Package docprovider reads dependency versions from manifests and answers documentation
// lookups from a local, versioned cache. The core never uses the network: entries reach the
// cache only through Put, which requires a source and an exact version.
package docprovider

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/dedeusgui/sidecar/internal/repo"
)

// ErrNotFound means no cached entry matches the library, the exact version and its provenance.
var ErrNotFound = errors.New("documentação não encontrada para esta biblioteca e versão")

// ErrInvalid means a cache entry exists but fails validation, so it is never returned.
var ErrInvalid = errors.New("entrada de documentação inválida")

// maxManifest bounds the size of one manifest read.
const maxManifest = 1 << 20

// Dependency is one declared dependency. Exact is true only when the manifest pins one release.
type Dependency struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Exact     bool   `json:"exact"`
	Source    string `json:"source"` // manifest path, relative to the root
}

// Doc is a documentation excerpt with its provenance.
type Doc struct {
	Library string
	Version string
	Source  string
	SHA256  string
	Text    string
}

// Provider answers "docs for this exact library version".
type Provider interface {
	Name() string
	Lookup(ctx context.Context, dep Dependency) (Doc, error)
}

var manifests = map[string]bool{
	"go.mod": true, "package.json": true, "Cargo.toml": true, "pyproject.toml": true, "pom.xml": true,
}

// Dependencies lists the dependencies of every supported manifest under root, with exact
// versions only where the manifest states them. Ranges are kept as written and marked inexact.
func Dependencies(root string) ([]Dependency, error) {
	files, err := repo.Walk(root)
	if err != nil {
		return nil, err
	}
	var out []Dependency
	for _, rel := range files {
		if !manifests[filepath.Base(rel)] {
			continue
		}
		data, reason := repo.ReadSource(filepath.Join(root, filepath.FromSlash(rel)), maxManifest)
		if reason != "" {
			continue
		}
		deps := parse(filepath.Base(rel), data)
		for i := range deps {
			deps[i].Source = rel
		}
		out = append(out, deps...)
	}
	slices.SortFunc(out, func(a, b Dependency) int {
		if c := strings.Compare(a.Source, b.Source); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func parse(base string, data []byte) []Dependency {
	switch base {
	case "go.mod":
		return parseGoMod(data)
	case "package.json":
		return parsePackageJSON(data)
	case "Cargo.toml":
		return parseCargo(data)
	case "pyproject.toml":
		return parsePyproject(data)
	case "pom.xml":
		return parsePom(data)
	}
	return nil
}

var (
	semver      = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.+-]+)?$`)
	goRequire   = regexp.MustCompile(`^(\S+)\s+(v\S+)`)
	cargoInline = regexp.MustCompile(`version\s*=\s*"([^"]+)"`)
	pyReq       = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(?:\[[^\]]*\])?\s*(==|>=|<=|~=|!=|>|<)?\s*([^;\s,]+)?`)
	quoted      = regexp.MustCompile(`"([^"]+)"`)
)

func parseGoMod(data []byte) []Dependency {
	var out []Dependency
	inBlock := false
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "require (":
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		case !inBlock && strings.HasPrefix(line, "require "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		case !inBlock:
			continue
		}
		if m := goRequire.FindStringSubmatch(line); m != nil {
			out = append(out, Dependency{Ecosystem: "go", Name: m[1], Version: m[2], Exact: true})
		}
	}
	return out
}

func parsePackageJSON(data []byte) []Dependency {
	var doc struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []Dependency
	for _, set := range []map[string]string{doc.Dependencies, doc.DevDependencies} {
		for name, v := range set {
			out = append(out, Dependency{Ecosystem: "npm", Name: name, Version: v, Exact: semver.MatchString(v)})
		}
	}
	return out
}

// parseCargo reads the dependency tables line by line. Inline tables and "name = version"
// lines are recognised; dotted table headers such as [dependencies.foo] are not, so they are
// skipped rather than guessed.
func parseCargo(data []byte) []Dependency {
	var out []Dependency
	section := ""
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		if !strings.HasSuffix(section, "dependencies") || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		version := ""
		if strings.HasPrefix(value, "\"") {
			version = strings.Trim(value, "\"")
		} else if m := cargoInline.FindStringSubmatch(value); m != nil {
			version = m[1]
		}
		if version == "" || name == "" {
			continue
		}
		pinned := strings.HasPrefix(version, "=") && semver.MatchString(strings.TrimPrefix(version, "="))
		out = append(out, Dependency{Ecosystem: "cargo", Name: name, Version: version, Exact: pinned})
	}
	return out
}

// parsePyproject reads PEP 621 "dependencies = [...]" and Poetry tables. "==" pins are exact
// in PEP 508; a bare semver in Poetry means an exact pin.
func parsePyproject(data []byte) []Dependency {
	var out []Dependency
	text := string(data)
	if i := strings.Index(text, "dependencies = ["); i >= 0 {
		rest := text[i:]
		if j := strings.Index(rest, "]"); j >= 0 {
			for _, q := range quoted.FindAllStringSubmatch(rest[:j], -1) {
				m := pyReq.FindStringSubmatch(q[1])
				if m == nil {
					continue
				}
				// A "==" pin is stored as the bare release; any other requirement keeps its operator.
				version := m[2] + m[3]
				if m[2] == "==" {
					version = m[3]
				}
				out = append(out, Dependency{Ecosystem: "pypi", Name: m[1], Version: version, Exact: m[2] == "==" && m[3] != "" && !strings.Contains(m[3], "*")})
			}
		}
	}
	section := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		if section != "tool.poetry.dependencies" || !strings.Contains(line, "=") {
			continue
		}
		name, value, _ := strings.Cut(line, "=")
		name, version := strings.TrimSpace(name), strings.Trim(strings.TrimSpace(value), "\"")
		if name == "python" || version == "" {
			continue
		}
		out = append(out, Dependency{Ecosystem: "pypi", Name: name, Version: version, Exact: semver.MatchString(version)})
	}
	return out
}

func parsePom(data []byte) []Dependency {
	var doc struct {
		Dependencies []struct {
			GroupID    string `xml:"groupId"`
			ArtifactID string `xml:"artifactId"`
			Version    string `xml:"version"`
		} `xml:"dependencies>dependency"`
	}
	if xml.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []Dependency
	for _, d := range doc.Dependencies {
		// Property references and version ranges are not pins; the version is kept as written.
		exact := d.Version != "" && !strings.ContainsAny(d.Version, "${[(")
		out = append(out, Dependency{Ecosystem: "maven", Name: d.GroupID + ":" + d.ArtifactID, Version: d.Version, Exact: exact})
	}
	return out
}

// CacheProvider reads documentation stored under Dir/<ecosystem>/<library>/<version>/.
type CacheProvider struct {
	Dir string
}

// Name identifies the provider in diagnostics.
func (p CacheProvider) Name() string { return "cache-local" }

// Lookup returns an excerpt only when the entry's library and version match the dependency
// exactly, the dependency is pinned, the source is recorded, and the text matches its digest.
func (p CacheProvider) Lookup(_ context.Context, dep Dependency) (Doc, error) {
	if !dep.Exact {
		return Doc{}, fmt.Errorf("%w: versão %q não é exata", ErrNotFound, dep.Version)
	}
	dir := p.entryDir(dep)
	meta, err := readMeta(filepath.Join(dir, "meta.json"))
	if err != nil {
		return Doc{}, ErrNotFound
	}
	text, err := os.ReadFile(filepath.Join(dir, "doc.md"))
	if err != nil {
		return Doc{}, ErrNotFound
	}
	switch {
	case meta.Library != dep.Name || meta.Version != dep.Version:
		return Doc{}, fmt.Errorf("%w: entrada é de %s@%s", ErrInvalid, meta.Library, meta.Version)
	case meta.Source == "":
		return Doc{}, fmt.Errorf("%w: sem proveniência", ErrInvalid)
	case digest(text) != meta.SHA256:
		return Doc{}, fmt.Errorf("%w: conteúdo não corresponde ao digest", ErrInvalid)
	}
	return Doc{Library: meta.Library, Version: meta.Version, Source: meta.Source, SHA256: meta.SHA256, Text: string(text)}, nil
}

// Put stores documentation for one exact dependency version. The source is mandatory, so
// nothing enters the cache without provenance.
func (p CacheProvider) Put(dep Dependency, source, text string) error {
	if !dep.Exact {
		return fmt.Errorf("%w: só se armazena documentação de versão exata", ErrInvalid)
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("%w: proveniência obrigatória", ErrInvalid)
	}
	dir := p.entryDir(dep)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	meta, err := json.Marshal(cacheMeta{Library: dep.Name, Version: dep.Version, Source: source, SHA256: digest([]byte(text))})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte(text), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o600)
}

type cacheMeta struct {
	Library string `json:"library"`
	Version string `json:"version"`
	Source  string `json:"source"`
	SHA256  string `json:"sha256"`
}

func readMeta(path string) (cacheMeta, error) {
	var m cacheMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// entryDir maps a dependency to its folder. Names and versions can contain "/" or ":", so
// each segment is made safe and suffixed with a short digest that keeps distinct names apart.
func (p CacheProvider) entryDir(dep Dependency) string {
	return filepath.Join(p.Dir, safe(dep.Ecosystem), safe(dep.Name), safe(dep.Version))
}

func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String() + "-" + digest([]byte(s))[:8]
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
