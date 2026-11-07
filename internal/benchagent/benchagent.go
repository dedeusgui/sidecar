// Package benchagent aggregates paired agent runs (OFF against Sidecar arms) over a task
// manifest. It never runs an agent, reads prompts or changes ranking: the trace collector
// writes one record per run, and this package only validates, pairs and summarises them.
package benchagent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Arms in report order. OFF is the reference; every other arm is compared against it.
var Arms = []string{"OFF", "ON-v1", "ON-v2", "v2-no-intent", "v2-no-contract", "v2-no-observer", "v2-no-lsp", "v2-no-memory", "codegraph", "native-search"}

// Verdicts for one task and one arm against OFF.
const (
	Helped       = "Sidecar ajudou"
	Hurt         = "Sidecar prejudicou"
	Inconclusive = "inconclusivo"
)

// MinPairs is the smallest number of repetitions both arms need before a task gets a verdict.
// Below it, a bootstrap interval cannot exclude zero for a reason other than chance.
const MinPairs = 3

const resamples = 2000

// Task is one entry of bench/agentic/manifest.jsonl. A draft task has no commit and is never
// counted; a verified task has gold and an independent oracle checked before any run.
type Task struct {
	ID     string `json:"id"`
	Status string `json:"status"` // draft | verified
	Repo   string `json:"repo"`
	Commit string `json:"commit,omitempty"` // full SHA, required once verified
	Prompt string `json:"prompt"`
	Gold   []Gold `json:"gold"`
	Oracle Oracle `json:"oracle"`
	Notes  string `json:"notes,omitempty"`
}

// Gold is the location a correct answer must reach. An empty Name means file-level gold.
type Gold struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

// Oracle is the test command that decides success. Independent means the agent cannot edit it.
type Oracle struct {
	Command     string `json:"command"`
	Independent bool   `json:"independent"`
}

// Run is one execution of one task under one arm. Pointer fields are nil when the run never
// reached that point; nil is not zero, so a missing value is never averaged in as a result.
type Run struct {
	Task             string   `json:"task"`
	Arm              string   `json:"arm"`
	Model            string   `json:"model,omitempty"`
	Rep              int      `json:"rep"`
	Order            string   `json:"order,omitempty"` // AB | BA: which arm ran first in the pair
	Resolved         bool     `json:"resolved"`
	PresentedMS      *float64 `json:"presented_ms,omitempty"` // correct context shown to the agent
	UsedMS           *float64 `json:"used_ms,omitempty"`      // correct context used or identified
	WallMS           float64  `json:"wall_ms"`
	ToolCallsToGold  *int     `json:"tool_calls_to_gold,omitempty"`
	RoundsToGold     *int     `json:"rounds_to_gold,omitempty"`
	ContractFidelity *float64 `json:"contract_fidelity,omitempty"` // [0,1], from the contract check
	LoopInefficiency *float64 `json:"loop_inefficiency,omitempty"` // [0,1]: repeated lookups without new evidence
	BurdenTokens     int      `json:"total_task_burden_tokens"`
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// LoadManifest reads one JSON object per line and rejects anything ambiguous.
func LoadManifest(path string) ([]Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var tasks []Task
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 1; sc.Scan(); line++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var t Task
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if err := validTask(t); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if seen[t.ID] {
			return nil, fmt.Errorf("%s:%d: task %s repeated", path, line, t.ID)
		}
		seen[t.ID] = true
		tasks = append(tasks, t)
	}
	return tasks, sc.Err()
}

func validTask(t Task) error {
	if t.ID == "" || strings.TrimSpace(t.Prompt) == "" || t.Repo == "" {
		return fmt.Errorf("task %q needs id, repo and prompt", t.ID)
	}
	switch t.Status {
	case "draft":
		return nil
	case "verified":
	default:
		return fmt.Errorf("task %s: status must be draft or verified, got %q", t.ID, t.Status)
	}
	if !shaRe.MatchString(t.Commit) {
		return fmt.Errorf("task %s: verified tasks need a full 40-character commit", t.ID)
	}
	if len(t.Gold) == 0 {
		return fmt.Errorf("task %s: verified tasks need gold", t.ID)
	}
	if strings.TrimSpace(t.Oracle.Command) == "" || !t.Oracle.Independent {
		return fmt.Errorf("task %s: verified tasks need an independent oracle command", t.ID)
	}
	return nil
}

