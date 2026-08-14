// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package forward

import (
	"io"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

type fakeForwardDB struct{ rules []model.ForwardRule }

func (f *fakeForwardDB) GetRules() ([]model.ForwardRule, error) { return f.rules, nil }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitDial(t *testing.T, addr string, attempts int) net.Conn {
	t.Helper()
	var lastErr error
	for i := 0; i < attempts; i++ {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			return conn
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("dial %s failed: %v", addr, lastErr)
	return nil
}

func startTCPEcho(t *testing.T) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				io.Copy(conn, conn)
				conn.Close()
			}(c)
		}
	}()
	return l.Addr().String(), func() { l.Close() }
}

func startUDPEcho(t *testing.T) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().String(), func() { pc.Close() }
}

func tcpRule(id string, listenPort int, target string, maxConns int) model.ForwardRule {
	host, port, _ := net.SplitHostPort(target)
	return model.ForwardRule{
		ID:         id,
		Name:       id,
		Protocol:   "tcp",
		ListenAddr: "0.0.0.0",
		ListenPort: listenPort,
		TargetAddr: host,
		TargetPort: mustAtoiPort(port),
		MaxConns:   maxConns,
		Enabled:    true,
	}
}

func mustAtoiPort(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func waitConnCount(t *testing.T, e *Engine, id string, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range e.Status() {
			if s.ID == id && s.Connections == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("conn count for %s did not reach %d (status=%+v)", id, want, e.Status())
}

func waitGoroutines(t *testing.T, baseline int, tolerance int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if numGoroutines() <= baseline+tolerance {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: baseline=%d now=%d", baseline, numGoroutines())
}

func numGoroutines() int {
	return runtime.NumGoroutine()
}

func TestForwardTCPDataIntegrity(t *testing.T) {
	target, stopTarget := startTCPEcho(t)
	defer stopTarget()

	listenPort := freePort(t)
	e := New(&fakeForwardDB{})
	e.startRule(tcpRule("r1", listenPort, target, 0))
	defer e.Remove("r1")

	conn := waitDial(t, net.JoinHostPort("127.0.0.1", itoa(listenPort)), 80)
	defer conn.Close()

	payload := make([]byte, 4<<20)
	rand.New(rand.NewSource(42)).Read(payload)

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite() // half-close so the forward sees EOF on the client side
	}
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("length mismatch: got %d want %d", len(got), len(payload))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("byte mismatch at %d", i)
		}
	}
	conn.Close()
	waitConnCount(t, e, "r1", 0)

	for _, s := range e.Status() {
		if s.ID == "r1" {
			if s.BytesIn != uint64(len(payload)) || s.BytesOut != uint64(len(payload)) {
				t.Fatalf("byte counters wrong: in=%d out=%d want=%d", s.BytesIn, s.BytesOut, len(payload))
			}
		}
	}
}

func TestForwardMaxConns(t *testing.T) {
	target, stopTarget := startTCPEcho(t)
	defer stopTarget()

	listenPort := freePort(t)
	e := New(&fakeForwardDB{})
	e.startRule(tcpRule("r1", listenPort, target, 2))
	defer e.Remove("r1")

	addr := net.JoinHostPort("127.0.0.1", itoa(listenPort))
	c1 := waitDial(t, addr, 80)
	c2 := waitDial(t, addr, 80)
	defer c1.Close()
	defer c2.Close()
	waitConnCount(t, e, "r1", 2)

	// Third connection must be closed immediately by the limit.
	c3 := waitDial(t, addr, 80)
	c3.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 1)
	n, rerr := c3.Read(buf)
	if rerr == nil || n > 0 {
		t.Fatalf("expected over-limit connection closed, got n=%d err=%v", n, rerr)
	}
	if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
		t.Fatal("over-limit connection stayed open (read timed out)")
	}
	c3.Close()

	c1.Write([]byte("ping1"))
	buf = make([]byte, 5)
	if _, err := io.ReadFull(c1, buf); err != nil || string(buf) != "ping1" {
		t.Fatalf("c1 echo failed: %v %q", err, buf)
	}
	c1.Close()
	c2.Close()
	waitConnCount(t, e, "r1", 0)
}

