package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

const maxReplayVariants = 32

// replayConfig is the versioned, reviewable input of the offline replay. `active` mirrors the
// compiled defaults (checked by --replay-check and a test); `variants` and `grid` are hypotheses.
type replayConfig struct {
	Version  string                     `json:"version"`
	Note     string                     `json:"note,omitempty"`
	Active   retrieve.Tuning            `json:"active"`
	Variants map[string]retrieve.Tuning `json:"variants,omitempty"`
	// Grid varies one or more parameters (json names); the cartesian product is bounded by maxReplayVariants.
	Grid map[string][]float64 `json:"grid,omitempty"`
}

type namedTuning struct {
	Name   string          `json:"name"`
	Tuning retrieve.Tuning `json:"tuning"`
}

type replayVariantReport struct {
	Name         string             `json:"name"`
	Tuning       retrieve.Tuning    `json:"tuning"`
	Summary      map[string]summary `json:"summary_by_split"`
	Regressions  []caseChange       `json:"regressions_vs_active"`
	Improvements []caseChange       `json:"improvements_vs_active"`
}

type replayCapture struct {
	Variant string                    `json:"variant"`
	Case    string                    `json:"case"`
	Top     []retrieve.CandidateTrace `json:"candidates"`
}

type replayRunKey struct {
	CasesSHA256  string            `json:"cases_sha256"`
	ConfigSHA256 string            `json:"config_sha256"`
	GitHead      string            `json:"git_head"`
	Indexes      map[string]string `json:"index_sha256_by_repo"`
	Go           string            `json:"go"`
}

func loadReplayConfig(path string) (replayConfig, []byte, error) {
	var cfg replayConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, raw, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Version == "" {
		return cfg, raw, fmt.Errorf("%s: version is required", path)
	}
	return cfg, raw, nil
}

// checkActiveTuning is the rollback guard: the committed `active` must equal the compiled defaults.
func checkActiveTuning(path string) error {
	cfg, _, err := loadReplayConfig(path)
	if err != nil {
		return err
	}
	if cfg.Active != retrieve.DefaultTuning() {
		return fmt.Errorf("%s: active tuning %+v differs from compiled defaults %+v", path, cfg.Active, retrieve.DefaultTuning())
	}
	return nil
}

// expandVariants returns the active tuning first, then explicit variants, then the grid, all validated.
func expandVariants(cfg replayConfig) ([]namedTuning, error) {
	out := []namedTuning{{"active", cfg.Active}}
	names := make([]string, 0, len(cfg.Variants))
	for n := range cfg.Variants {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, namedTuning{n, cfg.Variants[n]})
	}
	keys := make([]string, 0, len(cfg.Grid))
	for k := range cfg.Grid {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	combos := []map[string]float64{{}}
	for _, k := range keys {
		var next []map[string]float64
		for _, c := range combos {
			for _, v := range cfg.Grid[k] {
				m := map[string]float64{k: v}
				for ck, cv := range c {
					m[ck] = cv
				}
				next = append(next, m)
			}
		}
		combos = next
		if len(combos)+len(out) > maxReplayVariants {
			return nil, fmt.Errorf("grid expands beyond %d variants; shrink it (the grid is a bounded hypothesis set, not a search)", maxReplayVariants)
		}
	}
	if len(keys) > 0 {
		for _, c := range combos {
			base, _ := json.Marshal(cfg.Active)
			var m map[string]float64
			_ = json.Unmarshal(base, &m)
			var parts []string
			for _, k := range keys {
				if _, ok := m[k]; !ok {
					return nil, fmt.Errorf("grid parameter %q is not a tuning field", k)
				}
				m[k] = c[k]
				parts = append(parts, fmt.Sprintf("%s=%g", k, c[k]))
			}
			raw, _ := json.Marshal(m)
			var t retrieve.Tuning
			if err := json.Unmarshal(raw, &t); err != nil {
				return nil, err
			}
			out = append(out, namedTuning{strings.Join(parts, ","), t})
		}
	}
	seen := map[string]bool{}
	for _, v := range out {
		if !v.Tuning.Valid() {
			return nil, fmt.Errorf("variant %q has invalid tuning %+v", v.Name, v.Tuning)
		}
		if seen[v.Name] {
			return nil, fmt.Errorf("duplicate variant name %q", v.Name)
		}
		seen[v.Name] = true
	}
	return out, nil
}

