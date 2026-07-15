// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBusPublish(t *testing.T) {
	bus := NewBus()
	var called int32

	bus.Subscribe("test:created", func(e Event) {
		atomic.AddInt32(&called, 1)
	})
	bus.Subscribe("test:created", func(e Event) {
		atomic.AddInt32(&called, 1)
	})

	bus.Publish(Event{Type: "test:created", ID: "id-1"})
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&called) != 2 {
		t.Errorf("expected 2 handlers called, got %d", called)
	}
}

func TestEventBusGoroutineLeak(t *testing.T) {
	bus := NewBus()

	// Subscribe many handlers
	for i := 0; i < 100; i++ {
		bus.Subscribe(EventType("evt:"+string(rune('a'+i%26))), func(e Event) {})
	}

	start := runtime.NumGoroutine()

	// Rapid publish — should not leak goroutines
	for i := 0; i < 1000; i++ {
		bus.Publish(Event{Type: "evt:a", ID: "x"})
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("EventBus goroutines: before=%d after=%d", start, after)
	if after > start+20 {
		t.Errorf("possible goroutine leak: %d → %d", start, after)
	}
}

func TestEventBusConcurrentPublish(t *testing.T) {
	bus := NewBus()
	var count int32

	for i := 0; i < 10; i++ {
		bus.Subscribe(EventType("concurrent"), func(e Event) {
			atomic.AddInt32(&count, 1)
		})
	}

	var wg sync.WaitGroup
	start := runtime.NumGoroutine()

	for j := 0; j < 100; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{Type: "concurrent", ID: "c"})
		}()
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("Concurrent EventBus goroutines: before=%d after=%d events=%d",
		start, after, atomic.LoadInt32(&count))

	if after > start+30 {
		t.Errorf("goroutine leak on concurrent publish: %d → %d", start, after)
	}
}
