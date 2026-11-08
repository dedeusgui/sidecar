package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// These tests are copied into a disposable checkout only after an agent finishes.
func TestBenchmarkURLFormatEncodedSegments(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.URLFormat)
	r.Get("/articles/{id}", func(w http.ResponseWriter, req *http.Request) {
		format, _ := req.Context().Value(middleware.URLFormatCtxKey).(string)
		fmt.Fprintf(w, "%s|%s", chi.URLParam(req, "id"), format)
	})
	r.Get("/users/{u}/posts/{p}", func(w http.ResponseWriter, req *http.Request) {
		format, _ := req.Context().Value(middleware.URLFormatCtxKey).(string)
		fmt.Fprintf(w, "%s|%s|%s", chi.URLParam(req, "u"), chi.URLParam(req, "p"), format)
	})

	for _, tc := range []struct {
		name   string
		path   string
		status int
		want   string
	}{
		{"plain with extension", "/articles/ab.json", http.StatusOK, "ab|json"},
		{"plain without extension", "/articles/ab", http.StatusOK, "ab|"},
		{"encoded slash with extension", "/articles/a%2Fb.json", http.StatusOK, "a%2Fb|json"},
		{"encoded slash without extension", "/articles/a%2Fb", http.StatusOK, "a%2Fb|"},
		{"encoded dot is not a format separator", "/articles/x%2Ey", http.StatusOK, "x%2Ey|"},
		{"encoded dot with extension", "/articles/x%2Ey.json", http.StatusOK, "x%2Ey|json"},
		{"encoded segments in two params", "/users/a%2Fb/posts/c%2Fd.xml", http.StatusOK, "a%2Fb|c%2Fd|xml"},
		{"unicode with extension", "/articles/%C3%A7%C3%A3o.json", http.StatusOK, "ção|json"},
		{"unknown route with encoded slash", "/nothing/a%2Fb.json", http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.status, w.Body.String())
			}
			if tc.status == http.StatusOK && w.Body.String() != tc.want {
				t.Errorf("body = %q, want %q", w.Body.String(), tc.want)
			}
		})
	}
}
