package middleware_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// These tests are copied into a disposable checkout only after an agent finishes.
func TestBenchmarkRequestSizeKnownLength(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limit      int64
		body       string
		declared   int64
		wantStatus int
		wantCalls  int
	}{
		{"over limit", 3, "abcd", 4, http.StatusRequestEntityTooLarge, 0},
		{"equal to limit", 3, "abc", 3, http.StatusNoContent, 1},
		{"below limit", 3, "ab", 2, http.StatusNoContent, 1},
		{"zero limit", 0, "x", 1, http.StatusRequestEntityTooLarge, 0},
		{"unknown and within limit", 3, "abc", -1, http.StatusNoContent, 1},
		{"unknown and oversized", 3, "abcd", -1, http.StatusRequestEntityTooLarge, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				_, err := io.ReadAll(r.Body)
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					w.WriteHeader(http.StatusRequestEntityTooLarge)
					return
				}
				if err != nil {
					t.Errorf("reading request body: %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.ContentLength = tc.declared
			w := httptest.NewRecorder()
			middleware.RequestSize(tc.limit)(next).ServeHTTP(w, r)
			if w.Code != tc.wantStatus || calls != tc.wantCalls {
				t.Errorf("status=%d calls=%d, want status=%d calls=%d", w.Code, calls, tc.wantStatus, tc.wantCalls)
			}
		})
	}
}
