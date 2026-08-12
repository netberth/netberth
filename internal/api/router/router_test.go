// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/netberth/netberth/internal/api/websocket"
	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/db"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/internal/service"
)

type fakeCRUD struct{ calls []string }

func (f *fakeCRUD) record(method string) {
	f.calls = append(f.calls, method)
}

func (f *fakeCRUD) List(w http.ResponseWriter, r *http.Request) { f.record("GET"); w.WriteHeader(200) }
func (f *fakeCRUD) Create(w http.ResponseWriter, r *http.Request) {
	f.record("POST")
	w.WriteHeader(201)
}
func (f *fakeCRUD) Update(w http.ResponseWriter, r *http.Request) {
	f.record("PUT")
	w.WriteHeader(200)
}
func (f *fakeCRUD) Delete(w http.ResponseWriter, r *http.Request) {
	f.record("DELETE")
	w.WriteHeader(200)
}

func TestRegisterCRUD(t *testing.T) {
	r := chi.NewRouter()
	f := &fakeCRUD{}
	registerCRUD(r, "/things", f)

	reqs := []struct {
		method, target string
	}{
		{"GET", "/things"},
		{"POST", "/things"},
		{"PUT", "/things/abc"},
		{"DELETE", "/things/abc"},
	}
	for _, q := range reqs {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(q.method, q.target, nil))
	}
	want := []string{"GET", "POST", "PUT", "DELETE"}
	if len(f.calls) != len(want) {
		t.Fatalf("expected %d calls, got %d: %v", len(want), len(f.calls), f.calls)
	}
	for i, m := range want {
		if f.calls[i] != m {
			t.Fatalf("call %d: expected %s, got %s", i, m, f.calls[i])
		}
	}
}

