package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHookLog(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestActivityStatesLatencyAndNoLeaks(t *testing.T) {
	path := writeHookLog(t,
		`{"at":"2026-11-06T10:00:00Z","agent":"claude","repo":"r","status":"ok","injected_chars":120,"evidence":4,"total_ms":18,"session":"abc"}`,
		`{"at":"2026-11-06T10:00:01Z","agent":"claude","status":"no_evidence","evidence":0,"total_ms":5,"session":"abc"}`,
		`{"at":"2026-11-06T10:00:02Z","agent":"codex","status":"skipped","total_ms":1}`,
		`{"at":"2026-11-06T10:00:02Z","agent":"antigravity","status":"skipped_invocation","total_ms":0.1}`,
		`{"at":"2026-11-06T10:00:03Z","agent":"codex","status":"error","error":"boom: /secret/path","total_ms":2}`,
		`{"at":"2026-11-06T10:00:04Z","agent":"claude","status":"partial","injected_chars":10,"evidence":2,"total_ms":40}`,
	)
	rep := ReadActivity(path, 10)
	if rep.Runs != 6 {
		t.Fatalf("runs = %d", rep.Runs)
	}
	want := []string{"partial", "failed", "skipped", "skipped", "no_evidence", "injected"}
	for i, e := range rep.Entries {
		if e.State != want[i] {
			t.Errorf("entry %d state = %s, want %s", i, e.State, want[i])
		}
	}
	if rep.P50MS != 5 || rep.P95MS != 40 {
		t.Errorf("p50=%v p95=%v, want 5 and 40", rep.P50MS, rep.P95MS)
	}
	out, _ := json.Marshal(rep)
	if strings.Contains(string(out), "boom") || strings.Contains(string(out), "secret") {
		t.Fatalf("error text leaked into activity: %s", out)
	}
}

func TestStatusLineFormatsAndFiltersBySession(t *testing.T) {
	path := writeHookLog(t,
		`{"status":"ok","injected_chars":50,"evidence":4,"total_ms":18,"session":"abc"}`,
		`{"status":"no_evidence","evidence":0,"total_ms":5,"session":"other"}`,
	)
	if got := StatusLine(path, ""); got != "Sidecar: no match" {
		t.Fatalf("latest overall = %q", got)
	}
	if got := StatusLine(path, SessionKey("abc")); got != "Sidecar: no match" {
		// The record stores the raw ID, never the digest, so the digest filter must not match it.
		t.Fatalf("digest filter = %q", got)
	}
	path = writeHookLog(t,
		`{"status":"ok","injected_chars":50,"evidence":4,"total_ms":18,"session":"`+SessionKey("abc")+`"}`,
		`{"status":"no_evidence","evidence":0,"total_ms":5,"session":"`+SessionKey("other")+`"}`,
	)
	if got := StatusLine(path, SessionKey("abc")); got != "Sidecar: injected · 4 refs · 18ms" {
		t.Fatalf("session line = %q", got)
	}
	if got := StatusLine(path, SessionKey("abc")+"x"); got != "Sidecar: no match" {
		t.Fatalf("unknown session = %q", got)
	}
}

func TestMissingLogIsNotAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.jsonl")
	if rep := ReadActivity(missing, 5); rep.Runs != 0 || rep.Entries == nil {
		t.Fatalf("missing log: %+v", rep)
	}
	if got := StatusLine(missing, ""); got != "Sidecar: no match" {
		t.Fatalf("status line without log = %q", got)
	}
}

func TestActivityReadsOnlyTheTailOfALargeLog(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, fmt.Sprintf(`{"at":"t%04d","status":"no_evidence","evidence":0,"total_ms":1,"repo":"%s"}`, i, strings.Repeat("x", 80)))
	}
	path := writeHookLog(t, lines...)
	rep := ReadActivity(path, 3)
	if rep.Runs != 3 || rep.Entries[0].At != "t2999" || rep.Entries[2].At != "t2997" {
		t.Fatalf("tail entries = %+v", rep.Entries)
	}
}
