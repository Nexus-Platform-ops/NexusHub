package auth

import (
	"testing"
	"time"
)

func TestJWTRoundtrip(t *testing.T) {
	j := New("a-test-secret-key-value-32-bytes-ok")
	tok, err := j.Sign(42, "access", time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := j.Verify(tok, "access")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	id, err := UserID(claims)
	if err != nil || id != 42 {
		t.Fatalf("subject = %d err %v", id, err)
	}
}

func TestJWTTypeSeparation(t *testing.T) {
	j := New("a-test-secret-key-value-32-bytes-ok")
	access, _ := j.Sign(1, "access", time.Hour)
	refresh, _ := j.Sign(1, "refresh", time.Hour)
	if _, err := j.Verify(access, "refresh"); err == nil {
		t.Fatal("access token accepted as refresh")
	}
	if _, err := j.Verify(refresh, "access"); err == nil {
		t.Fatal("refresh token accepted as access")
	}
	if _, err := j.Verify(refresh, "refresh"); err != nil {
		t.Fatalf("refresh verify: %v", err)
	}
}

func TestJWTTampered(t *testing.T) {
	j := New("a-test-secret-key-value-32-bytes-ok")
	tok, _ := j.Sign(7, "access", time.Hour)
	tampered := tok[:len(tok)-2] + "xx"
	if _, err := j.Verify(tampered, "access"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestJWTExpired(t *testing.T) {
	j := New("a-test-secret-key-value-32-bytes-ok")
	tok, _ := j.Sign(7, "access", -time.Minute)
	if _, err := j.Verify(tok, "access"); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestJWTOtherKeyRejected(t *testing.T) {
	a := New("key-one-that-is-32-bytes-long-ok")
	b := New("key-two-that-is-32-bytes-long-ok")
	tok, _ := a.Sign(5, "access", time.Hour)
	if _, err := b.Verify(tok, "access"); err == nil {
		t.Fatal("token from other key accepted")
	}
}
