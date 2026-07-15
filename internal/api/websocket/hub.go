// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package websocket

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/netberth/netberth/internal/engine/forward"
	"github.com/netberth/netberth/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type StatusMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type SystemPayload struct {
	Uptime     int64  `json:"uptime"`
	CPUCount   int    `json:"cpu_count"`
	Goroutines int    `json:"goroutines"`
	MemoryMB   uint64 `json:"memory_mb"`
	Version    string `json:"version"`
}

type ForwardStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Connections int64  `json:"connections"`
	BytesIn     uint64 `json:"bytes_in"`
	BytesOut    uint64 `json:"bytes_out"`
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*websocket.Conn]bool
	forwardEng *forward.Engine
	db         *sql.DB
	startTime  time.Time
}

func NewHub(forwardEng *forward.Engine, db *sql.DB) *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		forwardEng: forwardEng,
		db:         db,
		startTime:  time.Now(),
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("websocket upgrade failed")
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	go h.readLoop(conn)
}

func (h *Hub) readLoop(conn *websocket.Conn) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *Hub) Broadcast() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		msg := h.buildStatus()
		h.mu.RLock()
		for conn := range h.clients {
			if err := conn.WriteJSON(msg); err != nil {
				logger.Log.Debug().Err(err).Msg("ws write error")
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) buildStatus() StatusMessage {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	sys := SystemPayload{
		Uptime:     int64(time.Since(h.startTime).Seconds()),
		CPUCount:   runtime.NumCPU(),
		Goroutines: runtime.NumGoroutine(),
		MemoryMB:   mem.Alloc / 1024 / 1024,
		Version:    "1.0.0-rc1",
	}

	stats := h.forwardEng.Status()
	forwardStatus := make([]ForwardStatus, len(stats))
	for i, s := range stats {
		forwardStatus[i] = ForwardStatus{
			ID:          s.ID,
			Active:      s.Active,
			Connections: s.Connections,
			BytesIn:     s.BytesIn,
			BytesOut:    s.BytesOut,
		}
	}
	// Enrich with names
	for i, fs := range forwardStatus {
		var name string
		h.db.QueryRow("SELECT name FROM forward_rules WHERE id=?", fs.ID).Scan(&name)
		forwardStatus[i].Name = name
	}

	payload := map[string]interface{}{
		"system":  sys,
		"forward": forwardStatus,
	}

	bytes, _ := json.Marshal(payload)
	return StatusMessage{Type: "status", Payload: json.RawMessage(bytes)}
}
