package explore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
)

func project(t *testing.T, files map[string]string) (string, *index.Index) {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	return root, ix
}

func goProject(t *testing.T) (string, *index.Index) {
	var long strings.Builder
	long.WriteString("package scale\n\n// Settle computes the final weight of a truck on the scale.\nfunc Settle(kg int) int {\n")
	for i := 0; i < 90; i++ {
		fmt.Fprintf(&long, "\tkg += %d // padding line\n", i)
	}
	long.WriteString("\tkg = applyTare(kg) // tare applied here\n\treturn kg\n}\n\nfunc applyTare(kg int) int { return kg - 10 }\n")
	return project(t, map[string]string{
		"scale/settle.go":      long.String(),
		"scale/settle_test.go": "package scale\n\nimport \"testing\"\n\nfunc TestSettle(t *testing.T) { if Settle(100) == 0 { t.Fatal() } }\n",
		"gate/gate.go":         "package gate\n\n// OpenGate lets the truck leave once the weight is settled.\nfunc OpenGate(kg int) bool { return kg > 0 }\n",
	})
}

func TestExploreReturnsNumberedSourceRelationsAndTests(t *testing.T) {
	root, ix := goProject(t)
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "where is the tare applied when the truck weight is settled on the scale?"})
	if err != nil {
		t.Fatal(err)
	}
	var f File
	for _, x := range r.Files {
		if x.Path == "scale/settle.go" {
			f = x
		}
	}
	if r.Schema != Schema || f.Path == "" {
		t.Fatalf("unexpected result: %+v", r.Files)
	}
	text := ""
	for _, s := range f.Snippets {
		text += s.Text
	}
	if !strings.Contains(text, "4\tfunc Settle(kg int) int {") {
		t.Fatalf("snippet lines are not numbered from the file: %q", text[:min(len(text), 200)])
	}
	if !strings.Contains(text, "applyTare(kg) // tare applied here") {
		t.Fatalf("long body cut lost the line the query is about:\n%s", text)
	}
	if strings.Contains(text, "kg += 50 //") {
		t.Fatal("long body was not elided")
	}
	var callee bool
	for _, e := range r.Relations {
		callee = callee || e.To.Name == "applyTare" && e.Status == Verified && e.Basis == "same_file"
	}
	if !callee {
		t.Fatalf("same-file call not reported as VERIFIED: %+v", r.Relations)
	}
	var tested bool
	for _, x := range r.Tests {
		tested = tested || x.Path == "scale/settle_test.go"
	}
	if !tested {
		t.Fatalf("test file not linked: %+v", r.Tests)
	}
	if r.Budget.UsedChars > r.Budget.MaxChars {
		t.Fatalf("budget exceeded: %+v", r.Budget)
	}
	for _, e := range r.EntryPoints {
		if e.Status != Candidate {
			t.Fatalf("entry points must be CANDIDATE: %+v", e)
		}
	}
	if len(r.Gaps) == 0 {
		t.Fatal("limits of the analysis must be stated")
	}
}

func TestExploreRespectsCharBudget(t *testing.T) {
	root, ix := goProject(t)
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "settle weight truck scale tare", MaxChars: 300, MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if r.Budget.UsedChars > 300 || len(r.Files) > 1 {
		t.Fatalf("budget not respected: used=%d files=%d", r.Budget.UsedChars, len(r.Files))
	}
	if len(r.NotShown) == 0 {
		t.Fatal("files beyond the limit must be pointed at, not silently dropped")
	}
}

func TestExploreFlagsStaleFiles(t *testing.T) {
	root, ix := goProject(t)
	if err := os.WriteFile(filepath.Join(root, "gate/gate.go"), []byte("package gate\n\n// moved\n\n// OpenGate lets the truck leave once the weight is settled.\nfunc OpenGate(kg int) bool { return kg > 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "OpenGate"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Files {
		if f.Path == "gate/gate.go" && f.Stale {
			return
		}
	}
	t.Fatalf("edited file not flagged stale: %+v", r.Files)
}

func TestExploreOffTopicQueryReportsTheGateInsteadOfInventing(t *testing.T) {
	root, ix := goProject(t)
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "qual o melhor horário para plantar tomate na primavera?"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Relevance != "irrelevant" {
		t.Fatalf("relevance=%s", r.Relevance)
	}
	if len(r.Gaps) == 0 || !strings.Contains(strings.Join(r.Gaps, " "), "relevance gate") {
		t.Fatalf("the gate verdict must be visible: %v", r.Gaps)
	}
}

func TestExploreJavaScriptMemberQuery(t *testing.T) {
	root, ix := project(t, map[string]string{
		"lib/response.js": "var res = module.exports = {}\n\n/** Send a response. */\nres.send = function send(body) {\n  var etag = this.app.get('etag fn')\n  this.set('ETag', etag(body))\n  return this.end(body)\n}\n\nres.end = function end(body) { return body }\n",
		"test/send.js":    "var res = require('../lib/response')\nit('sends', function () { res.send('x') })\n",
	})
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "how does res.send set the ETag?"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Files) == 0 || r.Files[0].Path != "lib/response.js" {
		t.Fatalf("files=%+v", r.Files)
	}
	found := false
	for _, s := range r.Files[0].Symbols {
		found = found || s.Name == "send" && s.Kind == "function"
	}
	if !found {
		t.Fatalf("assigned JS function not shown: %+v", r.Files[0].Symbols)
	}
}

func TestExploreJSONContractAndText(t *testing.T) {
	root, ix := goProject(t)
	r, err := Run(context.Background(), ix.Store(), Options{Root: root, Query: "Settle"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"schema":"sidecar-explore/1"`, `"entry_points":`, `"files":`, `"relations":`, `"consumers":`, `"dependencies":`, `"tests":`, `"gaps":`, `"budget":`} {
		if !bytes.Contains(data, []byte(key)) {
			t.Errorf("JSON contract misses %s", key)
		}
	}
	var buf bytes.Buffer
	if err := WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "**`scale/settle.go`**") || !strings.HasSuffix(buf.String(), "[END SIDECAR EXPLORE]\n") {
		t.Fatalf("text rendering: %s", buf.String())
	}
}

func TestBudgetScalesWithRepositorySize(t *testing.T) {
	f1, c1 := BudgetFor(50)
	f2, c2 := BudgetFor(50000)
	if f1 >= f2 || c1 >= c2 {
		t.Fatalf("budget must grow with the repository: %d/%d vs %d/%d", f1, c1, f2, c2)
	}
}

func TestMemberKeys(t *testing.T) {
	got := strings.Join(memberKeys("how does res.send() and `Router.prototype.handle` and Logger::info work with route! and filter!? e.g. example.com"), ",")
	want := "res,send,Router,prototype,handle,Logger,info,route!,filter!,example,com"
	if got != want {
		t.Fatalf("memberKeys = %s, want %s", got, want)
	}
}