func TestBusNotifier(t *testing.T) {
	bus := service.NewBus()
	got := make(chan service.Event, 3)
	bus.Subscribe("forward:created", func(e service.Event) { got <- e })
	bus.Subscribe("forward:updated", func(e service.Event) { got <- e })
	bus.Subscribe("forward:deleted", func(e service.Event) { got <- e })

	n := busNotifier(bus, "forward")
	n.OnCreate("forward", "id-1")
	n.OnUpdate("forward", "id-2")
	n.OnDelete("forward", "id-3")

	wantTypes := []service.EventType{"forward:created", "forward:updated", "forward:deleted"}
	wantIDs := []string{"id-1", "id-2", "id-3"}
	for i := 0; i < 3; i++ {
		select {
		case e := <-got:
			if e.Type != wantTypes[i] || e.ID != wantIDs[i] {
				t.Fatalf("event %d: got %+v", i, e)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for bus event")
		}
	}
}

func newTestRouter(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "netberth.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	authSvc := auth.NewService("router-test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, err := authSvc.HashPassword("testpass123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.SeedAdminUser(database, hash); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var userID, tenantID string
	if err := database.QueryRow("SELECT id, tenant_id FROM users WHERE username='admin'").Scan(&userID, &tenantID); err != nil {
		t.Fatalf("query admin: %v", err)
	}
	database.Exec("UPDATE users SET password_changed=1 WHERE id=?", userID)

	wire := service.NewWire(database, t.TempDir())
	hub := websocket.NewHub(wire.Forward, database)
	r := New(database, authSvc, wire, hub)

	adminTokens, err := authSvc.GenerateTokens(&model.User{ID: userID, Username: "admin", Role: "admin"})
	if err != nil {
		t.Fatalf("generate admin tokens: %v", err)
	}

	opHash, err := authSvc.HashPassword("operatorpass123")
	if err != nil {
		t.Fatalf("hash operator password: %v", err)
	}
	if _, err := database.Exec(
		"INSERT INTO users (id, tenant_id, username, email, password_hash, role, enabled, password_changed) VALUES (?,?,?,?,?,?,1,1)",
		"op-user", tenantID, "operator", "op@example.com", opHash, "operator",
	); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	opTokens, err := authSvc.GenerateTokens(&model.User{ID: "op-user", Username: "operator", Role: "operator"})
	if err != nil {
		t.Fatalf("generate operator tokens: %v", err)
	}
	return r, adminTokens.AccessToken, opTokens.AccessToken
}

func TestRouterIntegration(t *testing.T) {
	r, _, opToken := newTestRouter(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	get := func(path string) (*http.Response, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}
	do := func(method, path, body, authHeader string) (*http.Response, string) {
		var rdr io.Reader
		if body != "" {
			rdr = bytes.NewReader([]byte(body))
		}
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	// Public endpoints.
	if resp, _ := get("/api/v1/system/status"); resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if resp, _ := get("/api/v1/docs"); resp.StatusCode != http.StatusOK {
		t.Fatalf("docs: %d", resp.StatusCode)
	}
	if resp, _ := get("/"); resp.StatusCode != http.StatusOK {
		t.Fatalf("ui root: %d", resp.StatusCode)
	}

	// Login with seeded admin.
	loginResp, loginBody := do("POST", "/api/v1/auth/login", `{"username":"admin","password":"testpass123"}`, "")
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d %s", loginResp.StatusCode, loginBody)
	}
	var login struct {
		Data struct {
			Tokens auth.TokenPair `json:"tokens"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(loginBody), &login); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	if login.Data.Tokens.AccessToken == "" {
		t.Fatal("login returned no access token")
	}
	bearer := "Bearer " + login.Data.Tokens.AccessToken

	// Unauthenticated protected route.
	if resp, _ := get("/api/v1/auth/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}

	// Authenticated endpoints.
	if resp, _ := do("GET", "/api/v1/auth/me", "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("me: %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "/api/v1/license/status", "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("license status: %d", resp.StatusCode)
	}
	if resp, _ := do("GET", "/api/v1/system/dashboard", "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: %d", resp.StatusCode)
	}

	// CRUD routes are wired for every module.
	for _, p := range []string{"forward-rules", "proxy-rules", "ddns", "stun", "wol", "cron", "acme", "storage"} {
		if resp, _ := do("GET", "/api/v1/"+p, "", bearer); resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /%s: %d", p, resp.StatusCode)
		}
	}

	// Create + update + delete a forward rule through the full stack.
	fwdBody := `{"name":"router-fwd","protocol":"tcp","listen_port":23001,"target_addr":"192.0.2.50","target_port":80,"enabled":true}`
	createResp, createBody := do("POST", "/api/v1/forward-rules", fwdBody, bearer)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create forward: %d %s", createResp.StatusCode, createBody)
	}
	var created struct {
		Data model.ForwardRule `json:"data"`
	}
	if err := json.Unmarshal([]byte(createBody), &created); err != nil {
		t.Fatalf("create decode: %v", err)
	}
	if created.Data.ID == "" {
		t.Fatal("created rule has no id")
	}
	if resp, _ := do("PUT", "/api/v1/forward-rules/"+created.Data.ID, fwdBody, bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("update forward: %d", resp.StatusCode)
	}
	if resp, _ := do("DELETE", "/api/v1/forward-rules/"+created.Data.ID, "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("delete forward: %d", resp.StatusCode)
	}

	// 2FA setup route.
	if resp, _ := do("GET", "/api/v1/auth/2fa/setup", "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("2fa setup: %d", resp.StatusCode)
	}

	// WebSocket endpoint returns upgrade failure without WS headers (still routed).
	if resp, _ := get("/api/v1/ws"); resp.StatusCode == http.StatusNotFound {
		t.Fatalf("ws route not found")
	}

	// User management: admin only.
	if resp, body := do("GET", "/api/v1/users", "", bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin list users: %d %s", resp.StatusCode, body)
	}
	if resp, _ := do("GET", "/api/v1/users", "", "Bearer "+opToken); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("operator list users should be 403, got %d", resp.StatusCode)
	}
	userBody := `{"username":"newbie","email":"newbie@example.com","role":"viewer","password":"newbiepass123"}`
	if resp, _ := do("POST", "/api/v1/users", userBody, "Bearer "+opToken); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("operator create user should be 403, got %d", resp.StatusCode)
	}
	if resp, body := do("POST", "/api/v1/users", userBody, bearer); resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create user: %d %s", resp.StatusCode, body)
	}
	loginNew, loginNewBody := do("POST", "/api/v1/auth/login", `{"username":"newbie","password":"newbiepass123"}`, "")
	if loginNew.StatusCode != http.StatusOK {
		t.Fatalf("new user login: %d %s", loginNew.StatusCode, loginNewBody)
	}

	// Audit dashboard: admin only, and the earlier create is recorded.
	if resp, _ := do("GET", "/api/v1/audit", "", "Bearer "+opToken); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("operator audit should be 403, got %d", resp.StatusCode)
	}
	auditOK := false
	auditDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(auditDeadline) {
		auditResp, auditBody := do("GET", "/api/v1/audit?page=1&page_size=50", "", bearer)
		if auditResp.StatusCode != http.StatusOK {
			t.Fatalf("admin audit: %d %s", auditResp.StatusCode, auditBody)
		}
		var audit struct {
			Data []struct {
				ResourceType string `json:"resource_type"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(auditBody), &audit); err == nil {
			for _, e := range audit.Data {
				if e.ResourceType == "forward_rule" {
					auditOK = true
					break
				}
			}
		}
		if auditOK {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !auditOK {
		t.Fatal("audit log did not record the created forward rule")
	}

	// Metrics endpoint is public and machine-readable.
	if resp, body := get("/api/v1/system/metrics"); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"db_driver"`) {
		t.Fatalf("metrics: %d %s", resp.StatusCode, body)
	}

	// Logout revokes the refresh token.
	if resp, body := do("POST", "/api/v1/auth/logout", `{"refresh_token":"`+login.Data.Tokens.RefreshToken+`"}`, bearer); resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: %d %s", resp.StatusCode, body)
	}
	ref, refBody := do("POST", "/api/v1/auth/refresh", `{"refresh_token":"`+login.Data.Tokens.RefreshToken+`"}`, "")
	if ref.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after logout should be 401, got %d %s", ref.StatusCode, refBody)
	}

	// SPA fallback.
	if resp, body := get("/some/spa/route"); resp.StatusCode != http.StatusOK || strings.Contains(body, "Not Found") {
		t.Fatalf("spa fallback: %d", resp.StatusCode)
	}
}
