package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
)

// Regression: equal scores between same-named symbols in different files were broken by database row
// ID, which depends on the order the indexer wrote the files, so the ranking flipped between index builds
// (seen on serde-json serialize_str, ten-repos R05-3).
func TestRankingDoesNotDependOnIndexWriteOrder(t *testing.T) {
	files := map[string]string{
		"value/ser.go": "package value\n\n// SerializeStr writes a string as JSON text.\nfunc SerializeStr(s string) string { return s }\n",
		"ser/ser.go":   "package ser\n\n// SerializeStr writes a string as JSON text.\nfunc SerializeStr(s string) string { return s }\n",
	}
	order := func(first, second string) []string {
		root := t.TempDir()
		write := func(rel string) {
			p := filepath.Join(root, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(files[rel]), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ix.Close() }()
		write(first)
		if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
			t.Fatal(err)
		}
		write(second)
		if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
			t.Fatal(err)
		}
		var paths []string
		for _, engine := range []string{"", "v2"} {
			res, err := Search(context.Background(), ix.Store(), "where is a string written as JSON text?", Options{EngineVersion: engine, Root: root})
			if err != nil {
				t.Fatal(err)
			}
			for _, ev := range res.Evidence {
				paths = append(paths, engine+":"+ev.Hit.Path)
			}
		}
		return paths
	}
	a, b := order("value/ser.go", "ser/ser.go"), order("ser/ser.go", "value/ser.go")
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("different result sizes: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("ranking depends on index write order:\n%v\n%v", a, b)
		}
	}
}
