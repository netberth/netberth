// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ClientIPResolver maps a request to the real client IP. Proxy headers are
// honored only when the immediate TCP peer is in the trusted list, so a
// remote attacker cannot spoof X-Forwarded-For / X-Real-IP / True-Client-IP
// to bypass rate limiting or login lockout. With no trusted proxies
// configured (the default), proxy headers are ignored entirely.
type ClientIPResolver struct {
	trusted []*net.IPNet
}

// NewClientIPResolver builds a resolver from IPs and/or CIDRs. An empty
// trusted list means "trust no proxy headers".
func NewClientIPResolver(trusted []string) (*ClientIPResolver, error) {
	r := &ClientIPResolver{}
	for _, s := range trusted {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			_, ipNet, err := net.ParseCIDR(s)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", s, err)
			}
			r.trusted = append(r.trusted, ipNet)
			continue
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy IP %q", s)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		r.trusted = append(r.trusted, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return r, nil
}

func (r *ClientIPResolver) isTrusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range r.trusted {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func hostOnly(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

func firstValidIP(v string) string {
	ip := net.ParseIP(strings.TrimSpace(v))
	if ip == nil {
		return ""
	}
	return ip.String()
}

// clientFromXFF walks the chain from right to left and returns the first IP
// that is not one of our trusted proxies. If every hop is trusted, the
// leftmost (client-originated) value wins.
func (r *ClientIPResolver) clientFromXFF(xff string) string {
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if !r.isTrusted(ip.String()) {
			return ip.String()
		}
	}
	for _, p := range parts {
		if ip := net.ParseIP(strings.TrimSpace(p)); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// ClientIP returns the normalized client IP (no port) for the request.
func (r *ClientIPResolver) ClientIP(req *http.Request) string {
	peer := hostOnly(req.RemoteAddr)
	if peer == "" {
		return "unknown"
	}
	if !r.isTrusted(peer) {
		return peer
	}
	if ip := firstValidIP(req.Header.Get("True-Client-IP")); ip != "" {
		return ip
	}
	if ip := firstValidIP(req.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if ip := r.clientFromXFF(req.Header.Get("X-Forwarded-For")); ip != "" {
		return ip
	}
	return peer
}

// Middleware normalizes req.RemoteAddr to the resolved client IP so rate
// limiting, login lockout, audit and logging all use the same value.
func (r *ClientIPResolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.RemoteAddr = r.ClientIP(req)
		next.ServeHTTP(w, req)
	})
}
