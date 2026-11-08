package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// These tests are copied into a disposable checkout only after an agent finishes.
func TestBenchmarkAllowContentTypeWildcard(t *testing.T) {
	for _, tc := range []struct {
		name      string
		allow     []string
		body      string
		ctype     string
		wantCode  int
		wantCalls int
	}{
		{"wildcard matches png", []string{"image/*"}, "x", "image/png", http.StatusNoContent, 1},
		{"wildcard matches svg with plus", []string{"image/*"}, "x", "image/svg+xml", http.StatusNoContent, 1},
		{"wildcard is case-insensitive and ignores params", []string{"image/*"}, "x", "IMAGE/PNG; charset=utf-8", http.StatusNoContent, 1},
		{"padded wildcard entry is normalized", []string{"  Image/*  "}, "x", "image/jpeg", http.StatusNoContent, 1},
		{"wildcard rejects other top-level type", []string{"image/*"}, "x", "text/plain", http.StatusUnsupportedMediaType, 0},
		{"wildcard rejects top-level name prefix", []string{"image/*"}, "x", "imagex/png", http.StatusUnsupportedMediaType, 0},
		{"wildcard rejects bare top-level name", []string{"image/*"}, "x", "image", http.StatusUnsupportedMediaType, 0},
		{"wildcard rejects empty subtype", []string{"image/*"}, "x", "image/", http.StatusUnsupportedMediaType, 0},
		{"exact entry still matches", []string{"application/json"}, "{}", "application/json; charset=utf-8", http.StatusNoContent, 1},
		{"exact entry does not act as wildcard", []string{"application/json"}, "{}", "application/json-patch+json", http.StatusUnsupportedMediaType, 0},
		{"mixed exact and wildcard entries", []string{"application/json", "image/*"}, "x", "image/gif", http.StatusNoContent, 1},
		{"mixed entries still reject unknown", []string{"application/json", "image/*"}, "x", "video/mp4", http.StatusUnsupportedMediaType, 0},
		{"missing content type with body is rejected", []string{"image/*"}, "x", "", http.StatusUnsupportedMediaType, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusNoContent)
			})
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.ctype != "" {
				r.Header.Set("Content-Type", tc.ctype)
			}
			w := httptest.NewRecorder()
			middleware.AllowContentType(tc.allow...)(next).ServeHTTP(w, r)
			if w.Code != tc.wantCode || calls != tc.wantCalls {
				t.Errorf("code=%d calls=%d, want code=%d calls=%d", w.Code, calls, tc.wantCode, tc.wantCalls)
			}
		})
	}

	// Requests without a body skip the Content-Type check, as before.
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	middleware.AllowContentType("image/*")(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNoContent || calls != 1 {
		t.Errorf("empty body: code=%d calls=%d, want code=%d calls=1", w.Code, calls, http.StatusNoContent)
	}
}
