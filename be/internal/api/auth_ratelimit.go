package api

import (
	"net"
	"sync"
	"time"
)

const (
	rateLimitMax    = 5
	rateLimitWindow = 5 * time.Minute
)

type rateBucket struct {
	attempts []time.Time
}

func (b *rateBucket) tryAcquire(now time.Time) (bool, time.Duration) {
	cutoff := now.Add(-rateLimitWindow)
	valid := b.attempts[:0]
	for _, t := range b.attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	b.attempts = valid
	if len(b.attempts) >= rateLimitMax {
		retryAfter := b.attempts[0].Add(rateLimitWindow).Sub(now)
		return false, retryAfter
	}
	b.attempts = append(b.attempts, now)
	return true, 0
}

// loginRateLimiter guards login attempts per IP+email key.
type loginRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{buckets: make(map[string]*rateBucket)}
}

// TryAcquire checks if the key may proceed. Returns ok=false and retryAfter when rate exceeded.
func (l *loginRateLimiter) TryAcquire(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &rateBucket{}
		l.buckets[key] = b
	}
	return b.tryAcquire(time.Now())
}

// rateLimitKey builds a per-IP+email key for rate limiting.
func rateLimitKey(remoteAddr, email string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return host + "|" + email
}