func TestForwardTargetDisconnectCleanup(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target listen: %v", err)
	}
	defer l.Close()

	go func() {
		// First connection: read some data, then abort.
		c, err := l.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64<<10)
		io.ReadFull(c, buf)
		if tc, ok := c.(*net.TCPConn); ok {
			tc.SetLinger(0)
		}
		c.Close()

		// Second connection: normal echo, proving the rule survives.
		c2, err := l.Accept()
		if err != nil {
			return
		}
		io.Copy(c2, c2)
		c2.Close()
	}()

	listenPort := freePort(t)
	e := New(&fakeForwardDB{})
	e.startRule(tcpRule("r1", listenPort, l.Addr().String(), 0))
	defer e.Remove("r1")
	addr := net.JoinHostPort("127.0.0.1", itoa(listenPort))

	baseline := numGoroutines()
	conn := waitDial(t, addr, 80)
	payload := make([]byte, 1<<20)
	rand.New(rand.NewSource(7)).Read(payload)
	conn.Write(payload)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.Copy(io.Discard, conn); err == nil {
		// The target aborted; the forward side must surface an error/EOF.
		t.Log("read ended without error (EOF ok)")
	}
	conn.Close()
	waitConnCount(t, e, "r1", 0)
	waitGoroutines(t, baseline, 2)

	// The same rule must still accept and forward.
	conn2 := waitDial(t, addr, 80)
	defer conn2.Close()
	conn2.Write([]byte("alive"))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn2, buf); err != nil || string(buf) != "alive" {
		t.Fatalf("rule not alive after target disconnect: %v %q", err, buf)
	}
}

func TestForwardReloadAndRemove(t *testing.T) {
	port1 := freePort(t)
	target, stopTarget := startTCPEcho(t)
	defer stopTarget()

	e := New(&fakeForwardDB{rules: []model.ForwardRule{tcpRule("r1", port1, target, 0)}})
	if err := e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer e.Stop()

	addr1 := net.JoinHostPort("127.0.0.1", itoa(port1))
	c := waitDial(t, addr1, 80)
	c.Close()

	// Disable: listener must close.
	disabled := tcpRule("r1", port1, target, 0)
	disabled.Enabled = false
	e.Reload(disabled)
	if conn, err := net.DialTimeout("tcp", addr1, 300*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("expected dial to fail after disable")
	}

	// Re-enable on a new port.
	port2 := freePort(t)
	e.Reload(tcpRule("r1", port2, target, 0))
	addr2 := net.JoinHostPort("127.0.0.1", itoa(port2))
	c2 := waitDial(t, addr2, 80)
	c2.Close()

	// Remove: listener must close.
	e.Remove("r1")
	if conn, err := net.DialTimeout("tcp", addr2, 300*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("expected dial to fail after remove")
	}
}

func TestUDPForwardEchoAndConcurrency(t *testing.T) {
	target, stopTarget := startUDPEcho(t)
	defer stopTarget()

	host, port, _ := net.SplitHostPort(target)
	listenPort := freePort(t)
	e := New(&fakeForwardDB{})
	e.startRule(model.ForwardRule{
		ID: "u1", Name: "u1", Protocol: "udp",
		ListenAddr: "0.0.0.0", ListenPort: listenPort,
		TargetAddr: host, TargetPort: mustAtoiPort(port),
		Enabled: true,
	})
	defer e.Remove("u1")

	const clients = 8
	const msgs = 50
	var wg sync.WaitGroup
	errs := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pc, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				errs <- err
				return
			}
			defer pc.Close()
			raddr, _ := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", itoa(listenPort)))
			pc.SetReadDeadline(time.Now().Add(5 * time.Second))
			for m := 0; m < msgs; m++ {
				msg := []byte{byte(id), byte(m)}
				got := false
				for attempt := 0; attempt < 3 && !got; attempt++ {
					if _, err := pc.WriteTo(msg, raddr); err != nil {
						errs <- err
						return
					}
					pc.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
					buf := make([]byte, 8)
					n, _, err := pc.ReadFrom(buf)
					if err == nil && n == 2 && buf[0] == msg[0] && buf[1] == msg[1] {
						got = true
						break
					}
					if err == nil {
						errs <- errUnexpectedEcho{id: id, m: m, got: buf[:n]}
						return
					}
				}
				if !got {
					errs <- errNoEcho{id: id, m: m}
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("udp client error: %v", err)
	}
}

