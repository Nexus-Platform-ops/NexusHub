package observability

import (
	"net/http"
	"time"
)

// Middleware records request duration for every request with the ServeMux
// pattern as the route label. Go 1.22+ ServeMux sets r.Pattern before
// invoking the handler, so reading it after next() returns gives the matched
// pattern; requests that matched nothing are labelled "unmatched".
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &codeRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		m.ObserveHTTP(route, r.Method, time.Since(start), rec.status)
	})
}

type codeRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *codeRecorder) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *codeRecorder) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}
