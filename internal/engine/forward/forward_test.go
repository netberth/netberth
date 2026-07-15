// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package forward

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

type mockForwardDB struct {
	rules []model.ForwardRule
}
func osPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatalf("os port: %v", err) }
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func (m *mockForwardDB) GetRules() ([]model.ForwardRule, error) { return m.rules, nil }

func TestEngineStartStop(t *testing.T) {
	eng := New(&mockForwardDB{})
	if err := eng.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	eng.Stop()
}

func TestReloadAddRemove(t *testing.T) {
	eng := New(&mockForwardDB{})
	_ = eng.Start()
	defer eng.Stop()

	rule := model.ForwardRule{
		ID: "test-1", Name: "test", Protocol: "tcp",
		ListenAddr: "", ListenPort: osPort(t),
		TargetAddr: "127.0.0.1", TargetPort: 21001,
		EnableIPv6: false, Enabled: true,
	}

	// Add
	eng.Reload(rule)
	time.Sleep(100 * time.Millisecond)
	eng.mu.RLock()
	_, exists := eng.rules["test-1"]
	eng.mu.RUnlock()
	if !exists { t.Fatal("rule should exist after Reload") }

	// Disable via Reload
	rule.Enabled = false
	eng.Reload(rule)
	time.Sleep(100 * time.Millisecond)
	eng.mu.RLock()
	_, exists = eng.rules["test-1"]
	eng.mu.RUnlock()
	if exists { t.Fatal("rule should be removed after disable") }

	// Remove
	eng.Remove("test-1")
}

func TestTCPForwardAccept(t *testing.T) {
	// Start a local echo server
	echoLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echoLn.Close()
	echoPort := echoLn.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := echoLn.Accept()
			if err != nil { return }
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				c.Write(buf[:n])
				c.Close()
			}(conn)
		}
	}()

	eng := New(&mockForwardDB{})
	_ = eng.Start()
	defer eng.Stop()

	listenPort := osPort(t)
	rule := model.ForwardRule{
		ID: "tcp-test", Name: "tcp", Protocol: "tcp",
		ListenAddr: "127.0.0.1", ListenPort: listenPort,
		TargetAddr: "127.0.0.1", TargetPort: echoPort,
		EnableIPv6: false, Enabled: true,
	}
	eng.Reload(rule)
	time.Sleep(200 * time.Millisecond)

	// Connect and send data
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort), 2*time.Second)
	if err != nil { t.Fatalf("dial: %v", err) }
	defer conn.Close()

	msg := []byte("hello-forward")
	conn.Write(msg)
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil { t.Fatalf("read: %v", err) }
	if string(buf[:n]) != string(msg) {
		t.Errorf("expected %q, got %q", msg, buf[:n])
	}

	time.Sleep(100 * time.Millisecond)
	status := eng.Status()
	for _, s := range status {
		if s.ID == "tcp-test" { t.Logf("status: active=%v bytes=%d/%d", s.Active, s.BytesIn, s.BytesOut) }
	}
}

func TestMaxConns(t *testing.T) {
	if testing.Short() { t.Skip("skipping timing-sensitive concurrency test in short mode") }
	echoLn, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echoLn.Close()
	echoPort := echoLn.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := echoLn.Accept()
			if err != nil { return }
			go func(c net.Conn) {
				time.Sleep(5 * time.Second) // hold connection long enough
				c.Close()
			}(conn)
		}
	}()

	eng := New(&mockForwardDB{})
	_ = eng.Start()
	defer eng.Stop()

	listenPort := osPort(t)
	rule := model.ForwardRule{
		ID: "maxconn-test", Name: "maxconn", Protocol: "tcp",
		ListenAddr: "127.0.0.1", ListenPort: listenPort,
		TargetAddr: "127.0.0.1", TargetPort: echoPort,
		EnableIPv6: false, Enabled: true, MaxConns: 2,
	}
	eng.Reload(rule)
	time.Sleep(200 * time.Millisecond)

	// Open 3 connections, 3rd should be rejected
	var wg sync.WaitGroup
	rejected := 0
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", listenPort), 1*time.Second)
			if err != nil { rejected++; return }
			defer conn.Close()
			conn.Read(make([]byte, 1))
		}()
	}
	wg.Wait()

	if rejected < 1 {
		t.Errorf("expected at least 1 rejection with max_conns=2, got %d rejected", rejected)
	}
}

func TestGoroutineLeakOnReload(t *testing.T) {
	eng := New(&mockForwardDB{})
	_ = eng.Start()
	defer eng.Stop()

	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		rule := model.ForwardRule{
			ID: "leak-test", Name: "leak", Protocol: "tcp",
			ListenAddr: "127.0.0.1", ListenPort: osPort(t),
			TargetAddr: "127.0.0.1", TargetPort: 1,
			Enabled: true,
		}
		eng.Reload(rule)
		rule.Enabled = false
		eng.Reload(rule)
	}

	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	t.Logf("goroutines before=%d after=%d", before, after)
	if after > before+10 {
		t.Errorf("possible goroutine leak: %d → %d", before, after)
	}
}
