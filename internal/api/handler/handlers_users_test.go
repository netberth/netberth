// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
)

func setupUsersHandler(t *testing.T) (*UserHandler, *sql.DB) {
	t.Helper()
	db := setupFullTestDB(t)
	authSvc := auth.NewService("users-test-secret", 15*time.Minute, 7*24*time.Hour)
	return NewUserHandler(db, authSvc), db
}

func insertTestUser(t *testing.T, db *sql.DB, id, username, role string, enabled int, hash string) {
	t.Helper()
	if hash == "" {
		hash = "test-hash"
	}
	_, err := db.Exec(
		`INSERT INTO users (id, tenant_id, username, email, password_hash, role, enabled, password_changed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		id, "", username, username+"@example.com", hash, role, enabled,
	)
	if err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
}

func TestUsersCreateAndList(t *testing.T) {
	h, db := setupUsersHandler(t)

	body, _ := json.Marshal(createUserRequest{Username: "alice", Email: "alice@example.com", Role: "operator", Password: "testpass123"})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/users", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.User `json:"data"`
	}
	decodeResponse(t, w, &created)
	if created.Data.ID == "" || created.Data.Username != "alice" || created.Data.Role != "operator" || !created.Data.Enabled {
		t.Fatalf("unexpected created user: %+v", created.Data)
	}

	var hash string
	var changed int
	db.QueryRow("SELECT password_hash, password_changed FROM users WHERE id=?", created.Data.ID).Scan(&hash, &changed)
	if hash == "" || strings.Contains(hash, "testpass123") {
		t.Fatalf("password not hashed: %q", hash)
	}
	if changed != 0 {
		t.Fatalf("expected forced password change, got %d", changed)
	}

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/users", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.User `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Data[0].Username != "alice" {
		t.Fatalf("unexpected list: %+v", listed.Data)
	}
	if strings.Contains(lw.Body.String(), "password_hash") || strings.Contains(lw.Body.String(), "otp_secret") {
		t.Fatal("list response leaked password_hash or otp_secret")
	}
}

func TestUsersCreateValidation(t *testing.T) {
	h, _ := setupUsersHandler(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty username", `{"username":"","email":"a@b.com","role":"admin","password":"testpass123"}`, http.StatusBadRequest},
		{"short password", `{"username":"u","email":"a@b.com","role":"admin","password":"short"}`, http.StatusBadRequest},
		{"bad role", `{"username":"u","email":"a@b.com","role":"root","password":"testpass123"}`, http.StatusBadRequest},
		{"bad email", `{"username":"u","email":"not-an-email","role":"admin","password":"testpass123"}`, http.StatusBadRequest},
		{"malformed json", `{`, http.StatusBadRequest},
	}
	for _, c := range cases {
		w := doJSON(t, h.Create, http.MethodPost, "/api/v1/users", []byte(c.body))
		expectStatus(t, w, c.want)
	}

	body, _ := json.Marshal(createUserRequest{Username: "dup", Email: "dup@example.com", Role: "admin", Password: "testpass123"})
	if w := doJSON(t, h.Create, http.MethodPost, "/api/v1/users", body); w.Code != http.StatusCreated {
		t.Fatalf("first create: %d", w.Code)
	}
	if w := doJSON(t, h.Create, http.MethodPost, "/api/v1/users", body); w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate username, got %d", w.Code)
	}
}

func TestUsersUpdateRoleAndDisable(t *testing.T) {
	h, db := setupUsersHandler(t)
	insertTestUser(t, db, "admin1", "root", "admin", 1, "")
	insertTestUser(t, db, "u1", "bob", "operator", 1, "")

	body, _ := json.Marshal(updateUserRequest{Role: "viewer", Email: "bob@example.net", Enabled: boolPtr(false)})
	w := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/users/u1", "u1", body)
	expectStatus(t, w, http.StatusOK)

	var role string
	var enabled, changed int
	db.QueryRow("SELECT role, enabled, password_changed FROM users WHERE id='u1'").Scan(&role, &enabled, &changed)
	if role != "viewer" || enabled != 0 {
		t.Fatalf("update not applied: role=%s enabled=%d", role, enabled)
	}

	bad, _ := json.Marshal(updateUserRequest{Role: "superuser"})
	if w := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/users/u1", "u1", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad role, got %d", w.Code)
	}
	missing, _ := json.Marshal(updateUserRequest{})
	if w := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/users/nope", "nope", missing); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing user, got %d", w.Code)
	}
}

