// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"net/http"
	"sync"
	"time"
)

type BruteForceLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailures
	// Config
	maxFailures  int
	lockDuration time.Duration
	window       time.Duration
}

type loginFailures struct {
	count       int
	firstTry    time.Time
	lockedUntil time.Time
}

func NewBruteForceLimiter(maxFailures int, lockDuration, window time.Duration) *BruteForceLimiter {
	bl := &BruteForceLimiter{
		failures:     make(map[string]*loginFailures),
		maxFailures:  maxFailures,
		lockDuration: lockDuration,
		window:       window,
	}
	go bl.cleanup(5 * time.Minute)
	return bl
}

func (b *BruteForceLimiter) LoginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" || r.Method != "POST" {
			next.ServeHTTP(w, r)
			return
		}
		clientIP := r.RemoteAddr

		b.mu.Lock()
		f, exists := b.failures[clientIP]
		if !exists {
			f = &loginFailures{}
			b.failures[clientIP] = f
		}

		// Check lock
		if time.Now().Before(f.lockedUntil) {
			b.mu.Unlock()
			w.Header().Set("Retry-After", "300")
			http.Error(w, `{"error":"too many login attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}

		// Reset window
		if time.Since(f.firstTry) > b.window {
			f.count = 0
			f.firstTry = time.Now()
		}
		b.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func (b *BruteForceLimiter) RecordFailure(clientIP string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, exists := b.failures[clientIP]
	if !exists {
		f = &loginFailures{firstTry: time.Now()}
		b.failures[clientIP] = f
	}
	f.count++
	if f.count >= b.maxFailures {
		f.lockedUntil = time.Now().Add(b.lockDuration)
	}
}

func (b *BruteForceLimiter) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		b.mu.Lock()
		now := time.Now()
		for ip, f := range b.failures {
			if now.After(f.lockedUntil) && now.Sub(f.firstTry) > 2*b.window {
				delete(b.failures, ip)
			}
		}
		b.mu.Unlock()
	}
}
