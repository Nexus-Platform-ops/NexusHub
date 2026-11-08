// Package crypto provides envelope encryption for data at rest.
//
// Every plaintext is encrypted with a fresh random 256-bit data encryption
// key (DEK) using AES-256-GCM. The DEK itself is wrapped by a MasterKey,
// which is backed by AWS KMS, GCP KMS, or (development only) a local key.
// The master key never encrypts user data directly, so rotating it only
// requires rewrapping DEKs, never re-encrypting payloads.
package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// blob is the serialized envelope stored alongside the data.
type blob struct {
	V     int    `json:"v"`     // format version
	KeyID string `json:"kid"`   // master key identifier (rotation marker)
	DEK   string `json:"dek"`   // base64 master-key-wrapped DEK
	Nonce string `json:"nonce"` // base64 GCM nonce
	Ct    string `json:"ct"`    // base64 ciphertext (includes GCM tag)
}

// Service encrypts and decrypts using an envelope-encrypted MasterKey.
type Service struct {
	mk MasterKey
}

// New builds a Service over mk. mk must not be nil.
func New(mk MasterKey) (*Service, error) {
	if mk == nil {
		return nil, errors.New("crypto: nil master key")
	}
	return &Service{mk: mk}, nil
}

// Encrypt returns a self-contained envelope: wrapped DEK + nonce + ciphertext.
func (s *Service) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("crypto: generate DEK: %w", err)
	}
	wrapped, err := s.mk.Wrap(ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: wrap DEK: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("crypto: generate nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	b := blob{
		V:     1,
		KeyID: s.mk.KeyID(),
		DEK:   base64.StdEncoding.EncodeToString(wrapped),
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		Ct:    base64.StdEncoding.EncodeToString(ct),
	}
	out, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("crypto: marshal envelope: %w", err)
	}
	return out, nil
}

// Decrypt opens an envelope produced by Encrypt. A wrapped DEK bound to a
// different master key fails at unwrap time, which is what makes key
// rotation detectable: old blobs must be rewrapped, not silently misread.
func (s *Service) Decrypt(ctx context.Context, envelope []byte) ([]byte, error) {
	var b blob
	if err := json.Unmarshal(envelope, &b); err != nil || b.V != 1 {
		return nil, errors.New("crypto: unsupported envelope format")
	}
	wrapped, err := base64.StdEncoding.DecodeString(b.DEK)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode wrapped DEK: %w", err)
	}
	dek, err := s.mk.Unwrap(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap DEK: %w", err)
	}
	if len(dek) != 32 {
		return nil, errors.New("crypto: unwrapped DEK has invalid length")
	}
	nonce, err := base64.StdEncoding.DecodeString(b.Nonce)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode nonce: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(b.Ct)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, errors.New("crypto: envelope authentication failed")
	}
	return pt, nil
}

// Rewrap re-encrypts an envelope's DEK under a new master key without
// touching the payload. It is the primitive behind master key rotation:
// iterate stored envelopes, Rewrap(old -> new), store. The payload
// ciphertext stays byte-identical.
func (s *Service) Rewrap(ctx context.Context, envelope []byte, next MasterKey) ([]byte, error) {
	var b blob
	if err := json.Unmarshal(envelope, &b); err != nil || b.V != 1 {
		return nil, errors.New("crypto: unsupported envelope format")
	}
	wrapped, err := base64.StdEncoding.DecodeString(b.DEK)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode wrapped DEK: %w", err)
	}
	dek, err := s.mk.Unwrap(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap DEK: %w", err)
	}
	rewrapped, err := next.Wrap(ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("crypto: rewrap DEK: %w", err)
	}
	b.KeyID = next.KeyID()
	b.DEK = base64.StdEncoding.EncodeToString(rewrapped)
	out, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("crypto: marshal envelope: %w", err)
	}
	return out, nil
}
