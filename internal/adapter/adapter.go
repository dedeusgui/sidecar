// Package adapter translates between agent hook protocols and Sidecar. It is the only
// place that knows the JSON shapes of Claude Code and Codex hooks.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Agent names a supported coding agent.
type Agent string

// Supported agents.
const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
)

// MaxInputBytes bounds the hook payload read from stdin.
const MaxInputBytes = 1 << 20

// Input is the subset of a UserPromptSubmit payload that Sidecar uses.
type Input struct {
	SessionID string
	CWD       string
	Prompt    string
}

// ParseAgent validates an agent name.
func ParseAgent(name string) (Agent, error) {
	switch Agent(name) {
	case Claude, Codex:
		return Agent(name), nil
	}
	return "", fmt.Errorf("unsupported agent %q (want claude or codex)", name)
}

// Parse reads one UserPromptSubmit payload. Both agents send the same fields used here
// (session_id, cwd, prompt, hook_event_name); extra fields are ignored.
func Parse(agent Agent, r io.Reader) (Input, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return Input{}, err
	}
	if len(data) > MaxInputBytes {
		return Input{}, errors.New("hook payload too large")
	}
	var raw struct {
		SessionID     string `json:"session_id"`
		CWD           string `json:"cwd"`
		Prompt        string `json:"prompt"`
		HookEventName string `json:"hook_event_name"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Input{}, fmt.Errorf("%s payload: %w", agent, err)
	}
	if raw.HookEventName != "" && raw.HookEventName != "UserPromptSubmit" {
		return Input{}, fmt.Errorf("unexpected hook event %q", raw.HookEventName)
	}
	return Input{SessionID: raw.SessionID, CWD: raw.CWD, Prompt: raw.Prompt}, nil
}

// Format produces the bytes to write to stdout. Claude Code adds JSON
// hookSpecificOutput.additionalContext as context for the model; with no context it
// prints nothing. Codex requires a JSON object on stdout, so it gets {} instead.
func Format(agent Agent, context string) []byte {
	if context == "" {
		if agent == Codex {
			return []byte("{}")
		}
		return nil
	}
	out := struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}{}
	out.HookSpecificOutput.HookEventName = "UserPromptSubmit"
	out.HookSpecificOutput.AdditionalContext = context
	b, _ := json.Marshal(out)
	return b
}
