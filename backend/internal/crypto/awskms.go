package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// awsKMS implements MasterKey against AWS KMS using the JSON protocol over
// HTTPS with SigV4 request signing. It deliberately avoids the AWS SDK to
// keep the binary small; only Encrypt/Decrypt of DEKs is needed.
//
// Credentials come from the environment (AWS_ACCESS_KEY_ID /
// AWS_SECRET_ACCESS_KEY); on real infrastructure prefer an instance role
// and set the two env vars from the role metadata at container start.
type awsKMS struct {
	keyID  string
	region string
	creds  staticCreds
	client *httpClient
}

func (k *awsKMS) KeyID() string { return "awskms:" + k.keyID }

type kmsResponse struct {
	CiphertextBlob string `json:"CiphertextBlob"`
	Plaintext      string `json:"Plaintext"`
	Message        string `json:"message"`
}

func (k *awsKMS) Wrap(ctx context.Context, dek []byte) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{
		"KeyId":     k.keyID,
		"Plaintext": base64.StdEncoding.EncodeToString(dek),
	})
	raw, err := k.call(ctx, "Encrypt", body)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(raw.CiphertextBlob)
}

func (k *awsKMS) Unwrap(ctx context.Context, wrapped []byte) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{
		"CiphertextBlob": base64.StdEncoding.EncodeToString(wrapped),
	})
	raw, err := k.call(ctx, "Decrypt", body)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(raw.Plaintext)
}

func (k *awsKMS) call(ctx context.Context, target string, payload []byte) (*kmsResponse, error) {
	host := fmt.Sprintf("kms.%s.amazonaws.com", k.region)
	url := "https://" + host + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService."+target)
	signRequest(req, k.creds, k.region, payload, time.Now().UTC())
	resp, err := k.client.do(req)
	if err != nil {
		return nil, fmt.Errorf("aws kms %s: %w", target, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("aws kms %s: read response: %w", target, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e kmsResponse
		_ = json.Unmarshal(respBody, &e)
		return nil, fmt.Errorf("aws kms %s: status %d (%s)", target, resp.StatusCode, e.Message)
	}
	var out kmsResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("aws kms %s: decode response: %w", target, err)
	}
	return &out, nil
}

// signRequest signs the request in place (SigV4, service kms).
func signRequest(req *http.Request, creds staticCreds, region string, payload []byte, now time.Time) {
	const service = "kms"
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	if creds.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.sessionToken)
	}

	payloadHash := sha256Hex(payload)
	canonicalHeaders, signedHeaders := canonical(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	kSigning := sigV4Key(creds.secretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.accessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
}

func sigV4Key(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func canonicalURI(req *http.Request) string {
	if req.URL.Path == "" {
		return "/"
	}
	return req.URL.Path
}

// canonical builds the canonical headers block and the signed headers list.
func canonical(req *http.Request) (string, string) {
	headers := map[string]string{
		"content-type":    req.Header.Get("Content-Type"),
		"host":            req.URL.Host,
		"x-amz-date":      req.Header.Get("X-Amz-Date"),
		"x-amz-target":    req.Header.Get("X-Amz-Target"),
	}
	if token := req.Header.Get("X-Amz-Security-Token"); token != "" {
		headers["x-amz-security-token"] = token
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k + ":" + strings.TrimSpace(headers[k]) + "\n")
	}
	return sb.String(), strings.Join(keys, ";")
}
