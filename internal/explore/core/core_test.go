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

func TestBuildProseFallbackExpandsWithoutVerifiedSeed(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/command.py": `class Command:
    def main(self, args):
        """Turn command line arguments into callback parameters."""
        return self.make_context(args)

    def make_context(self, args):
        """Build a context from command line arguments."""
        return self.parse_args(args)

    def parse_args(self, args):
        """Parse command line options."""
        return args
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
	var mainSym store.SymbolHit
	for _, s := range symbols {
		if s.Name == "main" {
			mainSym = s
		}
	}
	records, err := st.FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	// Prose-only query: all evidence is INFERRED/lexical (no VERIFIED or exact seed).
	input := BuildInput{Records: records, Evidence: retrieve.Result{Evidence: []retrieve.Evidence{
		{Hit: mainSym, Verdict: "INFERRED", Source: "lexical", Matched: []string{"arguments", "parameters"}},
	}}}
	result, err := Build(context.Background(), st, Options{Root: root, Query: "how are command line arguments turned into callback parameters?"}, input)
	if err != nil {
		t.Fatal(err)
	}
	var foundParseArgs, foundEdge bool
	for _, f := range result.Files {
		for _, s := range f.Symbols {
			if s.Name == "parse_args" {
				foundParseArgs = true
			}
		}
	}
	for _, e := range result.Relations {
		if e.From.Name == "make_context" && e.To.Name == "parse_args" {
			foundEdge = true
		}
	}
	if !foundParseArgs || !foundEdge {
		t.Fatalf("prose-only fallback did not expand call chain: parse_args=%v edge=%v rels=%+v", foundParseArgs, foundEdge, result.Relations)
	}
}

func TestBuildCrossFileChainAllowsExtraSlotsAndThirdBranch(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/Monolog/Logger.php": `<?php
namespace Monolog;
use Monolog\Handler\AbstractProcessingHandler;
class Logger {
    public function addRecord($msg) {
        return $this->handle($msg);
    }
}
`,
		"src/Monolog/Handler/AbstractProcessingHandler.php": `<?php
namespace Monolog\Handler;
use Monolog\Handler\StreamHandler;
class AbstractProcessingHandler {
    public function handle($record) {
        $this->branchOne($record);
        $this->branchTwo($record);
        return $this->write($record);
    }
    protected function branchOne($r) {}
    protected function branchTwo($r) {}
}
`,
		"src/Monolog/Handler/StreamHandler.php": `<?php
namespace Monolog\Handler;
class StreamHandler {
    protected function write($record) {
        return $record;
    }
}
`,
		"src/noise1.php": "<?php\nfunction noiseOne() { return 'record'; }\n",
		"src/noise2.php": "<?php\nfunction noiseTwo() { return 'record'; }\n",
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
	var addRecord, noise1, noise2 store.SymbolHit
	for _, s := range mustSymbols(t, st, "src/Monolog/Logger.php") {
		if s.Name == "addRecord" {
			addRecord = s
		}
	}
	for _, s := range mustSymbols(t, st, "src/noise1.php") {
		if s.Name == "noiseOne" {
			noise1 = s
		}
	}
	for _, s := range mustSymbols(t, st, "src/noise2.php") {
		if s.Name == "noiseTwo" {
			noise2 = s
		}
	}
	records, err := st.FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	// Default BudgetFor(<200) sets maxFiles=3; noise1+noise2 fill all 3 initial file slots.
	// Verified cross-file chain neighbors (AbstractProcessingHandler.php, StreamHandler.php) must still be added.
	input := BuildInput{Records: records, Evidence: retrieve.Result{Evidence: []retrieve.Evidence{
		{Hit: addRecord, Verdict: "VERIFIED", Source: "exact", Matched: []string{"addRecord"}},
		{Hit: noise1, Verdict: "INFERRED", Source: "lexical", Matched: []string{"record"}},
		{Hit: noise2, Verdict: "INFERRED", Source: "lexical", Matched: []string{"record"}},
	}}}
	result, err := Build(context.Background(), st, Options{Root: root, Query: "how does addRecord write a record?"}, input)
	if err != nil {
		t.Fatal(err)
	}
	shown := map[string]bool{}
	for _, f := range result.Files {
		shown[f.Path] = true
	}
	if !shown["src/Monolog/Handler/AbstractProcessingHandler.php"] || !shown["src/Monolog/Handler/StreamHandler.php"] {
		t.Fatalf("cross-file call-chain files starved by maxFiles: files=%+v", result.Files)
	}
	var foundWriteEdge bool
	for _, e := range result.Relations {
		if e.From.Name == "handle" && e.To.Name == "write" && e.Status == Verified {
			foundWriteEdge = true
		}
	}
	if !foundWriteEdge {
		t.Fatalf("3rd callee branch handle -> write missing: %+v", result.Relations)
	}
}

func mustSymbols(t *testing.T, st *store.Store, path string) []store.SymbolHit {
	t.Helper()
	syms, err := st.SymbolsInFile(path, 32)
	if err != nil {
		t.Fatal(err)
	}
	return syms
}

func TestMemberKeysAndIsIdent(t *testing.T) {
	got := MemberKeys("How does Logger::info or res.send() call route! and valid? work?")
	want := []string{"Logger", "info", "res", "send", "route!", "valid?"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("MemberKeys = %v, want %v", got, want)
	}
	for _, valid := range []string{"route!", "valid?", "Logger", "res_1"} {
		if !IsIdent(valid) {
			t.Errorf("IsIdent(%q) = false, want true", valid)
		}
	}
	for _, invalid := range []string{"", "!", "a!b", "1abc"} {
		if IsIdent(invalid) {
			t.Errorf("IsIdent(%q) = true, want false", invalid)
		}
	}
}
