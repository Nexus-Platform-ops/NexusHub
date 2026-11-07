// Package sms delivers one-time-passcode messages through an outbound
// provider.
//
// The "log" provider writes only a masked delivery line and never prints the
// code; it is meant for development, where the API can expose dev_code via
// OTP_DEBUG. The "http" provider posts a small JSON document
// ({"to","sender_id","message"}) with a bearer token to the aggregator URL.
// Myanmar aggregators with a different wire format need a thin gateway or a
// new Sender implementation; nothing else in the API changes.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Provider identifiers accepted by Config.Provider.
const (
	Log  = "log"
	HTTP = "http"
)

// ErrUpstream means the provider rejected or failed the delivery request.
// The caller may retry; the API deletes the stored OTP so a retry is clean.
var ErrUpstream = errors.New("sms provider delivery failed")

// Sender delivers a verification code to a phone number.
type Sender interface {
	SendOTP(ctx context.Context, phone, code string) error
}

// Config configures the sender. Provider "log" is the safe default; an
// unknown provider value is rejected by config.Validate before startup.
type Config struct {
	Provider string
	APIURL   string // full endpoint, https:// outside development
	APIKey   string
	SenderID string
	Timeout  time.Duration
	Client   *http.Client // tests inject an httptest client
}

// New returns the configured sender. It never returns nil.
func New(c Config) Sender {
	if c.Provider == HTTP {
		return newHTTP(c)
	}
	return logSender{}
}

type logSender struct{}

func (logSender) SendOTP(_ context.Context, phone, _ string) error {
	log.Printf("sms: development delivery to %s (code not logged)", maskPhone(phone))
	return nil
}

type httpSender struct {
	url      string
	apiKey   string
	senderID string
	timeout  time.Duration
	client   *http.Client
}

func newHTTP(c Config) *httpSender {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &httpSender{url: c.APIURL, apiKey: c.APIKey, senderID: c.SenderID, timeout: timeout, client: client}
}

func (s *httpSender) SendOTP(ctx context.Context, phone, code string) error {
	payload, err := json.Marshal(map[string]string{
		"to":        phone,
		"sender_id": s.senderID,
		"message":   fmt.Sprintf("Nexus verification code: %s (valid 5 minutes). Do not share this code.", code),
	})
	if err != nil {
		return fmt.Errorf("%w: encode payload: %v", ErrUpstream, err)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	return nil
}

// maskPhone keeps only the last four digits so development logs identify the
// delivery attempt without recording the full number.
func maskPhone(phone string) string {
	if len(phone) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}
