package index

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/store"
)

func mkRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openIndex(t *testing.T, root string) *Index {
	t.Helper()
	ix, rep, err := Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if rep.Recovered {
		t.Fatal("fresh index reported recovery")
	}
	t.Cleanup(func() { _ = ix.Close() })
	return ix
}

func mustSync(t *testing.T, ix *Index, opts Options) Report {
	t.Helper()
	rep, err := ix.Sync(context.Background(), opts)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !rep.Complete {
		t.Fatalf("Sync reported incomplete without a deadline: %+v", rep)
	}
	return rep
}

const handlerGo = `package release

// ManualReleaseHandler releases vehicles waiting in the queue.
type ManualReleaseHandler struct{}

// Release advances the first vehicle.
func (h *ManualReleaseHandler) Release() error { return nil }
`

func TestSyncIndexesRepositoryAndFindsSymbols(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "release/handler.go", handlerGo)
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	rep := mustSync(t, ix, Options{})
	if rep.Parsed == 0 {
		t.Fatalf("nothing parsed on first sync: %+v", rep)
	}
	hits, err := ix.Store().SearchSymbols("manual release", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, h := range hits {
		if h.Name == "Release" && h.Kind == "method" && h.Path == "release/handler.go" && h.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("method Release with location not found for 'manual release': %+v", hits)
	}
}

// countNamed counts hits for a symbol name, ignoring the per-file "file" rows.
func countNamed(hits []store.SymbolHit, name string) int {
	n := 0
	for _, h := range hits {
		if h.Name == name && h.Kind != "file" {
			n++
		}
	}
	return n
}

func TestSecondSyncWithoutChangesParsesNothing(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "a.go", "package a\n\nfunc Alpha() {}\n")
	put(t, root, "b.go", "package a\n\nfunc Beta() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	if rep := mustSync(t, ix, Options{}); rep.Parsed != 0 {
		t.Fatalf("unchanged repo re-parsed %d files", rep.Parsed)
	}
	put(t, root, "a.go", "package a\n\nfunc Alpha2() {}\n")
	if rep := mustSync(t, ix, Options{}); rep.Parsed != 1 {
		t.Fatalf("one edited file should parse exactly once, parsed %d", rep.Parsed)
	}
	if hits, _ := ix.Store().SearchSymbols("alpha2", 10); countNamed(hits, "Alpha2") != 1 {
		t.Fatalf("edited symbol not searchable: %+v", hits)
	}
}

func TestUntrackedAndDeletedFilesFollowWorkingTree(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "keep.go", "package k\n\nfunc Keep() {}\n")
	put(t, root, "gone.go", "package k\n\nfunc Gone() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	put(t, root, "fresh.go", "package k\n\nfunc Fresh() {}\n")
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("fresh", 10); countNamed(hits, "Fresh") != 1 {
		t.Fatalf("untracked file not indexed: %+v", hits)
	}
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("gone", 10); countNamed(hits, "Gone") != 0 {
		t.Fatalf("deleted file still indexed: %+v", hits)
	}
}

func TestRevertedEditRestoresCommittedSymbols(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "f.go", "package f\n\nfunc Original() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	put(t, root, "f.go", "package f\n\nfunc Renamed() {}\n")
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("renamed", 10); countNamed(hits, "Renamed") != 1 {
		t.Fatalf("edit not indexed: %+v", hits)
	}
	git(t, root, "checkout", "--", "f.go")
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("renamed", 10); countNamed(hits, "Renamed") != 0 {
		t.Fatalf("stale edit survived revert: %+v", hits)
	}
	if hits, _ := ix.Store().SearchSymbols("original", 10); countNamed(hits, "Original") != 1 {
		t.Fatalf("committed symbol not restored after revert: %+v", hits)
	}
}

func TestSensitiveFilesAreRecordedButNeverParsed(t *testing.T) {
	root := mkRepo(t)
	put(t, root, ".env", "PAYMENT_TOKEN=abc123secret\n")
	put(t, root, "main.go", "package main\n\nfunc Main() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")
	put(t, root, ".env", "PAYMENT_TOKEN=rotated\n")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	recs, err := ix.Store().FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	if recs[".env"].Skipped != "sensitive" {
		t.Fatalf(".env skipped reason = %q, want sensitive", recs[".env"].Skipped)
	}
	if hits, _ := ix.Store().SearchSymbols("payment token rotated", 10); len(hits) != 0 {
		t.Fatalf("secret file content leaked into the index: %+v", hits)
	}
}

