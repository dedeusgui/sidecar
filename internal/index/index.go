// Package index keeps the SQLite index of one worktree in sync with the files on disk
// and with Git. Syncs are incremental: only files that Git reports as changed, that are
// dirty in the working tree, or whose stat changed (outside Git) are read and parsed.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dedeusgui/sidecar/internal/extract"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/lexical"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/store"
)

const (
	maxSymbolsPerFile = 5000
	maxRefsPerFile    = 20000
	maxHistoryCommits = 2000
	maxFilesPerCommit = 50
)

// Options controls one Sync.
type Options struct {
	WithHistory bool // also ingest Git history (slower; the hook leaves this to a background run)
	Workers     int  // parse workers; 0 means min(GOMAXPROCS, 8)
}

// Report summarizes one Sync.
type Report struct {
	Files     int // files indexed after the sync
	Parsed    int // files read and parsed by this sync
	Removed   int // files removed from the index
	Complete  bool
	Recovered bool // the database was rebuilt because it was unreadable
	Head      string
	Duration  time.Duration
}

// Index is the index of one worktree.
type Index struct {
	root  string
	isGit bool
	st    *store.Store
}

// Open opens (or creates, or rebuilds) the index for a worktree root.
func Open(root, dbPath string) (*Index, Report, error) {
	st, recovered, err := store.OpenOrRecover(dbPath)
	if err != nil {
		return nil, Report{}, err
	}
	_, statErr := os.Stat(filepath.Join(root, ".git"))
	return &Index{root: root, isGit: statErr == nil, st: st}, Report{Recovered: recovered}, nil
}

// Close releases the database.
func (ix *Index) Close() error { return ix.st.Close() }

// Store exposes the underlying store for retrieval.
func (ix *Index) Store() *store.Store { return ix.st }

// Root returns the worktree root this index describes.
func (ix *Index) Root() string { return ix.root }

// Sync reconciles the index with the worktree. When ctx ends early the sync stops
// between files, reports Complete=false, and the next Sync resumes the remaining work.
func (ix *Index) Sync(ctx context.Context, opts Options) (Report, error) {
	start := time.Now()
	var rep Report
	plan, err := ix.plan(ctx)
	if err != nil {
		if ctx.Err() != nil {
			rep.Duration = time.Since(start)
			return rep, nil // deadline reached while planning: nothing changed, resume later
		}
		return rep, err
	}
	rep.Head = plan.head

	for _, p := range plan.removed {
		if err := ix.st.DeleteFile(p); err != nil {
			return rep, err
		}
		rep.Removed++
	}

	records, err := ix.st.FileRecords()
	if err != nil {
		return rep, err
	}
	parsed, complete, err := ix.parseAll(ctx, plan.changed, records, opts.Workers)
	if err != nil {
		if ctx.Err() != nil {
			rep.Duration = time.Since(start)
			return rep, nil
		}
		return rep, err
	}
	rep.Parsed = parsed.count
	rep.Removed += parsed.removed

	if ix.isGit {
		dirty := plan.dirty
		if !complete {
			// Keep the previous dirty set until a full pass has re-checked those files.
			prev, _ := ix.st.DirtyPaths()
			dirty = append(append([]string{}, prev...), plan.dirty...)
		}
		if err := ix.st.SetDirtyPaths(dirty); err != nil {
			return rep, err
		}
	}
	if complete {
		if err := ix.resolveImports(ctx); err != nil {
			return rep, err
		}
		if opts.WithHistory && ix.isGit && plan.head != "" {
			if err := ix.ingestHistory(ctx, plan.head); err != nil {
				return rep, err
			}
		}
		if ix.isGit {
			if err := ix.st.SetMeta("indexed_head", plan.head); err != nil {
				return rep, err
			}
		}
	}

	files, err := ix.st.FileRecords()
	if err != nil {
		return rep, err
	}
	rep.Files = len(files)
	rep.Complete = complete && ctx.Err() == nil
	rep.Duration = time.Since(start)
	return rep, nil
}

type plan struct {
	head    string
	changed []string
	removed []string
	dirty   []string
}

// plan decides what must be read. Git is the freshness oracle inside a repository;
// outside Git, stat fingerprints decide.
func (ix *Index) plan(ctx context.Context) (plan, error) {
	if ix.isGit {
		return ix.gitPlan(ctx)
	}
	return ix.statPlan()
}

