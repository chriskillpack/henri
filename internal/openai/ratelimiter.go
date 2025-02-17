package openai

import (
	"context"
	"sync"
	"time"
)

// A simple rate limiter that uses the token bucket algorithm.
type rateLimiter struct {
	mu       sync.Mutex // protect access to lastTime and tokens
	lastTime time.Time
	tokens   int

	window time.Duration
	rate   int
}

// newRateLimiter creates a new rate limiter for the given number of tokens
// over the provided time window. E.g. newRateLimiter(10, time.Minute) will
// allow 10 units of work to happen over a minute.
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		window:   window,
		rate:     rate,
		lastTime: time.Now(),
		tokens:   rate,
	}
}

// Acquire returns nil if work can proceed. If the provided context is Done
// Acquire will return context.Err(). If the bucket is empty, Acquire will sleep
// until at least one token is available.
func (rl *rateLimiter) Acquire(ctx context.Context) error {
	for {
		if ok := rl.tryAcquire(); ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rl.window / time.Duration(rl.rate)):
			// If tryAcquire() returned false the token bucket is empty.
			// Assuming an even distribution of tokens across the window, wait
			// 1/Nth of the window duration to allow at least one token to
			// accumulate. And then try again.
		}
	}
}

func (rl *rateLimiter) tryAcquire() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// How much time has elapsed?
	now := time.Now()
	elapsed := now.Sub(rl.lastTime)
	rl.lastTime = now

	// Put tokens into the bucket, the number proportional to the duration since
	// last called.
	rl.tokens += int(elapsed.Nanoseconds() * int64(rl.rate) / rl.window.Nanoseconds())
	rl.tokens = min(rl.tokens, rl.rate)
	// If the bucket is exhausted then the caller cannot proceed immediately.
	if rl.tokens <= 0 {
		return false
	}

	// Success, remove a token.
	rl.tokens--
	return true
}
