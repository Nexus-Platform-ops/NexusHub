package email

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogSenderSucceeds(t *testing.T) {
	if err := New(Config{Provider: Log}).Send(context.Background(),
		"user@example.com", "Subject", "body with token"); err != nil {
		t.Fatalf("log sender: %v", err)
	}
}

func TestHTTPSenderDelivers(t *testing.T) {
	var got struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "m4il",
		From: "Nexus <no-reply@example.com>", Client: srv.Client()})
	if err := sender.Send(context.Background(),
		"user@example.com", "Export ready", "download within 30 days"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if auth != "Bearer m4il" {
		t.Fatalf("authorization = %q", auth)
	}
	if got.From != "Nexus <no-reply@example.com>" || got.To != "user@example.com" ||
		got.Subject != "Export ready" || got.Body != "download within 30 days" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestHTTPSenderMapsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	sender := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "k", Client: srv.Client()})
	err := sender.Send(context.Background(), "u@example.com", "s", "b")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error = %v, want ErrUpstream", err)
	}
}

func TestMaskAddr(t *testing.T) {
	if got := maskAddr("thandar@example.com"); got != "***@example.com" {
		t.Fatalf("masked addr = %q", got)
	}
	if got := maskAddr("not-an-address"); got != "***" {
		t.Fatalf("masked addr = %q", got)
	}
}