func TestUsersLastAdminAndSelfGuards(t *testing.T) {
	h, db := setupUsersHandler(t)
	insertTestUser(t, db, "admin1", "root", "admin", 1, "")

	// Another admin acting on the only admin: demote/disable/delete must fail.
	claims := &auth.Claims{UserID: "other-admin", Username: "other", Role: "admin"}
	reqUpdate := httptest.NewRequest("PUT", "/api/v1/users/admin1", strings.NewReader(`{"role":"operator"}`))
	reqUpdate = reqUpdate.WithContext(contextWithClaims(reqUpdate.Context(), claims))
	reqUpdate.SetPathValue("id", "admin1")
	w := httptest.NewRecorder()
	h.Update(w, reqUpdate)
	expectStatus(t, w, http.StatusBadRequest)

	reqDisable := httptest.NewRequest("PUT", "/api/v1/users/admin1", strings.NewReader(`{"enabled":false}`))
	reqDisable = reqDisable.WithContext(contextWithClaims(reqDisable.Context(), claims))
	reqDisable.SetPathValue("id", "admin1")
	w2 := httptest.NewRecorder()
	h.Update(w2, reqDisable)
	expectStatus(t, w2, http.StatusBadRequest)

	reqDelete := httptest.NewRequest("DELETE", "/api/v1/users/admin1", nil)
	reqDelete = reqDelete.WithContext(contextWithClaims(reqDelete.Context(), claims))
	reqDelete.SetPathValue("id", "admin1")
	w3 := httptest.NewRecorder()
	h.Delete(w3, reqDelete)
	expectStatus(t, w3, http.StatusBadRequest)

	// Self-delete and self-disable are blocked even with a second admin present.
	insertTestUser(t, db, "admin2", "root2", "admin", 1, "")
	selfClaims := &auth.Claims{UserID: "admin1", Username: "root", Role: "admin"}
	reqSelfDel := httptest.NewRequest("DELETE", "/api/v1/users/admin1", nil)
	reqSelfDel = reqSelfDel.WithContext(contextWithClaims(reqSelfDel.Context(), selfClaims))
	reqSelfDel.SetPathValue("id", "admin1")
	w4 := httptest.NewRecorder()
	h.Delete(w4, reqSelfDel)
	expectStatus(t, w4, http.StatusBadRequest)

	reqSelfDis := httptest.NewRequest("PUT", "/api/v1/users/admin1", strings.NewReader(`{"enabled":false}`))
	reqSelfDis = reqSelfDis.WithContext(contextWithClaims(reqSelfDis.Context(), selfClaims))
	reqSelfDis.SetPathValue("id", "admin1")
	w5 := httptest.NewRecorder()
	h.Update(w5, reqSelfDis)
	expectStatus(t, w5, http.StatusBadRequest)

	// With a second admin, the first admin can be deleted by the other admin.
	reqDel2 := httptest.NewRequest("DELETE", "/api/v1/users/admin1", nil)
	reqDel2 = reqDel2.WithContext(contextWithClaims(reqDel2.Context(), claims))
	reqDel2.SetPathValue("id", "admin1")
	w6 := httptest.NewRecorder()
	h.Delete(w6, reqDel2)
	expectStatus(t, w6, http.StatusOK)
}

func TestUsersResetPassword(t *testing.T) {
	h, db := setupUsersHandler(t)
	insertTestUser(t, db, "u1", "bob", "operator", 1, "old-hash")

	w := doPathJSON(t, h.ResetPassword, http.MethodPost, "/api/v1/users/u1/reset-password", "u1",
		[]byte(`{"new_password":"NewPass123!"}`))
	expectStatus(t, w, http.StatusOK)

	var hash string
	var changed int
	db.QueryRow("SELECT password_hash, password_changed FROM users WHERE id='u1'").Scan(&hash, &changed)
	if hash == "old-hash" || hash == "" {
		t.Fatalf("password not updated: %q", hash)
	}
	if changed != 0 {
		t.Fatalf("expected forced password change after reset, got %d", changed)
	}

	if w := doPathJSON(t, h.ResetPassword, http.MethodPost, "/api/v1/users/u1/reset-password", "u1",
		[]byte(`{"new_password":"short"}`)); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", w.Code)
	}
	if w := doPathJSON(t, h.ResetPassword, http.MethodPost, "/api/v1/users/nope/reset-password", "nope",
		[]byte(`{"new_password":"NewPass123!"}`)); w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing user, got %d", w.Code)
	}
}

func TestLoginDisabledUser(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("testpass123")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role, enabled, password_changed) VALUES (?,?,?,?,?,?,?)",
		"user-disabled", "", "disabled", hash, "admin", 0, 1)

	h.auth = authSvc
	body, _ := json.Marshal(loginRequest{Username: "disabled", Password: "testpass123"})
	w := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", body)
	expectStatus(t, w, http.StatusUnauthorized)
}

func boolPtr(v bool) *bool { return &v }
