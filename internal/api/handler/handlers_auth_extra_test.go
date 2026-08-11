// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
)

func TestLoginHandlerBadBody(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	w := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", []byte("{"))
	expectStatus(t, w, http.StatusBadRequest)

	w2 := doJSON(t, h.Login, http.MethodPost, "/api/v1/auth/login", []byte(`{}`))
	expectStatus(t, w2, http.StatusBadRequest)
}

func TestRefreshTokenHandler(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("testpass123")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role, password_changed) VALUES (?,?,?,?,?,?)",
		"user-1", "", "testuser", hash, "admin", 1)
	h.auth = authSvc

	tokens, err := authSvc.GenerateTokens(&model.User{ID: "user-1", Username: "testuser", Role: "admin"})
	if err != nil {
		t.Fatalf("generate tokens: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"refresh_token": tokens.RefreshToken})
	w := doJSON(t, h.RefreshToken, http.MethodPost, "/api/v1/auth/refresh", body)
	expectStatus(t, w, http.StatusOK)
	var resp struct {
		Data auth.TokenPair `json:"data"`
	}
	decodeResponse(t, w, &resp)
	if resp.Data.AccessToken == "" {
		t.Fatal("expected new access token")
	}

	w2 := doJSON(t, h.RefreshToken, http.MethodPost, "/api/v1/auth/refresh", []byte(`{"refresh_token":"not-a-token"}`))
	expectStatus(t, w2, http.StatusUnauthorized)

	w3 := doJSON(t, h.RefreshToken, http.MethodPost, "/api/v1/auth/refresh", []byte("{"))
	expectStatus(t, w3, http.StatusBadRequest)

	ghostTokens, _ := authSvc.GenerateTokens(&model.User{ID: "ghost", Username: "ghost", Role: "admin"})
	body4, _ := json.Marshal(map[string]string{"refresh_token": ghostTokens.RefreshToken})
	w4 := doJSON(t, h.RefreshToken, http.MethodPost, "/api/v1/auth/refresh", body4)
	expectStatus(t, w4, http.StatusUnauthorized)
}

func TestMeHandlerErrorPaths(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	w := doJSON(t, h.Me, http.MethodGet, "/api/v1/auth/me", nil)
	expectStatus(t, w, http.StatusUnauthorized)

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req = req.WithContext(contextWithClaims(req.Context(), &auth.Claims{UserID: "missing", Username: "x", Role: "admin"}))
	w2 := httptest.NewRecorder()
	h.Me(w2, req)
	expectStatus(t, w2, http.StatusNotFound)
}

func TestChangePasswordHandlerErrorPaths(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	w := doJSON(t, h.ChangePassword, http.MethodPost, "/api/v1/auth/change-password", nil)
	expectStatus(t, w, http.StatusUnauthorized)

	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("oldpass123")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role, password_changed) VALUES (?,?,?,?,?,?)",
		"user-2", "", "pwuser", hash, "admin", 0)
	h.auth = authSvc
	claims := &auth.Claims{UserID: "user-2", Username: "pwuser", Role: "admin"}

	postWithClaims := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewReader([]byte(body)))
		req = req.WithContext(contextWithClaims(req.Context(), claims))
		w := httptest.NewRecorder()
		h.ChangePassword(w, req)
		return w
	}

	expectStatus(t, postWithClaims("{"), http.StatusBadRequest)
	expectStatus(t, postWithClaims(`{"old_password":"oldpass123","new_password":"short"}`), http.StatusBadRequest)
	expectStatus(t, postWithClaims(`{"old_password":"wrong","new_password":"NewPass123!"}`), http.StatusBadRequest)

	missing := &auth.Claims{UserID: "ghost", Username: "g", Role: "admin"}
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewReader([]byte(`{"old_password":"x","new_password":"NewPass123!"}`)))
	req = req.WithContext(contextWithClaims(req.Context(), missing))
	wm := httptest.NewRecorder()
	h.ChangePassword(wm, req)
	expectStatus(t, wm, http.StatusInternalServerError)
}

