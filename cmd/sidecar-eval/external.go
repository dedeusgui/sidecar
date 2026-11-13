package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dedeusgui/sidecar/internal/index"
)

// externalManifest describes a corpus whose repositories are already checked out locally.
// Nothing is cloned or checked out here: each repo must be clean and sit at its pinned commit,
// so a run cannot silently mix versions. Gold may be file-level (no name), as corpora label files.
type externalManifest struct {
	Repos map[string]pinnedRepo `json:"repos"`
	Cases []externalCase        `json:"cases"`
}

type pinnedRepo struct {
	Path     string `json:"path"`               // local checkout, absolute or relative to the working directory
	Commit   string `json:"commit"`             // full 40-character SHA the checkout must be at
	Source   string `json:"source"`             // where the corpus came from: URL or dataset identifier
	License  string `json:"license"`            // licence noted at pin time; required so redistribution stays a conscious choice
	Language string `json:"language,omitempty"` // primary language declared by external cohort manifests
}

// externalCase is a caseSpec plus the no-gold flag used by abstention strata.
type externalCase struct {
	caseSpec
	NoGold bool `json:"no_gold,omitempty"` // the correct answer is no context at all
}

var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// openExternal loads a pinned manifest and indexes every repo at its pinned commit.
func openExternal(path, tmp string) (casesFile, map[string]*index.Index) {
	m, err := loadExternal(path)
	must(err)
	indexes := map[string]*index.Index{}
	for name, r := range m.Repos {
		ix, _, err := index.Open(r.Path, filepath.Join(tmp, name+".db"))
		must(err)
		rep, err := ix.Sync(context.Background(), index.Options{WithHistory: true})
		must(err)
		fmt.Fprintf(os.Stderr, "indexed %-5s files=%d parsed=%d complete=%v commit=%s\n", name, rep.Files, rep.Parsed, rep.Complete, r.Commit[:12])
		indexes[name] = ix
	}
	return casesFile{Cases: casesOf(m)}, indexes
}

// casesOf turns the manifest cases into the evaluator's shape: no_gold becomes a negative case.
func casesOf(m externalManifest) []caseSpec {
	var out []caseSpec
	for _, c := range m.Cases {
		spec := c.caseSpec
		if c.NoGold {
			spec.Negative = true
		}
		out = append(out, spec)
	}
	return out
}

// loadExternal reads and validates a manifest. Unknown fields are errors, so a misspelt
// gold or pin cannot pass silently.
func loadExternal(path string) (externalManifest, error) {
	var m externalManifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	if len(m.Repos) == 0 || len(m.Cases) == 0 {
		return m, fmt.Errorf("%s: needs at least one repo and one case", path)
	}
	for name, r := range m.Repos {
		if err := verifyPin(r); err != nil {
			return m, fmt.Errorf("repo %s: %w", name, err)
		}
	}
	seen := map[string]bool{}
	for _, c := range m.Cases {
		if err := validCase(c, m.Repos, seen); err != nil {
			return m, err
		}
	}
	return m, nil
}

// verifyPin checks that the checkout is clean and at the pinned commit.
func verifyPin(r pinnedRepo) error {
	if !commitRe.MatchString(r.Commit) {
		return fmt.Errorf("commit must be a full 40-character lowercase SHA, got %q", r.Commit)
	}
	if strings.TrimSpace(r.Source) == "" || strings.TrimSpace(r.License) == "" {
		return fmt.Errorf("source and license are required")
	}
	head, err := git(r.Path, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("%s is not a readable git checkout: %w", r.Path, err)
	}
	if strings.TrimSpace(head) != r.Commit {
		return fmt.Errorf("%s is at %s, pinned %s", r.Path, strings.TrimSpace(head), r.Commit)
	}
	status, err := git(r.Path, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%s has uncommitted changes; a pinned run needs a clean checkout", r.Path)
	}
	return nil
}

func validCase(c externalCase, repos map[string]pinnedRepo, seen map[string]bool) error {
	if c.ID == "" || seen[c.ID] {
		return fmt.Errorf("case id %q is empty or repeated", c.ID)
	}
	seen[c.ID] = true
	if _, ok := repos[c.Repo]; !ok {
		return fmt.Errorf("case %s: unknown repo %q", c.ID, c.Repo)
	}
	if c.Split == "" || strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("case %s: split and prompt are required", c.ID)
	}
	if c.NoGold && len(c.Relevant) > 0 {
		return fmt.Errorf("case %s: no_gold cases must not list relevant items", c.ID)
	}
	if !c.NoGold && len(c.Relevant) == 0 {
		return fmt.Errorf("case %s: a positive case needs relevant items (or no_gold: true)", c.ID)
	}
	for _, g := range c.Relevant {
		if g.Path == "" {
			return fmt.Errorf("case %s: relevant item without path", c.ID)
		}
	}
	return nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// buildFixtures copies each fixture into a fresh Git repository and indexes it.
func buildFixtures(cf casesFile, base, tmp string) map[string]*index.Index {
	indexes := map[string]*index.Index{}
	for name, spec := range cf.Repos {
		root := filepath.Join(tmp, name)
		buildFixtureRepo(filepath.Join(base, spec.Fixture), root, spec)
		ix, _, err := index.Open(root, filepath.Join(tmp, name+".db"))
		must(err)
		rep, err := ix.Sync(context.Background(), index.Options{WithHistory: true})
		must(err)
		fmt.Fprintf(os.Stderr, "indexed %-5s files=%d parsed=%d complete=%v\n", name, rep.Files, rep.Parsed, rep.Complete)
		indexes[name] = ix
	}
	return indexes
}
