//go:build !race

package retrieve

// raceDetector is false in ordinary builds, where the latency budget applies.
const raceDetector = false
