// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

func sqlOpen(dsn string) (*sql.DB, error) { return sql.Open("sqlite3", dsn) }


func TestLoginHandlerValidCredentials(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	// Seed test user
	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("testpass123")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role, password_changed) VALUES (?,?,?,?,?,?)",
		"user-1", "", "testuser", hash, "admin", 1)

	h.auth = authSvc

	body, _ := json.Marshal(loginRequest{Username: "testuser", Password: "testpass123"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != true {
		t.Fatal("expected success=true")
	}
}

func TestLoginHandlerInvalidCredentials(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	body, _ := json.Marshal(loginRequest{Username: "nobody", Password: "wrong"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestChangePasswordHandler(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("oldpass123")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role, password_changed) VALUES (?,?,?,?,?,?)",
		"user-2", "", "pwuser", hash, "admin", 0)
	h.auth = authSvc

	// Set claims in context
	claims := &auth.Claims{UserID: "user-2", Username: "pwuser", Role: "admin"}
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password",
		bytes.NewReader([]byte(`{"old_password":"oldpass123","new_password":"NewPass123!"}`)))
	req.Header.Set("Content-Type", "application/json")
	ctx := req.Context()
	ctx = contextWithClaims(ctx, claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.ChangePassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify password_changed was set to 1
	var changed int
	db.QueryRow("SELECT password_changed FROM users WHERE id='user-2'").Scan(&changed)
	if changed != 1 {
		t.Error("password_changed should be 1 after password change")
	}
}

func TestMeHandler(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	hash, _ := auth.NewService("x", 1, 1).HashPassword("x")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role) VALUES (?,?,?,?,?)",
		"user-3", "", "meuser", hash, "admin")

	h.auth = auth.NewService("test", 15*time.Minute, 7*24*time.Hour)
	claims := &auth.Claims{UserID: "user-3", Username: "meuser", Role: "admin"}
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	ctx := contextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.Me(w, req)

	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["success"] != true { t.Fatal("expected success") }
}

func TestSystemStatusHandler(t *testing.T) {
	_, db := setupAuthHandler(t)
	defer db.Close()

	h := NewSystemHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/system/status", nil)
	w := httptest.NewRecorder()
	h.Status(w, req)

	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	v := resp["data"].(map[string]interface{})["version"]
	if v != "1.0.0-rc1" {
		t.Errorf("expected version 1.0.0-rc1, got %v", v)
	}
}

func TestForwardCRUD(t *testing.T) {
	_, db := setupAuthHandler(t)
	defer db.Close()

	h := NewForwardHandler(db)
	// Create
	rule := model.ForwardRule{Name: "test", Protocol: "tcp", ListenAddr: "", ListenPort: 21000,
		TargetAddr: "192.0.2.1", TargetPort: 80, EnableIPv6: true, Enabled: true}
	body, _ := json.Marshal(rule)
	req := httptest.NewRequest("POST", "/api/v1/forward-rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusCreated { t.Fatalf("create: %d", w.Code) }

	// List
	req2 := httptest.NewRequest("GET", "/api/v1/forward-rules", nil)
	w2 := httptest.NewRecorder()
	h.List(w2, req2)
	if w2.Code != http.StatusOK { t.Fatalf("list: %d", w2.Code) }
}

// Helpers

func setupAuthHandler(t *testing.T) (*AuthHandler, *sql.DB) {
	t.Helper()
	db, err := sqlOpen(":memory:")
	if err != nil { t.Fatalf("open db: %v", err) }
	runTestMigrations(db)
	h := NewAuthHandler(db, auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour))
	return h, db
}

func runTestMigrations(db *sql.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, tenant_id TEXT DEFAULT '', username TEXT UNIQUE, email TEXT DEFAULT '', password_hash TEXT, role TEXT DEFAULT 'admin', otp_enabled INTEGER DEFAULT 0, otp_secret TEXT DEFAULT '', password_changed INTEGER DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS forward_rules (id TEXT PRIMARY KEY, tenant_id TEXT DEFAULT '', owner_id TEXT DEFAULT '', name TEXT, protocol TEXT DEFAULT 'tcp', listen_addr TEXT DEFAULT '', listen_port INTEGER, target_addr TEXT, target_port INTEGER, enable_ipv6 INTEGER DEFAULT 1, max_conns INTEGER DEFAULT 0, enabled INTEGER DEFAULT 0, schedule_on TEXT DEFAULT '', schedule_off TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS forward_whitelist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS forward_blacklist (id TEXT PRIMARY KEY, rule_id TEXT, value TEXT)`)
}

func contextWithClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, auth.ClaimsKey, claims)
}
