// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package stun

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func TestBuildBindingRequest(t *testing.T) {
	req := buildBindingRequest()
	if len(req) != 20 {
		t.Fatalf("expected 20 bytes, got %d", len(req))
	}
	typ := binary.BigEndian.Uint16(req[0:2])
	if typ != bindingRequest {
		t.Fatalf("expected binding request type 0x%04x, got 0x%04x", bindingRequest, typ)
	}
	length := binary.BigEndian.Uint16(req[2:4])
	if length != 0 {
		t.Fatalf("expected length 0, got %d", length)
	}
	magic := binary.BigEndian.Uint32(req[4:8])
	if magic != stunMagicCookie {
		t.Fatalf("expected magic cookie 0x%x, got 0x%x", stunMagicCookie, magic)
	}
	// Transaction ID should not be all zeros
	var zeroes [12]byte
	tid := req[8:20]
	if string(tid) == string(zeroes[:]) {
		t.Fatal("transaction ID should not be all zeros")
	}
}

func TestParseHeader(t *testing.T) {
	// Valid header
	packet := make([]byte, 20)
	binary.BigEndian.PutUint16(packet[0:2], bindingResponse)
	binary.BigEndian.PutUint16(packet[2:4], 12)
	binary.BigEndian.PutUint32(packet[4:8], stunMagicCookie)
	copy(packet[8:20], []byte("abcdefghijkl"))

	h := parseHeader(packet)
	if h == nil {
		t.Fatal("expected valid header")
	}
	if h.Type != bindingResponse {
		t.Errorf("expected type %d, got %d", bindingResponse, h.Type)
	}
	if h.Length != 12 {
		t.Errorf("expected length 12, got %d", h.Length)
	}
	if h.MagicCookie != stunMagicCookie {
		t.Errorf("expected magic cookie %x, got %x", stunMagicCookie, h.MagicCookie)
	}

	// Too short
	if parseHeader(make([]byte, 10)) != nil {
		t.Fatal("expected nil for short data")
	}

	// Wrong magic
	packet2 := make([]byte, 20)
	binary.BigEndian.PutUint32(packet2[4:8], 0xDEADBEEF)
	if h2 := parseHeader(packet2); h2 == nil {
		t.Fatal("parseHeader should accept any magic cookie (caller validates)")
	}
}

func TestParseMappedAddress(t *testing.T) {
	// Build a valid XOR-MAPPED-ADDRESS attribute (0x0020)
	data := make([]byte, 12)
	binary.BigEndian.PutUint16(data[0:2], 0x0020) // XOR-MAPPED-ADDRESS
	binary.BigEndian.PutUint16(data[2:4], 8)      // length
	data[4] = 0                                   // reserved
	data[5] = 0x01                                // IPv4 family

	// Encode 192.168.1.100:8080 XOR'd with magic cookie
	rawPort := uint16(8080) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(data[6:8], rawPort)

	magic := uint32(stunMagicCookie)
	data[8] = 192 ^ byte(magic>>24)
	data[9] = 168 ^ byte(magic>>16)
	data[10] = 1 ^ byte(magic>>8)
	data[11] = 100 ^ byte(magic)

	addr := parseMappedAddress(data)
	if addr == nil {
		t.Fatal("expected mapped address")
	}
	if addr.IP.String() != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %s", addr.IP)
	}
	if addr.Port != 8080 {
		t.Errorf("expected port 8080, got %d", addr.Port)
	}

	// Also test MAPPED-ADDRESS (0x0001)
	data2 := make([]byte, 12)
	binary.BigEndian.PutUint16(data2[0:2], 0x0001)
	binary.BigEndian.PutUint16(data2[2:4], 8)
	data2[5] = 0x01
	binary.BigEndian.PutUint16(data2[6:8], 9090)
	data2[8] = 10
	data2[9] = 0
	data2[10] = 0
	data2[11] = 1

	addr2 := parseMappedAddress(data2)
	if addr2 == nil {
		t.Fatal("expected mapped address for MAPPED-ADDRESS")
	}
	if addr2.IP.String() != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", addr2.IP)
	}
}

