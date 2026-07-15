// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package acl

import "testing"

func TestSingleIPMatch(t *testing.T) {
	m := NewMatcher()
	m.Load([]string{"192.168.1.100"}, nil)
	if !m.IsAllowed("192.168.1.100") {
		t.Fatal("192.168.1.100 should be allowed")
	}
	if m.IsAllowed("192.168.1.200") {
		t.Fatal("192.168.1.200 should be denied (not in whitelist)")
	}
}

func TestCIDRMatch(t *testing.T) {
	m := NewMatcher()
	m.Load([]string{"10.0.0.0/8", "192.168.0.0/16"}, nil)
	tests := []struct {
		ip      string
		allowed bool
	}{
		{"10.1.2.3", true},
		{"10.255.255.255", true},
		{"10.0.0.1", true},
		{"11.0.0.1", false},
		{"192.168.1.1", true},
		{"192.168.255.255", true},
		{"192.169.0.1", false},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		got := m.IsAllowed(tt.ip)
		if got != tt.allowed {
			t.Errorf("IsAllowed(%s) = %v, want %v", tt.ip, got, tt.allowed)
		}
	}
}

func TestBlacklistPriority(t *testing.T) {
	m := NewMatcher()
	m.Load([]string{"10.0.0.0/8"}, []string{"10.1.0.0/16"})
	if !m.IsAllowed("10.2.0.1") {
		t.Fatal("10.2.0.1 in whitelist range should be allowed")
	}
	if m.IsAllowed("10.1.0.1") {
		t.Fatal("10.1.0.1 is blacklisted, should be denied even though in whitelist range")
	}
}

func TestIPv6CIDR(t *testing.T) {
	m := NewMatcher()
	m.Load([]string{"2001:db8::/32"}, nil)
	if !m.IsAllowed("2001:db8::1") {
		t.Fatal("2001:db8::1 should be allowed")
	}
	if !m.IsAllowed("2001:db8:ffff::1") {
		t.Fatal("2001:db8:ffff::1 should be allowed")
	}
	if m.IsAllowed("2001:db9::1") {
		t.Fatal("2001:db9::1 outside /32 should be denied")
	}
}

func TestEmptyWhitelistAllowsAll(t *testing.T) {
	m := NewMatcher()
	m.Load(nil, []string{"10.0.0.1"})
	if m.IsAllowed("10.0.0.1") {
		t.Fatal("blacklisted IP should be denied")
	}
	if !m.IsAllowed("192.168.1.1") {
		t.Fatal("no whitelist, non-blacklisted IP should be allowed")
	}
}

func TestHostPortFormat(t *testing.T) {
	m := NewMatcher()
	m.Load([]string{"192.168.1.0/24"}, nil)
	if !m.IsAllowed("192.168.1.50:12345") {
		t.Fatal("192.168.1.50:12345 should parse as 192.168.1.50 and be allowed")
	}
}
