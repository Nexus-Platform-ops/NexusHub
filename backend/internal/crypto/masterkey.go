package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"nexus/backend/internal/gauth"
)

// MasterKey wraps and unwraps data encryption keys. Implementations talk to
// a key management service (AWS KMS, GCP KMS) or, in development, a local
// key. KeyID identifies the key material so rotation is visible in stored
// envelopes.
type MasterKey interface {
	KeyID() string
	Wrap(ctx context.Context, dek []byte) ([]byte, error)
	Unwrap(ctx context.Context, wrapped []byte) ([]byte, error)
}

// LocalMasterKey implements MasterKey with a static 256-bit key. The wrapped
// blob is prefixed with the key fingerprint, so pointing the service at a
// rotated key turns stale envelopes into explicit unwrap failures instead of
// silent corruption.
type LocalMasterKey struct {
	key []byte
	id  string
}

// NewLocalMasterKey decodes a base64 32-byte key. An empty key material
// generates an ephemeral random key (development default: survives process
// lifetime, never written anywhere).
func NewLocalMasterKey(keyB64 string) (*LocalMasterKey, error) {
	var key []byte
	if keyB64 == "" {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("crypto: generate ephemeral local key: %w", err)
		}
	} else {
		var err error
		key, err = base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, errors.New("crypto: CRYPTO_LOCAL_KEY must be base64")
		}
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: local master key must be 32 bytes, got %d", len(key))
	}
	fp := sha256.Sum256(key)
	return &LocalMasterKey{key: key, id: "local:" + hex.EncodeToString(fp[:4])}, nil
}

func (k *LocalMasterKey) KeyID() string { return k.id }

func (k *LocalMasterKey) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// localWrapped layout: 8-byte key fingerprint || nonce || ciphertext.
func (k *LocalMasterKey) Wrap(_ context.Context, dek []byte) ([]byte, error) {
	gcm, err := k.aead()
	if err != nil {
		return nil, err
	}
	fp := sha256.Sum256(k.key)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{}, fp[:8]...)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, dek, nil)
	return out, nil
}

func (k *LocalMasterKey) Unwrap(_ context.Context, wrapped []byte) ([]byte, error) {
	gcm, err := k.aead()
	if err != nil {
		return nil, err
	}
	fp := sha256.Sum256(k.key)
	if len(wrapped) < 8 || string(wrapped[:8]) != string(fp[:8]) {
		return nil, errors.New("crypto: wrapped DEK belongs to a different key (rotation rewrap required)")
	}
	rest := wrapped[8:]
	if len(rest) < gcm.NonceSize() {
		return nil, errors.New("crypto: truncated wrapped DEK")
	}
	nonce, ct := rest[:gcm.NonceSize()], rest[gcm.NonceSize():]
	dek, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errors.New("crypto: unwrap authentication failed")
	}
	return dek, nil
}

// NewMasterKey builds the configured MasterKey. provider is one of
// "local", "awskms", "gcpkms"; cfg carries the provider-specific fields
// from config.Config (kept as a flat struct to avoid an import cycle).
func NewMasterKey(provider string, cfg KMSConfig) (MasterKey, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "local", "":
		return NewLocalMasterKey(cfg.LocalKey)
	case "awskms":
		if cfg.AWSKeyID == "" || cfg.AWSRegion == "" || cfg.AWSAccessKeyID == "" || cfg.AWSSecretAccessKey == "" {
			return nil, errors.New("crypto: AWSKMS requires KMS_KEY_ID, AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY")
		}
		return &awsKMS{
			keyID:  cfg.AWSKeyID,
			region: cfg.AWSRegion,
			creds:  staticCreds{accessKeyID: cfg.AWSAccessKeyID, secretAccessKey: cfg.AWSSecretAccessKey},
			client: &httpClient{timeout: cfg.Timeout},
		}, nil
	case "gcpkms":
		if cfg.GCPKeyResource == "" || cfg.GCPServiceAccountJSON == "" {
			return nil, errors.New("crypto: GCPKMS requires GCP_KMS_KEY_RESOURCE and GCP_SERVICE_ACCOUNT_JSON")
		}
		ts, err := gauth.NewTokenSource(cfg.GCPServiceAccountJSON, "https://www.googleapis.com/auth/cloudkms")
		if err != nil {
			return nil, err
		}
		return &gcpKMS{keyResource: cfg.GCPKeyResource, ts: ts, client: &httpClient{timeout: cfg.Timeout}}, nil
	default:
		return nil, fmt.Errorf("crypto: unknown CRYPTO_PROVIDER %q", provider)
	}
}

// KMSConfig carries the provider-specific master key settings.
type KMSConfig struct {
	LocalKey              string
	AWSRegion             string
	AWSAccessKeyID        string
	AWSSecretAccessKey    string
	AWSKeyID              string
	GCPKeyResource        string
	GCPServiceAccountJSON string
	Timeout               time.Duration
}