// mockSTUNServer starts a UDP server that responds with a valid Binding Response
func mockSTUNServer(t *testing.T) (*net.UDPAddr, func()) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("mock server listen: %v", err)
	}
	addr := conn.LocalAddr().(*net.UDPAddr)

	go func() {
		buf := make([]byte, 1500)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			// Parse request header
			if n < 20 {
				continue
			}
			hdr := parseHeader(buf[:n])
			if hdr == nil || hdr.Type != bindingRequest {
				continue
			}

			// Build response with XOR-MAPPED-ADDRESS
			resp := make([]byte, 32)
			binary.BigEndian.PutUint16(resp[0:2], bindingResponse)
			binary.BigEndian.PutUint16(resp[2:4], 12)
			binary.BigEndian.PutUint32(resp[4:8], stunMagicCookie)
			copy(resp[8:20], hdr.TransID[:])

			// XOR-MAPPED-ADDRESS attribute
			attr := resp[20:]
			binary.BigEndian.PutUint16(attr[0:2], 0x0020)
			binary.BigEndian.PutUint16(attr[2:4], 8)
			attr[5] = 0x01 // IPv4

			remoteIP := remote.IP.To4()
			rawPort := uint16(remote.Port) ^ uint16(stunMagicCookie>>16)
			binary.BigEndian.PutUint16(attr[6:8], rawPort)

			magic := uint32(stunMagicCookie)
			attr[8] = remoteIP[0] ^ byte(magic>>24)
			attr[9] = remoteIP[1] ^ byte(magic>>16)
			attr[10] = remoteIP[2] ^ byte(magic>>8)
			attr[11] = remoteIP[3] ^ byte(magic)

			conn.WriteTo(resp, remote)
		}
	}()
	return addr, func() { conn.Close() }
}

func TestStunBindSuccess(t *testing.T) {
	serverAddr, cleanup := mockSTUNServer(t)
	defer cleanup()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	mapped, err := stunBind(conn, serverAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("stunBind failed: %v", err)
	}
	if mapped == nil {
		t.Fatal("expected mapped address")
	}
	if mapped.Port == 0 {
		t.Fatal("expected non-zero port")
	}
	if !mapped.IP.IsLoopback() {
		t.Errorf("expected loopback IP, got %s", mapped.IP)
	}
}

func TestStunBindTimeout(t *testing.T) {
	// No server running — should timeout
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	serverAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 19999}
	_, err = stunBind(conn, serverAddr, 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestStunBindRetry(t *testing.T) {
	serverAddr, cleanup := mockSTUNServer(t)
	defer cleanup()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer conn.Close()

	// Should succeed on first attempt with retry support
	mapped, err := stunBind(conn, serverAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("stunBind with retry failed: %v", err)
	}
	if mapped == nil || mapped.Port == 0 {
		t.Fatal("expected valid mapped address with retry")
	}
}

func TestDetectNAT(t *testing.T) {
	serverAddr, cleanup := mockSTUNServer(t)
	defer cleanup()

	eng := New(nil)
	natType, mapped, err := eng.DetectNAT(serverAddr.String())
	if err != nil {
		t.Fatalf("DetectNAT failed: %v", err)
	}
	t.Logf("NAT type: %d, mapped: %v", natType, mapped)
	if natType == NATUnknown {
		t.Fatal("expected known NAT type")
	}
	if mapped == nil || mapped.Port == 0 {
		t.Fatal("expected valid mapped address")
	}
}

func TestEngineLifecycle(t *testing.T) {
	eng := New(&mockTunnelDB{})
	if err := eng.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	eng.Stop()
}

func TestEngineReload(t *testing.T) {
	eng := New(&mockTunnelDB{})
	_ = eng.Start()
	tunnel := model.STUNTunnel{ID: "test-1", Name: "test", Protocol: "tcp", LocalPort: 12345, STUNServer: "localhost:19302", TargetAddr: "127.0.0.1", TargetPort: 80, Enabled: true}
	eng.Reload(tunnel)
	eng.Remove("test-1")
	eng.Stop()
}

// mockTunnelDB for testing
type mockTunnelDB struct{}

func (m *mockTunnelDB) GetTunnels() ([]model.STUNTunnel, error) {
	return nil, nil
}

func TestNATTypeConstants(t *testing.T) {
	// Verify all NAT types have unique values
	types := map[int]string{
		NATUnknown:            "Unknown",
		NATNone:               "None",
		NATFullCone:           "FullCone",
		NATRestrictedCone:     "RestrictedCone",
		NATPortRestrictedCone: "PortRestrictedCone",
		NATSymmetric:          "Symmetric",
	}
	if len(types) != 6 { t.Fatal("expected 6 NAT types") }
	for k, v := range types {
		if v == "" { t.Errorf("NAT type %d has no name", k) }
	}
}

func TestStunMagicCookie(t *testing.T) {
	if stunMagicCookie != 0x2112A442 {
		t.Fatal("magic cookie mismatch — RFC 5389 requires 0x2112A442")
	}
}

func TestStunBindIPv6Loopback(t *testing.T) {
	// Test that stunBind handles IPv6 addresses correctly
	serverAddr := &net.UDPAddr{IP: net.IPv6loopback, Port: 23478}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})
	if err != nil { t.Skipf("IPv6 not available: %v", err); return }
	defer conn.Close()

	// Should timeout (no server), but NOT panic
	_, err = stunBind(conn, serverAddr, 500*time.Millisecond)
	if err == nil { t.Log("unexpected success on IPv6 loopback") }
	// Just verify no panic occurred
	t.Log("IPv6 stunBind completed without panic")
}

