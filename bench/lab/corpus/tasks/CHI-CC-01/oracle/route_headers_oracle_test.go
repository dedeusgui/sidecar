package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestBenchmarkRouteHeadersDeterministic(t *testing.T) {
	tag := func(name string) func(next http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Route", name)
				next.ServeHTTP(w, r)
			})
		}
	}
	end := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	t.Run("same request always selects the same route", func(t *testing.T) {
		h := middleware.RouteHeaders().
			Route("Origin", "https://app.example.com", tag("origin")).
			Route("Host", "example.com", tag("host")).
			Handler(end)

		seen := map[string]int{}
		for i := 0; i < 200; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Origin", "https://app.example.com")
			req.Header.Set("Host", "example.com")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			seen[w.Header().Get("X-Route")]++
		}
		if len(seen) != 1 || seen[""] != 0 {
			t.Errorf("route chosen for identical requests varied across runs: %v", seen)
		}
	})

	t.Run("single matching header routes to its middleware", func(t *testing.T) {
		h := middleware.RouteHeaders().
			Route("Host", "example.com", tag("host")).
			Route("Host", "*.example.org", tag("org")).
			Handler(end)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Host", "api.example.org")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if got := w.Header().Get("X-Route"); got != "org" {
			t.Errorf("X-Route = %q, want \"org\"", got)
		}
	})

	t.Run("default route applies when no header matches", func(t *testing.T) {
		h := middleware.RouteHeaders().
			Route("Host", "example.com", tag("host")).
			RouteDefault(tag("default")).
			Handler(end)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Host", "other.test")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if got := w.Header().Get("X-Route"); got != "default" {
			t.Errorf("X-Route = %q, want \"default\"", got)
		}
	})
}
