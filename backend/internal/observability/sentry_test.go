package observability

import (
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"

	"nexus/backend/internal/config"
)

func TestScrubText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"duplicate key value: +959123456789", "duplicate key value: [redacted-number]"},
		{"contact aung@example.com failed", "contact [redacted-email] failed"},
		{"date 2026-11-07 and id 42 stay", "date 2026-11-07 and id 42 stay"},
		{"user 09123456789", "user [redacted-number]"},
	}
	for _, c := range cases {
		if got := ScrubText(c.in); got != c.want {
			t.Fatalf("ScrubText(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestScrubEventDropsPII(t *testing.T) {
	event := &sentry.Event{
		Message: "phone +959111222333",
		Request: &sentry.Request{URL: "https://api.example.com/radar?lat=16.8&lng=96.15", Data: "secret"},
		User:    sentry.User{ID: "42", Email: "aung@example.com"},
		Exception: []sentry.Exception{
			{Value: "db error for aung@example.com"},
		},
		Breadcrumbs: []*sentry.Breadcrumb{
			{Message: "http", Data: map[string]interface{}{"body": "otp=123456"}},
		},
		ServerName: "nexus-api-1",
	}
	out := scrubEvent(event, nil)
	if out.Request != nil {
		t.Fatal("request must be dropped")
	}
	if out.User.ID != "" || out.User.Email != "" {
		t.Fatalf("user must be cleared: %+v", out.User)
	}
	if out.ServerName != "" {
		t.Fatal("server name must be cleared")
	}
	if strings.Contains(out.Message, "959111222333") {
		t.Fatalf("message not scrubbed: %q", out.Message)
	}
	if strings.Contains(out.Exception[0].Value, "aung@example.com") {
		t.Fatalf("exception not scrubbed: %q", out.Exception[0].Value)
	}
	if out.Breadcrumbs[0].Data != nil {
		t.Fatal("breadcrumb data must be dropped")
	}
}

func TestInitSentryDisabledWithoutDSN(t *testing.T) {
	flush, err := InitSentry(config.Config{})
	if err != nil {
		t.Fatalf("disabled init: %v", err)
	}
	if flush == nil {
		t.Fatal("flush func must never be nil")
	}
}

func TestInitSentryRejectsInvalidDSN(t *testing.T) {
	if _, err := InitSentry(config.Config{SentryDSN: "not-a-dsn"}); err == nil {
		t.Fatal("expected an error for an invalid DSN")
	}
}
