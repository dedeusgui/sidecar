package extract

import (
	"os"
	"strings"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func extractFile(t *testing.T, lang, name string) Result {
	t.Helper()
	res, err := New().File(lang, load(t, name))
	if err != nil {
		t.Fatalf("File(%s): %v", name, err)
	}
	return res
}

func find(r Result, kind, name string) (Symbol, int, bool) {
	for i, s := range r.Symbols {
		if s.Kind == kind && s.Name == name {
			return s, i, true
		}
	}
	return Symbol{}, -1, false
}

func hasImport(r Result, module string) bool {
	for _, imp := range r.Imports {
		if imp == module {
			return true
		}
	}
	return false
}

func hasRef(r Result, from, name string) bool {
	for _, ref := range r.Refs {
		if ref.Name == name && ref.From >= 0 && r.Symbols[ref.From].Name == from {
			return true
		}
	}
	return false
}

func TestGoDefinitionsParentsConstantsAndDocs(t *testing.T) {
	r := extractFile(t, "go", "handler.go")
	s, _, ok := find(r, "struct", "ManualReleaseHandler")
	if !ok {
		t.Fatalf("struct ManualReleaseHandler missing: %+v", r.Symbols)
	}
	if !strings.Contains(s.Doc, "releases vehicles") {
		t.Errorf("doc comment not captured: %q", s.Doc)
	}
	m, mi, ok := find(r, "method", "Release")
	if !ok {
		t.Fatalf("method Release missing")
	}
	if m.Parent < 0 || r.Symbols[m.Parent].Name != "ManualReleaseHandler" {
		t.Errorf("method Release parent = %d, want ManualReleaseHandler", m.Parent)
	}
	_ = mi
	c, _, ok := find(r, "constant", "MaxActiveTickets")
	if !ok || c.Value != "1" {
		t.Errorf("constant MaxActiveTickets = %+v (ok=%v), want value 1", c, ok)
	}
	if !hasImport(r, "net/http") || !hasImport(r, "strings") {
		t.Errorf("imports = %v", r.Imports)
	}
	if !hasRef(r, "Release", "TrimSpace") || !hasRef(r, "Release", "Enqueue") {
		t.Errorf("call references from Release missing: %+v", r.Refs)
	}
}

func TestPythonTopLevelConstantsAndMethods(t *testing.T) {
	r := extractFile(t, "python", "queue.py")
	if c, _, ok := find(r, "constant", "MAX_ACTIVE_TICKETS"); !ok || c.Value != "1" {
		t.Errorf("MAX_ACTIVE_TICKETS = %+v (ok=%v)", c, ok)
	}
	if m, _, ok := find(r, "method", "enqueue"); !ok || r.Symbols[m.Parent].Name != "TicketQueue" {
		t.Errorf("enqueue parent wrong: %+v", m)
	}
	if _, _, ok := find(r, "function", "process_manual_release"); !ok {
		t.Errorf("function process_manual_release missing")
	}
	if !hasImport(r, "app.billing.invoice") || !hasImport(r, "json") {
		t.Errorf("imports = %v", r.Imports)
	}
	if !hasRef(r, "process_manual_release", "enqueue") {
		t.Errorf("call to enqueue not referenced from process_manual_release")
	}
}

func TestTypeScriptExportedDeclarations(t *testing.T) {
	r := extractFile(t, "typescript", "ticketQueue.ts")
	if c, _, ok := find(r, "constant", "MAX_ACTIVE_TICKETS"); !ok || c.Value != "2" {
		t.Errorf("MAX_ACTIVE_TICKETS = %+v (ok=%v)", c, ok)
	}
	if _, _, ok := find(r, "class", "TicketQueue"); !ok {
		t.Errorf("class TicketQueue missing: %+v", r.Symbols)
	}
	if _, _, ok := find(r, "function", "advanceVehicle"); !ok {
		t.Errorf("function advanceVehicle missing")
	}
	if !hasImport(r, "./billing/invoice") {
		t.Errorf("imports = %v", r.Imports)
	}
}

func TestJavaStaticFinalConstantsAndImports(t *testing.T) {
	r := extractFile(t, "java", "TicketQueue.java")
	if c, _, ok := find(r, "constant", "MAX_ACTIVE_TICKETS"); !ok || c.Value != "1" {
		t.Errorf("MAX_ACTIVE_TICKETS = %+v (ok=%v)", c, ok)
	}
	if m, _, ok := find(r, "method", "enqueue"); !ok || r.Symbols[m.Parent].Name != "TicketQueue" {
		t.Errorf("enqueue parent wrong: %+v", m)
	}
	if !hasImport(r, "app.billing.Invoice") {
		t.Errorf("imports = %v", r.Imports)
	}
}

func TestCSharpConstAndUsing(t *testing.T) {
	r := extractFile(t, "c_sharp", "TicketQueue.cs")
	if c, _, ok := find(r, "constant", "MaxActiveTickets"); !ok || c.Value != "1" {
		t.Errorf("MaxActiveTickets = %+v (ok=%v)", c, ok)
	}
	if _, _, ok := find(r, "method", "Enqueue"); !ok {
		t.Errorf("method Enqueue missing: %+v", r.Symbols)
	}
	if !hasImport(r, "System.Collections.Generic") {
		t.Errorf("imports = %v", r.Imports)
	}
}

func TestSQLTablesAreSymbols(t *testing.T) {
	r := extractFile(t, "sql", "schema.sql")
	if _, _, ok := find(r, "table", "tickets"); !ok {
		t.Errorf("table tickets missing: %+v", r.Symbols)
	}
	if _, _, ok := find(r, "table", "queue_entries"); !ok {
		t.Errorf("table queue_entries missing: %+v", r.Symbols)
	}
}

func TestMarkdownHeadingsAreSections(t *testing.T) {
	src := []byte("# Guia de Liberação\n\nTexto.\n\n## Fila de caminhões\n")
	res, err := New().File("markdown", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := find(res, "section", "Fila de caminhões"); !ok {
		t.Errorf("section missing: %+v", res.Symbols)
	}
}

func TestRoutesFromCommonFrameworks(t *testing.T) {
	src := []byte("app.get('/api/tickets', listTickets);\n@GetMapping(\"/owners/{id}\")\n")
	res, err := New().File("javascript", src)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := find(res, "route", "GET /api/tickets"); !ok {
		t.Errorf("express route missing: %+v", res.Symbols)
	}
	if _, _, ok := find(res, "route", "GET /owners/{id}"); !ok {
		t.Errorf("spring route missing: %+v", res.Symbols)
	}
}

func TestSecretLikeConstantsAreNeverStored(t *testing.T) {
	src := []byte("package c\n\nconst APIKey = \"sk_live_123456789\"\n")
	res, err := New().File("go", src)
	if err != nil {
		t.Fatal(err)
	}
	s, _, ok := find(res, "constant", "APIKey")
	if !ok {
		t.Fatalf("constant APIKey should still be listed by name: %+v", res.Symbols)
	}
	if s.Value != "" {
		t.Fatalf("string constant value stored: %q", s.Value)
	}
}

func TestPythonTripleQuotedDocstringIsExtracted(t *testing.T) {
	src := []byte("def echo(\n    message=None,\n    nl=True,\n):\n    \"\"\"Print a message and newline to stdout or a terminal.\"\"\"\n    pass\n")
	res, err := New().File("python", src)
	if err != nil {
		t.Fatal(err)
	}
	s, _, ok := find(res, "function", "echo")
	if !ok {
		t.Fatalf("function echo missing: %+v", res.Symbols)
	}
	if !strings.Contains(s.Doc, "terminal") {
		t.Fatalf("docstring not extracted for echo: got %q", s.Doc)
	}
}
