package extract

import "testing"

// Regression for gap L03: CommonJS definitions assigned to members must be indexed as symbols.
func TestJavaScriptAssignedFunctionsAreDefinitions(t *testing.T) {
	src := []byte(`'use strict';
var res = Object.create(http.ServerResponse.prototype)
module.exports = res

/**
 * Send a response.
 */
res.send = function send(body) {
  var chunk = body;
  return this;
};

res.json = (obj) => { return this.send(JSON.stringify(obj)); };

exports.compileETag = function(val) { return val; }

Router.prototype.handle = function handle(req, res, out) {
  next();
};

module.exports = function createApplication() { return {} };

app.set('etag fn', compile);
var x = 1;
x = 2;
`)
	res, err := New().File("javascript", src)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"send": "function", "json": "function", "compileETag": "function", "handle": "method", "createApplication": "function"}
	got := map[string]Symbol{}
	for _, s := range res.Symbols {
		got[s.Name] = s
	}
	for name, kind := range want {
		s, ok := got[name]
		if !ok {
			t.Errorf("%s not extracted; symbols=%v", name, res.Symbols)
			continue
		}
		if s.Kind != kind {
			t.Errorf("%s kind=%s want %s", name, s.Kind, kind)
		}
	}
	if s := got["send"]; s.Line != 7 || s.EndLine != 10 || s.Doc == "" {
		t.Errorf("send span/doc = %d-%d %q", s.Line, s.EndLine, s.Doc)
	}
	for _, bad := range []string{"exports", "module", "x"} {
		if s, ok := got[bad]; ok && (s.Kind == "function" || s.Kind == "method") {
			t.Errorf("%s must not be a function definition: %+v", bad, s)
		}
	}
}

func TestTypeScriptAssignedArrowFunction(t *testing.T) {
	res, err := New().File("typescript", []byte("export const h = {} as any\nh.validate = (input: string): boolean => input.length > 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Symbols {
		if s.Name == "validate" && s.Kind == "function" {
			return
		}
	}
	t.Fatalf("validate not extracted: %v", res.Symbols)
}
