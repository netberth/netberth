// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package proxy

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/netberth/netberth/internal/model"
)

func TestProxyWebSocketLongLived(t *testing.T) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
	defer upstream.Close()

	e := New(&mockProxyDB{rules: []model.ProxyRule{{
		ID: "p1", Name: "p1", Domains: []string{"127.0.0.1"},
		TargetURL: upstream.URL, Websocket: true, Enabled: true,
	}}})
	if err := e.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("proxy start: %v", err)
	}
	defer e.Stop()

	port := e.listener.Addr().(*net.TCPAddr).Port
	conn, _, err := websocket.DefaultDialer.Dial(
		fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("ws dial through proxy: %v", err)
	}
	defer conn.Close()

	payload := bytes.Repeat([]byte("x"), 1024)
	const msgs = 300
	for i := 0; i < msgs; i++ {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, got, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("echo mismatch at message %d", i)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Keep the connection alive longer with pings to exercise the
	// long-lived path, then confirm it is still usable.
	for i := 0; i < 5; i++ {
		if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			t.Fatalf("ping %d: %v", i, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("last")); err != nil {
		t.Fatalf("final write: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil || string(got) != "last" {
		t.Fatalf("final echo: %v %q", err, got)
	}
}
