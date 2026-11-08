// Package email sends transactional mail (DSR confirmations, export
// notifications) through an outbound provider.
//
// The "log" provider writes only a subject line and recipient — never the
// body, which may contain confirm tokens — and is development-only. The
// "http" provider posts {"from","to","subject","body"} with a bearer token,
// mirroring the SMS aggregator pattern; providers with a different wire
// format need a thin Sender implementation.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Providers accepted by Config.Provider.
const (
	Log  = "log"
	HTTP = "http"
)

// ErrUpstream means the provider rejected or failed the send.
var ErrUpstream = errors.New("email provider delivery failed")

// Sender delivers a transactional email.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Config configures the sender.
type Config struct {
	Provider string
	APIURL   string
	APIKey   string
	From     string
	Timeout  time.Duration
	Client   *http.Client // tests inject an httptest client
}

// New returns the configured sender; unknown providers fall back to log.
func New(c Config) Sender {
	if c.Provider == HTTP {
		return newHTTP(c)
	}
	return logSender{}
}

type logSender struct{}

func (logSender) Send(_ context.Context, to, subject, _ string) error {
	log.Printf("email: development delivery to %s: %s (body not logged)", maskAddr(to), subject)
	return nil
}

type httpSender struct {
	url     string
	apiKey  string
	from    string
	timeout time.Duration
	client  *http.Client
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
	return &httpSender{url: c.APIURL, apiKey: c.APIKey, from: c.From, timeout: timeout, client: client}
}

func (s *httpSender) Send(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(map[string]string{
		"from": s.from, "to": to, "subject": subject, "body": body,
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

// maskAddr keeps only the domain so development logs identify the delivery
// target without recording the full address.
func maskAddr(addr string) string {
	if i := bytes.IndexByte([]byte(addr), '@'); i > 0 {
		return "***@" + addr[i+1:]
	}
	return "***"
}
