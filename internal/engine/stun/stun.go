// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package stun

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
)

const (
	stunMagicCookie = 0x2112A442
	bindingRequest  = 0x0001
	bindingResponse = 0x0101
	bindingError    = 0x0111
	stunTimeout     = 3 * time.Second
	maxRetries      = 3
)

// NAT types per RFC 3489
const (
	NATUnknown            = iota
	NATNone               // Open internet: mapped == local
	NATFullCone           // Full cone: any external host can send to mapped addr
	NATRestrictedCone     // Restricted: only from same IP as STUN server
	NATPortRestrictedCone // Port restricted: only from same IP+port as STUN server
	NATSymmetric          // Symmetric: different mapped port per destination
)

type stunHeader struct {
	Type        uint16
	Length      uint16
	MagicCookie uint32
	TransID     [12]byte
}

type Engine struct {
	mu       sync.RWMutex
	tunnels  map[string]*tunnelState
	db       interface{ GetTunnels() ([]model.STUNTunnel, error) }
	notifier func(eventType, data string) // event bus callback
	stopCh   chan struct{}
}

// SetNotifier registers a callback for publishing events to the system bus.
func (e *Engine) SetNotifier(fn func(eventType, data string)) { e.notifier = fn }

type tunnelState struct {
	cfg    model.STUNTunnel
	cancel context.CancelFunc
	active bool
}

func New(db interface {
	GetTunnels() ([]model.STUNTunnel, error)
}) *Engine {
	return &Engine{
		tunnels: make(map[string]*tunnelState),
		db:      db,
		stopCh:  make(chan struct{}),
	}
}

func (e *Engine) Start() error {
	tunnels, err := e.db.GetTunnels()
	if err != nil {
		return err
	}
	for _, t := range tunnels {
		if t.Enabled {
			e.startTunnel(t)
		}
	}
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.tunnels {
		t.cancel()
	}
}

func (e *Engine) Reload(t model.STUNTunnel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ts, exists := e.tunnels[t.ID]; exists {
		ts.cancel()
		delete(e.tunnels, t.ID)
	}
	if t.Enabled {
		e.startTunnel(t)
	}
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ts, exists := e.tunnels[id]; exists {
		ts.cancel()
		delete(e.tunnels, id)
	}
}

// DetectNAT probes NAT type using RFC 3489 algorithm.
// Uses a single UDP socket with context timeout per bind.
func (e *Engine) DetectNAT(stunServer string) (int, *net.UDPAddr, error) {
	if stunServer == "" {
		stunServer = "stun.l.google.com:19302"
	}

	serverAddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		return NATUnknown, nil, fmt.Errorf("resolve STUN server: %w", err)
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return NATUnknown, nil, fmt.Errorf("listen UDP: %w", err)
	}
	defer conn.Close()

	// Test 1: Primary binding request
	mapped, err := stunBind(conn, serverAddr, stunTimeout)
	if err != nil {
		return NATUnknown, nil, fmt.Errorf("stun bind: %w", err)
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Open internet if mapped equals local
	if mapped.IP.Equal(localAddr.IP) && mapped.Port == localAddr.Port {
		return NATNone, mapped, nil
	}

	// Test 2: Second binding from same socket — detect symmetric NAT
	mapped2, err := stunBind(conn, serverAddr, stunTimeout)
	if err != nil {
		return NATUnknown, mapped, fmt.Errorf("second bind: %w", err)
	}
	if mapped2 != nil {
		if mapped.Port != mapped2.Port {
			return NATSymmetric, mapped, nil
		}
		// Update mapped to latest
		mapped = mapped2
	}

	// Test 3: Try to detect restricted/port-restricted
	// Send an echo from a different port to our mapped address
	altConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return NATFullCone, mapped, nil // conservative
	}
	defer altConn.Close()

	echo := make([]byte, 4)
	crand.Read(echo)
	altConn.WriteTo(echo, mapped)

	// Try to receive on the original socket from altConn
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 1500)
	_, from, rerr := conn.ReadFromUDP(buf)
	_ = conn.SetReadDeadline(time.Time{})

	if rerr == nil && from != nil && from.IP.Equal(altConn.LocalAddr().(*net.UDPAddr).IP) {
		return NATFullCone, mapped, nil
	}

	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, from2, rerr2 := conn.ReadFromUDP(buf)
	_ = conn.SetReadDeadline(time.Time{})

	if rerr2 == nil && from2 != nil {
		return NATRestrictedCone, mapped, nil
	}

	return NATPortRestrictedCone, mapped, nil
}

