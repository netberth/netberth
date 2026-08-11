// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package websocket

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