// LoadRuns reads runs.jsonl from dir and checks every record against the manifest.
func LoadRuns(dir string, tasks []Task) ([]Run, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, t := range tasks {
		known[t.ID] = true
	}
	var runs []Run
	seen := map[string]bool{}
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r Run
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("runs.jsonl:%d: %w", i+1, err)
		}
		if err := validRun(r, known); err != nil {
			return nil, fmt.Errorf("runs.jsonl:%d: %w", i+1, err)
		}
		key := fmt.Sprintf("%s|%s|%d", r.Task, r.Arm, r.Rep)
		if seen[key] {
			return nil, fmt.Errorf("runs.jsonl:%d: duplicate run %s", i+1, key)
		}
		seen[key] = true
		runs = append(runs, r)
	}
	return runs, nil
}

func validRun(r Run, known map[string]bool) error {
	switch {
	case !known[r.Task]:
		return fmt.Errorf("task %q is not in the manifest", r.Task)
	case r.Arm == "":
		return fmt.Errorf("run of task %s has no arm", r.Task)
	case r.Rep < 1:
		return fmt.Errorf("task %s arm %s: rep must be 1 or more", r.Task, r.Arm)
	case r.Order != "" && r.Order != "AB" && r.Order != "BA":
		return fmt.Errorf("task %s arm %s: order must be AB or BA", r.Task, r.Arm)
	case r.WallMS < 0 || r.BurdenTokens < 0:
		return fmt.Errorf("task %s arm %s: negative time or tokens", r.Task, r.Arm)
	case r.PresentedMS != nil && *r.PresentedMS < 0, r.UsedMS != nil && *r.UsedMS < 0:
		return fmt.Errorf("task %s arm %s: negative evidence time", r.Task, r.Arm)
	case r.ContractFidelity != nil && !unit(*r.ContractFidelity),
		r.LoopInefficiency != nil && !unit(*r.LoopInefficiency):
		return fmt.Errorf("task %s arm %s: fidelity and inefficiency must be in [0,1]", r.Task, r.Arm)
	}
	return nil
}

func unit(x float64) bool { return x >= 0 && x <= 1 }

// cell holds the runs of one arm on one task, keyed by repetition so arms can be paired.
type cell struct {
	n, censored int
	ttfe        map[int]float64 // used-evidence time, censored runs penalised with wall time
	resolved    map[int]float64 // 1 or 0
	tools       []float64
	rounds      []float64
	fidelity    []float64
	loop        []float64
	burden      []float64
}

// ArmSummary aggregates one arm over all tasks that have runs.
type ArmSummary struct {
	Arm                       string
	Tasks, Runs, Censored     int
	PairedTasks               int     // tasks that both this arm and OFF ran; an interval needs at least two
	Resolution                float64 // share of runs resolved
	TTFEMean                  float64 // mean over tasks of the per-task mean, ms
	TTFEDelta, TTFELo, TTFEHi float64 // paired against OFF over tasks, ms; relative % in TTFERel
	TTFERel                   float64
	Tools, Rounds             float64
	Fidelity, Loop            float64
	Burden                    float64
	BurdenDelta               float64
}

// TaskVerdict is one task and one arm compared with OFF.
type TaskVerdict struct {
	Task, Arm, Oracle string
	Pairs             int
	DeltaTTFE         float64
	TTFELo, TTFEHi    float64
	DeltaRes          float64
	ResLo, ResHi      float64
	Verdict, Reason   string
}

// Report is the result of Evaluate.
type Report struct {
	Tasks    []Task
	Runs     int
	Arms     []ArmSummary
	Verdicts []TaskVerdict
}

