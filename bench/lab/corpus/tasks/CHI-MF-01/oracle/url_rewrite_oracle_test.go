package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func TestBenchmarkURLRewriteEscapedAndMounted(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	t.Run("path rewrite with percent-encoded path", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.PathRewrite("/old", "/new"))
		r.Get("/new/{name}", okHandler)
		for _, p := range []string{"/old/plain", "/old/x%41", "/old/%7Euser", "/old/caf%C3%A9"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
			if w.Code != http.StatusOK || w.Body.String() != "ok" {
				t.Errorf("GET %s: status=%d body=%q, want 200 \"ok\"", p, w.Code, w.Body.String())
			}
		}
	})

	t.Run("redirect slashes inside a route is relative to the full path", func(t *testing.T) {
		r := chi.NewRouter()
		r.Route("/api", func(a chi.Router) {
			a.Use(middleware.RedirectSlashes)
			a.Get("/items", okHandler)
		})
		cases := map[string]string{
			"/api/items/":        "/api/items",
			"/api/items/?page=2": "/api/items?page=2",
		}
		for req, loc := range cases {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", req, nil))
			if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != loc {
				t.Errorf("GET %s: status=%d Location=%q, want 301 Location=%q", req, w.Code, w.Header().Get("Location"), loc)
			}
		}
	})

	t.Run("redirect slashes at top level still works", func(t *testing.T) {
		r := chi.NewRouter()
		r.Use(middleware.RedirectSlashes)
		r.Get("/api/items", okHandler)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/items/", nil))
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/api/items" {
			t.Errorf("status=%d Location=%q, want 301 Location=\"/api/items\"", w.Code, w.Header().Get("Location"))
		}
	})
}
