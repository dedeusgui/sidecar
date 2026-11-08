package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestBenchmarkCompressQValueZero(t *testing.T) {
	h := middleware.NewCompressor(5).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("hello ", 200)))
	}))

	cases := []struct {
		accept string
		want   string
	}{
		{"gzip;q=0", ""},
		{"gzip;q=0, deflate", "deflate"},
		{"deflate;q=0, gzip", "gzip"},
		{"gzip;q=0.5", "gzip"},
		{"gzip", "gzip"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", tc.accept)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if got := w.Header().Get("Content-Encoding"); got != tc.want {
			t.Errorf("Accept-Encoding %q: Content-Encoding = %q, want %q", tc.accept, got, tc.want)
		}
	}
}
