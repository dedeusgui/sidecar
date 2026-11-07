package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dedeusgui/sidecar/internal/app"
	"github.com/dedeusgui/sidecar/internal/paths"
)

const maxActivityLimit = 200

// cmdActivity prints the newest hook runs. It reads only the local hook log, which holds no
// prompt text, so the output is safe to paste into a bug report.
func cmdActivity(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("activity", flag.ContinueOnError)
	fs.SetOutput(errOut)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	limit := fs.Int("limit", 20, fmt.Sprintf("number of runs to list (1-%d)", maxActivityLimit))
	_ = fs.String("root", "", "optional repository root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *limit < 1 || *limit > maxActivityLimit {
		_, _ = fmt.Fprintf(errOut, "erro: --limit deve estar entre 1 e %d\n", maxActivityLimit)
		return 2
	}
	rep := app.ReadActivity(paths.HookLog(), *limit)
	if *asJSON {
		return writeJSON(out, errOut, rep)
	}
	if rep.Runs == 0 {
		_, _ = fmt.Fprintln(out, "Nenhuma execução do hook registrada ainda.")
		return 0
	}
	_, _ = fmt.Fprintf(out, "Últimas %d execuções · p50=%.1fms p95=%.1fms\n\n", rep.Runs, rep.P50MS, rep.P95MS)
	_, _ = fmt.Fprintf(out, "%-19s %-7s %-12s %5s %9s  %s\n", "quando", "agente", "estado", "refs", "tempo", "repo")
	for _, e := range rep.Entries {
		when := e.At
		if t, err := time.Parse(time.RFC3339, e.At); err == nil {
			when = t.Local().Format("2006-01-02 15:04:05")
		}
		_, _ = fmt.Fprintf(out, "%-19s %-7s %-12s %5d %8.1fms  %s\n", when, e.Agent, e.State, e.Evidence, e.TotalMS, e.Repo)
	}
	return 0
}

// cmdStatusline prints one line for the Claude Code status line. Claude sends session JSON on
// stdin; when it is present, the line describes that session's newest hook run. It always
// exits 0, because a broken status line must never affect the agent's UI.
func cmdStatusline(in io.Reader, out io.Writer) int {
	session := ""
	if !isTerminal(in) {
		// A pipe that never reaches EOF must not hang the status line, so the read is bounded.
		got := make(chan []byte, 1)
		go func() {
			data, _ := io.ReadAll(io.LimitReader(in, 64<<10))
			got <- data
		}()
		select {
		case data := <-got:
			var payload struct {
				SessionID string `json:"session_id"`
			}
			if json.Unmarshal(data, &payload) == nil {
				session = payload.SessionID
			}
		case <-time.After(300 * time.Millisecond):
		}
	}
	_, _ = fmt.Fprintln(out, app.StatusLine(paths.HookLog(), app.SessionKey(session)))
	return 0
}
