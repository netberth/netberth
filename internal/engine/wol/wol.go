// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package wol

import (
	"bytes"
	"fmt"
	"net"
	"strings"

	"github.com/netberth/netberth/internal/model"
)

type Engine struct {
	db interface {
		GetDevices() ([]model.WOLDevice, error)
	}
}

func New(db interface {
	GetDevices() ([]model.WOLDevice, error)
}) *Engine {
	return &Engine{db: db}
}

func SendMagicPacket(device model.WOLDevice) error {
	mac, err := parseMAC(device.MAC)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}
	var packet [102]byte
	copy(packet[:6], bytes.Repeat([]byte{0xFF}, 6))
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	addr := net.JoinHostPort(device.Broadcast, fmt.Sprintf("%d", device.Port))
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write(packet[:]); err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	return nil
}

func parseMAC(mac string) (net.HardwareAddr, error) {
	mac = strings.ReplaceAll(mac, "-", ":")
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return nil, err
	}
	if len(hw) != 6 {
		return nil, fmt.Errorf("MAC must be 6 bytes")
	}
	return hw, nil
}
