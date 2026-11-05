package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

const claudeInput = `{
  "session_id": "abc123",
  "transcript_path": "/tmp/t.jsonl",
  "cwd": "/work/repo",
  "permission_mode": "default",
  "hook_event_name": "UserPromptSubmit",
  "prompt": "Adiantar um caminhão na fila"
}`

const codexInput = `{
  "session_id": "sess-9",
  "turn_id": "turn-1",
  "transcript_path": null,
  "cwd": "/work/repo",
  "hook_event_name": "UserPromptSubmit",
  "model": "gpt-x",
  "permission_mode": "default",
  "prompt": "onde fica a liberação?"
}`

func TestParseClaudeInput(t *testing.T) {
	in, err := Parse(Claude, strings.NewReader(claudeInput))
	if err != nil {
		t.Fatal(err)
	}
	if in.SessionID != "abc123" || in.CWD != "/work/repo" || in.Prompt != "Adiantar um caminhão na fila" {
		t.Fatalf("parsed %+v", in)
	}
}

func TestParseCodexInputWithNullTranscript(t *testing.T) {
	in, err := Parse(Codex, strings.NewReader(codexInput))
	if err != nil {
		t.Fatal(err)
	}
	if in.SessionID != "sess-9" || in.Prompt != "onde fica a liberação?" {
		t.Fatalf("parsed %+v", in)
	}
}

func TestParseRejectsOversizedAndMalformedInput(t *testing.T) {
	if _, err := Parse(Claude, strings.NewReader("{")); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	big := `{"prompt":"` + strings.Repeat("a", MaxInputBytes) + `"}`
	if _, err := Parse(Claude, strings.NewReader(big)); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestFormatClaudeUsesOfficialAdditionalContextShape(t *testing.T) {
	out := Format(Claude, "[SIDECAR CONTEXT]\nx\n[END SIDECAR CONTEXT]")
	var v struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, out)
	}
	if v.HookSpecificOutput.HookEventName != "UserPromptSubmit" || !strings.Contains(v.HookSpecificOutput.AdditionalContext, "SIDECAR CONTEXT") {
		t.Fatalf("unexpected shape: %s", out)
	}
}

func TestFormatEmptyContextProducesNoClaudeOutputButValidCodexJSON(t *testing.T) {
	if out := Format(Claude, ""); len(out) != 0 {
		t.Fatalf("Claude empty context should print nothing, got %q", out)
	}
	out := Format(Codex, "")
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("Codex needs a JSON object even without context: %q", out)
	}
}

func TestFormatCodexUsesSameSchemaAsClaude(t *testing.T) {
	out := Format(Codex, "ctx")
	if !strings.Contains(string(out), `"hookEventName":"UserPromptSubmit"`) || !strings.Contains(string(out), `"additionalContext":"ctx"`) {
		t.Fatalf("codex output %s", out)
	}
}
