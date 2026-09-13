// Package ratelimit provides a minimal, concurrency-safe, in-memory
// fixed-window rate limiter keyed by an arbitrary client identifier.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	count   int
	resetAt time.Time
}

// Limiter allows up to `limit` requests per `window` for each key. It is safe
// for concurrent use and periodically purges expired keys so memory stays
// bounded.
type Limiter struct {
	mu          sync.Mutex
	limit       int
	window      time.Duration
	buckets     map[string]*bucket
	lastCleanup time.Time
}

// New creates a Limiter allowing `limit` requests per `window`.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:       limit,
		window:      window,
		buckets:     make(map[string]*bucket),
		lastCleanup: time.Now(),
	}
}

// Allow records a request for key and reports whether it is permitted. When
// denied, retryAfter is the time until the current window resets.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.cleanup(now)

	b, ok := l.buckets[key]
	if !ok || !now.Before(b.resetAt) {
		l.buckets[key] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}

	if b.count >= l.limit {
		return false, b.resetAt.Sub(now)
	}

	b.count++
	return true, 0
}

// cleanup removes expired buckets at most once per window.
func (l *Limiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < l.window {
		return
	}
	l.lastCleanup = now
	for k, b := range l.buckets {
		if !now.Before(b.resetAt) {
			delete(l.buckets, k)
		}
	}
}
