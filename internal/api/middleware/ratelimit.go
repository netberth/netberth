// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int
	burst    int
}

type visitor struct {
	tokens    float64
	lastCheck time.Time
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		burst:    burst,
	}
	go rl.cleanup(5 * time.Minute)
	return rl
}

func (rl *RateLimiter) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastCheck) > interval {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl.mu.Lock()
		v, exists := rl.visitors[r.RemoteAddr]
		if !exists {
			v = &visitor{tokens: float64(rl.burst), lastCheck: time.Now()}
			rl.visitors[r.RemoteAddr] = v
		}
		elapsed := time.Since(v.lastCheck).Seconds()
		v.tokens += elapsed * float64(rl.rate)
		if v.tokens > float64(rl.burst) {
			v.tokens = float64(rl.burst)
		}
		v.lastCheck = time.Now()
		if v.tokens < 1 {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		v.tokens--
		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
