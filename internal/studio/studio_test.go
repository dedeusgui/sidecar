package studio

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestBuildEmbedsDataAndNeverBreaksOutOfTheDataBlock(t *testing.T) {
	evil := `</script><img src=x onerror="window.__pwned=1"><!--  `
	page, err := Build([]Input{{Name: "explain.json", Raw: []byte(`{"candidates":[{"Name":"` + strings.ReplaceAll(evil, `"`, `\"`) + `"}],"evidence":[],"context":"ok"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if strings.Contains(s, "<img src=x") || strings.Count(s, "</script>") != 1 {
		t.Fatal("untrusted data escaped the JSON data block")
	}
	if !strings.Contains(s, `</script>`) {
		t.Fatal("the closing tag must be escaped inside the data")
	}
	if strings.Count(s, "<script>") != 1 {
		t.Fatalf("exactly one script block expected, got %d", strings.Count(s, "<script>"))
	}
}

func TestPageHasNoExternalResourcesAndAStrictCSP(t *testing.T) {
	page, err := Build([]Input{{Name: "x", Raw: []byte(`{"schema":"perf/1"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?(https?:)?//`).MatchString(s) {
		t.Fatal("the page must not load external resources")
	}
	if !strings.Contains(s, "default-src 'none'") {
		t.Fatal("missing restrictive CSP")
	}
	if strings.Contains(s, "innerHTML") || strings.Contains(s, "document.write") || strings.Contains(s, "eval(") {
		t.Fatal("data must only ever reach the DOM as text")
	}
}

func TestKindIsDecidedByContentNotName(t *testing.T) {
	cases := map[string]string{
		`{"schema":"replay/1"}`:                        "replay/1",
		`{"schema":"delivery/1"}`:                      "delivery/1",
		`{"engines":["v3"],"summary":[],"rows":[]}`:    "delivery/1",
		`{"schema":"e2-offline/1"}`:                    "e2-offline/1",
		`{"summary":{},"rows":[]}`:                     "e2-offline/1",
		`{"candidates":[],"evidence":[]}`:              "explain",
		`[{"schema":"lab-run/1","task":"T"}]`:          "lab-run/1[]",
		`{"anything":1}`:                               "unknown",
		`[1,2]`:                                        "list",
	}
	for raw, want := range cases {
		page, err := Build([]Input{{Name: "lab-run.json", Raw: []byte(raw)}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(page), `"kind":"`+want+`"`) {
			t.Errorf("%s: want kind %s", raw, want)
		}
	}
}

func TestJSONLinesAreAcceptedAndInvalidInputIsAnError(t *testing.T) {
	page, err := Build([]Input{{Name: "runs.jsonl", Raw: []byte("{\"schema\":\"lab-run/1\",\"task\":\"A\"}\n\n{\"schema\":\"lab-run/1\",\"task\":\"B\"}\n")}})
	if err != nil || !strings.Contains(string(page), `"kind":"lab-run/1[]"`) {
		t.Fatalf("jsonl: %v", err)
	}
	if _, err := Build([]Input{{Name: "bad", Raw: []byte("not json")}}); err == nil {
		t.Fatal("invalid input must be an error, not silently dropped")
	}
	if _, err := Build(nil); err == nil {
		t.Fatal("no input must be an error")
	}
}

func TestSizeLimits(t *testing.T) {
	big := []byte(`{"x":"` + strings.Repeat("a", MaxInputBytes) + `"}`)
	if _, err := Build([]Input{{Name: "big", Raw: big}}); err == nil {
		t.Fatal("an input over the limit must be rejected")
	}
}

func TestHostileArmNamesDoNotNeedPrototypeObjects(t *testing.T) {
	if !strings.Contains(template, "Object.create(null)") || strings.Contains(template, "const arms = {};") {
		t.Fatal("run summaries must use prototype-less maps: an arm named __proto__ would corrupt Object.prototype")
	}
}

func TestTemplateSlotExists(t *testing.T) {
	if _, err := os.Stat("template.html"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(template, "/*__DATA__*/null") {
		t.Fatal("the data slot is missing")
	}
}