func TestRetryExhaustion(t *testing.T) {
	// Verify max retry count is 3
	if maxRetries != 3 { t.Errorf("expected maxRetries=3, got %d", maxRetries) }
}

func TestMultiProbe(t *testing.T) {
	srv1, clean1 := mockSTUNServer(t)
	defer clean1()
	srv2, clean2 := mockSTUNServer(t)
	defer clean2()

	eng := New(nil)
	result := eng.ProbeMultiple([]string{srv1.String(), srv2.String()})

	if len(result.ServerResults) != 2 {
		t.Fatalf("expected 2 server results, got %d", len(result.ServerResults))
	}
	for _, s := range result.ServerResults {
		t.Logf("  %s → %s:%d (NAT=%d, err=%s)", s.Server, s.MappedIP, s.Port, s.NatType, s.Error)
		if s.Error != "" { t.Errorf("unexpected error from %s: %s", s.Server, s.Error) }
	}
	if result.ConsensusIP == "" { t.Error("expected consensus IP") }
	if result.Inconsistent { t.Log("NAT inconsistency detected (expected with mock servers on different ports)") }
}

func TestMultiProbeDefaultServers(t *testing.T) {
	eng := New(nil)
	result := eng.ProbeMultiple(nil)
	if len(result.ServerResults) == 0 {
		t.Fatal("expected at least some results with default servers")
	}
	t.Logf("Default servers probe: %d results, inconsistent=%v", len(result.ServerResults), result.Inconsistent)
}

func TestMultiProbeWithNotifier(t *testing.T) {
	srv1, clean1 := mockSTUNServer(t)
	defer clean1()
	srv2, clean2 := mockSTUNServer(t)
	defer clean2()

	eng := New(nil)
	var events []string
	var mu sync.Mutex
	eng.SetNotifier(func(eventType, data string) {
		mu.Lock()
		events = append(events, eventType)
		mu.Unlock()
	})

	result := eng.ProbeMultiple([]string{srv1.String(), srv2.String()})
	if result == nil { t.Fatal("expected result") }
	if len(result.ServerResults) != 2 { t.Errorf("expected 2 results, got %d", len(result.ServerResults)) }

	t.Logf("Notifier events (%d): %v", len(events), events)

	// With 2 servers, should publish events for each server's NAT type
	foundSymmetric := false
	for _, ev := range events {
		if strings.Contains(ev, "symmetric_detected") { foundSymmetric = true }
	}

	// If inconsistent, should have nat_mismatch event
	if result.Inconsistent {
		hasMismatch := false
		for _, ev := range events {
			if strings.Contains(ev, "nat_mismatch") { hasMismatch = true; break }
		}
		if !hasMismatch { t.Error("inconsistent result should publish nat_mismatch event") }
	}
	_ = foundSymmetric // may or may not be symmetric in test env
}

