package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/paths"
)

// BenchmarkHookPathLatency compares the same hook fixture with a new index and a warm index.
// Custom p50/p95 metrics use the per-invocation durations collected by testing.B.
func BenchmarkHookPathLatency(b *testing.B) {
	root := makeV35Project(b)
	prompt := "How does Router.Serve work in this repository?"
	input, err := json.Marshal(map[string]string{
		"session_id": "", "cwd": root, "prompt": prompt, "hook_event_name": "UserPromptSubmit",
	})
	if err != nil {
		b.Fatal(err)
	}
	for _, engineName := range []string{"v2", "v3", engine.Version35} {
		for _, temperature := range []string{"cold", "hot"} {
			b.Run(engineName+"/"+temperature, func(b *testing.B) {
				home := filepath.Join(b.TempDir(), "sidecar-home")
				b.Setenv("SIDECAR_HOME", home)
				b.Setenv("SIDECAR_ENGINE", engineName)
				b.Setenv("SIDECAR_FEATURES", "")
				b.Setenv("SIDECAR_PROFILE", "")
				logPath := filepath.Join(home, "logs", "hook.jsonl")
				dbPath := paths.IndexDB(root)
				run := func() {
					Hook(HookOptions{Agent: adapter.Claude, Stdin: bytes.NewReader(input), Stdout: io.Discard,
						LogPath: logPath, Budget: 5 * time.Second,
						Spawn: func(string, bool) error { return nil },
					})
				}
				if temperature == "hot" {
					run()
				}
				b.ReportAllocs()
				samples := make([]int64, 0, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if temperature == "cold" {
						for _, suffix := range []string{"", "-wal", "-shm"} {
							_ = os.Remove(dbPath + suffix)
						}
					}
					started := time.Now()
					run()
					samples = append(samples, time.Since(started).Nanoseconds())
				}
				b.StopTimer()
				sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
				b.ReportMetric(float64(samplePercentile(samples, 50)), "p50-ns")
				b.ReportMetric(float64(samplePercentile(samples, 95)), "p95-ns")
			})
		}
	}
}

func samplePercentile(samples []int64, percentile int) int64 {
	if len(samples) == 0 {
		return 0
	}
	index := (len(samples)*percentile+99)/100 - 1
	return samples[max(0, min(index, len(samples)-1))]
}
