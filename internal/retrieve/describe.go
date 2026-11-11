package retrieve

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Describe renders one evidence item as the single line the context block shows. Packing
// uses the same text to price an item, so the estimate matches what is actually sent.
func Describe(ev Evidence) string { return describe(ev, false) }

// DescribeBare is Describe without the location and the verdict, for renderers that state
// both once themselves (the V3.6 packer appends one trust label and one source location).
func DescribeBare(ev Evidence) string { return describe(ev, true) }

func describe(ev Evidence, bare bool) string {
	if ev.Note != "" {
		return ev.Note
	}
	h := ev.Hit
	var b strings.Builder
	switch {
	case bare && h.Kind == "file":
		fmt.Fprintf(&b, "file %s", path.Base(h.Path))
	case bare:
		fmt.Fprintf(&b, "%s %s", h.Kind, h.Name)
	case h.Kind == "file":
		fmt.Fprintf(&b, "file %s", h.Path)
	default:
		fmt.Fprintf(&b, "%s %s (%s:%d-%d)", h.Kind, h.Name, h.Path, h.Line, h.EndLine)
	}
	if ev.Verdict != "" && !bare {
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

// Key identifies an item across prompts of one session. Conflict notes are keyed by their
// own ID, so a note is not suppressed because the code symbol it concerns was sent before.
func (e Evidence) Key() string {
	if e.Note != "" {
		return "concern|" + e.ID
	}
	return e.Hit.Kind + "|" + e.Hit.Path + "|" + e.Hit.Name
}

// Fingerprint changes when the item's source changes, so a changed item is sent again.
func (e Evidence) Fingerprint() string {
	h := e.Hit
	return h.FileHash + ":" + strconv.Itoa(h.Line) + "-" + strconv.Itoa(h.EndLine)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
