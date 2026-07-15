// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package acl

import (
	"net"
	"net/netip"
	"sync"
)

type Matcher struct {
	mu        sync.RWMutex
	whitelist []entry
	blacklist []entry
}

type entry struct {
	raw     string
	single  netip.Addr
	network *netip.Prefix
}

func NewMatcher() *Matcher { return &Matcher{} }

func (m *Matcher) Load(whitelist, blacklist []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.whitelist = make([]entry, 0, len(whitelist))
	for _, v := range whitelist {
		if e, ok := parseEntry(v); ok {
			m.whitelist = append(m.whitelist, e)
		}
	}
	m.blacklist = make([]entry, 0, len(blacklist))
	for _, v := range blacklist {
		if e, ok := parseEntry(v); ok {
			m.blacklist = append(m.blacklist, e)
		}
	}
}

func (m *Matcher) IsAllowed(ipStr string) bool {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		// Try parsing as host:port
		if host, _, splitErr := net.SplitHostPort(ipStr); splitErr == nil {
			if a, parseErr := netip.ParseAddr(host); parseErr == nil {
				addr = a
			} else {
				return false
			}
		} else {
			return false
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Blacklist check — deny first
	for _, e := range m.blacklist {
		if e.match(addr) {
			return false
		}
	}

	// If whitelist is set, only allow matches
	if len(m.whitelist) > 0 {
		for _, e := range m.whitelist {
			if e.match(addr) {
				return true
			}
		}
		return false
	}

	// No whitelist, not blacklisted → allow
	return true
}

func parseEntry(raw string) (entry, bool) {
	// Try CIDR prefix
	if prefix, err := netip.ParsePrefix(raw); err == nil {
		return entry{raw: raw, network: &prefix}, true
	}
	// Try single IP
	if addr, err := netip.ParseAddr(raw); err == nil {
		return entry{raw: raw, single: addr}, true
	}
	return entry{}, false
}

func (e entry) match(addr netip.Addr) bool {
	if e.network != nil {
		return e.network.Contains(addr)
	}
	return e.single == addr
}
