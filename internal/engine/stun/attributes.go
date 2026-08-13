// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package stun

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net"
)

// RFC 5389 attribute types
const (
	attrMappedAddress    = 0x0001
	attrErrorCode        = 0x0009
	attrXorMappedAddress = 0x0020
	attrAlternateServer  = 0x8023
	attrFingerprint      = 0x8028
)

// STUNError holds a parsed RFC 5389 error-code attribute.
type STUNError struct {
	Code   int // 300-699
	Reason string
}

func (e *STUNError) Error() string { return fmt.Sprintf("STUN %d %s", e.Code, e.Reason) }

// AlternateServer holds a parsed ALTERNATE-SERVER attribute.
type AlternateServer struct {
	IP   net.IP
	Port int
}

func (a *AlternateServer) String() string { return fmt.Sprintf("%s:%d", a.IP, a.Port) }

// parseAttributes parses all STUN attributes from the message body (the bytes
// after the 20-byte STUN header). tid is the 12-byte transaction ID from the
// message header; it is required to decode IPv6 XOR-MAPPED-ADDRESS values
// (RFC 5389 §15.2). Returns the mapped address, error code, and alternate
// server if present.
func parseAttributes(data []byte, tid [12]byte) (mapped *net.UDPAddr, errCode *STUNError, altServer *AlternateServer) {
	pos := 0
	for pos+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[pos:])
		attrLen := int(binary.BigEndian.Uint16(data[pos+2:]))
		valuePos := pos + 4
		if attrLen > len(data)-valuePos {
			break // truncated attribute: the rest of the message is unusable
		}
		value := data[valuePos : valuePos+attrLen]

		switch attrType {
		case attrMappedAddress:
			if addr := parseAddressValue(value, false, tid); addr != nil {
				mapped = addr
			}
		case attrXorMappedAddress:
			if addr := parseAddressValue(value, true, tid); addr != nil {
				mapped = addr
			}
		case attrErrorCode:
			if attrLen >= 4 {
				// RFC 5389 §15.6: reserved(21 bits) | class(3 bits) | number(8 bits)
				_class := int(value[2] & 0x07)
				_number := int(value[3])
				_code := _class*100 + _number
				reason := ""
				if attrLen > 4 {
					reason = string(value[4:])
				}
				errCode = &STUNError{Code: _code, Reason: reason}
			}
		case attrAlternateServer:
			if addr := parseAddressValue(value, false, tid); addr != nil {
				altServer = &AlternateServer{IP: addr.IP, Port: addr.Port}
			}
		}
		pos = valuePos + attrLen
		if pad := pos % 4; pad != 0 {
			pos += 4 - pad
		}
	}
	return
}

// parseAddressValue decodes one MAPPED-ADDRESS-style attribute value
// (RFC 5389 §15.1/§15.2). MAPPED-ADDRESS and ALTERNATE-SERVER use the plain
// encoding; only XOR-MAPPED-ADDRESS (xor=true) is XORed with the magic cookie
// (IPv4) or magic cookie || transaction ID (IPv6).
func parseAddressValue(value []byte, xor bool, tid [12]byte) *net.UDPAddr {
	if len(value) < 4 {
		return nil
	}
	family := value[1]
	port := int(binary.BigEndian.Uint16(value[2:4]))

	switch family {
	case 0x01: // IPv4
		if len(value) < 8 {
			return nil
		}
		ip := net.IPv4(value[4], value[5], value[6], value[7])
		if xor {
			port ^= int(stunMagicCookie>>16) & 0xFFFF
			m := uint32(stunMagicCookie)
			ip = net.IPv4(
				value[4]^byte(m>>24),
				value[5]^byte(m>>16),
				value[6]^byte(m>>8),
				value[7]^byte(m),
			)
		}
		return &net.UDPAddr{IP: ip.To4(), Port: port}
	case 0x02: // IPv6
		if len(value) < 20 {
			return nil
		}
		addr := make(net.IP, net.IPv6len)
		copy(addr, value[4:20])
		if xor {
			port ^= int(stunMagicCookie>>16) & 0xFFFF
			var key [16]byte
			binary.BigEndian.PutUint32(key[0:4], stunMagicCookie)
			copy(key[4:16], tid[:])
			for i := range addr {
				addr[i] ^= key[i]
			}
		}
		return &net.UDPAddr{IP: addr, Port: port}
	}
	return nil
}

// fingerprintOffset returns the byte offset of the FINGERPRINT attribute
// header within a full STUN message (including the 20-byte header), or -1 if
// the attribute is absent or the message is truncated.
func fingerprintOffset(pkt []byte) int {
	if len(pkt) < 24 {
		return -1
	}
	body := pkt[20:]
	pos := 0
	for pos+4 <= len(body) {
		attrType := binary.BigEndian.Uint16(body[pos:])
		attrLen := int(binary.BigEndian.Uint16(body[pos+2:]))
		valuePos := pos + 4
		if attrLen > len(body)-valuePos {
			return -1
		}
		if attrType == attrFingerprint {
			return 20 + pos
		}
		pos = valuePos + attrLen
		if pad := pos % 4; pad != 0 {
			pos += 4 - pad
		}
	}
	return -1
}

// validFingerprint reports whether pkt carries a valid FINGERPRINT attribute
// (RFC 5389 §15.5). When present, FINGERPRINT must be the last attribute and
// its value is crc32(message up to the attribute) XOR 0x5354554e. Messages
// without FINGERPRINT are accepted (the attribute is optional).
func validFingerprint(pkt []byte) bool {
	off := fingerprintOffset(pkt)
	if off < 0 {
		return true
	}
	// Value length is 4; attribute header 4 bytes, so it must end the message.
	if off+8 != len(pkt) {
		return false
	}
	want := crc32.ChecksumIEEE(pkt[:off]) ^ 0x5354554e
	got := binary.BigEndian.Uint32(pkt[off+4 : off+8])
	return want == got
}
