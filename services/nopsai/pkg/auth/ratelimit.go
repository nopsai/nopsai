package auth

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string][]time.Time),
	}
}

// Allow returns false when the number of attempts in the window exceeds the limit.
func (r *RateLimiter) Allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.attempts[key]

	threshold := now.Add(-window)
	filtered := list[:0]
	for _, ts := range list {
		if ts.After(threshold) {
			filtered = append(filtered, ts)
		}
	}
	filtered = append(filtered, now)
	r.attempts[key] = filtered

	return len(filtered) <= limit
}

// Reset clears the attempts for a key (e.g., after a successful login).
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, key)
}
