package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestBuildExpandsRelevantClassCallChainAndDropsPathOnlyMatches(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/command.py": `class Command:
    def main(self, args):
        """Turn command line arguments into callback parameters."""
        ctx = self.make_context(args)
        ctx.invoke(self.callback, **ctx.params)
        return ctx

    def make_context(self, args):
        """Build a context from command line arguments."""
        self.parse_args(args)

    def parse_args(self, args):
        """Parse arguments into keyword parameters."""
        opts = self.parser.parse_args(args)
        self.ctx.params = vars(opts)
        return opts
`,
		"src/command_helpers.py": `def unrelated():
    return "no command behavior here"
`,
	}
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	st := ix.Store()
	symbols, err := st.SymbolsInFile("src/command.py", 16)
	if err != nil {
		t.Fatal(err)
	}
	var commandClass, unrelated store.SymbolHit
	for _, hit := range symbols {
		if hit.Name == "Command" {
			commandClass = hit
		}
	}
	if commandClass.ID == 0 {
		t.Fatalf("Command class not indexed: %+v", symbols)
	}
	otherSymbols, err := st.SymbolsInFile("src/command_helpers.py", 16)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range otherSymbols {
		if hit.Name == "unrelated" {
			unrelated = hit
		}
	}
	records, err := st.FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	input := BuildInput{Records: records, Evidence: retrieve.Result{Evidence: []retrieve.Evidence{
		{Hit: commandClass, Verdict: "VERIFIED", Source: "lexical", Matched: []string{"command"}},
		{Hit: unrelated, Verdict: "INFERRED", Source: "lexical", Matched: []string{"command"}},
	}}}
	result, err := Build(context.Background(), st, Options{Root: root, Query: "How does Command turn command line arguments into callback parameters?"}, input)
	if err != nil {
		t.Fatal(err)
	}
	var parseArgs, unrelatedShown, bodyPresent, callVerified bool
	for _, file := range result.Files {
		for _, symbol := range file.Symbols {
			if symbol.Name == "parse_args" {
				parseArgs = true
			}
			if symbol.Name == "unrelated" {
				unrelatedShown = true
			}
		}
		for _, snippet := range file.Snippets {
			bodyPresent = bodyPresent || strings.Contains(snippet.Text, "self.parser.parse_args(args)")
		}
	}
	for _, edge := range result.Relations {
		callVerified = callVerified || edge.From.Name == "make_context" && edge.To.Name == "parse_args" && edge.Status == Verified
	}
	if !parseArgs || !bodyPresent || !callVerified {
		t.Fatalf("call chain missing: parse_args=%v body=%v verified edge=%v, files=%+v relations=%+v", parseArgs, bodyPresent, callVerified, result.Files, result.Relations)
	}
	if unrelatedShown {
		t.Fatal("symbol matched only through its path leaked into Explore output")
	}
	if result.Budget.UsedChars > result.Budget.MaxChars || len(result.Files) > result.Budget.MaxFiles {
		t.Fatalf("call-chain expansion exceeded limits: %+v", result.Budget)
	}
}
