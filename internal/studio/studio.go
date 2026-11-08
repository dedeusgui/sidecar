// Package studio renders one self-contained HTML page from structured Sidecar outputs (explain traces, lab
// scorecards and runs, replay, perf, clarification and impact benchmarks). It is a viewer: no server, no network,
// no external script or style, and nothing from the data is ever interpreted as markup or as an instruction. The
// page is for people; it is never put in an agent's prompt.
package studio

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed template.html
var template string

const (
	// MaxInputBytes bounds one input; a page that embeds hundreds of megabytes is not an inspection tool.
	MaxInputBytes = 16 << 20
	// MaxTotalBytes bounds the whole page.
	MaxTotalBytes = 48 << 20
)

// Input is one named data source. Raw is a JSON document or, for JSON Lines, an array built by the caller.
type Input struct {
	Name string
	Raw  []byte
}

// Kind names what an input is, from its own fields; it never trusts the file name.
func Kind(v any) string {
	switch m := v.(type) {
	case map[string]any:
		if s, ok := m["schema"].(string); ok {
			return s
		}
		if _, ok := m["candidates"]; ok {
			if _, ok := m["evidence"]; ok {
				return "explain"
			}
		}
	case []any:
		if len(m) > 0 {
			if first, ok := m[0].(map[string]any); ok {
				if s, _ := first["schema"].(string); s != "" {
					return s + "[]"
				}
			}
		}
		return "list"
	}
	return "unknown"
}

type doc struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Data any    `json:"data"`
}

// Build returns the HTML page. Inputs that are not valid JSON are an error: a viewer that silently drops
// data would present an incomplete picture as a whole one.
func Build(inputs []Input) ([]byte, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("studio needs at least one input")
	}
	var docs []doc
	total := 0
	for _, in := range inputs {
		if len(in.Raw) > MaxInputBytes {
			return nil, fmt.Errorf("%s: %d bytes exceeds the %d byte limit", in.Name, len(in.Raw), MaxInputBytes)
		}
		total += len(in.Raw)
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("inputs exceed the %d byte page limit", MaxTotalBytes)
		}
		var v any
		dec := json.NewDecoder(bytes.NewReader(in.Raw))
		err := dec.Decode(&v)
		if err == nil && dec.More() {
			err = fmt.Errorf("more than one JSON value") // JSON Lines: never keep only the first line
		}
		if err != nil {
			// JSON Lines: one object per line becomes a list.
			lines, lerr := jsonLines(in.Raw)
			if lerr != nil {
				return nil, fmt.Errorf("%s: not JSON or JSON Lines: %w", in.Name, err)
			}
			v = lines
		}
		docs = append(docs, doc{Name: in.Name, Kind: Kind(v), Data: v})
	}
	payload, err := json.Marshal(docs)
	if err != nil {
		return nil, err
	}
	// "</script" inside a string would end the data block early; <!-- and the line separators are escaped too.
	safe := strings.NewReplacer("<", `<`, ">", `>`, "&", `&`, " ", ` `, " ", ` `).Replace(string(payload))
	if !strings.Contains(template, "/*__DATA__*/") {
		return nil, fmt.Errorf("template has no data slot")
	}
	return []byte(strings.Replace(template, "/*__DATA__*/null", safe, 1)), nil
}

func jsonLines(raw []byte) ([]any, error) {
	var out []any
	for i, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty")
	}
	return out, nil
}
