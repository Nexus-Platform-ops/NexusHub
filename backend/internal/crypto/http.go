package crypto

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

// httpClient is a thin wrapper so tests can swap transport and both KMS
// adapters share one timeout-aware client shape.
type httpClient struct {
	timeout time.Duration
	client  *http.Client
}

func (c *httpClient) do(req *http.Request) (*http.Response, error) {
	client := c.client
	if client == nil {
		timeout := c.timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return client.Do(req)
}

// postJSON issues a POST with a JSON body and returns the response body,
// bounded. Caller closes via the returned cleanup inside readBytes.
func (c *httpClient) postJSON(ctx context.Context, url string, headers map[string]string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

type staticCreds struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}