func TestNonGitFolderIsIndexedAndTracked(t *testing.T) {
	root := t.TempDir()
	put(t, root, "service.py", "def process_manual_release():\n    return 1\n")
	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("manual release", 10); len(hits) == 0 {
		t.Fatal("plain folder not indexed")
	}
	if rep := mustSync(t, ix, Options{}); rep.Parsed != 0 {
		t.Fatalf("plain folder re-parsed %d files without changes", rep.Parsed)
	}
	put(t, root, "service.py", "def process_automatic_dispatch():\n    return 2\n")
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("automatic dispatch", 10); countNamed(hits, "process_automatic_dispatch") != 1 {
		t.Fatalf("plain folder edit not indexed: %+v", hits)
	}
}

func TestDeadlineLeavesIndexResumable(t *testing.T) {
	root := mkRepo(t)
	for i := 0; i < 30; i++ {
		put(t, root, filepathFor(i), "package p\n\nfunc F"+strings.Repeat("x", i%5)+"() {}\n")
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	rep, err := ix.Sync(cancelled, Options{})
	if err != nil {
		t.Fatalf("cancelled Sync returned error: %v", err)
	}
	if rep.Complete {
		t.Fatalf("cancelled Sync claimed completion: %+v", rep)
	}
	rep = mustSync(t, ix, Options{})
	recs, _ := ix.Store().FileRecords()
	if len(recs) != 30 {
		t.Fatalf("resumed sync indexed %d files, want 30 (report %+v)", len(recs), rep)
	}
}

func TestImportsResolveToRepositoryFiles(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "web/app.ts", "import { Invoice } from './billing/invoice';\nexport const x = 1;\n")
	put(t, root, "web/billing/invoice.ts", "export class Invoice {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	got, err := ix.Store().ResolvedImportPaths("web/app.ts")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "web/billing/invoice.ts" {
		t.Fatalf("resolved imports = %v, want [web/billing/invoice.ts]", got)
	}
}

func TestHistoryIsIngestedIncrementallyAndCountsCoChanges(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "queue.go", "package q\n\nfunc Q() {}\n")
	put(t, root, "queue_test.go", "package q\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "feat queue")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{WithHistory: true})
	put(t, root, "queue.go", "package q\n\nfunc Q2() {}\n")
	git(t, root, "commit", "-q", "-am", "fix queue")
	mustSync(t, ix, Options{WithHistory: true})
	put(t, root, "queue.go", "package q\n\nfunc Q3() {}\n")
	git(t, root, "commit", "-q", "-am", "again queue")
	mustSync(t, ix, Options{WithHistory: true})

	co, err := ix.Store().CoChanged("queue.go", 10)
	if err != nil {
		t.Fatal(err)
	}
	// Each of the three commits touched queue.go; only the first also added queue_test.go.
	if co["queue_test.go"] != 1 {
		t.Fatalf("co-change for queue_test.go = %d, want 1 (commits must not be duplicated): %v", co["queue_test.go"], co)
	}
}

func filepathFor(i int) string {
	return filepath.ToSlash(filepath.Join("pkg", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".go"))
}

func TestParentLinkPointsAtTheContainingSymbol(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "queue.go", "package q\n\n// Queue holds waiting vehicles.\ntype Queue struct{}\n\n// Push adds a vehicle.\nfunc (q *Queue) Push() {}\n\nconst MaxSlots = 3\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	hits, err := ix.Store().SearchSymbols("push", 10)
	if err != nil || len(hits) == 0 {
		t.Fatalf("Push not found: %v %v", hits, err)
	}
	var method store.SymbolHit
	for _, h := range hits {
		if h.Name == "Push" {
			method = h
		}
	}
	parent, ok, err := ix.Store().ParentOf(method.ID)
	if err != nil || !ok || parent.Name != "Queue" {
		t.Fatalf("parent of Push = %+v (ok=%v, err=%v), want Queue", parent, ok, err)
	}
	if parent.ID == method.ID {
		t.Fatal("a symbol is recorded as its own parent")
	}
}

func TestIndexRebuildsWhenExtractionLogicChanges(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "a.go", "package a\n\nfunc Alpha() {}\n")
	put(t, root, "b.go", "package a\n\nfunc Beta() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})
	// Simulate an index written by an older extraction: rows are stale even though files are unchanged.
	if err := ix.Store().ResetExtractVersion(0); err != nil {
		t.Fatal(err)
	}
	rep := mustSync(t, ix, Options{})
	if rep.Parsed != 2 {
		t.Fatalf("stale index was not rebuilt: parsed %d files, want 2", rep.Parsed)
	}
	if rep := mustSync(t, ix, Options{}); rep.Parsed != 0 {
		t.Fatalf("rebuilt index should be incremental again, parsed %d", rep.Parsed)
	}
}

