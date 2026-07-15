// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package forward

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netberth/netberth/internal/model"
	acl "github.com/netberth/netberth/pkg/acl"
	"github.com/netberth/netberth/pkg/logger"
)

const udpSessionTTL = 30 * time.Second
const udpCleanInterval = 60 * time.Second
const udpBufSize = 65535

type Engine struct {
	mu    sync.RWMutex
	rules map[string]*ruleInstance
	db    interface {
		GetRules() ([]model.ForwardRule, error)
	}
	stopCh chan struct{}
}

type ruleInstance struct {
	rule     model.ForwardRule
	matcher  *acl.Matcher
	cancel   context.CancelFunc
	conns    int64
	bytesIn  uint64
	bytesOut uint64
}

// === UDP Session Map ===

type udpSessionKey struct {
	network string
	addr    string
}

type udpSession struct {
	conn       *net.UDPConn
	lastUsed   atomic.Int64
	clientConn net.PacketConn
	clientAddr net.Addr
}

type udpSessionMap struct {
	mu       sync.Mutex
	sessions map[udpSessionKey]*udpSession
	done     chan struct{}
	target   *net.UDPAddr // shared target address for all sessions in this rule
}

func newUDPSessionMap(target *net.UDPAddr) *udpSessionMap {
	m := &udpSessionMap{
		sessions: make(map[udpSessionKey]*udpSession),
		done:     make(chan struct{}),
		target:   target,
	}
	go m.cleaner()
	return m
}

// getOrCreate returns an existing session or creates a new one.
// On creation, starts a long-lived reader goroutine that loops
// reading from backend→client until the session is cleaned up.
func (m *udpSessionMap) getOrCreate(key udpSessionKey, clientConn net.PacketConn, clientAddr net.Addr) (*net.UDPConn, error) {
	m.mu.Lock()
	if sess, ok := m.sessions[key]; ok {
		sess.lastUsed.Store(time.Now().Unix())
		m.mu.Unlock()
		return sess.conn, nil
	}

	conn, err := net.DialUDP(key.network, nil, m.target)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	sess := &udpSession{
		conn:       conn,
		clientConn: clientConn,
		clientAddr: clientAddr,
	}
	sess.lastUsed.Store(time.Now().Unix())
	m.sessions[key] = sess
	m.mu.Unlock()

	// Start long-lived reader goroutine bound to this session's lifetime
	go m.sessionReader(key, sess)

	return conn, nil
}

// sessionReader loops reading backend→client for the lifetime of this session.
// Exits when the connection errors, session TTL expires, or the map is closed.
func (m *udpSessionMap) sessionReader(key udpSessionKey, sess *udpSession) {
	buf := make([]byte, udpBufSize)
	for {
		select {
		case <-m.done:
			return
		default:
		}

		sess.conn.SetReadDeadline(time.Now().Add(udpSessionTTL))
		n, _, err := sess.conn.ReadFrom(buf)
		if err != nil {
			// Connection error or timeout — session is dead
			m.mu.Lock()
			if s, ok := m.sessions[key]; ok && s == sess {
				sess.conn.Close()
				delete(m.sessions, key)
			}
			m.mu.Unlock()
			return
		}
		if n > 0 {
			sess.clientConn.WriteTo(buf[:n], sess.clientAddr)
		}
	}
}

func (m *udpSessionMap) close() {
	close(m.done)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		s.conn.Close()
		delete(m.sessions, k)
	}
}

