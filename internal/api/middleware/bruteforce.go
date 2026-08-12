// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// maxTrackedLoginFailures bounds the in-memory failure table so an attacker
// cannot grow it without bound (defense against memory exhaustion).
const maxTrackedLoginFailures = 100_000

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
		clientIP := clientIPKey(r)

		// Locked out: reject before touching the (expensive) login handler.
		b.mu.Lock()
		if f, ok := b.failures[clientIP]; ok && time.Now().Before(f.lockedUntil) {
			b.mu.Unlock()
			w.Header().Set("Retry-After", strconv.Itoa(int(b.lockDuration.Seconds())))
			http.Error(w, `{"error":"too many login attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		b.mu.Unlock()

		// Observe the response so failures are recorded automatically and
		// successful logins clear the slate for this peer.
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		switch rw.status {
		case http.StatusUnauthorized:
			b.RecordFailure(clientIP)
		case http.StatusOK:
			b.Reset(clientIP)
		}
	})
}

func (b *BruteForceLimiter) RecordFailure(clientIP string) {
	clientIP = normalizeIP(clientIP)
	b.mu.Lock()
	defer b.mu.Unlock()
	f, exists := b.failures[clientIP]
	if !exists {
		if len(b.failures) >= maxTrackedLoginFailures {
			return
		}
		f = &loginFailures{firstTry: time.Now()}
		b.failures[clientIP] = f
	}
	f.count++
	// Roll the window so a stale burst from long ago cannot lock a peer.
	if time.Since(f.firstTry) > b.window {
		f.count = 1
		f.firstTry = time.Now()
	}
	if f.count >= b.maxFailures {
		f.lockedUntil = time.Now().Add(b.lockDuration)
	}
}

// Reset clears a peer's failure state after a successful login.
func (b *BruteForceLimiter) Reset(clientIP string) {
	b.mu.Lock()
	delete(b.failures, normalizeIP(clientIP))
	b.mu.Unlock()
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

// clientIPKey returns a normalized, non-spoofable client identity. It prefers
// the TCP peer address recorded by chi's ClientIPFromRemoteAddr middleware;
// proxy headers are deliberately never trusted for security decisions.
func clientIPKey(r *http.Request) string {
	if ip := chimw.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	return normalizeIP(r.RemoteAddr)
}

func normalizeIP(s string) string {
	if s == "" {
		return "unknown"
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return s
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return host
}
