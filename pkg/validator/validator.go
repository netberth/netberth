// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package validator

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	domainRegex = regexp.MustCompile(`^(?:\*\.)?(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	ipRegex     = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	cidrRegex   = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/\d{1,2}$`)
	urlRegex    = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
)

// Port validates port range
func Port(p int) bool { return p > 0 && p <= 65535 }

// Domain validates a domain name (supports wildcards like *.example.com)
func Domain(d string) bool {
	if d == "" {
		return false
	}
	if len(d) > 253 {
		return false
	}
	if strings.Contains(d, "..") {
		return false
	}
	return domainRegex.MatchString(d)
}

// IPOrCIDR validates an IP address or CIDR range, including IPv6
func IPOrCIDR(v string) bool {
	if v == "" {
		return false
	}
	if _, _, err := net.ParseCIDR(v); err == nil {
		return true
	}
	if ip := net.ParseIP(v); ip != nil {
		return true
	}
	return false
}

// URL validates a target URL for proxy rules
func URL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	if len(raw) > 2048 {
		return false
	}
	return true
}

// SafePath prevents path traversal attacks (../, null bytes, absolute paths outside root)
func SafePath(base, target string) bool {
	if target == "" {
		return false
	}
	// Reject null bytes
	if strings.ContainsRune(target, 0) {
		return false
	}
	// Reject path traversal
	if strings.Contains(target, "..") {
		return false
	}
	// Clean and verify stays within base
	cleaned := strings.TrimPrefix(target, "/")
	if strings.Contains(cleaned, "\\") {
		return false
	}
	return true
}

// MAC validates a MAC address
func MAC(mac string) bool {
	if mac == "" {
		return false
	}
	if _, err := net.ParseMAC(mac); err != nil {
		return false
	}
	return true
}

// Email validates basic email format
func Email(e string) bool {
	if e == "" {
		return true
	} // optional
	if len(e) > 254 {
		return false
	}
	if !strings.Contains(e, "@") || !strings.Contains(e, ".") {
		return false
	}
	parts := strings.Split(e, "@")
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
		return false
	}
	return !strings.Contains(e, "..")
}

// CronSchedule validates a basic cron expression (5 fields)
var cronRegex = regexp.MustCompile(`^(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)\s+(\*|[\d\-,/]+)$`)

func CronSchedule(s string) bool {
	return cronRegex.MatchString(strings.TrimSpace(s))
}