func (ix *Index) gitPlan(ctx context.Context) (plan, error) {
	var p plan
	head, err := gitx.Head(ctx, ix.root)
	if err != nil && !errors.Is(err, gitx.ErrNoHead) {
		return p, err
	}
	p.head = head
	indexedHead, err := ix.st.GetMeta("indexed_head")
	if err != nil {
		return p, err
	}

	fullScan := head == "" || indexedHead == ""
	if !fullScan && head != indexedHead {
		changes, err := gitx.ChangedBetween(ctx, ix.root, indexedHead, head)
		if err != nil {
			fullScan = true // indexed commit is unknown to this clone (rebase, gc, new clone)
		} else {
			for _, c := range changes {
				switch c.Status {
				case "D":
					p.removed = append(p.removed, c.Path)
				case "R":
					p.removed = append(p.removed, c.OldPath)
					p.changed = append(p.changed, c.Path)
				default:
					p.changed = append(p.changed, c.Path)
				}
			}
		}
	}
	if fullScan {
		files, err := gitx.ListFiles(ctx, ix.root)
		if err != nil {
			return p, err
		}
		p.changed = append(p.changed, files...)
	}

	status, err := gitx.Status(ctx, ix.root)
	if err != nil {
		return p, err
	}
	for _, e := range status {
		if e.OldPath != "" {
			p.removed = append(p.removed, e.OldPath)
		}
		if e.Deleted {
			p.removed = append(p.removed, e.Path)
			continue
		}
		if e.Path != "" {
			p.changed = append(p.changed, e.Path)
			p.dirty = append(p.dirty, e.Path)
		}
	}
	// Files that were dirty last time may have been reverted to the committed content.
	prev, err := ix.st.DirtyPaths()
	if err != nil {
		return p, err
	}
	p.changed = append(p.changed, prev...)
	p.changed, p.removed = cleanPaths(p.changed), cleanPaths(p.removed)
	p.dirty = cleanPaths(p.dirty)
	return p, nil
}

func (ix *Index) statPlan() (plan, error) {
	var p plan
	files, err := repo.Walk(ix.root)
	if err != nil {
		return p, err
	}
	records, err := ix.st.FileRecords()
	if err != nil {
		return p, err
	}
	seen := make(map[string]bool, len(files))
	for _, rel := range files {
		seen[rel] = true
		rec, ok := records[rel]
		info, statErr := os.Lstat(filepath.Join(ix.root, filepath.FromSlash(rel)))
		if statErr != nil {
			continue
		}
		if !ok || rec.Size != info.Size() || rec.MtimeNS != info.ModTime().UnixNano() {
			p.changed = append(p.changed, rel)
		}
	}
	for rel := range records {
		if !seen[rel] {
			p.removed = append(p.removed, rel)
		}
	}
	sort.Strings(p.removed)
	return p, nil
}

type parseTotals struct {
	count   int
	removed int
}