// Evaluate pairs the runs and computes the arm summaries and the per-task verdicts.
func Evaluate(tasks []Task, runs []Run) Report {
	byTask := map[string]Task{}
	for _, t := range tasks {
		byTask[t.ID] = t
	}
	cells := map[string]map[string]*cell{} // arm -> task -> cell
	for _, r := range runs {
		if cells[r.Arm] == nil {
			cells[r.Arm] = map[string]*cell{}
		}
		c := cells[r.Arm][r.Task]
		if c == nil {
			c = &cell{ttfe: map[int]float64{}, resolved: map[int]float64{}}
			cells[r.Arm][r.Task] = c
		}
		c.add(r)
	}
	rep := Report{Tasks: tasks, Runs: len(runs)}
	arms := orderedArms(cells)
	off := cells["OFF"]
	for _, arm := range arms {
		rep.Arms = append(rep.Arms, summarizeArm(arm, cells[arm], off))
		if arm == "OFF" {
			continue
		}
		for _, t := range tasks {
			if c := cells[arm][t.ID]; c != nil {
				rep.Verdicts = append(rep.Verdicts, verdictFor(t, arm, c, off[t.ID]))
			}
		}
	}
	return rep
}

func (c *cell) add(r Run) {
	c.n++
	ttfe := r.UsedMS
	if ttfe == nil {
		c.censored++
		ttfe = &r.WallMS // never reached the evidence: its wall time is a lower bound on the cost
	}
	c.ttfe[r.Rep] = *ttfe
	if r.Resolved {
		c.resolved[r.Rep] = 1
	} else {
		c.resolved[r.Rep] = 0
	}
	if r.ToolCallsToGold != nil {
		c.tools = append(c.tools, float64(*r.ToolCallsToGold))
	}
	if r.RoundsToGold != nil {
		c.rounds = append(c.rounds, float64(*r.RoundsToGold))
	}
	if r.ContractFidelity != nil {
		c.fidelity = append(c.fidelity, *r.ContractFidelity)
	}
	if r.LoopInefficiency != nil {
		c.loop = append(c.loop, *r.LoopInefficiency)
	}
	c.burden = append(c.burden, float64(r.BurdenTokens))
}

func summarizeArm(arm string, perTask map[string]*cell, off map[string]*cell) ArmSummary {
	s := ArmSummary{Arm: arm}
	var ttfeTask, toolsTask, roundsTask, burdenTask []float64
	var fid, loop []float64
	// Paired lists hold only tasks that both this arm and OFF ran, so index i is the same task.
	var pairedTTFE, pairedOffTTFE, pairedBurden, pairedOffBurden []float64
	var resolved float64
	var ids []string
	for id := range perTask {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := perTask[id]
		s.Tasks++
		s.Runs += c.n
		s.Censored += c.censored
		resolved += sum(c.resolved)
		ttfeTask = append(ttfeTask, mean(values(c.ttfe)))
		burdenTask = append(burdenTask, mean(c.burden))
		if len(c.tools) > 0 {
			toolsTask = append(toolsTask, mean(c.tools))
		}
		if len(c.rounds) > 0 {
			roundsTask = append(roundsTask, mean(c.rounds))
		}
		fid = append(fid, c.fidelity...)
		loop = append(loop, c.loop...)
		if o := off[id]; o != nil && arm != "OFF" {
			pairedTTFE = append(pairedTTFE, mean(values(c.ttfe)))
			pairedOffTTFE = append(pairedOffTTFE, mean(values(o.ttfe)))
			pairedBurden = append(pairedBurden, mean(c.burden))
			pairedOffBurden = append(pairedOffBurden, mean(o.burden))
		}
	}
	if s.Runs > 0 {
		s.Resolution = resolved / float64(s.Runs)
	}
	s.TTFEMean = mean(ttfeTask)
	s.Tools = mean(toolsTask)
	s.Rounds = mean(roundsTask)
	s.Fidelity = mean(fid)
	s.Loop = mean(loop)
	s.Burden = mean(burdenTask)
	if len(pairedTTFE) > 0 {
		// Per-task differences against OFF, bootstrapped over tasks.
		var d, db []float64
		for i := range pairedTTFE {
			d = append(d, pairedTTFE[i]-pairedOffTTFE[i])
			db = append(db, pairedBurden[i]-pairedOffBurden[i])
		}
		s.PairedTasks = len(d)
		s.TTFEDelta, s.TTFELo, s.TTFEHi = bootstrap(d, seedFor(arm, "ttfe"))
		if m := mean(pairedOffTTFE); m != 0 {
			s.TTFERel = 100 * s.TTFEDelta / m
		}
		s.BurdenDelta, _, _ = bootstrap(db, seedFor(arm, "burden"))
	}
	return s
}

