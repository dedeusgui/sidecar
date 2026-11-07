package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ToolKind says what a tool call does. It is inferred from the tool name and, for shell
// commands, from the command's first words. Tool output is never read.
type ToolKind string

// Tool kinds of the agent-feedback schema v1.
const (
	KindSearch ToolKind = "search"
	KindRead   ToolKind = "read"
	KindEdit   ToolKind = "edit"
	KindTest   ToolKind = "test"
	KindLSP    ToolKind = "lsp"
	KindOther  ToolKind = "other"
)

// ToolCall is one tool event after parsing. Path, Query and Error are raw values kept in
// memory only; callers persist hashes of them, never the strings.
type ToolCall struct {
	Agent   Agent
	Phase   string // "pre" for PreToolUse, "post" for PostToolUse, "compact" for PreCompact
	Session string
	CWD     string
	Name    string
	Kind    ToolKind
	Path    string
	Query   string // search pattern or shell command
	CallID  string
	Result  string // "ok", "error" or "unknown" (post only; "" before the call)
	Error   string
}

// ParseTool reads one tool event. event is the hook name the command was installed for
// (PreToolUse, PostToolUse or PreCompact), because Antigravity payloads do not name it. Claude
// Code and Codex do not document a failure field on PostToolUse, so their result is "unknown".
func ParseTool(agent Agent, event string, r io.Reader) (ToolCall, error) {
	phase := map[string]string{"PreToolUse": "pre", "PostToolUse": "post", "PreCompact": "compact"}[event]
	if phase == "" {
		return ToolCall{}, fmt.Errorf("unsupported tool event %q", event)
	}
	data, err := readPayload(r)
	if err != nil {
		return ToolCall{}, err
	}
	if agent == Antigravity {
		var raw struct {
			ConversationID string          `json:"conversationId"`
			WorkspacePaths json.RawMessage `json:"workspacePaths"`
			ToolCall       struct {
				Name string         `json:"name"`
				Args map[string]any `json:"args"`
			} `json:"toolCall"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return ToolCall{}, fmt.Errorf("%s payload: %w", agent, err)
		}
		var workspaces []string
		_ = json.Unmarshal(raw.WorkspacePaths, &workspaces)
		c := ToolCall{Agent: agent, Phase: phase, Session: raw.ConversationID, Name: raw.ToolCall.Name}
		if len(workspaces) > 0 {
			c.CWD = workspaces[0]
		}
		c.Path = argString(raw.ToolCall.Args, "file_path", "path", "filePath", "AbsolutePath", "TargetFile")
		c.Query = argString(raw.ToolCall.Args, "command", "CommandLine", "pattern", "query", "Query")
		c.Kind = classify(c.Name, c.Query)
		if phase == "post" {
			c.Result, c.Error = "ok", raw.Error
			if raw.Error != "" {
				c.Result = "error"
			}
		}
		return c, nil
	}

	var raw struct {
		SessionID     string         `json:"session_id"`
		CWD           string         `json:"cwd"`
		HookEventName string         `json:"hook_event_name"`
		ToolName      string         `json:"tool_name"`
		ToolInput     map[string]any `json:"tool_input"`
		ToolUseID     string         `json:"tool_use_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ToolCall{}, fmt.Errorf("%s payload: %w", agent, err)
	}
	if raw.HookEventName != "" && raw.HookEventName != event {
		return ToolCall{}, fmt.Errorf("hook event %q does not match %q", raw.HookEventName, event)
	}
	c := ToolCall{
		Agent: agent, Phase: phase, Session: raw.SessionID, CWD: raw.CWD, Name: raw.ToolName,
		CallID: raw.ToolUseID,
		Path:   argString(raw.ToolInput, "file_path", "notebook_path", "path"),
		Query:  argString(raw.ToolInput, "pattern", "command"),
	}
	c.Kind = classify(c.Name, c.Query)
	if phase == "post" {
		c.Result = "unknown"
	}
	return c, nil
}

// testCommand matches common test runners anywhere in a shell command.
var testCommand = regexp.MustCompile(`(?i)\b(go\s+test|pytest|npm\s+(run\s+)?test|yarn\s+test|pnpm\s+(run\s+)?test|npx\s+(jest|vitest)|jest|vitest|mvn\s.*\btest\b|gradle\w*\s.*\btest\b|cargo\s+test|dotnet\s+test|rspec|phpunit|make\s+test)\b`)

// segmentSplit separates the commands of a compound shell line.
var segmentSplit = regexp.MustCompile(`&&|\|\||[;|\n]`)

func classify(name, command string) ToolKind {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "lsp"):
		return KindLSP
	case n == "bash" || n == "shell" || n == "terminal" || n == "execute" ||
		strings.Contains(n, "command") || strings.Contains(n, "exec"):
		return classifyCommand(command)
	case strings.Contains(n, "edit") || strings.Contains(n, "patch") || n == "write" ||
		n == "write_file" || n == "create_file":
		return KindEdit
	case strings.Contains(n, "read") || strings.Contains(n, "view"):
		return KindRead
	case strings.Contains(n, "grep") || strings.Contains(n, "search") || strings.Contains(n, "glob") ||
		strings.Contains(n, "find") || n == "ls":
		return KindSearch
	}
	return KindOther
}

func classifyCommand(command string) ToolKind {
	if testCommand.MatchString(command) {
		return KindTest
	}
	words := strings.Fields(segmentSplit.Split(command, 2)[0])
	if len(words) == 0 {
		return KindOther
	}
	switch words[0] {
	case "rg", "grep", "egrep", "fgrep", "find", "fd", "ag", "ack":
		return KindSearch
	case "git":
		if len(words) > 1 && words[1] == "grep" {
			return KindSearch
		}
	case "cat", "head", "tail", "less", "bat":
		return KindRead
	case "sed":
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-i") {
				return KindEdit
			}
		}
		return KindRead
	}
	return KindOther
}

func argString(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := args[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
