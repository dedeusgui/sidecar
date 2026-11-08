package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// statusFormatter records the status passed to LogEntry.Write for every request.
type statusFormatter struct{ statuses *[]int }

type statusEntry struct{ statuses *[]int }

func (f statusFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	return statusEntry{statuses: f.statuses}
}

func (e statusEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra any) {
	*e.statuses = append(*e.statuses, status)
}

func (e statusEntry) Panic(v any, stack []byte) {}

func TestBenchmarkLoggerRecordsClientStatus(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		handler http.HandlerFunc
		want    int
	}{
		{"handler writes nothing", "/empty", func(w http.ResponseWriter, r *http.Request) {}, http.StatusOK},
		{"handler writes body", "/write", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }, http.StatusOK},
		{"handler sets 201", "/created", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }, http.StatusCreated},
		{"handler sets 404", "/missing", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var statuses []int
			r := chi.NewRouter()
			r.Use(middleware.RequestLogger(statusFormatter{statuses: &statuses}))
			r.Get(tc.path, tc.handler)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))

			if w.Code != tc.want {
				t.Fatalf("client status = %d, want %d", w.Code, tc.want)
			}
			if len(statuses) != 1 || statuses[0] != tc.want {
				t.Errorf("logged statuses = %v, want [%d] (the status the client received)", statuses, tc.want)
			}
		})
	}
}