func Test2FAFlow(t *testing.T) {
	h, db := setupAuthHandler(t)
	defer db.Close()

	authSvc := auth.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, _ := authSvc.HashPassword("x")
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role) VALUES (?,?,?,?,?)",
		"user-6", "", "otpuser", hash, "admin")
	h.auth = authSvc
	claims := &auth.Claims{UserID: "user-6", Username: "otpuser", Role: "admin"}

	// Unauthorized paths.
	w0 := doJSON(t, h.Setup2FA, http.MethodPost, "/api/v1/auth/2fa/setup", nil)
	expectStatus(t, w0, http.StatusUnauthorized)

	setupReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/setup", nil)
	setupReq = setupReq.WithContext(contextWithClaims(setupReq.Context(), claims))
	w := httptest.NewRecorder()
	h.Setup2FA(w, setupReq)
	expectStatus(t, w, http.StatusOK)
	var setupResp struct {
		Data otpSetupResponse `json:"data"`
	}
	decodeResponse(t, w, &setupResp)
	if setupResp.Data.Secret == "" || setupResp.Data.QRCode == "" {
		t.Fatal("expected secret and qr code")
	}
	var stored string
	db.QueryRow("SELECT otp_secret FROM users WHERE id='user-6'").Scan(&stored)
	if stored != setupResp.Data.Secret {
		t.Fatalf("otp_secret not stored: %s vs %s", stored, setupResp.Data.Secret)
	}

	// Bad JSON.
	badReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/enable", bytes.NewReader([]byte("{")))
	badReq = badReq.WithContext(contextWithClaims(badReq.Context(), claims))
	bw := httptest.NewRecorder()
	h.Enable2FA(bw, badReq)
	expectStatus(t, bw, http.StatusBadRequest)

	// Invalid code.
	invalidReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/enable", bytes.NewReader([]byte(`{"code":"000000"}`)))
	invalidReq = invalidReq.WithContext(contextWithClaims(invalidReq.Context(), claims))
	iw := httptest.NewRecorder()
	h.Enable2FA(iw, invalidReq)
	expectStatus(t, iw, http.StatusBadRequest)

	// Valid code.
	code := testTOTP(t, setupResp.Data.Secret)
	validReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/enable", bytes.NewReader([]byte(`{"code":"`+code+`"}`)))
	validReq = validReq.WithContext(contextWithClaims(validReq.Context(), claims))
	vw := httptest.NewRecorder()
	h.Enable2FA(vw, validReq)
	expectStatus(t, vw, http.StatusOK)
	var enabled int
	db.QueryRow("SELECT otp_enabled FROM users WHERE id='user-6'").Scan(&enabled)
	if enabled != 1 {
		t.Fatal("otp_enabled should be 1")
	}

	// Disable.
	disReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/disable", nil)
	disReq = disReq.WithContext(contextWithClaims(disReq.Context(), claims))
	dw := httptest.NewRecorder()
	h.Disable2FA(dw, disReq)
	expectStatus(t, dw, http.StatusOK)
	db.QueryRow("SELECT otp_enabled, otp_secret FROM users WHERE id='user-6'").Scan(&enabled, &stored)
	if enabled != 0 || stored != "" {
		t.Fatalf("2fa not disabled: enabled=%d secret=%q", enabled, stored)
	}

	// Enable without prior setup.
	db.Exec("INSERT INTO users (id, tenant_id, username, password_hash, role) VALUES (?,?,?,?,?)",
		"user-7", "", "otpuser2", hash, "admin")
	claims7 := &auth.Claims{UserID: "user-7", Username: "otpuser2", Role: "admin"}
	noSetupReq := httptest.NewRequest("POST", "/api/v1/auth/2fa/enable", bytes.NewReader([]byte(`{"code":"123456"}`)))
	noSetupReq = noSetupReq.WithContext(contextWithClaims(noSetupReq.Context(), claims7))
	nw := httptest.NewRecorder()
	h.Enable2FA(nw, noSetupReq)
	expectStatus(t, nw, http.StatusBadRequest)
}

// testTOTP replicates the RFC 6238 TOTP used by internal/auth so the test can
// produce a valid code without importing private helpers.
func testTOTP(t *testing.T, secret string) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		t.Fatalf("decode otp secret: %v", err)
	}
	counter := time.Now().Unix() / 30
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	b := int32(hash[offset]&0x7f)<<24 | int32(hash[offset+1])<<16 | int32(hash[offset+2])<<8 | int32(hash[offset+3])
	return fmt.Sprintf("%06d", int(b)%1000000)
}
