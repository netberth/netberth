// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package stun

import (
	"encoding/binary"
	"fmt"
	"net"
)

// RFC 5389 attribute types
const (
	attrMappedAddress    = 0x0001
	attrErrorCode        = 0x0009
	attrXorMappedAddress = 0x0020
	attrAlternateServer  = 0x8023
)

// STUNError holds a parsed RFC 5389 error-code attribute.
type STUNError struct {
	Code   int    // 300-699
	Reason string
}

func (e *STUNError) Error() string { return fmt.Sprintf("STUN %d %s", e.Code, e.Reason) }

// AlternateServer holds a parsed ALTERNATE-SERVER attribute.
type AlternateServer struct {
	IP   net.IP
	Port int
}

func (a *AlternateServer) String() string { return fmt.Sprintf("%s:%d", a.IP, a.Port) }

// parseAttributes parses all STUN attributes from the given data buffer.
// Returns mapped address, error code, and alternate server if present.
func parseAttributes(data []byte) (mapped *net.UDPAddr, errCode *STUNError, altServer *AlternateServer) {
	pos := 0
	for pos+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[pos:])
		attrLen := int(binary.BigEndian.Uint16(data[pos+2:]))
		pos += 4
		if pos+attrLen > len(data) { break }

		switch attrType {
		case attrMappedAddress, attrXorMappedAddress:
			if attrLen >= 8 && data[pos+1] == 0x01 {
				port := int(binary.BigEndian.Uint16(data[pos+2:])) ^ (int(stunMagicCookie>>16) & 0xFFFF)
				a, b, c, d := data[pos+4], data[pos+5], data[pos+6], data[pos+7]
				if attrType == attrXorMappedAddress {
					m := uint32(stunMagicCookie)
					a ^= byte(m >> 24); b ^= byte(m >> 16); c ^= byte(m >> 8); d ^= byte(m)
				}
				mapped = &net.UDPAddr{IP: net.IPv4(a, b, c, d).To4(), Port: port}
			}
		case attrErrorCode:
			if attrLen >= 4 {
				// RFC 5389 §15.6: reserved(21 bits) | class(3 bits) | number(8 bits)
				_class := int(data[pos+2] & 0x07)
				_number := int(data[pos+3])
				_code := _class*100 + _number
				reason := ""
				if attrLen > 4 { reason = string(data[pos+4 : pos+attrLen]) }
				errCode = &STUNError{Code: _code, Reason: reason}
			}
		case attrAlternateServer:
			if attrLen >= 8 && data[pos+1] == 0x01 {
				port := int(binary.BigEndian.Uint16(data[pos+2:])) ^ (int(stunMagicCookie>>16) & 0xFFFF)
				ip := net.IPv4(data[pos+4], data[pos+5], data[pos+6], data[pos+7])
				altServer = &AlternateServer{IP: ip.To4(), Port: port}
			}
		}
		pos += attrLen
		if pos%4 != 0 { pos += 4 - pos%4 }
	}
	return
}
