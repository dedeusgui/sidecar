package brief

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// Status separates what the index verified from what it only suggests. A claim is CONFIRMED only when a
// checkable fact backs it (a symbol exists at path:line, a test file names the symbol, an import resolves
// a call, a document agrees with the code). CANDIDATE is a heuristic relation. UNKNOWN is a gap the agent
// should not fill by assumption. CONFLICT is two cited sources that disagree; Sidecar does not pick one.
type Status string

const (
	Confirmed Status = "CONFIRMED"
	Candidate Status = "CANDIDATE"
	Unknown   Status = "UNKNOWN"
	Conflict  Status = "CONFLICT"
)

// Per-field caps keep the brief compact; the token budget of the block applies on top of them.
const (
	maxEntrypoints = 4
	maxConsumers   = 4
	maxTests       = 3
	maxContracts   = 3
	maxUnknowns    = 3
	maxHints       = 2
)

// Line is one traceable statement of the brief.
type Line struct {
	Field  string // goal | entrypoint | consumer | test | contract | surface | unknown | verify
	Status Status
	Text   string // always carries a path (and a line when known) or says what is missing
	Key    string // session-dedup identity
	FP     string // changes when the underlying source changes
}

// TaskBrief is the compact, task-oriented view of a retrieval Result. It adds no new analysis: every
// line comes from the evidence, contract findings, structural relations or repository fingerprint that
// retrieval already produced.
type TaskBrief struct {
	Lines []Line
}

// Build derives the brief. ok is false when the evidence is too thin for a brief to save the agent any
// discovery work, in which case the caller keeps the standard block. A brief needs one confirmed entry
// point plus at least one more fact (a consumer, a test, a contract, a conflict or a stated gap).
func Build(res retrieve.Result) (TaskBrief, bool) {
	var b TaskBrief
	var entries, consumers, tests []Line
	files := map[string]bool{}
	testFiles := map[string]bool{}
	confirmedEntry := false
	seenEntry := map[string]bool{}

	for _, ev := range res.Evidence {
		if ev.Note != "" || ev.Hit.Kind == "file" || ev.Hit.Kind == "section" {
			continue
		}
		loc := fmt.Sprintf("%s:%d", ev.Hit.Path, ev.Hit.Line)
		if _, kind := repo.Classify(ev.Hit.Path); kind == "test" {
			// A test symbol is not an entry point: it is a confirmed test that matches the request wording.
			testFiles[ev.Hit.Path] = true
			continue
		}
		isRel := len(ev.Relations) > 0 && ev.Source != "exact" && ev.Source != "lexical" && ev.Source != "contract"
		if isRel {
			// A structural neighbour: classify by relation, never as an entry point.
			for _, rel := range ev.Relations {
				switch {
				case strings.HasPrefix(rel, "calls:") || strings.HasPrefix(rel, "called_by:"):
					st := Candidate
					if strings.Contains(rel, "(import)") {
						st = Confirmed
					}
					label := "consumer"
					if strings.HasPrefix(rel, "called_by:") {
						label = "dependency"
					}
					consumers = append(consumers, Line{Field: "consumer", Status: st,
						Text: fmt.Sprintf("%s %s %s (%s)", label, ev.Hit.Kind, ev.Hit.Name, loc), Key: "brief|rel|" + ev.Key(), FP: ev.Fingerprint()})
					files[ev.Hit.Path] = true
				case strings.HasPrefix(rel, "tested_by:"):
					tf := strings.TrimPrefix(rel, "tested_by:")
					testFiles[tf] = true
				}
			}
			continue
		}
		if seenEntry[ev.Key()] {
			continue
		}
		seenEntry[ev.Key()] = true
		st := Candidate
		if ev.Verdict == "VERIFIED" || ev.Source == "exact" {
			st, confirmedEntry = Confirmed, true
		}
		entries = append(entries, Line{Field: "entrypoint", Status: st,
			Text: entryText(ev, loc), Key: "brief|entry|" + ev.Key(), FP: ev.Fingerprint()})
		files[ev.Hit.Path] = true
		for _, rel := range ev.Relations {
			if strings.HasPrefix(rel, "tested_by:") {
				testFiles[strings.TrimPrefix(rel, "tested_by:")] = true
			}
		}
	}
	var contracts, conflicts, unknowns []Line
	for _, f := range res.Trace.Contracts {
		loc := f.Path
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		switch f.Status {
		case retrieve.TestExpected:
			if f.Path != "" {
				testFiles[f.Path] = true
			}
		case retrieve.Documented:
			contracts = append(contracts, Line{Field: "contract", Status: Confirmed,
				Text: fmt.Sprintf("%s documented in %s%s", shortSubject(f.Subject), loc, claimOf(f)), Key: "brief|contract|" + f.Subject + "|" + f.Path, FP: f.Claim})
		case retrieve.ConflictCandidate:
			conflicts = append(conflicts, Line{Field: "contract", Status: Conflict,
				Text: fmt.Sprintf("%s: code and %s disagree%s; Sidecar does not choose an authority", shortSubject(f.Subject), loc, claimOf(f)),
				Key:  "brief|conflict|" + f.Subject + "|" + f.Path, FP: f.Claim})
		case retrieve.Unverified:
			unknowns = append(unknowns, Line{Field: "unknown", Status: Candidate,
				Text: fmt.Sprintf("%s mentioned in %s but not verified against the code", shortSubject(f.Subject), loc), Key: "brief|unv|" + f.Subject + "|" + f.Path, FP: f.Claim})
		}
	}
	sortedTests := make([]string, 0, len(testFiles))
	for tf := range testFiles {
		sortedTests = append(sortedTests, tf)
	}
	sort.Strings(sortedTests)
	for _, tf := range sortedTests {
		tests = append(tests, Line{Field: "test", Status: Confirmed, Text: "test file references the entry point by name: " + tf, Key: "brief|test|" + tf, FP: tf})
		files[tf] = true
	}
	if confirmedEntry && len(tests) == 0 {
		unknowns = append(unknowns, Line{Field: "unknown", Status: Unknown,
			Text: "no test file in the index references " + entryName(entries[0].Text) + "; behaviour is not covered by a known test", Key: "brief|notest|" + entries[0].Key, FP: entries[0].FP})
	}

	if !confirmedEntry {
		return b, false
	}
	extra := len(consumers) + len(tests) + len(contracts) + len(conflicts) + len(unknowns)
	if extra == 0 {
		return b, false
	}

	if g := goalLine(res); g != nil {
		b.Lines = append(b.Lines, *g)
	}
	b.Lines = append(b.Lines, capLines(entries, maxEntrypoints)...)
	b.Lines = append(b.Lines, capLines(conflicts, maxContracts)...)
	b.Lines = append(b.Lines, capLines(contracts, maxContracts)...)
	b.Lines = append(b.Lines, capLines(consumers, maxConsumers)...)
	b.Lines = append(b.Lines, capLines(tests, maxTests)...)
	if len(files) > 1 {
		names := make([]string, 0, len(files))
		for f := range files {
			names = append(names, f)
		}
		sort.Strings(names)
		if len(names) > 6 {
			names = append(names[:6], fmt.Sprintf("+%d more", len(names)-6))
		}
		b.Lines = append(b.Lines, Line{Field: "surface", Status: Candidate,
			Text: fmt.Sprintf("estimated change surface from the evidence: %d files (%s)", len(files), strings.Join(names, ", ")), Key: "brief|surface|" + strings.Join(names, ","), FP: strings.Join(names, ",")})
	}
	b.Lines = append(b.Lines, capLines(unknowns, maxUnknowns)...)
	b.Lines = append(b.Lines, capLines(verifyHints(sortedTests, res), maxHints)...)
	return b, true
}

