// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package stun

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/netberth/netberth/pkg/logger"
)

// HolePunchResult holds the outcome of a P2P hole punch attempt.
type HolePunchResult struct {
	Success    bool     `json:"success"`
	LocalPort  int      `json:"local_port"`
	RemotePort int      `json:"remote_port"`
	Attempts   int      `json:"attempts"`
	Method     string   `json:"method"` // "delta_predict", "birthday"
	ProbedPorts []int   `json:"probed_ports,omitempty"`
}

// HolePunch attempts P2P UDP hole punching using port delta prediction.
// localIP/localPort: our side, remoteIP: peer's public IP.
// delta analysis: from AnalyzeSymmetricNAT, provides port prediction.
func HolePunch(localIP string, localPort int, remoteIP string, analysis *SymmetricNATAnalysis) (*HolePunchResult, error) {
	if analysis == nil || analysis.Prediction == 0 {
		return birthdayPunch(localIP, localPort, remoteIP)
	}
	return deltaPunch(localIP, localPort, remoteIP, analysis)
}

// deltaPunch uses the port delta analysis to predict and probe the remote port.
func deltaPunch(localIP string, localPort int, remoteIP string, analysis *SymmetricNATAnalysis) (*HolePunchResult, error) {
	result := &HolePunchResult{LocalPort: localPort, Method: "delta_predict"}
	predicted := analysis.Prediction
	if predicted < 1024 || predicted > 65535 { predicted = 20000 + localPort%40000 }

	// Candidate ports: predicted, ±delta, ±2*delta
	candidates := []int{predicted}
	if analysis.AvgDelta > 0 {
		d := int(analysis.AvgDelta)
		candidates = append(candidates, predicted+d, predicted-d, predicted+2*d, predicted-2*d)
	}
	result.ProbedPorts = make([]int, 0, len(candidates))

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(localIP), Port: localPort})
	if err != nil { return nil, fmt.Errorf("local listen: %w", err) }
	defer conn.Close()

	punch := make([]byte, 4)
	copy(punch, []byte("PUNCH"))
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	var wg sync.WaitGroup

	for _, port := range candidates {
		if port < 1 || port > 65535 { continue }
		result.ProbedPorts = append(result.ProbedPorts, port)
		result.RemotePort = port
		remoteAddr := &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: port}

		wg.Add(1)
		go func(ra *net.UDPAddr) {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				conn.WriteTo(punch, ra)
				time.Sleep(50 * time.Millisecond)
			}
		}(remoteAddr)
	}
	wg.Wait()

	// Check for response
	buf := make([]byte, 1500)
	n, from, err := conn.ReadFromUDP(buf)
	if err == nil && n > 0 && from.IP.String() == remoteIP {
		result.Success = true
		result.RemotePort = from.Port
		logger.Log.Info().Str("remote", from.String()).Msg("hole punch successful via delta prediction")
		return result, nil
	}

	result.Attempts = len(result.ProbedPorts)
	logger.Log.Warn().Int("probes", result.Attempts).Msg("hole punch incomplete — no response")
	return result, nil
}

// birthdayPunch uses birthday paradox brute-force port probing when no delta data is available.
func birthdayPunch(localIP string, localPort int, remoteIP string) (*HolePunchResult, error) {
	result := &HolePunchResult{LocalPort: localPort, Method: "birthday"}
	const burstSize = 64 // sqrt of ephemeral port range

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(localIP), Port: localPort})
	if err != nil { return nil, fmt.Errorf("local listen: %w", err) }
	defer conn.Close()

	punch := make([]byte, 4)
	copy(punch, []byte("PUNCH"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	startPort := 20000 + (localPort % 40000)
	var wg sync.WaitGroup

	for i := 0; i < burstSize; i++ {
		port := startPort + i*11 // stepped probing
		if port > 65535 { break }
		remoteAddr := &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: port}
		result.ProbedPorts = append(result.ProbedPorts, port)
		wg.Add(1)
		go func(ra *net.UDPAddr) {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				conn.WriteTo(punch, ra)
				time.Sleep(25 * time.Millisecond)
			}
		}(remoteAddr)
	}
	wg.Wait()

	buf := make([]byte, 1500)
	n, from, err := conn.ReadFromUDP(buf)
	if err == nil && n > 0 && from.IP.String() == remoteIP {
		result.Success = true
		result.RemotePort = from.Port
	}
	result.Attempts = burstSize
	return result, nil
}