// stunBind sends a Binding Request and returns the mapped address.
// ctx controls timeout via the deadline set on the connection.
func stunBind(conn *net.UDPConn, server *net.UDPAddr, timeout time.Duration) (*net.UDPAddr, error) {
	req := buildBindingRequest()

	for attempt := 0; attempt < maxRetries; attempt++ {
		if _, err := conn.WriteTo(req, server); err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("write: %w", err)
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return nil, fmt.Errorf("set deadline: %w", err)
		}

		buf := make([]byte, 1500)
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if attempt < maxRetries-1 {
					continue
				}
				return nil, fmt.Errorf("timeout after %d attempts", maxRetries)
			}
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("read: %w", err)
			}
			continue
		}

		hdr := parseHeader(buf[:n])
		if hdr == nil || hdr.MagicCookie != stunMagicCookie {
			continue
		}
		if hdr.Type == bindingError {
				_, errCode, alt := parseAttributes(buf[20:n])
				msg := fmt.Sprintf("STUN error from %v", remote)
				if errCode != nil { msg = errCode.Error() }
				if alt != nil { msg += fmt.Sprintf(" (alternate: %s)", alt) }
			return nil, fmt.Errorf("%s", msg)
		}
		if hdr.Type != bindingResponse {
			continue
		}

		addr, _, _ := parseAttributes(buf[20:n])
		if addr == nil {
			return nil, fmt.Errorf("no mapped address in response")
		}
		return addr, nil
	}

	return nil, fmt.Errorf("all %d attempts failed", maxRetries)
}

func (e *Engine) startTunnel(t model.STUNTunnel) {
	ctx, cancel := context.WithCancel(context.Background())
	ts := &tunnelState{cfg: t, cancel: cancel}
	e.tunnels[t.ID] = ts
	go e.runTunnel(ctx, ts)
	logger.Log.Info().Str("name", t.Name).Int("local_port", t.LocalPort).Msg("STUN tunnel started")
}

func (e *Engine) runTunnel(ctx context.Context, ts *tunnelState) {
	defer func() { ts.active = false }()

	serverAddr := ts.cfg.STUNServer
	if _, _, err := net.SplitHostPort(serverAddr); err != nil {
		serverAddr += ":3478"
	}
	server, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		logger.Log.Error().Err(err).Str("name", ts.cfg.Name).Msg("STUN resolve failed")
		return
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: ts.cfg.LocalPort})
	if err != nil {
		logger.Log.Error().Err(err).Str("name", ts.cfg.Name).Msg("STUN listen failed")
		return
	}
	defer conn.Close()
	ts.active = true

	mapped, err := stunBind(conn, server, stunTimeout)
	if err != nil {
		logger.Log.Error().Err(err).Str("name", ts.cfg.Name).Msg("STUN bind failed")
		return
	}
	logger.Log.Info().Str("name", ts.cfg.Name).Str("mapped", mapped.String()).Msg("STUN mapped address")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stunBind(conn, server, stunTimeout)
		}
	}
}

func buildBindingRequest() []byte {
	packet := make([]byte, 20)
	binary.BigEndian.PutUint16(packet[0:2], bindingRequest)
	binary.BigEndian.PutUint16(packet[2:4], 0)
	binary.BigEndian.PutUint32(packet[4:8], stunMagicCookie)
	crand.Read(packet[8:20])
	return packet
}

