package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func localKey(t *testing.T, seed byte) *LocalMasterKey {
	t.Helper()
	key := bytes.Repeat([]byte{seed}, 32)
	mk, err := NewLocalMasterKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("NewLocalMasterKey: %v", err)
	}
	return mk
}

func TestEnvelopeRoundTrip(t *testing.T) {
	svc, err := New(localKey(t, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plaintext := []byte("phone=+959123456789; real_name=Thidar Aung")
	env, err := svc.Encrypt(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// The envelope must not contain the plaintext.
	if bytes.Contains(env, plaintext) {
		t.Fatal("envelope leaks plaintext")
	}
	got, err := svc.Decrypt(context.Background(), env)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: %q", got)
	}
}

func TestEnvelopeTamperFails(t *testing.T) {
	svc, _ := New(localKey(t, 2))
	env, _ := svc.Encrypt(context.Background(), []byte("secret"))
	var b blob
	if err := json.Unmarshal(env, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ct, _ := base64.StdEncoding.DecodeString(b.Ct)
	ct[len(ct)-1] ^= 0xff // flip a bit in the GCM tag region
	b.Ct = base64.StdEncoding.EncodeToString(ct)
	tampered, _ := json.Marshal(b)
	if _, err := svc.Decrypt(context.Background(), tampered); err == nil {
		t.Fatal("tampered envelope decrypted successfully")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	a, _ := New(localKey(t, 3))
	b, _ := New(localKey(t, 4))
	env, _ := a.Encrypt(context.Background(), []byte("x"))
	if _, err := b.Decrypt(context.Background(), env); err == nil {
		t.Fatal("envelope decrypted under a different master key")
	}
}

func TestRotationRewrap(t *testing.T) {
	oldSvc, _ := New(localKey(t, 5))
	env, err := oldSvc.Encrypt(context.Background(), []byte("pre-rotation data"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Rotate: build the next master key, rewrap every stored envelope.
	next := localKey(t, 6)
	rotated, err := oldSvc.Rewrap(context.Background(), env, next)
	if err != nil {
		t.Fatalf("Rewrap: %v", err)
	}
	var b blob
	if err := json.Unmarshal(rotated, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b.KeyID != next.KeyID() {
		t.Fatalf("kid = %q, want %q", b.KeyID, next.KeyID())
	}
	newSvc, _ := New(next)
	got, err := newSvc.Decrypt(context.Background(), rotated)
	if err != nil {
		t.Fatalf("Decrypt after rotation: %v", err)
	}
	if string(got) != "pre-rotation data" {
		t.Fatalf("payload changed across rotation: %q", got)
	}
	// The old key must no longer open the rewrapped envelope.
	if _, err := oldSvc.Decrypt(context.Background(), rotated); err == nil {
		t.Fatal("old master key still opens rewrapped envelope")
	}
}

func TestEphemeralLocalKey(t *testing.T) {
	mk, err := NewLocalMasterKey("")
	if err != nil {
		t.Fatalf("ephemeral key: %v", err)
	}
	if mk.KeyID() == "" {
		t.Fatal("ephemeral key has no ID")
	}
	svc, _ := New(mk)
	env, _ := svc.Encrypt(context.Background(), []byte("dev data"))
	if _, err := svc.Decrypt(context.Background(), env); err != nil {
		t.Fatalf("Decrypt with ephemeral key: %v", err)
	}
}

func TestNewMasterKeyValidation(t *testing.T) {
	if _, err := NewMasterKey("local", KMSConfig{LocalKey: "!!not-base64!!"}); err == nil {
		t.Fatal("invalid base64 local key accepted")
	}
	if _, err := NewMasterKey("awskms", KMSConfig{}); err == nil {
		t.Fatal("awskms without credentials accepted")
	}
	if _, err := NewMasterKey("gcpkms", KMSConfig{}); err == nil {
		t.Fatal("gcpkms without key resource accepted")
	}
	if _, err := NewMasterKey("bogus", KMSConfig{}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
