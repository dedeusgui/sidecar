// Package brief turns retrieval results into the [SIDECAR CONTEXT] block and remembers,
// per agent session, which evidence was already sent so it is not repeated.
package brief

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/retrieve"
)

// TTLTurns is how many prompts an injected item stays suppressed. After that it is sent
// again, so context lost to compaction or a new conversation is restored.
const TTLTurns = 15

const maxRemembered = 400

// Render builds the block for a result with no session memory.
func Render(res retrieve.Result, seen ctxbuild.Seen) string {
	text, _ := ctxbuild.Compose(items(res), res.Trace.Budget, seen)
	return text
}

// RenderForSession builds the block and records what it sent in statePath. An empty path
// disables memory. Unreadable or corrupt state is treated as empty, never as an error.
func RenderForSession(res retrieve.Result, statePath string) string {
	if statePath == "" {
		return Render(res, nil)
	}
	st := loadState(statePath)
	st.Turn++
	seen := func(key, fingerprint string) bool {
		e, ok := st.Emitted[key]
		return ok && e.Fingerprint == fingerprint && st.Turn-e.Turn < TTLTurns
	}
	text, used := ctxbuild.Compose(items(res), res.Trace.Budget, seen)
	for _, it := range used {
		st.Emitted[it.Key] = entry{Fingerprint: it.Fingerprint, Turn: st.Turn}
	}
	saveState(statePath, st)
	return text
}

type entry struct {
	Fingerprint string `json:"fp"`
	Turn        int    `json:"turn"`
}

type state struct {
	Turn    int              `json:"turn"`
	Emitted map[string]entry `json:"emitted"`
}

func loadState(path string) state {
	st := state{Emitted: map[string]entry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	var loaded state
	if err := json.Unmarshal(data, &loaded); err != nil || loaded.Emitted == nil {
		return st
	}
	return loaded
}

func saveState(path string, st state) {
	if len(st.Emitted) > maxRemembered {
		keys := make([]string, 0, len(st.Emitted))
		for k := range st.Emitted {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return st.Emitted[keys[i]].Turn < st.Emitted[keys[j]].Turn })
		for _, k := range keys[:len(keys)-maxRemembered] {
			delete(st.Emitted, k)
		}
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// items converts evidence into context items, adding a git note for dirty files.
func items(res retrieve.Result) []ctxbuild.Item {
	var out []ctxbuild.Item
	dirtyNoted := map[string]bool{}
	for _, ev := range res.Evidence {
		h := ev.Hit
		key := h.Kind + "|" + h.Path + "|" + h.Name
		fp := h.FileHash + ":" + strconv.Itoa(h.Line) + "-" + strconv.Itoa(h.EndLine)
		out = append(out, ctxbuild.Item{Section: ev.Section, Key: key, Fingerprint: fp, Text: describe(ev)})
		if ev.Dirty && !dirtyNoted[h.Path] {
			dirtyNoted[h.Path] = true
			out = append(out, ctxbuild.Item{
				Section: ctxbuild.Git, Key: "dirty|" + h.Path, Fingerprint: h.FileHash,
				Text: fmt.Sprintf("uncommitted changes in %s (working tree differs from HEAD)", h.Path),
			})
		}
	}
	return out
}

func describe(ev retrieve.Evidence) string {
	h := ev.Hit
	var b strings.Builder
	if h.Kind == "file" {
		fmt.Fprintf(&b, "file %s", h.Path)
	} else {
		fmt.Fprintf(&b, "%s %s (%s:%d-%d)", h.Kind, h.Name, h.Path, h.Line, h.EndLine)
	}
	if ev.Verdict != "" {
		fmt.Fprintf(&b, " [%s]", ev.Verdict)
	}
	if h.Value != "" {
		fmt.Fprintf(&b, " = %s", h.Value)
	}
	if h.Kind != "file" && h.Signature != "" {
		fmt.Fprintf(&b, " — %s", truncate(h.Signature, 160))
	}
	if h.Doc != "" {
		fmt.Fprintf(&b, " · %s", truncate(h.Doc, 140))
	}
	if len(ev.Relations) > 0 {
		rels := ev.Relations
		if len(rels) > 3 {
			rels = rels[:3]
		}
		fmt.Fprintf(&b, " | %s", strings.Join(rels, "; "))
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
