package adapter

import (
	"encoding/json"
	"strings"
	"testing"
)

const antigravityInput = `{
  "conversationId": "conv-7",
  "workspacePaths": ["/work/repo"],
  "transcriptPath": "/tmp/t.jsonl",
  "modelName": "gemini-x",
  "lastUserInput": "onde fica a liberação?",
  "invocationNum": 0,
  "initialNumSteps": 1
}`

func TestParseAgentAcceptsAntigravity(t *testing.T) {
	a, err := ParseAgent("antigravity")
	if err != nil || a != Antigravity {
		t.Fatalf("ParseAgent = %q, %v", a, err)
	}
	if _, err := ParseAgent("gemini"); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func TestParseAntigravityPreInvocationPayload(t *testing.T) {
	in, err := Parse(Antigravity, strings.NewReader(antigravityInput))
	if err != nil {
		t.Fatal(err)
	}
	if in.SessionID != "conv-7" || in.CWD != "/work/repo" || in.Prompt != "onde fica a liberação?" || in.Invocation != 0 {
		t.Fatalf("parsed %+v", in)
	}
}

func TestParseAntigravityUnexpectedWorkspaceShapeKeepsPrompt(t *testing.T) {
	in, err := Parse(Antigravity, strings.NewReader(`{"conversationId":"c","workspacePaths":{"a":1},"lastUserInput":"oi","invocationNum":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.CWD != "" || in.Prompt != "oi" || in.Invocation != 2 {
		t.Fatalf("parsed %+v", in)
	}
}

func TestFormatAntigravityUsesInjectStepsOnlyWhenThereIsContext(t *testing.T) {
	if out := string(Format(Antigravity, "")); out != "{}" {
		t.Fatalf("empty context must print {}, got %q", out)
	}
	var v struct {
		InjectSteps []struct {
			EphemeralMessage string `json:"ephemeralMessage"`
		} `json:"injectSteps"`
	}
	if err := json.Unmarshal(Format(Antigravity, "ctx"), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.InjectSteps) != 1 || v.InjectSteps[0].EphemeralMessage != "ctx" {
		t.Fatalf("unexpected output %+v", v)
	}
}
