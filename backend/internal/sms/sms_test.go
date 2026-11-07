package sms

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogSenderSucceeds(t *testing.T) {
	if err := New(Config{Provider: Log}).SendOTP(context.Background(), "+959700000001", "123456"); err != nil {
		t.Fatalf("log sender: %v", err)
	}
}

func TestHTTPSenderDelivers(t *testing.T) {
	var got struct {
		To       string `json:"to"`
		SenderID string `json:"sender_id"`
		Message  string `json:"message"`
	}
	var auth, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "k3y", SenderID: "Nexus", Client: srv.Client()})
	if err := sender.SendOTP(context.Background(), "+959700000002", "654321"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if auth != "Bearer k3y" {
		t.Fatalf("authorization = %q", auth)
	}
	if contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	if got.To != "+959700000002" || got.SenderID != "Nexus" {
		t.Fatalf("payload = %+v", got)
	}
	if !strings.Contains(got.Message, "654321") || !strings.Contains(got.Message, "5 minutes") {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestHTTPSenderMapsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	sender := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "k3y", SenderID: "Nexus", Client: srv.Client()})
	err := sender.SendOTP(context.Background(), "+959700000003", "111111")
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error = %v, want ErrUpstream", err)
	}
}

func TestMaskPhone(t *testing.T) {
	if got := maskPhone("+959700000001"); strings.Contains(got, "597") || !strings.HasSuffix(got, "0001") {
		t.Fatalf("masked phone = %q", got)
	}
	if got := maskPhone("12"); got != "****" {
		t.Fatalf("short phone = %q", got)
	}
}
