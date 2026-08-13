// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package websocket

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/netberth/netberth/internal/engine/forward"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/version"
)

type mockForwardDB struct{}

func (mockForwardDB) GetRules() ([]model.ForwardRule, error) { return nil, nil }

func openHubDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:hubtest?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS forward_rules (id TEXT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func (h *Hub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func TestNewHubAndBuildStatus(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)

	msg := hub.buildStatus()
	if msg.Type != "status" {
		t.Fatalf("expected type status, got %q", msg.Type)
	}
	raw, ok := msg.Payload.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage payload, got %T", msg.Payload)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	sys, ok := payload["system"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing system payload: %+v", payload)
	}
	if sys["version"] != version.Version {
		t.Fatalf("unexpected version: %v", sys["version"])
	}
	if _, ok := payload["forward"]; !ok {
		t.Fatalf("missing forward payload: %+v", payload)
	}
}

func TestHandleWSConnectDisconnect(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for hub.clientCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 1 {
		t.Fatalf("expected 1 registered client, got %d", hub.clientCount())
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.Close()

	deadline = time.Now().Add(2 * time.Second)
	for hub.clientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 0 {
		t.Fatalf("client not cleaned up after disconnect, got %d", hub.clientCount())
	}
}

func TestHandleWSUpgradeFailure(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)

	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	hub.HandleWS(w, req) // no upgrade headers → must not panic
	if w.Code != http.StatusBadRequest && w.Code != 0 {
		t.Fatalf("unexpected status %d", w.Code)
	}
	if hub.clientCount() != 0 {
		t.Fatalf("expected no clients after failed upgrade, got %d", hub.clientCount())
	}
}

func TestBuildStatusWithForwardNames(t *testing.T) {
	db := openHubDB(t)
	for _, row := range [][2]string{{"f1", "Alpha"}, {"f2", "Beta"}} {
		if _, err := db.Exec("INSERT INTO forward_rules (id, name) VALUES (?, ?)", row[0], row[1]); err != nil {
			t.Fatalf("insert %s: %v", row[0], err)
		}
	}

	hub := NewHub(forward.New(mockForwardDB{}), db)
	hub.statusFn = func() []model.ForwardRuleStatus {
		return []model.ForwardRuleStatus{
			{ID: "f1", Active: true, Connections: 3, BytesIn: 10, BytesOut: 20},
			{ID: "f3", Active: true},
		}
	}

	msg := hub.buildStatus()
	raw, ok := msg.Payload.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage payload, got %T", msg.Payload)
	}
	var payload struct {
		System  map[string]interface{} `json:"system"`
		Forward []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Active      bool   `json:"active"`
			Connections int64  `json:"connections"`
			BytesIn     uint64 `json:"bytes_in"`
			BytesOut    uint64 `json:"bytes_out"`
		} `json:"forward"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Forward) != 2 {
		t.Fatalf("expected 2 forward entries, got %d", len(payload.Forward))
	}
	if payload.Forward[0].Name != "Alpha" {
		t.Fatalf("expected enriched name Alpha, got %q", payload.Forward[0].Name)
	}
	if payload.Forward[0].Connections != 3 || payload.Forward[0].BytesIn != 10 || payload.Forward[0].BytesOut != 20 {
		t.Fatalf("expected counters to pass through, got %+v", payload.Forward[0])
	}
	if payload.Forward[1].Name != "" {
		t.Fatalf("expected empty name for unknown id, got %q", payload.Forward[1].Name)
	}
	if payload.System["version"] != version.Version {
		t.Fatalf("unexpected version in payload: %v", payload.System["version"])
	}
}

func TestBroadcastDeliversMessage(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)
	hub.broadcastInterval = 50 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for hub.clientCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 1 {
		t.Fatalf("expected 1 registered client, got %d", hub.clientCount())
	}

	go hub.Broadcast()
	defer close(hub.stopCh)

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read broadcast message: %v", err)
	}
	var msg StatusMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	if msg.Type != "status" {
		t.Fatalf("expected type status, got %q", msg.Type)
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object payload, got %T", msg.Payload)
	}
	if _, ok := payload["system"]; !ok {
		t.Fatalf("missing system field in broadcast payload: %+v", payload)
	}
	if _, ok := payload["forward"]; !ok {
		t.Fatalf("missing forward field in broadcast payload: %+v", payload)
	}
}

func TestConcurrentConnectDisconnect(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	const n = 5
	conns := make([]*websocket.Conn, 0, n)
	for i := 0; i < n; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	deadline := time.Now().Add(3 * time.Second)
	for hub.clientCount() != n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != n {
		t.Fatalf("expected %d registered clients, got %d", n, hub.clientCount())
	}

	var wg sync.WaitGroup
	for _, conn := range conns {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			c.Close()
		}(conn)
	}
	wg.Wait()

	deadline = time.Now().Add(3 * time.Second)
	for hub.clientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 0 {
		t.Fatalf("clients not cleaned up after concurrent disconnect, got %d", hub.clientCount())
	}
}

func TestReadLoopAbnormalClose(t *testing.T) {
	db := openHubDB(t)
	hub := NewHub(forward.New(mockForwardDB{}), db)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for hub.clientCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 1 {
		t.Fatalf("expected 1 registered client, got %d", hub.clientCount())
	}

	tcp, ok := conn.UnderlyingConn().(*net.TCPConn)
	if !ok {
		conn.Close()
		t.Fatal("expected TCP underlying connection")
	}
	tcp.SetLinger(0)
	tcp.Close()

	deadline = time.Now().Add(2 * time.Second)
	for hub.clientCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.clientCount() != 0 {
		conn.Close()
		t.Fatalf("client not cleaned up after abnormal close, got %d", hub.clientCount())
	}
}
