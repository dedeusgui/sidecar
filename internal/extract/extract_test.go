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

func TestPHPCallsConstantsImportsAndTypeRefs(t *testing.T) {
	src := []byte(`<?php
namespace Monolog;
use Monolog\Handler\HandlerInterface;
use Monolog\Handler\AbstractProcessingHandler as BaseHandler;
use Psr\Log\{LoggerInterface, LogLevel};

const VERSION = 2;
const SECRET_TOKEN = "sk_live_secret";

class Logger extends \Monolog\BaseLogger implements LoggerInterface, ResettableInterface {
    const DEBUG = 100;
    public function addRecord($level, $message) {
        $this->handle($record);
        $handler?->write($record);
        self::format($message);
        \Monolog\Formatter::build($message);
        helper_fn($level);
        return new \Monolog\LogRecord($message);
    }
}
`)
	res, err := New().File("php", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range []string{
		`Monolog\Handler\HandlerInterface`,
		`Monolog\Handler\AbstractProcessingHandler`,
		`Psr\Log\LoggerInterface`,
		`Psr\Log\LogLevel`,
	} {
		if !hasImport(res, imp) {
			t.Errorf("missing PHP import %q in %v", imp, res.Imports)
		}
	}
	if c, _, ok := find(res, "constant", "VERSION"); !ok || c.Value != "2" {
		t.Errorf("VERSION constant = %+v (ok=%v)", c, ok)
	}
	if c, _, ok := find(res, "constant", "SECRET_TOKEN"); !ok || c.Value != "" {
		t.Errorf("SECRET_TOKEN constant = %+v (ok=%v), string value must not be stored", c, ok)
	}
	if c, _, ok := find(res, "constant", "DEBUG"); !ok || c.Value != "100" || c.Parent < 0 || res.Symbols[c.Parent].Name != "Logger" {
		t.Errorf("DEBUG class constant = %+v (ok=%v)", c, ok)
	}
	for _, callee := range []string{"handle", "write", "format", "build", "helper_fn", "LogRecord"} {
		if !hasRef(res, "addRecord", callee) {
			t.Errorf("missing PHP call ref from addRecord -> %s: %+v", callee, res.Refs)
		}
	}
	for _, typ := range []string{"BaseLogger", "LoggerInterface", "ResettableInterface"} {
		found := false
		for _, r := range res.Refs {
			if r.Kind == "type" && r.Name == typ && r.From >= 0 && res.Symbols[r.From].Name == "Logger" {
				found = true
			}
		}
		if !found {
			t.Errorf("missing PHP type ref Logger -> %s: %+v", typ, res.Refs)
		}
	}
}

func TestRubyCallsImportsAndSinatraRoutes(t *testing.T) {
	src := []byte(`require 'rack'
require_relative 'sinatra/base'

class Base
  before '/admin/*' do
    filter!(:before)
  end

  get '/hello' do
    dispatch!
  end

  def dispatch!
    route!
    filter!(:dispatch)
    valid?(@env)
    self.process_route
  end

  def self.configure
    setup_routes
  end
end
`)
	res, err := New().File("ruby", src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasImport(res, "rack") || !hasImport(res, "./sinatra/base") {
		t.Errorf("ruby imports = %v", res.Imports)
	}
	for _, route := range []string{"BEFORE /admin/*", "GET /hello"} {
		if _, _, ok := find(res, "route", route); !ok {
			t.Errorf("missing ruby route %q in %+v", route, res.Symbols)
		}
	}
	for _, callee := range []string{"route!", "filter!", "valid?", "process_route"} {
		if !hasRef(res, "dispatch!", callee) {
			t.Errorf("missing ruby call ref dispatch! -> %s: %+v", callee, res.Refs)
		}
	}
	for _, r := range res.Refs {
		if (r.Name == "require" || r.Name == "require_relative") && r.Kind == "call" {
			t.Errorf("require must not be emitted as call ref: %+v", r)
		}
		if r.From >= 0 && res.Symbols[r.From].Name == r.Name {
			t.Errorf("self-declaration emitted as call ref: %+v", r)
		}
	}
}

func TestCommonJSRequireAndPythonDecoratorCalls(t *testing.T) {
	js := []byte(`const utils = require('./utils');
var app = module.exports = function(req, res) { return utils.normalize(req); };
`)
	jsRes, err := New().File("javascript", js)
	if err != nil {
		t.Fatal(err)
	}
	if !hasImport(jsRes, "./utils") {
		t.Errorf("CommonJS require('./utils') missing: %v", jsRes.Imports)
	}
	if _, _, ok := find(jsRes, "function", "app"); !ok {
		t.Errorf("chained CommonJS var app = module.exports = function missing: %+v", jsRes.Symbols)
	}

	py := []byte(`import click

@click.group()
def main():
    pass

@main.command()
@click.option('--count', default=1)
@simple_dec
def hello(count):
    helper()

@app.get('/items')
def get_items():
    pass
`)
	pyRes, err := New().File("python", py)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main", "command", "simple_dec", "helper"} {
		if !hasRef(pyRes, "hello", name) {
			t.Errorf("missing Python decorator/body call ref hello -> %s: %+v", name, pyRes.Refs)
		}
	}
	if hasRef(pyRes, "get_items", "get") {
		t.Errorf("route decorator @app.get should not emit noisy 'get' call ref: %+v", pyRes.Refs)
	}
}
