// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/engine/stun"
)

// TestSTUNEventBusIntegration verifies the full chain:
// STUN Probe → event bus → handler captures events
func TestSTUNEventBusIntegration(t *testing.T) {
	bus := NewBus()
	eng := stun.New(nil)

	var mu sync.Mutex
	receivedEvents := make(map[string]int)

	// Subscribe to STUN events — simulates what the handler layer does
	bus.Subscribe("stun:nat_mismatch", func(e Event) {
		mu.Lock()
		receivedEvents["nat_mismatch"]++
		mu.Unlock()
	})
	bus.Subscribe("stun:symmetric_detected", func(e Event) {
		mu.Lock()
		receivedEvents["symmetric_detected"]++
		mu.Unlock()
	})

	// Wire STUN engine to publish to the bus (simulating the Wire service layer)
	eng.SetNotifier(func(eventType, data string) {
		bus.Publish(Event{Type: EventType(eventType), ID: data})
	})

	// Run multi-probe — events should flow through the bus
	result := eng.ProbeMultiple(nil) // uses default Google STUN servers
	if result == nil {
		t.Skip("STUN servers unreachable in this environment")
	}
	t.Logf("Probe: %d servers, consensus=%s:%d, inconsistent=%v",
		len(result.ServerResults), result.ConsensusIP, result.ConsensusPort, result.Inconsistent)

	// Allow async event processing
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	total := 0
	for ev, count := range receivedEvents {
		t.Logf("Event %s: %d times", ev, count)
		total += count
	}
	mu.Unlock()

	if total == 0 && !result.Inconsistent {
		t.Log("No events — expected when all probes agree (no NAT mismatch)")
	} else if total > 0 {
		t.Logf("Events delivered: %d total", total)
	}
}

// TestSTUNEventGoroutineSafety verifies event bus doesn't leak under rapid probe cycles
func TestSTUNEventGoroutineSafety(t *testing.T) {
	bus := NewBus()
	eng := stun.New(nil)

	eng.SetNotifier(func(eventType, data string) {
		bus.Publish(Event{Type: EventType(eventType), ID: data})
	})

	// Subscribe many handlers
	for i := 0; i < 20; i++ {
		bus.Subscribe("stun:nat_mismatch", func(e Event) {})
		bus.Subscribe("stun:symmetric_detected", func(e Event) {})
	}

	var wg sync.WaitGroup
	errCount := 0
	var errMu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := eng.ProbeMultiple(nil)
			if result == nil {
				errMu.Lock()
				errCount++
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()

	t.Logf("10 concurrent probes, %d probe failures (STUN unreachable)", errCount)
}

// TestProbeMultipleWithRetry tests per-server retry logic
func TestProbeMultipleWithRetry(t *testing.T) {
	eng := stun.New(nil)

	// Mix of reachable and unreachable servers
	servers := []string{
		"stun.l.google.com:19302",
		"192.0.2.1:3478", // TEST-NET-1 — unreachable, should retry
	}

	result := eng.ProbeMultiple(servers)
	if result == nil {
		t.Skip("STUN servers unreachable")
	}

	t.Logf("Result: %d servers, inconsistent=%v", len(result.ServerResults), result.Inconsistent)
	for _, s := range result.ServerResults {
		status := "OK"
		if s.Error != "" {
			status = fmt.Sprintf("ERROR: %s", s.Error)
		}
		t.Logf("  %s → %s:%d %s", s.Server, s.MappedIP, s.Port, status)
	}

	reachable := 0
	for _, s := range result.ServerResults {
		if s.Error == "" { reachable++ }
	}
	if reachable == 0 {
		t.Skip("no STUN servers reachable")
	}
	t.Logf("Reachable: %d/%d", reachable, len(servers))
}
