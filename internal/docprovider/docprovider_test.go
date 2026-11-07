package docprovider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func find(deps []Dependency, name string) (Dependency, bool) {
	for _, d := range deps {
		if d.Name == name {
			return d, true
		}
	}
	return Dependency{}, false
}

func TestDependenciesFromManifestsKeepExactness(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, "go.mod", "module example.com/app\n\ngo 1.24\n\nrequire (\n\tgithub.com/a/lib v1.2.3\n\tgithub.com/b/ind v0.1.0 // indirect\n)\n\nrequire github.com/c/single v2.0.0\n")
	writeManifest(t, root, "web/package.json", `{"dependencies":{"react":"18.3.1","lodash":"^4.17.21"},"devDependencies":{"vite":"~5.0.0"}}`)
	writeManifest(t, root, "crates/core/Cargo.toml", "[package]\nname = \"core\"\nversion = \"0.1.0\"\n\n[dependencies]\nserde = \"=1.0.210\"\nanyhow = \"1.0\"\ntokio = { version = \"=1.40.0\", features = [\"rt\"] }\n")
	writeManifest(t, root, "py/pyproject.toml", "[project]\nname = \"svc\"\ndependencies = [\n  \"requests==2.31.0\",\n  \"click>=8.1\",\n]\n")
	writeManifest(t, root, "java/pom.xml", "<project><dependencies><dependency><groupId>org.x</groupId><artifactId>y</artifactId><version>3.4.5</version></dependency><dependency><groupId>org.x</groupId><artifactId>z</artifactId><version>${z.version}</version></dependency></dependencies></project>")
	writeManifest(t, root, "vendor/ignored/go.mod", "module ignored\n\nrequire github.com/no/no v9.9.9\n")

	deps, err := Dependencies(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		eco, version string
		exact        bool
	}{
		"github.com/a/lib":    {"go", "v1.2.3", true},
		"github.com/c/single": {"go", "v2.0.0", true},
		"react":               {"npm", "18.3.1", true},
		"lodash":              {"npm", "^4.17.21", false},
		"vite":                {"npm", "~5.0.0", false},
		"serde":               {"cargo", "=1.0.210", true},
		"anyhow":              {"cargo", "1.0", false},
		"tokio":               {"cargo", "=1.40.0", true},
		"requests":            {"pypi", "2.31.0", true},
		"click":               {"pypi", ">=8.1", false},
		"org.x:y":             {"maven", "3.4.5", true},
		"org.x:z":             {"maven", "${z.version}", false},
	}
	for name, w := range want {
		d, ok := find(deps, name)
		if !ok {
			t.Errorf("dependency %s missing", name)
			continue
		}
		if d.Ecosystem != w.eco || d.Version != w.version || d.Exact != w.exact {
			t.Errorf("%s = %+v, want ecosystem=%s version=%s exact=%v", name, d, w.eco, w.version, w.exact)
		}
	}
	if _, ok := find(deps, "github.com/no/no"); ok {
		t.Error("vendor manifest was read")
	}
	if d, _ := find(deps, "react"); d.Source != "web/package.json" {
		t.Errorf("source = %q, want manifest path", d.Source)
	}
}

func TestCacheReturnsOnlyExactVerifiedEntries(t *testing.T) {
	p := CacheProvider{Dir: t.TempDir()}
	dep := Dependency{Ecosystem: "go", Name: "github.com/a/lib", Version: "v1.2.3", Exact: true, Source: "go.mod"}
	ctx := context.Background()

	if _, err := p.Lookup(ctx, dep); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty cache: %v", err)
	}
	if err := p.Put(dep, "https://docs.example.com/lib/v1.2.3", "Use Lib.Do(ctx).\n"); err != nil {
		t.Fatal(err)
	}
	doc, err := p.Lookup(ctx, dep)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Library != dep.Name || doc.Version != dep.Version || doc.Source == "" || doc.Text != "Use Lib.Do(ctx).\n" {
		t.Fatalf("doc = %+v", doc)
	}

	// Another version of the same library is a different entry: never a silent substitute.
	if _, err := p.Lookup(ctx, Dependency{Ecosystem: "go", Name: "github.com/a/lib", Version: "v1.2.4", Exact: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other version served: %v", err)
	}
	// A range cannot identify a documented version.
	if _, err := p.Lookup(ctx, Dependency{Ecosystem: "npm", Name: "x", Version: "^1.0.0", Exact: false}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("range served: %v", err)
	}
	// Put refuses to store documentation without a source or for a range.
	if err := p.Put(dep, "  ", "text"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stored without provenance: %v", err)
	}
	if err := p.Put(Dependency{Ecosystem: "npm", Name: "x", Version: "^1.0.0"}, "https://x", "text"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stored a range: %v", err)
	}
}

func TestCacheRejectsTamperedText(t *testing.T) {
	p := CacheProvider{Dir: t.TempDir()}
	dep := Dependency{Ecosystem: "pypi", Name: "requests", Version: "2.31.0", Exact: true}
	if err := p.Put(dep, "https://docs.example.com/requests", "original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.entryDir(dep), "doc.md"), []byte("injected instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lookup(context.Background(), dep); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered text served: %v", err)
	}
}

func TestCacheRejectsMismatchedMetadata(t *testing.T) {
	p := CacheProvider{Dir: t.TempDir()}
	dep := Dependency{Ecosystem: "pypi", Name: "requests", Version: "2.31.0", Exact: true}
	if err := p.Put(dep, "https://docs.example.com/requests", "text"); err != nil {
		t.Fatal(err)
	}
	// Point the same folder at a different library: the metadata check must refuse it.
	other := Dependency{Ecosystem: "pypi", Name: "urllib3", Version: "2.31.0", Exact: true}
	if err := os.MkdirAll(p.entryDir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"meta.json", "doc.md"} {
		data, _ := os.ReadFile(filepath.Join(p.entryDir(dep), f))
		if err := os.WriteFile(filepath.Join(p.entryDir(other), f), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Lookup(context.Background(), other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("entry for another library served: %v", err)
	}
}
