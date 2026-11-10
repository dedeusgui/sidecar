// Package adapter translates between agent hook protocols and Sidecar. It is the only
// place that knows the JSON shapes of Claude Code, Codex and Antigravity CLI hooks.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dedeusgui/sidecar/internal/engine/contract"
)

// Agent names a supported coding agent.
type Agent string

// Supported agents.
const (
	Claude      Agent = "claude"
	Codex       Agent = "codex"
	Antigravity Agent = "antigravity"
)

// ObservationHook is a client event whose payload has a tested adapter contract.
// Name is the event registered with the client; ObserveEvent is passed to
// `sidecar observe` and may differ for clients with different event names.
type ObservationHook struct {
	Name         string
	ObserveEvent string
}

// Capabilities describes prompt/observation contracts and their verification status for a client.
// ObservationHooks also retains historical opt-in registrations whose client contract is unverified.
// ExploreCapability remains unverified until a client-native path has been tested.
type Capabilities struct {
	PromptEvent          string
	PromptCapability     contract.CapabilityStatus
	ObservationHooks     []ObservationHook
	Observability        contract.Observability
	ToolEventsCapability contract.CapabilityStatus
	PreCompactCapability contract.CapabilityStatus
	ExploreCapability    contract.CapabilityStatus
}

// CapabilitiesFor returns hook capabilities established by adapter and install
// contract tests. Unknown agents have no registered events and remain unverified.
func CapabilitiesFor(agent Agent) Capabilities {
	capabilities := Capabilities{
		Observability: contract.Observability{
			ToolEvents: contract.ObservationUnsupported,
			PreCompact: contract.ObservationUnsupported,
		},
		PromptCapability:     contract.CapabilityUnverified,
		ToolEventsCapability: contract.CapabilityUnverified,
		PreCompactCapability: contract.CapabilityUnverified,
		ExploreCapability:    contract.CapabilityUnverified,
	}
	switch agent {
	case Claude:
		capabilities.PromptEvent = "UserPromptSubmit"
		capabilities.PromptCapability = contract.CapabilityAvailable
		capabilities.ObservationHooks = []ObservationHook{
			{Name: "PreToolUse", ObserveEvent: "PreToolUse"},
			{Name: "PostToolUse", ObserveEvent: "PostToolUse"},
			{Name: "PreCompact", ObserveEvent: "PreCompact"},
		}
		capabilities.Observability.ToolEvents = contract.ObservationSeen
		capabilities.Observability.PreCompact = contract.ObservationSeen
		capabilities.ToolEventsCapability = contract.CapabilityAvailable
		capabilities.PreCompactCapability = contract.CapabilityAvailable
	case Codex:
		capabilities.PromptEvent = "UserPromptSubmit"
		capabilities.PromptCapability = contract.CapabilityAvailable
		capabilities.ToolEventsCapability = contract.CapabilityUnsupported
		capabilities.PreCompactCapability = contract.CapabilityUnsupported
	case Antigravity:
		capabilities.PromptEvent = "PreInvocation"
		capabilities.ObservationHooks = []ObservationHook{
			{Name: "PreTool", ObserveEvent: "PreToolUse"},
			{Name: "PostTool", ObserveEvent: "PostToolUse"},
		}
		// Adapter parsing and generated configuration do not prove that the client consumes
		// these events. Keep the historical opt-in wiring, but do not use it as V3.5 evidence.
		// Observability stays unsupported/unobserved (no client-consumption proof), and the
		// absence of a PreCompact event is equally unproven, so every capability is unverified.
		capabilities.ToolEventsCapability = contract.CapabilityUnverified
		capabilities.PreCompactCapability = contract.CapabilityUnverified
	}
	return capabilities
}

// MaxInputBytes bounds the hook payload read from stdin.
const MaxInputBytes = 1 << 20

// Input is the subset of a prompt-time payload that Sidecar uses. Invocation is only set
// by Antigravity CLI, whose PreInvocation runs once per model call; 0 is the first call of
// a turn, the only one that may receive context.
type Input struct {
	SessionID  string
	CWD        string
	Prompt     string
	Invocation int
}

// ParseAgent validates an agent name.
func ParseAgent(name string) (Agent, error) {
	switch Agent(name) {
	case Claude, Codex, Antigravity:
		return Agent(name), nil
	}
	return "", fmt.Errorf("unsupported agent %q (want claude, codex or antigravity)", name)
}

// Parse reads one prompt-time payload. Claude Code and Codex send session_id, cwd, prompt
// and hook_event_name; Antigravity CLI sends conversationId, workspacePaths, lastUserInput
// and invocationNum. Extra fields are ignored.
func Parse(agent Agent, r io.Reader) (Input, error) {
	data, err := readPayload(r)
	if err != nil {
		return Input{}, err
	}
	var raw struct {
		SessionID      string          `json:"session_id"`
		CWD            string          `json:"cwd"`
		Prompt         string          `json:"prompt"`
		HookEventName  string          `json:"hook_event_name"`
		ConversationID string          `json:"conversationId"`
		WorkspacePaths json.RawMessage `json:"workspacePaths"`
		LastUserInput  string          `json:"lastUserInput"`
		InvocationNum  int             `json:"invocationNum"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Input{}, fmt.Errorf("%s payload: %w", agent, err)
	}
	if agent == Antigravity {
		// workspacePaths is not documented as a typed list; an unexpected shape means no
		// CWD, and the hook falls back to its own working directory.
		var workspaces []string
		_ = json.Unmarshal(raw.WorkspacePaths, &workspaces)
		cwd := ""
		if len(workspaces) > 0 {
			cwd = workspaces[0]
		}
		return Input{SessionID: raw.ConversationID, CWD: cwd, Prompt: raw.LastUserInput, Invocation: raw.InvocationNum}, nil
	}
	if raw.HookEventName != "" && raw.HookEventName != "UserPromptSubmit" {
		return Input{}, fmt.Errorf("unexpected hook event %q", raw.HookEventName)
	}
	return Input{SessionID: raw.SessionID, CWD: raw.CWD, Prompt: raw.Prompt}, nil
}

// readPayload reads one hook payload, refusing anything above MaxInputBytes.
func readPayload(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxInputBytes {
		return nil, errors.New("hook payload too large")
	}
	return data, nil
}

// Format produces the bytes to write to stdout. Claude Code adds JSON
// hookSpecificOutput.additionalContext as context for the model; with no context it
// prints nothing. Codex and Antigravity CLI require a JSON object on stdout, so they get
// {} instead. Antigravity receives context as an ephemeral injected step.
func Format(agent Agent, context string) []byte {
	if agent == Antigravity {
		if context == "" {
			return []byte("{}")
		}
		b, _ := json.Marshal(map[string]any{
			"injectSteps": []map[string]string{{"ephemeralMessage": context}},
		})
		return b
	}
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
