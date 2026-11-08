package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/index"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// stageStats is the latency distribution of one pipeline stage in milliseconds.
type stageStats struct {
	N   int     `json:"n"`
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

// perfReport separates the first call per repository (cold: SQLite pages and Go allocations not warmed)
// from repeated calls (warm). hook_probe_ms is a different measurement and is not mixed in here.
type perfReport struct {
	Schema     string                           `json:"schema"`
	WarmRuns   int                              `json:"warm_runs_per_case"`
	Cold       map[string]map[string]stageStats `json:"cold_by_stage"`
	Warm       map[string]map[string]stageStats `json:"warm_by_stage"`
	Samples    map[string]int                   `json:"samples"`
	Hardware   map[string]any                   `json:"hardware"`
	Limitation string                           `json:"limitation"`
}

func stats(xs []float64) stageStats {
	s := stageStats{N: len(xs), P50: percentile(xs, 50), P95: percentile(xs, 95), P99: percentile(xs, 99)}
	for _, x := range xs {
		s.Max = max(s.Max, x)
	}
	return s
}

// runPerf times the in-process retrieval pipeline stage by stage using Trace.Timings, cold then warm.
// Percentiles from fewer than 100 samples are indicative only; the report states the sample count.
func runPerf(cf casesFile, indexes map[string]*index.Index, warm int, outPath string) error {
	cold := map[string]map[string][]float64{"v1": {}, "v2": {}, "v2-adaptive": {}}
	hot := map[string]map[string][]float64{"v1": {}, "v2": {}, "v2-adaptive": {}}
	add := func(m map[string][]float64, tr retrieve.Trace, wall time.Duration) {
		for st, d := range tr.Timings {
			m[st] = append(m[st], float64(d.Microseconds())/1000)
		}
		m["wall"] = append(m["wall"], float64(wall.Microseconds())/1000)
	}
	seenRepo := map[string]bool{}
	for _, c := range cf.Cases {
		ix := indexes[c.Repo]
		for _, eng := range []string{"v1", "v2", "v2-adaptive"} {
			opts := retrieve.Options{EngineVersion: strings.TrimSuffix(eng, "-adaptive"), Adaptive: strings.HasSuffix(eng, "-adaptive"), Root: ix.Root()}
			first := !seenRepo[c.Repo+eng]
			seenRepo[c.Repo+eng] = true
			for i := 0; i <= warm; i++ {
				t := time.Now()
				res, err := retrieve.Search(context.Background(), ix.Store(), c.Prompt, opts)
				if err != nil {
					return err
				}
				if first && i == 0 {
					add(cold[eng], res.Trace, time.Since(t))
				} else {
					add(hot[eng], res.Trace, time.Since(t))
				}
			}
		}
	}
	rep := perfReport{Schema: "perf/1", WarmRuns: warm, Cold: map[string]map[string]stageStats{}, Warm: map[string]map[string]stageStats{},
		Samples: map[string]int{}, Hardware: hardware(),
		Limitation: "in-process pipeline only: no hook process start, JSON IO or client overhead; cold = first call per repo+engine after Open+Sync"}
	for eng := range cold {
		rep.Cold[eng], rep.Warm[eng] = map[string]stageStats{}, map[string]stageStats{}
		for st, xs := range cold[eng] {
			rep.Cold[eng][st] = stats(xs)
		}
		for st, xs := range hot[eng] {
			rep.Warm[eng][st] = stats(xs)
		}
		rep.Samples[eng+"_warm_wall"] = len(hot[eng]["wall"])
		rep.Samples[eng+"_cold_wall"] = len(cold[eng]["wall"])
	}
	for _, eng := range []string{"v1", "v2", "v2-adaptive"} {
		stages := make([]string, 0)
		for st := range rep.Warm[eng] {
			stages = append(stages, st)
		}
		sort.Strings(stages)
		fmt.Printf("%s warm (n=%d) per stage, ms:\n", eng, rep.Samples[eng+"_warm_wall"])
		for _, st := range stages {
			s := rep.Warm[eng][st]
			fmt.Printf("  %-12s p50=%.2f p95=%.2f p99=%.2f\n", st, s.P50, s.P95, s.P99)
		}
		if w, ok := rep.Cold[eng]["wall"]; ok {
			fmt.Printf("  cold wall (n=%d): p50=%.2f max=%.2f\n", w.N, w.P50, w.Max)
		}
	}
	if outPath != "" {
		data, _ := json.MarshalIndent(rep, "", "  ")
		return os.WriteFile(outPath, data, 0o644)
	}
	return nil
}
