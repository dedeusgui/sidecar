package benchagent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

func verifiedTask(id string) Task {
	return Task{ID: id, Status: "verified", Repo: "r", Commit: sha, Prompt: "p",
		Gold: []Gold{{Path: "a.go", Name: "F"}}, Oracle: Oracle{Command: "go test ./...", Independent: true}}
}

func ptr(x float64) *float64 { return &x }

// runsOf builds one arm's runs for a task: one record per repetition, with the given
// used-evidence times (nil = censored) and resolution.
func runsOf(task, arm string, used []*float64, resolved []bool) []Run {
	var out []Run
	for i := range used {
		out = append(out, Run{Task: task, Arm: arm, Rep: i + 1, Resolved: resolved[i], UsedMS: used[i], WallMS: 9000, BurdenTokens: 1000})
	}
	return out
}

func allResolved(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = true
	}
	return out
}

func verdictOfArm(t *testing.T, rep Report, task, arm string) TaskVerdict {
	t.Helper()
	for _, v := range rep.Verdicts {
		if v.Task == task && v.Arm == arm {
			return v
		}
	}
	t.Fatalf("no verdict for %s/%s in %+v", task, arm, rep.Verdicts)
	return TaskVerdict{}
}

func TestSidecarHelpsWhenEvidenceIsFasterInEveryRepetition(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	off := runsOf("T1", "OFF", []*float64{ptr(9000), ptr(8000), ptr(10000)}, allResolved(3))
	on := runsOf("T1", "ON-v2", []*float64{ptr(4000), ptr(5000), ptr(3000)}, allResolved(3))
	v := verdictOfArm(t, Evaluate(tasks, append(off, on...)), "T1", "ON-v2")
	if v.Verdict != Helped || v.Pairs != 3 {
		t.Fatalf("verdict %q over %d pairs, want helped over 3", v.Verdict, v.Pairs)
	}
}

func TestSidecarIsHurtWhenEvidenceArrivesLaterInEveryRepetition(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	off := runsOf("T1", "OFF", []*float64{ptr(4000), ptr(5000), ptr(3000)}, allResolved(3))
	on := runsOf("T1", "ON-v2", []*float64{ptr(9000), ptr(8000), ptr(10000)}, allResolved(3))
	if v := verdictOfArm(t, Evaluate(tasks, append(off, on...)), "T1", "ON-v2"); v.Verdict != Hurt {
		t.Fatalf("verdict %q, want %q", v.Verdict, Hurt)
	}
}

func TestLossOfResolutionIsHurtEvenWhenEvidenceIsSame(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	off := runsOf("T1", "OFF", []*float64{ptr(5000), ptr(5000), ptr(5000)}, []bool{true, true, true})
	on := runsOf("T1", "ON-v2", []*float64{ptr(5000), ptr(5000), ptr(5000)}, []bool{false, false, false})
	if v := verdictOfArm(t, Evaluate(tasks, append(off, on...)), "T1", "ON-v2"); v.Verdict != Hurt {
		t.Fatalf("resolution loss gave %q, want %q", v.Verdict, Hurt)
	}
}

func TestMixedDirectionsAreInconclusive(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	off := runsOf("T1", "OFF", []*float64{ptr(5000), ptr(5000), ptr(5000)}, allResolved(3))
	on := runsOf("T1", "ON-v2", []*float64{ptr(4000), ptr(6000), ptr(5000)}, allResolved(3))
	if v := verdictOfArm(t, Evaluate(tasks, append(off, on...)), "T1", "ON-v2"); v.Verdict != Inconclusive {
		t.Fatalf("mixed signs gave %q, want %q", v.Verdict, Inconclusive)
	}
}

func TestDraftTasksAndFewPairsNeverGetAVerdict(t *testing.T) {
	draft := verifiedTask("D1")
	draft.Status = "draft"
	off := runsOf("D1", "OFF", []*float64{ptr(9000), ptr(9000), ptr(9000)}, allResolved(3))
	on := runsOf("D1", "ON-v2", []*float64{ptr(1000), ptr(1000), ptr(1000)}, allResolved(3))
	v := verdictOfArm(t, Evaluate([]Task{draft}, append(off, on...)), "D1", "ON-v2")
	if v.Verdict != Inconclusive || !strings.Contains(v.Reason, "draft") {
		t.Fatalf("draft task got %q (%s)", v.Verdict, v.Reason)
	}

	tasks := []Task{verifiedTask("T2")}
	off2 := runsOf("T2", "OFF", []*float64{ptr(9000), ptr(9000)}, allResolved(2))
	on2 := runsOf("T2", "ON-v2", []*float64{ptr(1000), ptr(1000)}, allResolved(2))
	if v := verdictOfArm(t, Evaluate(tasks, append(off2, on2...)), "T2", "ON-v2"); v.Verdict != Inconclusive {
		t.Fatalf("two pairs got %q, want inconclusive below %d", v.Verdict, MinPairs)
	}
}

