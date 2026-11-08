// Package brief turns retrieval results into the [SIDECAR CONTEXT] block and remembers,
// per agent session, which evidence was already sent so it is not repeated.
package brief

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

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
	text, _ := RenderSession(res, statePath)
	return text
}

// RenderSession is RenderForSession that also returns the keys of the evidence it sent,
// so the session observer can count what the agent was given.
func RenderSession(res retrieve.Result, statePath string) (string, []string) {
	return renderSession(res, statePath, items(res))
}

// RenderTaskSession is RenderSession with the Task Evidence Brief (opt-in feature "brief"). When the
// evidence is too thin for a brief it renders the standard block, so enabling it never removes context.
func RenderTaskSession(res retrieve.Result, statePath string) (text string, keys []string, briefed bool) {
	its, ok := TaskItems(res)
	if !ok {
		its = items(res)
	}
	text, keys = renderSession(res, statePath, its)
	return text, keys, ok
}

// RenderTask renders the brief without session memory; ok reports whether a brief applied.
func RenderTask(res retrieve.Result, seen ctxbuild.Seen) (string, bool) {
	its, ok := TaskItems(res)
	if !ok {
		its = items(res)
	}
	text, _ := ctxbuild.Compose(its, res.Trace.Budget, seen)
	return text, ok
}

func renderSession(res retrieve.Result, statePath string, its []ctxbuild.Item) (string, []string) {
	if statePath == "" {
		text, _ := ctxbuild.Compose(its, res.Trace.Budget, nil)
		return text, nil
	}
	st := loadState(statePath)
	st.Turn++
	seen := func(key, fingerprint string) bool {
		e, ok := st.Emitted[key]
		return ok && e.Fingerprint == fingerprint && st.Turn-e.Turn < TTLTurns
	}
	text, used := ctxbuild.Compose(its, res.Trace.Budget, seen)
	keys := make([]string, 0, len(used))
	for _, it := range used {
		st.Emitted[it.Key] = entry{Fingerprint: it.Fingerprint, Turn: st.Turn}
		keys = append(keys, it.Key)
	}
	saveState(statePath, st)
	return text, keys
}

// SeenFor returns the session memory as it stands before the next prompt, so the packer can
// avoid evidence that RenderSession would suppress. An empty path has no memory.
func SeenFor(statePath string) ctxbuild.Seen {
	if statePath == "" {
		return nil
	}
	st := loadState(statePath)
	next := st.Turn + 1
	return func(key, fingerprint string) bool {
		e, ok := st.Emitted[key]
		return ok && e.Fingerprint == fingerprint && next-e.Turn < TTLTurns
	}
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
	_ = writeAtomic(path, data)
}

// items converts evidence into context items, adding a git note for dirty files.
func items(res retrieve.Result) []ctxbuild.Item {
	var out []ctxbuild.Item
	dirtyNoted := map[string]bool{}
	for _, ev := range res.Evidence {
		h := ev.Hit
		out = append(out, ctxbuild.Item{Section: ev.Section, Key: ev.Key(), Fingerprint: ev.Fingerprint(), Text: retrieve.Describe(ev)})
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