func TestBoundedSyncParsesAtMostNFilesAndResumes(t *testing.T) {
	root := mkRepo(t)
	for i := 0; i < 30; i++ {
		put(t, root, filepathFor(i), "package p\n\nfunc F"+strings.Repeat("y", i%7)+"() {}\n")
	}
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	rep, err := ix.Sync(context.Background(), Options{MaxFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Parsed > 10 || rep.Complete {
		t.Fatalf("bounded sync parsed %d files (complete=%v), want at most 10 and incomplete", rep.Parsed, rep.Complete)
	}
	rep2 := mustSync(t, ix, Options{})
	if rep.Parsed+rep2.Parsed != 30 {
		t.Fatalf("work was not resumed exactly: %d + %d, want 30", rep.Parsed, rep2.Parsed)
	}
}

func TestCommittedHeadChangePlansOnlyChangedPaths(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "a.go", "package p\n\nfunc Original() {}\n")
	put(t, root, "b.go", "package p\n\nfunc Untouched() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "initial")
	ix := openIndex(t, root)
	mustSync(t, ix, Options{})

	put(t, root, "a.go", "package p\n\nfunc Updated() {}\n")
	git(t, root, "add", "a.go")
	git(t, root, "commit", "-q", "-m", "update a")
	plan, err := ix.plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.changed) != 1 || plan.changed[0] != "a.go" {
		t.Fatalf("only a.go should be in the incremental plan, got %+v", plan)
	}
	mustSync(t, ix, Options{})
	if hits, _ := ix.Store().SearchSymbols("updated", 10); countNamed(hits, "Updated") != 1 {
		t.Fatalf("updated symbol missing after commit: %+v", hits)
	}
}

func TestFullGitRescanRemovesFilesMissingFromCleanWorktree(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "keep.go", "package p\n\nfunc Keep() {}\n")
	put(t, root, "old.go", "package p\n\nfunc Obsolete() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "initial")
	ix := openIndex(t, root)
	mustSync(t, ix, Options{})

	git(t, root, "rm", "old.go")
	git(t, root, "commit", "-q", "-m", "remove old file")
	// Simulate an index pointing to a commit lost during history replacement.
	// The working tree is clean, so git status alone cannot discover old.go.
	if err := ix.Store().SetMeta("indexed_head", "invalid-pruned-commit"); err != nil {
		t.Fatal(err)
	}
	rep := mustSync(t, ix, Options{})
	if rep.Removed == 0 {
		t.Fatalf("full reconciliation did not prune missing file: %+v", rep)
	}
	records, err := ix.Store().FileRecords()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := records["old.go"]; ok {
		t.Fatal("stale old.go file record survived full reconciliation")
	}
	if hits, _ := ix.Store().SearchSymbols("obsolete", 10); countNamed(hits, "Obsolete") != 0 {
		t.Fatalf("deleted symbol remains searchable: %+v", hits)
	}
}

func TestPHPRubyAndCommonJSImportResolution(t *testing.T) {
	root := mkRepo(t)
	put(t, root, "src/Monolog/Logger.php", `<?php
namespace Monolog;
use Monolog\Handler\HandlerInterface;
use Monolog\Handler\AbstractProcessingHandler;
class Logger {
    public function addRecord($msg) { return $this->handle($msg); }
}
`)
	put(t, root, "src/Monolog/Handler/HandlerInterface.php", `<?php
namespace Monolog\Handler;
interface HandlerInterface {
    public function handle($record);
}
`)
	put(t, root, "src/Monolog/Handler/AbstractProcessingHandler.php", `<?php
namespace Monolog\Handler;
abstract class AbstractProcessingHandler implements HandlerInterface {
    public function handle($record) { return $this->write($record); }
    abstract protected function write($record);
}
`)
	put(t, root, "lib/sinatra.rb", "require_relative 'sinatra/base'\n")
	put(t, root, "lib/sinatra/base.rb", "class Base\n  def dispatch!\n    route!\n  end\n  def route!\n    1\n  end\nend\n")
	put(t, root, "test/app_spec.rb", "require 'sinatra/base'\n")
	put(t, root, "lib/response.js", "var utils = require('./utils');\nexports.send = function send(x) { return utils.norm(x); };\n")
	put(t, root, "lib/utils.js", "exports.norm = function norm(x) { return x; };\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")

	ix := openIndex(t, root)
	mustSync(t, ix, Options{})

	checkImports := func(from string, want ...string) {
		t.Helper()
		got, err := ix.Store().ResolvedImportPaths(from)
		if err != nil {
			t.Fatal(err)
		}
		gotSet := map[string]bool{}
		for _, p := range got {
			gotSet[p] = true
		}
		for _, w := range want {
			if !gotSet[w] {
				t.Errorf("ResolvedImportPaths(%q) = %v, missing %q", from, got, w)
			}
		}
	}
	checkImports("src/Monolog/Logger.php",
		"src/Monolog/Handler/HandlerInterface.php",
		"src/Monolog/Handler/AbstractProcessingHandler.php",
	)
	checkImports("lib/sinatra.rb", "lib/sinatra/base.rb")
	checkImports("test/app_spec.rb", "lib/sinatra/base.rb")
	checkImports("lib/response.js", "lib/utils.js")
}
