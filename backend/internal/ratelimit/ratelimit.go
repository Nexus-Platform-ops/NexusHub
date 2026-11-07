// Package ratelimit implements a fixed-window in-process limiter used for
// both per-IP public endpoint protection and per-user authenticated limits.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	count int
	reset time.Time
}

// Limiter allows `limit` events per `window` for each distinct key.
type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

// New creates a limiter; limit and window must both be positive, otherwise
// sensible defaults are applied.
func New(limit int, window time.Duration) *Limiter {
	if limit <= 0 {
		limit = 120
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{limit: limit, window: window, buckets: map[string]*bucket{}}
}

// Allow reports whether one event for key fits inside the current window.
func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok || now.After(b.reset) {
		l.buckets[key] = &bucket{count: 1, reset: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// Reset clears all counters (used by tests and administrative actions).
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = map[string]*bucket{}
}
