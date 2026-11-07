package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterAllowsLimitThenDenies(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("request %d denied", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("limit not enforced")
	}
	if !l.Allow("other") {
		t.Fatal("independent keys should not share buckets")
	}
}

func TestLimiterReset(t *testing.T) {
	l := New(1, time.Minute)
	if !l.Allow("k") || l.Allow("k") {
		t.Fatal("limit broken")
	}
	l.Reset()
	if !l.Allow("k") {
		t.Fatal("reset failed")
	}
}

func TestLimiterWindowExpiry(t *testing.T) {
	l := New(1, 20*time.Millisecond)
	if !l.Allow("x") || l.Allow("x") {
		t.Fatal("window limit broken")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.Allow("x") {
		t.Fatal("window did not reset")
	}
}

func TestLimiterDefaults(t *testing.T) {
	l := New(0, 0)
	if !l.Allow("k") {
		t.Fatal("default limiter must allow")
	}
}