// verdictFor pairs one arm with OFF on one task. The rule is fixed in advance (see
// docs/evaluation-plan.md): helped needs a faster interval below zero and no resolution loss;
// hurt needs a slower or less-resolved interval clear of zero; anything else is inconclusive.
func verdictFor(t Task, arm string, c, off *cell) TaskVerdict {
	v := TaskVerdict{Task: t.ID, Arm: arm, Verdict: Inconclusive, Oracle: "não verificado"}
	if t.Status == "verified" {
		v.Oracle = "verificado"
	}
	if t.Status != "verified" {
		v.Reason = "tarefa draft: gold e oracle ainda não verificados"
		return v
	}
	if off == nil {
		v.Reason = "sem execuções OFF para parear"
		return v
	}
	var d, dr []float64
	for _, rep := range sortedKeys(c.ttfe) {
		o, ok := off.ttfe[rep]
		if !ok {
			continue
		}
		d = append(d, c.ttfe[rep]-o)
		dr = append(dr, c.resolved[rep]-off.resolved[rep])
	}
	v.Pairs = len(d)
	if v.Pairs < MinPairs {
		v.Reason = fmt.Sprintf("%d pares; mínimo %d", v.Pairs, MinPairs)
		return v
	}
	v.DeltaTTFE, v.TTFELo, v.TTFEHi = bootstrap(d, seedFor(arm, t.ID))
	v.DeltaRes, v.ResLo, v.ResHi = bootstrap(dr, seedFor(arm, t.ID+"/res"))
	switch {
	case v.TTFEHi < 0 && v.ResLo >= 0:
		v.Verdict, v.Reason = Helped, "evidência correta usada mais cedo, sem perda de resolução"
	case v.TTFELo > 0 || v.ResHi < 0:
		v.Verdict, v.Reason = Hurt, "evidência mais tardia ou resolução menor, com IC fora de zero"
	default:
		v.Reason = "IC 95% cruza zero"
	}
	return v
}

// bootstrap returns the mean of xs and its 95% percentile interval. The seed is fixed so the
// report is repeatable from the same records.
func bootstrap(xs []float64, seed uint64) (m, lo, hi float64) {
	m = mean(xs)
	if len(xs) < 2 {
		return m, m, m
	}
	r := rand.New(rand.NewPCG(seed, 0x5d1))
	means := make([]float64, resamples)
	buf := make([]float64, len(xs))
	for i := range means {
		for j := range buf {
			buf[j] = xs[r.IntN(len(xs))]
		}
		means[i] = mean(buf)
	}
	sort.Float64s(means)
	return m, means[resamples*25/1000], means[resamples*975/1000-1]
}

func seedFor(parts ...string) uint64 {
	var h uint64 = 1469598103934665603 // FNV-1a
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			h ^= uint64(p[i])
			h *= 1099511628211
		}
	}
	return h
}

