// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
	"github.com/netberth/netberth/pkg/utils"
)

type AuthHandler struct {
	db   *sql.DB
	auth *auth.Service
}

func NewAuthHandler(db *sql.DB, authService *auth.Service) *AuthHandler {
	return &AuthHandler{db: db, auth: authService}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		utils.Error(w, http.StatusBadRequest, "username and password required")
		return
	}
	var user model.User
	err := h.db.QueryRow(
		"SELECT id, username, email, password_hash, role, otp_enabled, otp_secret FROM users WHERE username = ?",
		req.Username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.OTPEnabled, &user.OTPSecret)
	if err == sql.ErrNoRows {
		time.Sleep(200 * time.Millisecond) // mitigate timing attack
		utils.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		logger.Log.Error().Err(err).Msg("login query failed")
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	valid, err := h.auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !valid {
		utils.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	tokens, err := h.auth.GenerateTokens(&user)
	if err != nil {
		logger.Log.Error().Err(err).Msg("generate tokens failed")
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	utils.Success(w, map[string]interface{}{
		"tokens": tokens,
		"user":   h.sanitizeUser(&user),
	})
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	claims, err := h.auth.ValidateToken(body.RefreshToken)
	if err != nil {
		utils.Error(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	var user model.User
	err = h.db.QueryRow(
		"SELECT id, username, email, role, otp_enabled, otp_secret FROM users WHERE id = ?",
		claims.UserID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.OTPEnabled, &user.OTPSecret)
	if err != nil {
		utils.Error(w, http.StatusUnauthorized, "user not found")
		return
	}
	tokens, err := h.auth.GenerateTokens(&user)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	utils.Success(w, tokens)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
	if !ok {
		utils.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var user model.User
	err := h.db.QueryRow(
		"SELECT id, username, email, role, otp_enabled, otp_secret FROM users WHERE id = ?",
		claims.UserID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Role, &user.OTPEnabled, &user.OTPSecret)
	if err != nil {
		utils.Error(w, http.StatusNotFound, "user not found")
		return
	}
	utils.Success(w, h.sanitizeUser(&user))
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
	if !ok {
		utils.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		utils.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	var currentHash string
	err := h.db.QueryRow("SELECT password_hash FROM users WHERE id = ?", claims.UserID).Scan(&currentHash)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	valid, _ := h.auth.VerifyPassword(req.OldPassword, currentHash)
	if !valid {
		utils.Error(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	newHash, err := h.auth.HashPassword(req.NewPassword)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	_, err = h.db.Exec("UPDATE users SET password_hash = ?, password_changed = 1, updated_at = ? WHERE id = ?",
		newHash, time.Now(), claims.UserID)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	utils.Message(w, "password changed successfully")
}

func (h *AuthHandler) sanitizeUser(u *model.User) map[string]interface{} {
	return map[string]interface{}{
		"id":          u.ID,
		"username":    u.Username,
		"email":       u.Email,
		"role":        u.Role,
		"otp_enabled": u.OTPEnabled,
	}
}

type otpSetupResponse struct {
	Secret string `json:"secret"`
	QRCode string `json:"qr_code"` // otpauth:// URL
}

func (h *AuthHandler) Setup2FA(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
	if !ok { utils.Error(w, http.StatusUnauthorized, "unauthorized"); return }

	secret, err := h.auth.GenerateOTPSecret()
	if err != nil { utils.Error(w, http.StatusInternalServerError, "generate failed"); return }

	var username string
	h.db.QueryRow("SELECT username FROM users WHERE id=?", claims.UserID).Scan(&username)
	qrURL := fmt.Sprintf("otpauth://totp/NetBerth:%s?secret=%s&issuer=NetBerth", username, secret)

	// Store secret temporarily
	h.db.Exec("UPDATE users SET otp_secret=? WHERE id=?", secret, claims.UserID)

	utils.Success(w, otpSetupResponse{Secret: secret, QRCode: qrURL})
}

func (h *AuthHandler) Enable2FA(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
	if !ok { utils.Error(w, http.StatusUnauthorized, "unauthorized"); return }

	var body struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { utils.Error(w, http.StatusBadRequest, "invalid body"); return }

	var secret string
	h.db.QueryRow("SELECT otp_secret FROM users WHERE id=?", claims.UserID).Scan(&secret)
	if secret == "" { utils.Error(w, http.StatusBadRequest, "no OTP setup found — call /auth/2fa/setup first"); return }

	if !h.auth.ValidateTOTP(secret, body.Code) {
		utils.Error(w, http.StatusBadRequest, "invalid OTP code")
		return
	}

	h.db.Exec("UPDATE users SET otp_enabled=1 WHERE id=?", claims.UserID)
	utils.Message(w, "2FA enabled")
}

func (h *AuthHandler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
	if !ok { utils.Error(w, http.StatusUnauthorized, "unauthorized"); return }

	h.db.Exec("UPDATE users SET otp_enabled=0, otp_secret='' WHERE id=?", claims.UserID)
	utils.Message(w, "2FA disabled")
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