func TestCensoredRunsArePenalisedNotDropped(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	runs := runsOf("T1", "OFF", []*float64{nil, ptr(3000)}, allResolved(2))
	rep := Evaluate(tasks, runs)
	var off ArmSummary
	for _, a := range rep.Arms {
		if a.Arm == "OFF" {
			off = a
		}
	}
	if off.Censored != 1 || off.Runs != 2 {
		t.Fatalf("censored = %d runs = %d, want 1 and 2", off.Censored, off.Runs)
	}
	// The censored run counts with its wall time (9000 ms), so the task mean is (9000+3000)/2.
	if off.TTFEMean != 6000 {
		t.Fatalf("TTFE mean = %v, want 6000: censored runs must not be dropped or zeroed", off.TTFEMean)
	}
}

func TestBootstrapIsRepeatableFromTheSameRecords(t *testing.T) {
	xs := []float64{-1000, -900, -1100, 200}
	m1, lo1, hi1 := bootstrap(xs, seedFor("a"))
	m2, lo2, hi2 := bootstrap(xs, seedFor("a"))
	if m1 != m2 || lo1 != lo2 || hi1 != hi2 {
		t.Fatal("bootstrap is not deterministic for a fixed seed")
	}
	if !(lo1 <= m1 && m1 <= hi1) {
		t.Fatalf("interval [%v, %v] does not contain the mean %v", lo1, hi1, m1)
	}
}

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadManifestRejectsAmbiguousTasks(t *testing.T) {
	cases := map[string]string{
		"verified without commit":             `{"id":"T","status":"verified","repo":"r","prompt":"p","gold":[{"path":"a"}],"oracle":{"command":"x","independent":true}}`,
		"verified without independent oracle": `{"id":"T","status":"verified","repo":"r","commit":"` + sha + `","prompt":"p","gold":[{"path":"a"}],"oracle":{"command":"x","independent":false}}`,
		"unknown status":                      `{"id":"T","status":"ok","repo":"r","prompt":"p","gold":[],"oracle":{"command":"x"}}`,
		"unknown field":                       `{"id":"T","status":"draft","repo":"r","prompt":"p","extra":1}`,
		"repeated id":                         `{"id":"T","status":"draft","repo":"r","prompt":"p"}` + "\n" + `{"id":"T","status":"draft","repo":"r","prompt":"p"}`,
	}
	for name, line := range cases {
		if _, err := LoadManifest(writeTemp(t, "m.jsonl", line)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadRunsRejectsRecordsOutsideTheContract(t *testing.T) {
	tasks := []Task{verifiedTask("T1")}
	cases := map[string]string{
		"unknown task":       `{"task":"Z","arm":"OFF","rep":1,"wall_ms":1}`,
		"duplicate run":      `{"task":"T1","arm":"OFF","rep":1,"wall_ms":1}` + "\n" + `{"task":"T1","arm":"OFF","rep":1,"wall_ms":1}`,
		"fidelity above one": `{"task":"T1","arm":"OFF","rep":1,"wall_ms":1,"contract_fidelity":1.5}`,
		"bad order":          `{"task":"T1","arm":"OFF","rep":1,"wall_ms":1,"order":"XY"}`,
		"unknown field":      `{"task":"T1","arm":"OFF","rep":1,"wall_ms":1,"judge_score":9}`,
	}
	for name, body := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "runs.jsonl"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRuns(dir, tasks); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestShippedManifestLoadsAndIsDraftUntilVerified(t *testing.T) {
	tasks, err := LoadManifest("../../bench/agentic/manifest.jsonl")
	if err != nil {
		t.Fatalf("shipped manifest invalid: %v", err)
	}
	if len(tasks) < 4 {
		t.Fatalf("manifest has %d tasks, want at least 4", len(tasks))
	}
	for _, task := range tasks {
		if task.Status != "draft" {
			t.Errorf("%s is %q: a task becomes verified only after its oracle is run", task.ID, task.Status)
		}
	}
}

func TestMarkdownStatesThatNothingIsVerified(t *testing.T) {
	tasks, err := LoadManifest("../../bench/agentic/manifest.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	Evaluate(tasks, nil).WriteMarkdown(&buf, "dir")
	if !strings.Contains(buf.String(), "Sem tarefas verificadas") {
		t.Fatal("report with only drafts must say that no verdict is issued")
	}
}