func (m *udpSessionMap) cleaner() {
	ticker := time.NewTicker(udpCleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			now := time.Now().Unix()
			m.mu.Lock()
			for k, s := range m.sessions {
				if now-s.lastUsed.Load() > int64(udpSessionTTL.Seconds()) {
					s.conn.Close()
					delete(m.sessions, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// === Engine ===

func New(db interface {
	GetRules() ([]model.ForwardRule, error)
}) *Engine {
	return &Engine{
		rules:  make(map[string]*ruleInstance),
		db:     db,
		stopCh: make(chan struct{}),
	}
}

func (e *Engine) Start() error {
	rules, err := e.db.GetRules()
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	for _, r := range rules {
		if r.Enabled {
			e.startRule(r)
		}
	}
	go e.watchdog()
	return nil
}

func (e *Engine) Stop() {
	close(e.stopCh)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, inst := range e.rules {
		inst.cancel()
	}
}

func (e *Engine) Reload(rule model.ForwardRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inst, exists := e.rules[rule.ID]; exists {
		inst.cancel()
	}
	if rule.Enabled {
		e.startRule(rule)
	} else {
		delete(e.rules, rule.ID)
	}
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inst, exists := e.rules[id]; exists {
		inst.cancel()
		delete(e.rules, id)
	}
}

func (e *Engine) Status() []model.ForwardRuleStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	statuses := make([]model.ForwardRuleStatus, 0, len(e.rules))
	for _, inst := range e.rules {
		statuses = append(statuses, model.ForwardRuleStatus{
			ID:          inst.rule.ID,
			Active:      true,
			Connections: atomic.LoadInt64(&inst.conns),
			BytesIn:     atomic.LoadUint64(&inst.bytesIn),
			BytesOut:    atomic.LoadUint64(&inst.bytesOut),
		})
	}
	return statuses
}

// === Rule lifecycle ===

func (e *Engine) startRule(rule model.ForwardRule) {
	ctx, cancel := context.WithCancel(context.Background())
	wl := make([]string, len(rule.Whitelist))
	for i, w := range rule.Whitelist {
		wl[i] = w.Value
	}
	bl := make([]string, len(rule.Blacklist))
	for i, b := range rule.Blacklist {
		bl[i] = b.Value
	}
	matcher := acl.NewMatcher()
	matcher.Load(wl, bl)

	inst := &ruleInstance{rule: rule, matcher: matcher, cancel: cancel}
	e.rules[rule.ID] = inst

	var sessionMap *udpSessionMap
	if rule.Protocol == "udp" || rule.Protocol == "both" {
		targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", rule.TargetAddr, rule.TargetPort))
		if err == nil {
			sessionMap = newUDPSessionMap(targetAddr)
		}
	}

	for _, network := range e.networks(rule.Protocol) {
		go e.listen(ctx, inst, network, sessionMap)
	}
}

func (e *Engine) networks(protocol string) []string {
	switch protocol {
	case "tcp":
		return []string{"tcp", "tcp6"}
	case "udp":
		return []string{"udp", "udp6"}
	case "both":
		return []string{"tcp", "tcp6", "udp", "udp6"}
	default:
		return []string{"tcp", "tcp6"}
	}
}

// === TCP Listener ===

func (e *Engine) listen(ctx context.Context, inst *ruleInstance, network string, sessionMap *udpSessionMap) {
	addr := fmt.Sprintf("%s:%d", inst.rule.ListenAddr, inst.rule.ListenPort)
	var lc net.ListenConfig

	isUDP := network == "udp" || network == "udp6"
	if isUDP && sessionMap != nil {
		defer sessionMap.close()
		packetConn, err := lc.ListenPacket(ctx, network, addr)
		if err != nil {
			logger.Log.Error().Err(err).Str("id", inst.rule.ID).Str("network", network).Msg("forward listen failed")
			return
		}
		defer packetConn.Close()
		e.handleUDP(ctx, inst, packetConn, sessionMap)
		return
	}

	listener, err := lc.Listen(ctx, network, addr)
	if err != nil {
		logger.Log.Error().Err(err).Str("id", inst.rule.ID).Str("network", network).Msg("forward listen failed")
		return
	}
	defer listener.Close()

	logger.Log.Info().Str("id", inst.rule.ID).Str("name", inst.rule.Name).
		Str("addr", addr).Str("proto", network).Msg("forward rule started")

	go func() { <-ctx.Done(); listener.Close() }()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Log.Warn().Err(err).Str("id", inst.rule.ID).Msg("accept error")
				continue
			}
		}

		tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			conn.Close()
			continue
		}

		if !inst.matcher.IsAllowed(tcpAddr.IP.String()) {
			conn.Close()
			continue
		}

		// Atomic pre-increment with rollback to avoid check-then-act race
		if inst.rule.MaxConns > 0 {
			if atomic.AddInt64(&inst.conns, 1) > int64(inst.rule.MaxConns) {
				atomic.AddInt64(&inst.conns, -1)
				conn.Close()
				continue
			}
		} else {
			atomic.AddInt64(&inst.conns, 1)
		}
		go e.handleTCP(ctx, inst, conn)
	}
}

func (e *Engine) handleTCP(ctx context.Context, inst *ruleInstance, client net.Conn) {
	defer atomic.AddInt64(&inst.conns, -1)
	defer client.Close()

	target := fmt.Sprintf("%s:%d", inst.rule.TargetAddr, inst.rule.TargetPort)
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	targetConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", target)
	if err != nil {
		logger.Log.Warn().Err(err).Str("id", inst.rule.ID).Str("target", target).Msg("forward connect failed")
		return
	}
	defer targetConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(targetConn, client)
		atomic.AddUint64(&inst.bytesIn, uint64(n))
		if tc, ok := targetConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(client, targetConn)
		atomic.AddUint64(&inst.bytesOut, uint64(n))
		if tc, ok := client.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	wg.Wait()
}

// === UDP Handler with Session Map ===

func (e *Engine) handleUDP(ctx context.Context, inst *ruleInstance, conn net.PacketConn, sessions *udpSessionMap) {
	buf := make([]byte, udpBufSize)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		// ACL check
		if udpAddr, ok := remoteAddr.(*net.UDPAddr); ok {
			if !inst.matcher.IsAllowed(udpAddr.IP.String()) {
				continue
			}
		}

		// getOrCreate starts a long-lived reader goroutine on first call
		key := udpSessionKey{network: "udp", addr: remoteAddr.String()}
		targetConn, err := sessions.getOrCreate(key, conn, remoteAddr)
		if err != nil {
			continue
		}

		targetConn.Write(buf[:n])
	}
}

// === Watchdog ===

func (e *Engine) watchdog() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			rules, err := e.db.GetRules()
			if err != nil {
				continue
			}
			e.mu.Lock()
			for _, r := range rules {
				if inst, exists := e.rules[r.ID]; exists {
					if !r.Enabled {
						inst.cancel()
						delete(e.rules, r.ID)
					}
				} else if r.Enabled {
					e.startRule(r)
				}
			}
			e.mu.Unlock()
		}
	}
}
