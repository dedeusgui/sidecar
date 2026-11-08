package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// These tests are copied into a disposable checkout only after an agent finishes.
func TestBenchmarkSupressNotFoundParity(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }
	build := func(suppress bool, calls *int) *chi.Mux {
		r := chi.NewRouter()
		if suppress {
			r.Use(middleware.SupressNotFound(r))
		}
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				*calls++
				next.ServeHTTP(w, req)
			})
		})
		r.Get("/only", ok)
		r.Get("/a/{x}", ok)
		r.Post("/items", ok)
		r.Route("/api", func(s chi.Router) {
			s.Get("/items/{id}", ok)
			s.Delete("/items/{id}", ok)
		})
		return r
	}

	type result struct {
		code  int
		allow string
		body  string
	}
	serve := func(r *chi.Mux, method, path string) result {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader("")))
		// The order of methods in Allow is not part of the contract: sort them.
		var allow []string
		for _, v := range w.Header().Values("Allow") {
			allow = append(allow, strings.Split(v, ",")...)
		}
		for i := range allow {
			allow[i] = strings.TrimSpace(allow[i])
		}
		slices.Sort(allow)
		return result{w.Code, strings.Join(allow, ","), w.Body.String()}
	}

	for _, tc := range []struct {
		method, path string
		wantCode     int
	}{
		{"GET", "/only", http.StatusOK},
		{"POST", "/only", http.StatusMethodNotAllowed},
		{"HEAD", "/only", http.StatusMethodNotAllowed},
		{"GET", "/a/x%2Fy", http.StatusOK},
		{"GET", "/a/b%20c", http.StatusOK},
		{"GET", "/items", http.StatusMethodNotAllowed},
		{"POST", "/items", http.StatusOK},
		{"DELETE", "/api/items/1", http.StatusOK},
		{"GET", "/api/items/1%2F2", http.StatusOK},
		{"PUT", "/api/items/1", http.StatusMethodNotAllowed},
		{"GET", "/nope", http.StatusNotFound},
		{"POST", "/api/nope", http.StatusNotFound},
		{"GET", "/only/extra", http.StatusNotFound},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			plainCalls, suppCalls := 0, 0
			plain := serve(build(false, &plainCalls), tc.method, tc.path)
			supp := serve(build(true, &suppCalls), tc.method, tc.path)
			if plain.code != tc.wantCode {
				t.Fatalf("reference router status = %d, want %d (test setup)", plain.code, tc.wantCode)
			}
			if supp != plain {
				t.Errorf("with middleware = %+v, without = %+v", supp, plain)
			}
			if tc.wantCode == http.StatusNotFound && suppCalls != 0 {
				t.Errorf("unknown route reached the next middleware %d time(s); want short-circuit", suppCalls)
			}
		})
	}
}
