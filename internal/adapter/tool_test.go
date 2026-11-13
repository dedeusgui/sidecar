package adapter

import (
	"strings"
	"testing"
)

func TestParseToolClaudeKindsAndPaths(t *testing.T) {
	cases := []struct {
		name, payload string
		kind          ToolKind
		path, query   string
	}{
		{"read", `{"session_id":"s","cwd":"/r","tool_name":"Read","tool_input":{"file_path":"/r/a.go"},"tool_use_id":"t1"}`, KindRead, "/r/a.go", ""},
		{"grep", `{"session_id":"s","tool_name":"Grep","tool_input":{"pattern":"Release"}}`, KindSearch, "", "Release"},
		{"edit", `{"tool_name":"Edit","tool_input":{"file_path":"a.go"}}`, KindEdit, "a.go", ""},
		{"test", `{"tool_name":"Bash","tool_input":{"command":"cd x && go test ./..."}}`, KindTest, "", "cd x && go test ./..."},
		{"todo is not an edit", `{"tool_name":"TodoWrite","tool_input":{}}`, KindOther, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTool(Claude, "PreToolUse", strings.NewReader(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != c.kind || got.Path != c.path || got.Query != c.query || got.Phase != "pre" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestParseToolPostHasUnknownResultForClaudeAndCodex(t *testing.T) {
	got, err := ParseTool(Codex, "PostToolUse", strings.NewReader(`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"rg foo"},"tool_response":"secret output"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindSearch || got.Phase != "post" || got.Result != "unknown" || got.Error != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseToolRejectsWrongEventAndUnknownEvent(t *testing.T) {
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Read","tool_input":{}}`
	if _, err := ParseTool(Claude, "PreToolUse", strings.NewReader(payload)); err == nil {
		t.Fatal("mismatched hook event accepted")
	}
	// Some client builds emit camelCase event names (e.g. "preToolUse") in hook_event_name.
	camel := `{"hook_event_name":"preToolUse","tool_name":"Read","tool_input":{"file_path":"a.go"}}`
	if got, err := ParseTool(Claude, "PreToolUse", strings.NewReader(camel)); err != nil || got.Phase != "pre" || got.Path != "a.go" {
		t.Fatalf("camelCase hook_event_name failed: got=%+v err=%v", got, err)
	}
	if _, err := ParseTool(Claude, "Stop", strings.NewReader(payload)); err == nil {
		t.Fatal("unsupported event accepted")
	}
	big := `{"tool_name":"` + strings.Repeat("a", MaxInputBytes) + `"}`
	if _, err := ParseTool(Claude, "PreToolUse", strings.NewReader(big)); err == nil {
		t.Fatal("oversized payload accepted")
	}
}

func TestParseToolAntigravityErrorIsTheFailureSignal(t *testing.T) {
	const args = `"conversationId":"c","workspacePaths":["/w"],"toolCall":{"name":"run_command","args":{"command":"go test ./internal/x"}}`
	ok, err := ParseTool(Antigravity, "PostToolUse", strings.NewReader(`{`+args+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok.Result != "ok" || ok.Kind != KindTest || ok.CWD != "/w" || ok.Session != "c" {
		t.Fatalf("success parsed as %+v", ok)
	}
	failed, err := ParseTool(Antigravity, "PostToolUse", strings.NewReader(`{`+args+`,"error":"exit status 1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Result != "error" || failed.Error != "exit status 1" {
		t.Fatalf("failure parsed as %+v", failed)
	}
	pre, err := ParseTool(Antigravity, "PreToolUse", strings.NewReader(`{`+args+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if pre.Result != "" || pre.Phase != "pre" {
		t.Fatalf("pre event has result %+v", pre)
	}
}

func TestClassifyCommand(t *testing.T) {
	cases := map[string]ToolKind{
		"go test ./...":          KindTest,
		"npm run test -- x":      KindTest,
		"rg -n Release internal": KindSearch,
		"git grep Release":       KindSearch,
		"sed -n 1,20p a.go":      KindRead,
		"sed -i s/a/b/ a.go":     KindEdit,
		"cat a.go | wc -l":       KindRead,
		"ls -la":                 KindOther,
		"":                       KindOther,
	}
	for cmd, want := range cases {
		if got := classifyCommand(cmd); got != want {
			t.Errorf("classifyCommand(%q) = %s, want %s", cmd, got, want)
		}
	}
}