func sha(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func indexFingerprint(ix *index.Index) string {
	all, err := ix.Store().AllSymbols()
	if err != nil {
		return "error:" + err.Error()
	}
	lines := make([]string, 0, len(all))
	for _, h := range all {
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%d", h.Path, h.Kind, h.Name, h.Line))
	}
	sort.Strings(lines)
	return sha([]byte(strings.Join(lines, "\n")))
}

func gitHead() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// runReplay replays the frozen cases over every variant. Nothing calls a model or the network, so a
// hypothesis costs one pipeline pass. The active variant is run twice to prove the replay is deterministic.
func runReplay(path string, cf casesFile, indexes map[string]*index.Index, outPath string) error {
	cfg, raw, err := loadReplayConfig(path)
	if err != nil {
		return err
	}
	variants, err := expandVariants(cfg)
	if err != nil {
		return err
	}
	casesRaw, _ := json.Marshal(cf.Cases)
	key := replayRunKey{CasesSHA256: sha(casesRaw), ConfigSHA256: sha(raw), GitHead: gitHead(), Go: hardware()["go"].(string),
		Indexes: map[string]string{}}
	for name, ix := range indexes {
		key.Indexes[name] = indexFingerprint(ix)
	}
	var all []caseOutcome
	var captures []replayCapture
	runVariant := func(name string, t retrieve.Tuning, capture bool) ([]caseOutcome, error) {
		tt := t
		run := retrieveRunWith(retrieve.Options{EngineVersion: "v2", Tuning: &tt})
		var outs []caseOutcome
		for _, c := range cf.Cases {
			res, err := run(indexes[c.Repo], c.Prompt, nil)
			if err != nil {
				return nil, err
			}
			outs = append(outs, score(name, c, res))
			if capture {
				captures = append(captures, replayCapture{Variant: name, Case: c.ID, Top: res.cands})
			}
		}
		return outs, nil
	}
	byVariant := map[string][]caseOutcome{}
	for _, v := range variants {
		outs, err := runVariant(v.Name, v.Tuning, true)
		if err != nil {
			return err
		}
		byVariant[v.Name] = outs
		all = append(all, outs...)
	}
	again, err := runVariant("active", cfg.Active, false)
	if err != nil {
		return err
	}
	deterministic := true
	for i, o := range byVariant["active"] {
		if strings.Join(o.Top, ";") != strings.Join(again[i].Top, ";") || o.Empty != again[i].Empty {
			deterministic = false
			break
		}
	}
	var reports []replayVariantReport
	for _, v := range variants {
		r := replayVariantReport{Name: v.Name, Tuning: v.Tuning, Summary: map[string]summary{}}
		for _, split := range append(splitsOf(all), "all") {
			r.Summary[split] = summarize(v.Name, split, all)
		}
		// Diff against active: relabel active's outcomes to this variant so detectChanges pairs them by case.
		var base []caseOutcome
		for _, o := range byVariant["active"] {
			o.Method = v.Name
			base = append(base, o)
		}
		r.Regressions, r.Improvements = detectChanges(base, byVariant[v.Name])
		reports = append(reports, r)
	}
	fmt.Printf("replay %s: %d variants × %d cases, deterministic=%v (cases %s…)\n", cfg.Version, len(variants), len(cf.Cases), deterministic, key.CasesSHA256[:12])
	fmt.Printf("%-40s %6s %6s %6s %7s %7s\n", "variant (all splits)", "Hit@1", "Hit@5", "MRR", "FP%", "tokens")
	for _, r := range reports {
		s := r.Summary["all"]
		fmt.Printf("%-40s %6.1f %6.1f %6.3f %7.1f %7.0f   reg=%d imp=%d\n", r.Name, s.Hit1, s.Hit5, s.MRR, s.FalsePos, s.Tokens, len(r.Regressions), len(r.Improvements))
	}
	if outPath != "" {
		report := map[string]any{"schema": "replay/1", "config_version": cfg.Version, "run_key": key, "hardware": hardware(),
			"deterministic": deterministic, "variants": reports, "outcomes": all, "captures": captures}
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "wrote", outPath)
	}
	if !deterministic {
		return fmt.Errorf("replay is not deterministic: two runs of the active tuning differ")
	}
	return nil
}