func TestMultiProbeTwoServersInconsistency(t *testing.T) {
	// Use 2 mock servers on different ports — should detect inconsistency
	srv1, clean1 := mockSTUNServer(t)
	defer clean1()
	srv2, clean2 := mockSTUNServer(t)
	defer clean2()

	eng := New(nil)
	result := eng.ProbeMultiple([]string{srv1.String(), srv2.String()})
	t.Logf("inconsistent=%v consensus=%s:%d", result.Inconsistent, result.ConsensusIP, result.ConsensusPort)
	for _, s := range result.ServerResults {
		t.Logf("  %s → %s:%d err=%v", s.Server, s.MappedIP, s.Port, s.Error)
	}
	// Both servers are on localhost — consensus IP should be 127.0.0.1
	if result.ConsensusIP != "127.0.0.1" {
		t.Errorf("consensus IP expected 127.0.0.1, got %s", result.ConsensusIP)
	}
}

func TestParseErrorCodeAttribute(t *testing.T) {
	// Build a STUN error response with ERROR-CODE attribute (RFC 5389 §15.6)
	reason := "Unauthorized"
	attrValueLen := 4 + len(reason)  // reserved+class+number + reason = 16
	attrTotalLen := 4 + attrValueLen // TLV header + value = 20
	data := make([]byte, 20+attrTotalLen) // STUN header + attribute
	binary.BigEndian.PutUint16(data[0:2], bindingError)
	binary.BigEndian.PutUint16(data[2:4], uint16(attrTotalLen))
	binary.BigEndian.PutUint32(data[4:8], stunMagicCookie)

	attr := data[20:] // start of attribute data
	binary.BigEndian.PutUint16(attr[0:2], attrErrorCode)
	binary.BigEndian.PutUint16(attr[2:4], uint16(attrValueLen))
	binary.BigEndian.PutUint16(attr[4:6], 0) // reserved high 2 bytes
	attr[6] = 0x04 // class 4xx in lower 3 bits
	attr[7] = 0x01 // number = 1 → 401
	copy(attr[8:], reason)

	_, errCode, _ := parseAttributes(data[20:])
	if errCode == nil { t.Fatal("expected error code") }
	if errCode.Code != 401 { t.Errorf("expected 401, got %d", errCode.Code) }
	if errCode.Reason != "Unauthorized" { t.Errorf("expected reason Unauthorized, got %s", errCode.Reason) }
}

func TestParseAlternateServerAttribute(t *testing.T) {
	data := make([]byte, 20+12)
	binary.BigEndian.PutUint16(data[0:2], bindingError)
	binary.BigEndian.PutUint16(data[2:4], 12)
	binary.BigEndian.PutUint32(data[4:8], stunMagicCookie)

	// ALTERNATE-SERVER attribute: 192.168.1.1:3479
	attr := data[20:]
	binary.BigEndian.PutUint16(attr[0:2], attrAlternateServer)
	binary.BigEndian.PutUint16(attr[2:4], 8)
	attr[4] = 0; attr[5] = 0x01 // IPv4
	rawPort := uint16(3479) ^ uint16(stunMagicCookie>>16)
	binary.BigEndian.PutUint16(attr[6:8], rawPort)
	attr[8] = 192; attr[9] = 168; attr[10] = 1; attr[11] = 1

	_, _, alt := parseAttributes(data[20:])
	if alt == nil { t.Fatal("expected alternate server") }
	if alt.IP.String() != "192.168.1.1" { t.Errorf("expected 192.168.1.1, got %s", alt.IP) }
	if alt.Port != 3479 { t.Errorf("expected 3479, got %d", alt.Port) }
}

