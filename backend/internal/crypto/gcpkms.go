package crypto

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"nexus/backend/internal/gauth"
)

// gcpKMS implements MasterKey against Google Cloud KMS over its REST API,
// authenticating with a service account JWT bearer token.
type gcpKMS struct {
	keyResource string
	ts          *gauth.TokenSource
	client      *httpClient
}

func (k *gcpKMS) KeyID() string { return "gcpkms:" + k.keyResource }

func (k *gcpKMS) Wrap(ctx context.Context, plaintext []byte) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{
		"plaintext": base64.StdEncoding.EncodeToString(plaintext),
	})
	resp, err := k.call(ctx, ":encrypt", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(resp, &out); err != nil || out.Ciphertext == "" {
		return nil, fmt.Errorf("gcp kms encrypt: decode response: %v", err)
	}
	return base64.StdEncoding.DecodeString(out.Ciphertext)
}

func (k *gcpKMS) Unwrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{
		"ciphertext": base64.StdEncoding.EncodeToString(wrapped),
	})
	resp, err := k.call(ctx, ":decrypt", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Plaintext string `json:"plaintext"`
	}
	if err := json.Unmarshal(resp, &out); err != nil || out.Plaintext == "" {
		return nil, fmt.Errorf("gcp kms decrypt: decode response: %v", err)
	}
	return base64.StdEncoding.DecodeString(out.Plaintext)
}

func (k *gcpKMS) call(ctx context.Context, method string, payload []byte) ([]byte, error) {
	token, err := k.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("gcp kms auth: %w", err)
	}
	url := "https://cloudkms.googleapis.com/v1/" + k.keyResource + method
	status, body, err := k.client.postJSON(ctx, url, map[string]string{
		"Authorization": "Bearer " + token,
	}, payload)
	if err != nil {
		return nil, fmt.Errorf("gcp kms %s: %w", strings.TrimPrefix(method, ":"), err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gcp kms %s: status %d", strings.TrimPrefix(method, ":"), status)
	}
	return body, nil
}
