// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package wol

import (
	"net"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func TestParseMAC(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"AA:BB:CC:DD:EE:FF", false},
		{"aa-bb-cc-dd-ee-ff", false},
		{"AA:BB:CC:DD:EE", true},
		{"", true},
		{"not-a-mac", true},
	}
	for _, c := range cases {
		hw, err := parseMAC(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMAC(%q): expected error, got %v", c.in, hw)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMAC(%q): unexpected error %v", c.in, err)
		}
		if len(hw) != 6 {
			t.Errorf("parseMAC(%q): expected 6 bytes, got %d", c.in, len(hw))
		}
	}
}

func TestSendMagicPacketInvalidMAC(t *testing.T) {
	if err := SendMagicPacket(model.WOLDevice{MAC: "bad", Broadcast: "127.0.0.1", Port: 9}); err == nil {
		t.Fatal("expected error for invalid MAC")
	}
}

func TestSendMagicPacketBadBroadcast(t *testing.T) {
	if err := SendMagicPacket(model.WOLDevice{MAC: "AA:BB:CC:DD:EE:FF", Broadcast: "999.999.999.999", Port: 9}); err == nil {
		t.Fatal("expected error for unresolvable broadcast address")
	}
}

func TestSendMagicPacketDelivers(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer pc.Close()
	addr := pc.LocalAddr().(*net.UDPAddr)

	mac := "AA:BB:CC:DD:EE:FF"
	if err := SendMagicPacket(model.WOLDevice{MAC: mac, Broadcast: "127.0.0.1", Port: addr.Port}); err != nil {
		t.Fatalf("send: %v", err)
	}

	pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 102)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if n != 102 {
		t.Fatalf("expected 102-byte magic packet, got %d", n)
	}
	for i := 0; i < 6; i++ {
		if buf[i] != 0xFF {
			t.Fatalf("expected 0xFF prefix at byte %d, got %#x", i, buf[i])
		}
	}
	hw, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatalf("parse mac: %v", err)
	}
	for i := 0; i < 16; i++ {
		start := 6 + i*6
		for j := 0; j < 6; j++ {
			if buf[start+j] != hw[j] {
				t.Fatalf("magic packet MAC mismatch at offset %d", start+j)
			}
		}
	}
}

type mockWOLDB struct{}

func (mockWOLDB) GetDevices() ([]model.WOLDevice, error) { return nil, nil }

func TestNewEngine(t *testing.T) {
	e := New(mockWOLDB{})
	if e == nil {
		t.Fatal("New returned nil engine")
	}
}