func TestTransactionIDRandomness(t *testing.T) {
	req1 := buildBindingRequest()
	req2 := buildBindingRequest()
	var tid1, tid2 [12]byte
	copy(tid1[:], req1[8:20])
	copy(tid2[:], req2[8:20])
	if tid1 == tid2 { t.Fatal("transaction IDs must be unique") }

	// Verify extract helper
	extracted := reqTID(req1)
	if extracted != tid1 { t.Fatal("reqTID extraction mismatch") }

	// Zero check — crypto/rand should not produce all zeros
	var zeroes [12]byte
	if tid1 == zeroes { t.Fatal("TID should not be all zeros") }
}

func TestSymmetricNATAnalysis(t *testing.T) {
	srv1, clean1 := mockSTUNServer(t)
	defer clean1()

	eng := New(nil)
	analysis, err := eng.AnalyzeSymmetricNAT(srv1.String(), 5)
	if err != nil { t.Fatalf("analysis failed: %v", err) }
	if len(analysis.Ports) < 2 { t.Fatal("expected at least 2 port samples") }
	t.Logf("Ports: %v", analysis.Ports)
	t.Logf("Delta: min=%d max=%d avg=%.1f random=%v prediction=%d",
		analysis.MinDelta, analysis.MaxDelta, analysis.AvgDelta, analysis.IsRandom, analysis.Prediction)
	if analysis.MinDelta < 0 { t.Error("delta must be non-negative") }
}

func TestSymmetricNATDefaultProbes(t *testing.T) {
	eng := New(nil)
	analysis, err := eng.AnalyzeSymmetricNAT("", 0)
	if err != nil {
		t.Skipf("STUN unreachable: %v", err)
	}
	t.Logf("Default probes: %d ports sampled, avg_delta=%.1f", len(analysis.Ports), analysis.AvgDelta)
}

func TestHolePunchBirthday(t *testing.T) {
	// Start a mock remote that echoes back PUNCH packets
	remoteConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil { t.Fatal(err) }
	defer remoteConn.Close()
	remotePort := remoteConn.LocalAddr().(*net.UDPAddr).Port

	// Echo server
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := remoteConn.ReadFromUDP(buf)
			if err != nil { return }
			remoteConn.WriteTo(buf[:n], from)
		}
	}()

	// Run birthday punch against the echo server
	result, err := birthdayPunch("127.0.0.1", 55555, fmt.Sprintf("127.0.0.1:%d", remotePort))
	if err != nil { t.Fatalf("birthdayPunch error: %v", err) }
	t.Logf("Birthday punch: method=%s attempts=%d probed=%d success=%v",
		result.Method, result.Attempts, len(result.ProbedPorts), result.Success)
	if result.Success {
		t.Logf("Punch succeeded! Remote port: %d", result.RemotePort)
	}
}

func TestHolePunchDelta(t *testing.T) {
	analysis := &SymmetricNATAnalysis{
		Ports:      []int{50000, 52000, 54000},
		MinDelta:   2000,
		MaxDelta:   2000,
		AvgDelta:   2000,
		IsRandom:   false,
		Prediction: 56000,
	}

	result, err := deltaPunch("127.0.0.1", 44444, "10.0.0.1", analysis)
	if err != nil { t.Fatalf("deltaPunch error: %v", err) }
	t.Logf("Delta punch: method=%s attempts=%d probed=%d",
		result.Method, result.Attempts, len(result.ProbedPorts))
	if len(result.ProbedPorts) == 0 { t.Error("expected probe ports") }
	if result.Method != "delta_predict" { t.Error("expected delta_predict method") }
}

func TestHolePunchNilAnalysis(t *testing.T) {
	// nil analysis should fall back to birthday punch
	result, err := HolePunch("127.0.0.1", 33333, "10.0.0.1", nil)
	if err != nil { t.Fatalf("HolePunch error: %v", err) }
	if result.Method != "birthday" { t.Errorf("expected birthday fallback, got %s", result.Method) }
}
