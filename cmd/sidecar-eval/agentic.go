package main

import (
	"io"

	"github.com/dedeusgui/sidecar/internal/benchagent"
)

// printAgenticReport pairs the run records of one directory against the task manifest and
// prints the A/B report. It reads records only; it never starts an agent.
func printAgenticReport(w io.Writer, dir, manifest string) error {
	tasks, err := benchagent.LoadManifest(manifest)
	if err != nil {
		return err
	}
	runs, err := benchagent.LoadRuns(dir, tasks)
	if err != nil {
		return err
	}
	benchagent.Evaluate(tasks, runs).WriteMarkdown(w, dir)
	return nil
}