func orderedArms(cells map[string]map[string]*cell) []string {
	var out []string
	known := map[string]bool{}
	for _, a := range Arms {
		if _, ok := cells[a]; ok {
			out = append(out, a)
		}
		known[a] = true
	}
	var extra []string
	for a := range cells {
		if !known[a] {
			extra = append(extra, a)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// WriteMarkdown renders the report. It states the caveats next to the numbers it limits.
func (rep Report) WriteMarkdown(w io.Writer, source string) {
	verified := 0
	for _, t := range rep.Tasks {
		if t.Status == "verified" {
			verified++
		}
	}
	fmt.Fprintf(w, "# Relatório agentic (A/B pareado)\n\n")
	fmt.Fprintf(w, "Fonte: `%s` · execuções: %d · tarefas no manifesto: %d (verificadas: %d)\n\n", source, rep.Runs, len(rep.Tasks), verified)
	if verified == 0 {
		fmt.Fprintf(w, "> **Sem tarefas verificadas.** Todas as tarefas estão em `draft`: gold e oráculo independente ainda não foram confirmados. Nenhum veredito é emitido.\n\n")
	}
	fmt.Fprintf(w, "## Braços\n\n")
	fmt.Fprintln(w, "| Braço | Tarefas | Execuções | Censuradas | Resolução | TTFE usado (ms) | Δ TTFE vs OFF (ms, IC 95%) | Δ % | Tool calls até gold | Rounds até gold | Fidelidade contrato | Ineficiência de loop | Tokens da tarefa | Δ tokens |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, a := range rep.Arms {
		delta, rel, dburden := "—", "—", "—"
		if a.Arm != "OFF" && a.PairedTasks > 0 {
			delta = fmt.Sprintf("%+.0f [%+.0f, %+.0f]", a.TTFEDelta, a.TTFELo, a.TTFEHi)
			if a.PairedTasks < 2 {
				delta = fmt.Sprintf("%+.0f (IC n/d: 1 tarefa pareada)", a.TTFEDelta)
			}
			rel = fmt.Sprintf("%+.1f%%", a.TTFERel)
			dburden = fmt.Sprintf("%+.0f", a.BurdenDelta)
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %.1f%% | %.0f | %s | %s | %s | %s | %s | %s | %.0f | %s |\n",
			a.Arm, a.Tasks, a.Runs, a.Censored, 100*a.Resolution, a.TTFEMean, delta, rel,
			optional(a.Tools), optional(a.Rounds), optional(a.Fidelity), optional(a.Loop), a.Burden, dburden)
	}
	fmt.Fprintf(w, "\nCensuradas: execuções em que a evidência correta nunca apareceu; entram no TTFE com o tempo de parede (cota inferior), nunca como zero.\n\n")
	fmt.Fprintf(w, "## Veredito por tarefa (braço contra OFF)\n\n")
	fmt.Fprintln(w, "| Tarefa | Braço | Oráculo | Pares | Δ TTFE ms [IC 95%] | Δ resolução [IC 95%] | Veredito | Motivo |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|")
	for _, v := range rep.Verdicts {
		fmt.Fprintf(w, "| %s | %s | %s | %d | %+.0f [%+.0f, %+.0f] | %+.2f [%+.2f, %+.2f] | %s | %s |\n",
			v.Task, v.Arm, v.Oracle, v.Pairs, v.DeltaTTFE, v.TTFELo, v.TTFEHi, v.DeltaRes, v.ResLo, v.ResHi, v.Verdict, v.Reason)
	}
	fmt.Fprintf(w, "\nRegra fixada antes das execuções: `%s` exige oráculo verificado, ao menos %d pares, IC 95%% do Δ TTFE abaixo de zero e IC 95%% do Δ resolução não negativo. `%s` exige IC 95%% do Δ TTFE acima de zero ou do Δ resolução abaixo de zero. Os pares são por repetição: com %d repetições, o IC só exclui zero quando todas as diferenças têm o mesmo sinal.\n",
		Helped, MinPairs, Hurt, MinPairs)
}

func optional(x float64) string {
	if x == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", x)
}

func values[K comparable](m map[K]float64) []float64 {
	out := make([]float64, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func sortedKeys(m map[int]float64) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

func sum(m map[int]float64) float64 {
	s := 0.0
	for _, v := range m {
		s += v
	}
	return s
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}