// TaskItems converts the brief into context items. The standard evidence sections are dropped (the
// brief already names the same symbols, so nothing is shown twice); git notes for dirty files stay.
func TaskItems(res retrieve.Result) ([]ctxbuild.Item, bool) {
	b, ok := Build(res)
	if !ok {
		return nil, false
	}
	var out []ctxbuild.Item
	for _, l := range b.Lines {
		out = append(out, ctxbuild.Item{Section: ctxbuild.TaskBrief, Key: l.Key, Fingerprint: l.FP,
			Text: fmt.Sprintf("[%s] %s: %s", l.Status, l.Field, l.Text)})
	}
	for _, it := range items(res) {
		if it.Section == ctxbuild.Git || it.Section == ctxbuild.TaskCheck {
			out = append(out, it)
		}
	}
	return out, true
}

func goalLine(res retrieve.Result) *Line {
	in := res.Trace.Intent
	if len(in.Kinds) == 0 || (len(in.Kinds) == 1 && in.Kinds[0] == "general") {
		return nil
	}
	return &Line{Field: "goal", Status: Candidate,
		Text: fmt.Sprintf("request reads as %s (intent score %.2f, from prompt wording only; the agent owns the goal)", strings.Join(in.Kinds, "+"), in.Confidence),
		Key:  "brief|goal|" + strings.Join(in.Kinds, "+"), FP: strings.Join(in.Kinds, "+")}
}

func verifyHints(testFiles []string, res retrieve.Result) []Line {
	var out []Line
	for _, tf := range testFiles {
		if len(out) >= maxHints {
			break
		}
		hint := "run the project's existing tests that reference it: " + tf
		if strings.HasSuffix(tf, "_test.go") {
			hint = fmt.Sprintf("existing Go test covers it; candidate command: go test ./%s (not run by Sidecar)", path.Dir(tf))
		}
		out = append(out, Line{Field: "verify", Status: Candidate, Text: hint, Key: "brief|verify|" + tf, FP: tf})
	}
	return out
}

func claimOf(f retrieve.Finding) string {
	if f.Claim == "" {
		return ""
	}
	return " (states " + f.Claim + ")"
}

func shortSubject(s string) string {
	if i := strings.LastIndex(s, " "); i > 0 && strings.Contains(s[i:], ":") {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func entryName(text string) string {
	f := strings.Fields(text)
	if len(f) >= 2 {
		return f[1]
	}
	return text
}

func capLines(l []Line, n int) []Line {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// entryText names the symbol with its location and, when the index has one, its signature, so the agent
// can decide whether to open the file. Signatures come from the index (a fact), never from a model.
func entryText(ev retrieve.Evidence, loc string) string {
	text := fmt.Sprintf("%s %s (%s)", ev.Hit.Kind, ev.Hit.Name, loc)
	if sig := strings.Join(strings.Fields(ev.Hit.Signature), " "); sig != "" && ev.Hit.Kind != "file" {
		r := []rune(sig)
		if len(r) > 110 {
			sig = string(r[:110]) + "…"
		}
		text += " — " + sig
	}
	return text
}
