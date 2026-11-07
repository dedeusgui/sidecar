//go:build race

package retrieve

// raceDetector is true under -race, which slows Go code several-fold, so latency budgets are not checked.
const raceDetector = true
