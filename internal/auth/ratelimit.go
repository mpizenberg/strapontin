package auth

import (
	"sync"
	"time"
)

// Limiter allows at most Max attempts per key within Window. It lives in
// memory, so a restart resets it.
type Limiter struct {
	Max    int
	Window time.Duration

	mu       sync.Mutex
	attempts map[string][]time.Time
}

// NewLoginLimiter allows 10 attempts per 15 minutes.
func NewLoginLimiter() *Limiter {
	return &Limiter{Max: 10, Window: 15 * time.Minute, attempts: map[string][]time.Time{}}
}

// Allow records an attempt for every key and reports whether all of them
// are still under the limit.
func (l *Limiter) Allow(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	ok := true
	for _, k := range keys {
		recent := l.attempts[k][:0]
		for _, t := range l.attempts[k] {
			if now.Sub(t) < l.Window {
				recent = append(recent, t)
			}
		}
		if len(recent) >= l.Max {
			ok = false
		}
		l.attempts[k] = append(recent, now)
	}
	if len(l.attempts) > 10000 {
		l.attempts = map[string][]time.Time{}
	}
	return ok
}