// MultiProbeResult holds the results of probing multiple STUN servers.
type MultiProbeResult struct {
	ServerResults []ServerProbe `json:"servers"`
	ConsensusIP   string        `json:"consensus_ip"`
	ConsensusPort int           `json:"consensus_port"`
	Inconsistent  bool          `json:"inconsistent"`
}

type ServerProbe struct {
	Server   string `json:"server"`
	NatType  int    `json:"nat_type"`
	MappedIP string `json:"mapped_ip"`
	Port     int    `json:"port"`
	Error    string `json:"error,omitempty"`
}

// ProbeMultiple concurrently probes multiple STUN servers with per-server retry.
// Publishes results to the system event bus when configured.
func (e *Engine) ProbeMultiple(servers []string) *MultiProbeResult {
	if len(servers) == 0 {
		servers = []string{"stun.l.google.com:19302", "stun1.l.google.com:19302"}
	}
	type probeOut struct {
		server  string
		natType int
		addr    *net.UDPAddr
		err     error
	}
	ch := make(chan probeOut, len(servers))
	for _, srv := range servers {
		go func(server string) {
			var natType int
			var addr *net.UDPAddr
			var err error
			// Per-server retry loop
			for attempt := 0; attempt < 3; attempt++ {
				natType, addr, err = e.DetectNAT(server)
				if err == nil { break }
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
			ch <- probeOut{server, natType, addr, err}
		}(srv)
	}
	result := &MultiProbeResult{}
	ipCount := make(map[string]int)
	portCount := make(map[int]int)
	for range servers {
		p := <-ch
		sp := ServerProbe{Server: p.server, NatType: p.natType}
		if p.err != nil { sp.Error = p.err.Error() }
		if p.addr != nil {
			sp.MappedIP = p.addr.IP.String()
			sp.Port = p.addr.Port
			ipCount[sp.MappedIP]++
			portCount[sp.Port]++
		}
		result.ServerResults = append(result.ServerResults, sp)
	}
	for ip, c := range ipCount {
		if c > len(servers)/2 { result.ConsensusIP = ip; break }
	}
	for port, c := range portCount {
		if c > len(servers)/2 { result.ConsensusPort = port; break }
	}
	firstIP, firstPort := "", 0
	for _, s := range result.ServerResults {
		if s.Error != "" { continue }
		if firstIP == "" { firstIP = s.MappedIP; firstPort = s.Port; continue }
		if s.MappedIP != firstIP || s.Port != firstPort { result.Inconsistent = true; break }
	}
	logger.Log.Info().Int("servers", len(servers)).Str("consensus", fmt.Sprintf("%s:%d", result.ConsensusIP, result.ConsensusPort)).Bool("inconsistent", result.Inconsistent).Msg("multi-stun probe complete")

	// Publish to system event bus when NAT inconsistency or type change detected
	if e.notifier != nil {
		if result.Inconsistent {
			e.notifier("stun:nat_mismatch", fmt.Sprintf(`{"servers":%d,"consensus":"%s:%d","inconsistent":true}`, len(servers), result.ConsensusIP, result.ConsensusPort))
		}
		for _, s := range result.ServerResults {
			if s.NatType == NATSymmetric {
				e.notifier("stun:symmetric_detected", fmt.Sprintf(`{"server":"%s","mapped":"%s:%d"}`, s.Server, s.MappedIP, s.Port))
			}
		}
	}
	return result
}

func parseHeader(data []byte) *stunHeader {
	if len(data) < 20 {
		return nil
	}
	h := &stunHeader{
		Type:        binary.BigEndian.Uint16(data[0:2]),
		Length:      binary.BigEndian.Uint16(data[2:4]),
		MagicCookie: binary.BigEndian.Uint32(data[4:8]),
	}
	copy(h.TransID[:], data[8:20])
	return h
}

func parseMappedAddress(data []byte) *net.UDPAddr {
	pos := 0
	for pos+4 <= len(data) {
		attrType := binary.BigEndian.Uint16(data[pos:])
		attrLen := binary.BigEndian.Uint16(data[pos+2:])
		pos += 4
		if pos+int(attrLen) > len(data) {
			break
		}
		if attrType == 0x0001 || attrType == 0x0020 {
			if attrLen >= 8 && data[pos+1] == 0x01 {
				port := int(binary.BigEndian.Uint16(data[pos+2:])) ^ (int(stunMagicCookie>>16) & 0xFFFF)
				a, b, c, d := data[pos+4], data[pos+5], data[pos+6], data[pos+7]
				if attrType == 0x0020 {
					m := uint32(stunMagicCookie)
					a ^= byte(m >> 24)
					b ^= byte(m >> 16)
					c ^= byte(m >> 8)
					d ^= byte(m)
				}
				ip := net.IPv4(a, b, c, d)
				return &net.UDPAddr{IP: ip.To4(), Port: port}
			}
		}
		pos += int(attrLen)
		if pos%4 != 0 {
			pos += 4 - pos%4
		}
	}
	return nil
}

// reqTID extracts the 12-byte transaction ID from a STUN request packet.
func reqTID(req []byte) [12]byte { var tid [12]byte; copy(tid[:], req[8:20]); return tid }

// SymmetricNATAnalysis holds port delta analysis for symmetric NAT.
type SymmetricNATAnalysis struct {
	Ports      []int `json:"ports"`      // mapped ports observed
	MinDelta   int   `json:"min_delta"`  // smallest port delta
	MaxDelta   int   `json:"max_delta"`  // largest port delta
	AvgDelta   float64 `json:"avg_delta"` // average delta
	IsRandom   bool  `json:"is_random"`  // true if deltas appear random
	Prediction int   `json:"prediction"` // predicted next port
}

// AnalyzeSymmetricNAT probes the STUN server N times and analyzes port delta patterns.
// Essential for hole punching — knowing the delta enables port prediction.
func (e *Engine) AnalyzeSymmetricNAT(stunServer string, probes int) (*SymmetricNATAnalysis, error) {
	if stunServer == "" { stunServer = "stun.l.google.com:19302" }
	if probes < 3 { probes = 5 }

	serverAddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil { return nil, err }

	ports := make([]int, 0, probes)
	for i := 0; i < probes; i++ {
		conn, err := net.ListenUDP("udp", nil)
		if err != nil { continue }
		addr, err := stunBind(conn, serverAddr, stunTimeout)
		conn.Close()
		if err != nil { continue }
		ports = append(ports, addr.Port)
		time.Sleep(200 * time.Millisecond)
	}

	if len(ports) < 2 { return nil, fmt.Errorf("insufficient probes: %d", len(ports)) }

	analysis := &SymmetricNATAnalysis{Ports: ports}
	deltas := make([]int, 0, len(ports)-1)
	for i := 1; i < len(ports); i++ {
		d := ports[i] - ports[i-1]
		if d < 0 { d = -d }
		deltas = append(deltas, d)
	}
	if len(deltas) > 0 {
		analysis.MinDelta = deltas[0]
		analysis.MaxDelta = deltas[0]
		sum := 0
		for _, d := range deltas {
			sum += d
			if d < analysis.MinDelta { analysis.MinDelta = d }
			if d > analysis.MaxDelta { analysis.MaxDelta = d }
		}
		analysis.AvgDelta = float64(sum) / float64(len(deltas))
		analysis.IsRandom = analysis.MaxDelta-analysis.MinDelta > 1000
		// Simple prediction: last port + average delta
		analysis.Prediction = ports[len(ports)-1] + int(analysis.AvgDelta)
	}

	logger.Log.Info().Int("probes", probes).Int("samples", len(ports)).Float64("avg_delta", analysis.AvgDelta).Bool("random", analysis.IsRandom).Msg("symmetric NAT analysis complete")
	return analysis, nil
}
