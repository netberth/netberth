// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package retry

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Config for exponential backoff with jitter and circuit breaker.
type Config struct {
	BaseDelay       time.Duration // initial delay
	MaxDelay        time.Duration // cap
	Factor          float64       // multiplier per attempt
	Jitter          float64       // 0.0-1.0 randomness factor
	MaxAttempts     int
	CBThreshold     int           // consecutive failures before circuit opens
	CBHalfOpenAfter time.Duration // wait before testing circuit again
}

func DefaultConfig() Config {
	return Config{
		BaseDelay:       500 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		Factor:          2.0,
		Jitter:          0.2,
		MaxAttempts:     5,
		CBThreshold:     7,
		CBHalfOpenAfter: 30 * time.Second,
	}
}

type Breaker struct {
	mu           sync.Mutex
	failures     int
	lastFail     time.Time
	state        int32 // 0=closed, 1=open, 2=half-open
	threshold    int
	halfOpenWait time.Duration
}

func NewBreaker(threshold int, halfOpen time.Duration) *Breaker {
	return &Breaker{threshold: threshold, halfOpenWait: halfOpen}
}

func (b *Breaker) Allow() bool {
	state := atomic.LoadInt32(&b.state)
	switch state {
	case 0:
		return true // closed
	case 2:
		return true // half-open (allow probe)
	default:
		b.mu.Lock()
		defer b.mu.Unlock()
		if time.Since(b.lastFail) > b.halfOpenWait {
			atomic.StoreInt32(&b.state, 2)
			return true
		}
		return false
	}
}

func (b *Breaker) Success() {
	b.mu.Lock()
	b.failures = 0
	b.mu.Unlock()
	atomic.StoreInt32(&b.state, 0)
}

func (b *Breaker) Failure() {
	b.mu.Lock()
	b.failures++
	b.lastFail = time.Now()
	if b.failures >= b.threshold {
		atomic.StoreInt32(&b.state, 1)
	}
	b.mu.Unlock()
}

// Do executes fn with exponential backoff and circuit breaker.
// Returns (result, attempt, error).
func Do[T any](cfg Config, breaker *Breaker, name string, fn func() (T, error)) (T, int, error) {
	var zero T
	delay := cfg.BaseDelay

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if breaker != nil && !breaker.Allow() {
			return zero, attempt, ErrCircuitOpen
		}

		result, err := fn()
		if err == nil {
			if breaker != nil {
				breaker.Success()
			}
			return result, attempt + 1, nil
		}

		if breaker != nil {
			breaker.Failure()
		}
		if attempt == cfg.MaxAttempts-1 {
			return zero, attempt + 1, err
		}

		jitter := time.Duration(float64(delay) * cfg.Jitter * (rand.Float64() - 0.5))
		time.Sleep(delay + jitter)
		delay = time.Duration(math.Min(float64(delay)*cfg.Factor, float64(cfg.MaxDelay)))
	}

	return zero, cfg.MaxAttempts, ErrMaxRetries
}

type RetryError string

func (e RetryError) Error() string { return string(e) }

const (
	ErrCircuitOpen RetryError = "circuit breaker open"
	ErrMaxRetries  RetryError = "max retries exhausted"
)
