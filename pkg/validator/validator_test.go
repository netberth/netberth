// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package validator

import (
	"strings"
	"testing"
)

func TestPort(t *testing.T) {
	cases := []struct {
		in   int
		want bool
	}{
		{1, true}, {65535, true}, {80, true},
		{0, false}, {-1, false}, {65536, false},
	}
	for _, c := range cases {
		if got := Port(c.in); got != c.want {
			t.Errorf("Port(%d) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDomain(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"*.example.com", true},
		{"a-b.example.co.uk", true},
		{"", false},
		{"not a domain", false},
		{"-bad.example.com", false},
		{"bad-.example.com", false},
		{"a..b.com", false},
		{".example.com", false},
		{"example.c", false},
		{"example.com.", false},
		{strings.Repeat("a", 250) + ".com", false},
	}
	for _, c := range cases {
		if got := Domain(c.in); got != c.want {
			t.Errorf("Domain(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIPOrCIDR(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.0/8", true},
		{"2001:db8::1", true},
		{"2001:db8::/32", true},
		{"", false},
		{"999.1.1.1", false},
		{"1.2.3", false},
		{"10.0.0.0/99", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := IPOrCIDR(c.in); got != c.want {
			t.Errorf("IPOrCIDR(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com/path?q=1", true},
		{"", false},
		{"ftp://example.com", false},
		{"http://", false},
		{"example.com", false},
		{"javascript:alert(1)", false},
		{"http://example.com/" + strings.Repeat("a", 2048), false},
	}
	for _, c := range cases {
		if got := URL(c.in); got != c.want {
			t.Errorf("URL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSafePath(t *testing.T) {
	cases := []struct {
		base, target string
		want         bool
	}{
		{"/srv", "a.txt", true},
		{"/srv", "dir/file.txt", true},
		{"/srv", "sub/a.txt", true},
		{"/srv", "", false},
		{"/srv", "../etc/passwd", false},
		{"/srv", "..", false},
		{"/srv", "a\\b", false},
		{"/srv", "a\x00b", false},
		{"/srv", "sub/../x", false},
		{"/srv", "/etc/passwd", false},
		{"/srv", "/srv/secret", false},
		{"", "a.txt", true},
	}
	for _, c := range cases {
		if got := SafePath(c.base, c.target); got != c.want {
			t.Errorf("SafePath(%q,%q) = %v, want %v", c.base, c.target, got, c.want)
		}
	}
}

func TestMAC(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"AA:BB:CC:DD:EE:FF", true},
		{"aa-bb-cc-dd-ee-ff", true},
		{"AA:BB:CC:DD:EE", false},
		{"", false},
		{"not-a-mac", false},
	}
	for _, c := range cases {
		if got := MAC(c.in); got != c.want {
			t.Errorf("MAC(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true}, // optional
		{"a@b.com", true},
		{"first.last+tag@example.co.uk", true},
		{"a@b", false},
		{"@b.com", false},
		{"a@", false},
		{"a@b..com", false},
		{"a..b@c.com", false},
		{strings.Repeat("a", 250) + "@b.com", false},
	}
	for _, c := range cases {
		if got := Email(c.in); got != c.want {
			t.Errorf("Email(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCronSchedule(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"*/5 * * * *", true},
		{"0 0 1 1 *", true},
		{"1,2,3 * * * *", true},
		{"*/5 * * * * *", true},
		{"0 0 0 0 0 0", true},
		{"", false},
		{"not cron", false},
		{"0 0 0 0 0 0 0", false},
		{"a * * * *", false},
	}
	for _, c := range cases {
		if got := CronSchedule(c.in); got != c.want {
			t.Errorf("CronSchedule(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
