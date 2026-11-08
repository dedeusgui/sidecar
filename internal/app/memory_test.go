package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestDecisionMemoryIsInjectedInV2AndNeverWhenStale(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	if _, err := MemoryAdd(root, "MaxActiveTickets", "limite fixo de tickets por decisão de produto", DecisionTTL, time.Now()); err != nil {
		t.Fatal(err)
	}
	const prompt = "o MAX_ACTIVE_TICKETS limita a fila?"
	var spawned []string

	t.Setenv("SIDECAR_ENGINE", "")
	if out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "m0"), &spawned); strings.Contains(out, "[DECISION]") {
		t.Fatalf("decision injected under the default v1 engine: %q", out)
	}

	t.Setenv("SIDECAR_ENGINE", "v2")
	out, _ := runHook(t, adapter.Claude, payload(t, root, prompt, "m1"), &spawned)
	if !strings.Contains(out, "[DECISION] MaxActiveTickets") || !strings.Contains(out, "limite fixo de tickets") {
		t.Fatalf("active decision not injected under v2: %q", out)
	}

	// Editing the symbol's file and reindexing makes the decision stale: the next prompt must not carry it.
	limits := filepath.Join(root, "queue", "limits.go")
	if err := os.WriteFile(limits, []byte("package queue\n\n// MaxActiveTickets limits concurrent tickets.\nconst MaxActiveTickets = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	out, _ = runHook(t, adapter.Claude, payload(t, root, prompt, "m2"), &spawned)
	if strings.Contains(out, "[DECISION]") {
		t.Fatalf("stale decision injected after the edit: %q", out)
	}
	list, err := MemoryList(root, time.Now())
	if err != nil || len(list) != 1 || list[0].Status != store.DecisionStale {
		t.Fatalf("decision after the edit = %+v, %v; want stale", list, err)
	}
}

func TestMemoryDecisionsTrackFileAndSymbolChanges(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	now := time.Now()

	fileDec, err := MemoryAdd(root, "queue/limits.go", "um ticket ativo por vez", DecisionTTL, now)
	if err != nil {
		t.Fatal(err)
	}
	symDec, err := MemoryAdd(root, "MaxActiveTickets", "limite fixo de tickets", DecisionTTL, now)
	if err != nil {
		t.Fatal(err)
	}
	if symDec.FilePath != "queue/limits.go" || symDec.Symbol != "MaxActiveTickets" || symDec.FileHash == "" {
		t.Fatalf("symbol decision = %+v", symDec)
	}
	if fileDec.Status != store.DecisionValid {
		t.Fatalf("new decision status = %q", fileDec.Status)
	}

	list, err := MemoryList(root, now)
	if err != nil || len(list) != 2 || list[0].Status != store.DecisionValid || list[1].Status != store.DecisionValid {
		t.Fatalf("fresh list = %+v, %v", list, err)
	}

	// Changing the file makes both decisions stale: they describe code that no longer matches.
	limits := filepath.Join(root, "queue", "limits.go")
	if err := os.WriteFile(limits, []byte("package queue\n\nconst MaxActiveTickets = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, _ = MemoryList(root, now)
	for _, d := range list {
		if d.Status != store.DecisionStale {
			t.Fatalf("after edit, %s is %q, want stale", d.SymbolOrPath, d.Status)
		}
	}

	// Removing the symbol from the code and syncing the index invalidates its decision.
	if err := os.WriteFile(limits, []byte("package queue\n\nconst Limit = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunIndexer(root, false); err != nil {
		t.Fatal(err)
	}
	list, _ = MemoryList(root, now)
	statuses := map[string]string{}
	for _, d := range list {
		statuses[d.SymbolOrPath] = d.Status
	}
	if statuses["MaxActiveTickets"] != store.DecisionInvalidated || statuses["queue/limits.go"] != store.DecisionStale {
		t.Fatalf("statuses after symbol removal = %v", statuses)
	}

	purged, err := MemoryPurge(root, false, now)
	if err != nil || purged != 1 {
		t.Fatalf("purge invalidated = %d, %v; want 1", purged, err)
	}
	if left, _ := MemoryList(root, now); len(left) != 1 || left[0].SymbolOrPath != "queue/limits.go" {
		t.Fatalf("after purge = %+v", left)
	}
}

func TestMemoryRejectsUnknownSensitiveAndEmptyInput(t *testing.T) {
	setupHome(t)
	root := makeProject(t)
	now := time.Now()
	if _, err := MemoryAdd(root, "NoSuchThing", "x", DecisionTTL, now); err == nil || !strings.Contains(err.Error(), "neither") {
		t.Fatalf("unknown target accepted: %v", err)
	}
	if _, err := MemoryAdd(root, "queue/limits.go", "  ", DecisionTTL, now); err == nil {
		t.Fatal("empty summary accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := MemoryAdd(root, ".env", "credenciais", DecisionTTL, now); err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("decision for a sensitive file: %v", err)
	}
}

func TestMemoryAddRefusesToIndexHomeDirectory(t *testing.T) {
	setupHome(t)
	home := t.TempDir()
	setUserHome(t, home)
	_, err := MemoryAdd(home, "a.go", "x", DecisionTTL, time.Now())
	if err == nil || !strings.Contains(err.Error(), "not a project folder") {
		t.Fatalf("home directory accepted: %v", err)
	}
	if _, statErr := os.Stat(paths.IndexDB(home)); !os.IsNotExist(statErr) {
		t.Fatalf("home directory was indexed: %v", statErr)
	}
}

func TestMemoryListDoesNotCreateIndexForNeverIndexedWorktree(t *testing.T) {
	setupHome(t)
	other := t.TempDir()
	list, err := MemoryList(other, time.Now())
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("list = %v, %v", list, err)
	}
	if _, err := os.Stat(paths.IndexDB(other)); !os.IsNotExist(err) {
		t.Fatalf("listing created an index: %v", err)
	}
}
