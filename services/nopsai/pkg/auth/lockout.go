package auth

import (
	"sync"
	"time"
)

type LockoutTracker struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	threshold int
	window    time.Duration
}

func NewLockoutTracker(threshold int, window time.Duration) *LockoutTracker {
	return &LockoutTracker{
		failures:  make(map[string][]time.Time),
		threshold: threshold,
		window:    window,
	}
}

func (l *LockoutTracker) RecordFailure(key string) bool {
	if l.threshold <= 0 {
		return false
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	attempts := l.pruneLocked(key, now)
	attempts = append(attempts, now)
	l.failures[key] = attempts
	return len(attempts) >= l.threshold
}

func (l *LockoutTracker) IsLocked(key string) bool {
	if l.threshold <= 0 {
		return false
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	attempts := l.pruneLocked(key, now)
	l.failures[key] = attempts
	return len(attempts) >= l.threshold
}

func (l *LockoutTracker) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

func (l *LockoutTracker) pruneLocked(key string, now time.Time) []time.Time {
	attempts := l.failures[key]
	if len(attempts) == 0 {
		return attempts
	}
	threshold := now.Add(-l.window)
	filtered := attempts[:0]
	for _, ts := range attempts {
		if ts.After(threshold) {
			filtered = append(filtered, ts)
		}
	}
	return filtered
}
