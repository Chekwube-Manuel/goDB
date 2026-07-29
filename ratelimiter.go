package main

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	burst    int
	window   int
}

func newRateLimiter(burst, window int) *rateLimiter {
	if burst <= 0 {
		burst = 30
	}
	if window <= 0 {
		window = 60_000
	}
	return &rateLimiter{requests: make(map[string][]time.Time), burst: burst, window: window}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	entries := rl.requests[key]
	filtered := entries[:0]
	for _, ts := range entries {
		if now.Sub(ts) < time.Duration(rl.window)*time.Millisecond {
			filtered = append(filtered, ts)
		}
	}
	if len(filtered) >= rl.burst {
		rl.requests[key] = filtered
		return false
	}
	filtered = append(filtered, now)
	rl.requests[key] = filtered
	return true
}