func TestUDPForwardSingleDatagram(t *testing.T) {
	target, stopTarget := startUDPEcho(t)
	defer stopTarget()
	t.Logf("echo target at %s", target)

	host, port, _ := net.SplitHostPort(target)
	listenPort := freePort(t)
	e := New(&fakeForwardDB{})
	e.startRule(model.ForwardRule{
		ID: "u1", Name: "u1", Protocol: "udp",
		ListenAddr: "0.0.0.0", ListenPort: listenPort,
		TargetAddr: host, TargetPort: mustAtoiPort(port),
		Enabled: true,
	})
	defer e.Remove("u1")

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	defer pc.Close()
	raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", itoa(listenPort)))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := pc.WriteTo([]byte("hello"), raddr); err != nil {
			t.Fatalf("write: %v", err)
		}
		pc.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		buf := make([]byte, 64)
		n, from, err := pc.ReadFrom(buf)
		if err == nil && string(buf[:n]) == "hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("read echo: %v (from=%v)", err, from)
		}
	}
}

type errUnexpectedEcho struct {
	id, m int
	got   []byte
}

func (e errUnexpectedEcho) Error() string {
	return "unexpected echo"
}

type errNoEcho struct {
	id, m int
}

func (e errNoEcho) Error() string {
	return "no echo received"
}

func TestUDPSessionAging(t *testing.T) {
	echo, stopEcho := startUDPEcho(t)
	defer stopEcho()
	target, _ := net.ResolveUDPAddr("udp", echo)

	m := newUDPSessionMapWithOptions(target, 200*time.Millisecond, 50*time.Millisecond)
	defer m.close()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer clientConn.Close()
	clientAddr := clientConn.LocalAddr()

	key := udpSessionKey{network: "udp", addr: clientAddr.String()}
	if _, err := m.getOrCreate(key, clientConn, clientAddr); err != nil {
		t.Fatalf("getOrCreate: %v", err)
	}
	m.mu.Lock()
	_, exists := m.sessions[key]
	m.mu.Unlock()
	if !exists {
		t.Fatal("session missing after create")
	}

	time.Sleep(700 * time.Millisecond) // well past ttl + clean interval
	m.mu.Lock()
	_, exists = m.sessions[key]
	m.mu.Unlock()
	if exists {
		t.Fatal("idle session was not aged out")
	}
}

func TestUDPSessionActiveSurvivesCleaner(t *testing.T) {
	echo, stopEcho := startUDPEcho(t)
	defer stopEcho()
	target, _ := net.ResolveUDPAddr("udp", echo)

	m := newUDPSessionMapWithOptions(target, 200*time.Millisecond, 50*time.Millisecond)
	defer m.close()

	clientConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer clientConn.Close()
	clientAddr := clientConn.LocalAddr()
	key := udpSessionKey{network: "udp", addr: clientAddr.String()}
	if _, err := m.getOrCreate(key, clientConn, clientAddr); err != nil {
		t.Fatalf("getOrCreate: %v", err)
	}

	// Keep the session active past several clean cycles.
	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		m.getOrCreate(key, clientConn, clientAddr)
		time.Sleep(80 * time.Millisecond)
	}
	m.mu.Lock()
	_, exists := m.sessions[key]
	m.mu.Unlock()
	if !exists {
		t.Fatal("active session was aged out")
	}
}

func TestUDPConcurrentSessionCreation(t *testing.T) {
	echo, stopEcho := startUDPEcho(t)
	defer stopEcho()
	target, _ := net.ResolveUDPAddr("udp", echo)

	m := newUDPSessionMap(target)
	defer m.close()

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pc, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Errorf("listen: %v", err)
				return
			}
			defer pc.Close()
			key := udpSessionKey{network: "udp", addr: pc.LocalAddr().String()}
			if _, err := m.getOrCreate(key, pc, pc.LocalAddr()); err != nil {
				t.Errorf("getOrCreate: %v", err)
			}
		}(i)
	}
	wg.Wait()

	m.mu.Lock()
	got := len(m.sessions)
	m.mu.Unlock()
	if got != n {
		t.Fatalf("expected %d sessions, got %d", n, got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
