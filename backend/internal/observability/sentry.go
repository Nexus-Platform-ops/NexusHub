// Package observability configures optional Sentry error reporting.
//
// Reporting is opt-in: an empty SENTRY_DSN leaves the SDK uninitialised and
// every capture becomes a no-op. Events never carry request bodies, headers,
// cookies, query strings or user identifiers; free-text values are scrubbed
// for phone numbers and e-mail addresses before leaving the process.
package observability

import (
	"regexp"
	"time"

	"github.com/getsentry/sentry-go"

	"nexus/backend/internal/config"
)

var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phonePattern = regexp.MustCompile(`\+?\d{7,}`)
)

// ScrubText masks e-mail addresses and long digit runs (phone numbers,
// payment references) in free-text error values.
func ScrubText(s string) string {
	s = emailPattern.ReplaceAllString(s, "[redacted-email]")
	return phonePattern.ReplaceAllString(s, "[redacted-number]")
}

func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Request = nil
	event.User = sentry.User{}
	event.ServerName = ""
	event.Message = ScrubText(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = ScrubText(event.Exception[i].Value)
	}
	for i := range event.Breadcrumbs {
		event.Breadcrumbs[i].Data = nil
	}
	return event
}

// InitSentry initialises the global hub. It returns a flush function and
// reports invalid DSNs so startup can fail fast. With no DSN the returned
// function is a no-op and reporting stays disabled.
func InitSentry(cfg config.Config) (func(), error) {
	if cfg.SentryDSN == "" {
		return func() {}, nil
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.SentryDSN,
		Environment:      cfg.SentryEnvironment,
		Release:          cfg.SentryRelease,
		TracesSampleRate: cfg.SentryTracesSampleRate,
		SendDefaultPII:   false,
		AttachStacktrace: true,
		BeforeSend:       scrubEvent,
	}); err != nil {
		return func() {}, err
	}
	return func() { sentry.Flush(2 * time.Second) }, nil
}
