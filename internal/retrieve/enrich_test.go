package retrieve

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/docprovider"
)

func docItems(evs []Evidence) []Evidence {
	var out []Evidence
	for _, ev := range evs {
		if ev.Source == "docprovider" {
			out = append(out, ev)
		}
	}
	return out
}

func TestNamedPinnedDependencyWithLocalDocsReachesV2Pipeline(t *testing.T) {
	ix, root := contractRepo(t, map[string]string{
		"go.mod":      "module example.com/app\n\ngo 1.22\n\nrequire modernc.org/sqlite v1.2.3\n",
		"store/db.go": "package store\n\nfunc OpenDB(path string) error {\n\treturn nil\n}\n",
	})
	home := t.TempDir()
	t.Setenv("SIDECAR_HOME", home)
	dep := docprovider.Dependency{Ecosystem: "go", Name: "modernc.org/sqlite", Version: "v1.2.3", Exact: true}
	if err := (docprovider.CacheProvider{Dir: filepath.Join(home, "docs")}).Put(dep, "fixture-docs", "# sqlite\nWAL mode is enabled by the driver.\n"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := Search(ctx, ix.Store(), "How does modernc.org/sqlite open a WAL database?", Options{EngineVersion: "v2", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.Fingerprint == nil || len(res.Trace.Fingerprint.Manifests) != 1 {
		t.Fatalf("v2 did not populate the repository fingerprint: %+v", res.Trace.Fingerprint)
	}
	ev := docItems(res.Evidence)
	if len(ev) != 1 || ev[0].Verdict != "VERIFIED" || ev[0].Section != ctxbuild.Contracts {
		t.Fatalf("docprovider evidence = %+v, want one VERIFIED contract item", ev)
	}
	for _, want := range []string{"modernc.org/sqlite@v1.2.3", "source fixture-docs"} {
		if !strings.Contains(ev[0].Note, want) {
			t.Errorf("note %q lacks provenance %q", ev[0].Note, want)
		}
	}

	res, err = Search(ctx, ix.Store(), "How does OpenDB open a database?", Options{EngineVersion: "v2", Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(docItems(res.Evidence)); n != 0 {
		t.Fatalf("prompt that does not name the dependency got %d docprovider items", n)
	}
}

func TestLanguageServerIsNotConsultedUnlessOptedInAndTrusted(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		trusted bool
		want    int // calls to the server hook
	}{
		{"opt-in off", "", true, 0},
		{"root not trusted", "1", false, 0},
		{"opted in and trusted", "1", true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SIDECAR_LSP", c.env)
			ix, root := contractRepo(t, map[string]string{
				"svc/orders.go": "package svc\n\nfunc ProcessOrder(id string) error {\n\treturn nil\n}\n",
			})
			calls := 0
			opts := Options{
				EngineVersion: "v2", Root: root,
				LSPTrusted: func(string) bool { return c.trusted },
				LSPServer: func(_ context.Context, _, _ string) string {
					calls++
					return "gopls"
				},
			}
			res, err := Search(context.Background(), ix.Store(), "Where is ProcessOrder defined?", opts)
			if err != nil {
				t.Fatal(err)
			}
			if calls != c.want {
				t.Fatalf("server hook called %d times, want %d", calls, c.want)
			}
			noted := false
			for _, ev := range res.Evidence {
				if ev.Verdict == "VERIFIED" && strings.Contains(strings.Join(ev.Relations, ";"), "lsp gopls trusted") {
					noted = true
				}
			}
			if noted != (c.want > 0) {
				t.Fatalf("language-server note present = %v, want %v: %+v", noted, c.want > 0, res.Evidence)
			}
		})
	}
}