// parseAll reads changed files with a worker pool and writes results from one goroutine.
// It reports complete=false when ctx ended before every file was handled.
func (ix *Index) parseAll(ctx context.Context, changed []string, records map[string]store.FileRecord, workers int) (parseTotals, bool, error) {
	var totals parseTotals
	if workers <= 0 {
		workers = min(runtime.GOMAXPROCS(0), 8)
	}
	jobs := make(chan string)
	results := make(chan outcome)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ex := extract.New()
			for rel := range jobs {
				prev, has := records[rel]
				results <- ix.parse(ex, rel, prev, has)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, rel := range changed {
			if ctx.Err() != nil {
				return
			}
			select {
			case jobs <- rel:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var firstErr error
	handled := 0
	for out := range results {
		handled++
		switch {
		case out.err != nil:
			if firstErr == nil {
				firstErr = out.err
			}
		case out.missing:
			if err := ix.st.DeleteFile(out.path); err != nil && firstErr == nil {
				firstErr = err
			}
			totals.removed++
		case out.unchanged:
			if err := ix.st.SetFileStat(out.path, out.file.Size, out.file.MtimeNS); err != nil && firstErr == nil {
				firstErr = err
			}
		default:
			if err := ix.st.ReplaceFile(out.file, out.syms, out.refs, out.imports); err != nil && firstErr == nil {
				firstErr = err
			}
			totals.count++
		}
	}
	if firstErr != nil {
		return totals, false, firstErr
	}
	complete := ctx.Err() == nil && handled == len(uniqueCount(changed))
	return totals, complete, nil
}

func uniqueCount(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

type outcome struct {
	path      string
	missing   bool
	unchanged bool
	file      store.FileRecord
	syms      []store.SymbolRecord
	refs      []store.RefRecord
	imports   []store.ImportRecord
	err       error
}

// parse produces the index rows for one file. Sensitive, binary and oversized files
// are recorded by name only; their content is never kept.
func (ix *Index) parse(ex *extract.Extractor, rel string, prev store.FileRecord, hasPrev bool) outcome {
	out := outcome{path: rel}
	if excludedPath(rel) {
		out.missing = true // treat like a removal so stale rows disappear
		return out
	}
	abs := filepath.Join(ix.root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		out.missing = true
		return out
	}
	if err != nil {
		out.err = err
		return out
	}
	lang, kind := repo.Classify(rel)
	out.file = store.FileRecord{
		Path: rel, Lang: lang, Kind: kind, Size: info.Size(),
		MtimeNS: info.ModTime().UnixNano(), PathTerms: pathTerms(rel),
	}
	switch {
	case !info.Mode().IsRegular():
		out.file.Skipped = "not_regular"
	case kind == "binary":
		out.file.Skipped = "binary"
	case repo.IsSensitive(rel):
		out.file.Skipped = "sensitive"
	}
	if out.file.Skipped != "" {
		out.syms = []store.SymbolRecord{fileSymbol(rel, 1, nil)}
		return out
	}

	data, reason := repo.ReadSource(abs, repo.MaxSourceBytes)
	if reason != "" {
		out.file.Skipped = reason
		out.syms = []store.SymbolRecord{fileSymbol(rel, 1, nil)}
		return out
	}
	sum := sha256.Sum256(data)
	out.file.Hash = hex.EncodeToString(sum[:])
	if hasPrev && prev.Hash == out.file.Hash && prev.Skipped == "" {
		out.unchanged = true
		out.file.Kind, out.file.Lang = prev.Kind, prev.Lang
		return out
	}

	lines := 1 + strings.Count(string(data), "\n")
	out.syms = []store.SymbolRecord{fileSymbol(rel, lines, data)}
	if lang == "" {
		return out
	}
	res, err := ex.File(lang, data)
	if err != nil {
		out.file.Skipped = "parse_error"
		return out
	}
	out.syms = append(out.syms, convertSymbols(res)...)
	out.refs = convertRefs(res)
	for _, m := range res.Imports {
		out.imports = append(out.imports, store.ImportRecord{Module: m})
	}
	return out
}

func convertSymbols(res extract.Result) []store.SymbolRecord {
	n := min(len(res.Symbols), maxSymbolsPerFile)
	out := make([]store.SymbolRecord, 0, n)
	for _, s := range res.Symbols[:n] {
		parent := s.Parent
		if parent >= n {
			parent = -1
		}
		out = append(out, store.SymbolRecord{
			Kind: s.Kind, Name: s.Name, Signature: s.Signature, Doc: s.Doc, Value: s.Value,
			Line: s.Line, EndLine: s.EndLine, Parent: parent,
			NameTerms: nameTerms(s.Name), SigTerms: terms(s.Signature), DocTerms: terms(s.Doc),
		})
	}
	return out
}

func convertRefs(res extract.Result) []store.RefRecord {
	n := min(len(res.Refs), maxRefsPerFile)
	out := make([]store.RefRecord, 0, n)
	for _, r := range res.Refs[:n] {
		out = append(out, store.RefRecord{From: r.From, Name: r.Name, Kind: r.Kind, Line: r.Line})
	}
	return out
}

// fileSymbol gives every file a searchable row, so queries can match by path or name.
func fileSymbol(rel string, lines int, data []byte) store.SymbolRecord {
	base := path.Base(rel)
	doc := ""
	if len(data) > 0 {
		doc = firstLineOf(data)
	}
	return store.SymbolRecord{
		Kind: "file", Name: base, Signature: rel, Doc: doc,
		Line: 1, EndLine: lines, Parent: -1,
		NameTerms: pathTerms(rel), SigTerms: terms(rel), DocTerms: terms(doc),
	}
}

func firstLineOf(data []byte) string {
	s := string(data)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(strings.TrimLeft(s, "#/*- "))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func nameTerms(name string) string {
	return strings.TrimSpace(lexical.Fold(name) + " " + terms(name))
}

func terms(text string) string {
	return strings.Join(lexical.Analyze(text), " ")
}

func pathTerms(rel string) string {
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	return strings.TrimSpace(lexical.Fold(stem) + " " + terms(rel))
}

// excludedPath keeps vendored and generated trees out of the index.
func excludedPath(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if repo.ExcludedDir(part) {
			return true
		}
	}
	return false
}

func cleanPaths(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		x = filepath.ToSlash(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// ingestHistory stores commits newer than the last ingested one.
func (ix *Index) ingestHistory(ctx context.Context, head string) error {
	indexed, err := ix.st.GetMeta("indexed_history_head")
	if err != nil {
		return err
	}
	if head == indexed {
		return nil
	}
	commits, err := gitx.ReadHistory(ctx, ix.root, indexed, maxHistoryCommits)
	if err != nil && indexed != "" {
		commits, err = gitx.ReadHistory(ctx, ix.root, "", maxHistoryCommits)
	}
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}
	recs := make([]store.CommitRecord, 0, len(commits))
	for _, c := range commits {
		rec := store.CommitRecord{Hash: c.Hash, Time: c.Time, Subject: c.Subject, SubjectTerms: terms(c.Subject)}
		for _, f := range c.Files {
			rec.Files = append(rec.Files, store.CommitFile{Status: f.Status, Path: filepath.ToSlash(f.Path), OldPath: filepath.ToSlash(f.OldPath)})
		}
		recs = append(recs, rec)
	}
	if err := ix.st.InsertCommits(recs, maxFilesPerCommit); err != nil {
		return err
	}
	return ix.st.SetMeta("indexed_history_head", head)
}
