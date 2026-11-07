package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// recordingTransport captures events instead of sending them.
type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *recordingTransport) Configure(sentry.ClientOptions) {}
func (t *recordingTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}
func (t *recordingTransport) Flush(time.Duration) bool              { return true }
func (t *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordingTransport) Close()                                {}
func (t *recordingTransport) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.events)
}

func withRecordingSentry(t *testing.T) *recordingTransport {
	t.Helper()
	tr := &recordingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: tr,
	})
	if err != nil {
		t.Fatalf("sentry client: %v", err)
	}
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return tr
}

func TestSentryCapturesServerError(t *testing.T) {
	tr := withRecordingSentry(t)
	h := logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverError(w, "load profile", errors.New("boom +959123456789"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/me?token=secret", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if tr.count() != 1 {
		t.Fatalf("events = %d want 1 (no duplicate 5xx message)", tr.count())
	}
	ev := tr.events[0]
	if got := ev.Tags["handler"]; got != "load profile" {
		t.Fatalf("handler tag = %q", got)
	}
	if ev.Tags["http.path"] != "/api/v1/me" {
		t.Fatalf("path tag = %q", ev.Tags["http.path"])
	}
	if ev.Tags["http.method"] != "GET" {
		t.Fatalf("method tag = %q", ev.Tags["http.method"])
	}
}

func TestSentryRecoversPanic(t *testing.T) {
	tr := withRecordingSentry(t)
	h := logMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/posts", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d want 500", rec.Code)
	}
	if tr.count() != 1 {
		t.Fatalf("events = %d want 1", tr.count())
	}
}

func TestSentryIgnoresHealthFailures(t *testing.T) {
	tr := withRecordingSentry(t)
	h := logMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if tr.count() != 0 {
		t.Fatalf("health failure must not page Sentry, got %d events", tr.count())
	}
}
