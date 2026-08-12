// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/utils"
	"github.com/netberth/netberth/pkg/validator"
)

type UserHandler struct {
	db   *sql.DB
	auth *auth.Service
}

func NewUserHandler(db *sql.DB, authService *auth.Service) *UserHandler {
	return &UserHandler{db: db, auth: authService}
}

type createUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type updateUserRequest struct {
	Email   string `json:"email"`
	Role    string `json:"role"`
	Enabled *bool  `json:"enabled"`
}

type resetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func validRole(role string) bool {
	return role == "admin" || role == "operator" || role == "viewer"
}

// List returns all users without exposing password hashes or OTP secrets.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, username, email, role, enabled, otp_enabled, created_at FROM users ORDER BY created_at`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		var enabled int
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &enabled, &u.OTPEnabled, &u.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "scan failed")
			return
		}
		u.Enabled = enabled == 1
		users = append(users, u)
	}
	utils.Success(w, users)
}

// Create adds a new user with a forced password change on first login.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxAuthBodyBytes)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			utils.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		utils.Error(w, http.StatusBadRequest, "username required")
		return
	}
	if !validRole(req.Role) {
		utils.Error(w, http.StatusBadRequest, "role must be admin, operator or viewer")
		return
	}
	if len(req.Password) < 8 {
		utils.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > maxPasswordBytes {
		utils.Error(w, http.StatusBadRequest, "password too long")
		return
	}
	if len(req.Username) > maxUsernameBytes {
		utils.Error(w, http.StatusBadRequest, "username too long")
		return
	}
	if !validator.Email(req.Email) {
		utils.Error(w, http.StatusBadRequest, "invalid email")
		return
	}

	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "hash failed")
		return
	}

	// New users belong to the creating admin's tenant.
	var tenantID string
	if claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims); ok {
		if err := h.db.QueryRow("SELECT tenant_id FROM users WHERE id=?", claims.UserID).Scan(&tenantID); err != nil {
			utils.Error(w, http.StatusInternalServerError, "tenant lookup failed")
			return
		}
	}

	id := generateUUID()
	_, err = h.db.Exec(
		`INSERT INTO users (id, tenant_id, username, email, password_hash, role, enabled, password_changed)
		 VALUES (?, ?, ?, ?, ?, ?, 1, 0)`,
		id, tenantID, req.Username, req.Email, hash, req.Role,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			utils.Error(w, http.StatusConflict, "username already exists")
			return
		}
		utils.Error(w, http.StatusInternalServerError, "create failed")
		return
	}

	u := model.User{ID: id, Username: req.Username, Email: req.Email, Role: req.Role, Enabled: true}
	utils.Created(w, u)
}

// Update changes email, role and enabled state with last-admin protection.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limitBody(w, r, maxAuthBodyBytes)
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			utils.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role != "" && !validRole(req.Role) {
		utils.Error(w, http.StatusBadRequest, "role must be admin, operator or viewer")
		return
	}
	if !validator.Email(req.Email) {
		utils.Error(w, http.StatusBadRequest, "invalid email")
		return
	}

	var role string
	var enabled int
	var currentEmail string
	if err := h.db.QueryRow("SELECT role, enabled, email FROM users WHERE id=?", id).Scan(&role, &enabled, &currentEmail); err != nil {
		utils.Error(w, http.StatusNotFound, "user not found")
		return
	}

	newRole := role
	if req.Role != "" {
		newRole = req.Role
	}
	newEmail := currentEmail
	if req.Email != "" {
		newEmail = req.Email
	}
	newEnabled := enabled == 1
	if req.Enabled != nil {
		newEnabled = *req.Enabled
	}

	if claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims); ok && claims.UserID == id && !newEnabled {
		utils.Error(w, http.StatusBadRequest, "cannot disable your own account")
		return
	}
	if (newRole != "admin" || !newEnabled) && role == "admin" && enabled == 1 && isLastEnabledAdmin(h.db, id) {
		utils.Error(w, http.StatusBadRequest, "cannot demote or disable the last admin")
		return
	}

	enabledVal := 0
	if newEnabled {
		enabledVal = 1
	}
	if _, err := h.db.Exec(
		"UPDATE users SET email=?, role=?, enabled=? WHERE id=?",
		newEmail, newRole, enabledVal, id,
	); err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	utils.Message(w, "updated")
}

// Delete removes a user, blocking self-deletion and removal of the last admin.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims); ok && claims.UserID == id {
		utils.Error(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var role string
	var enabled int
	if err := h.db.QueryRow("SELECT role, enabled FROM users WHERE id=?", id).Scan(&role, &enabled); err != nil {
		utils.Error(w, http.StatusNotFound, "user not found")
		return
	}
	if role == "admin" && enabled == 1 && isLastEnabledAdmin(h.db, id) {
		utils.Error(w, http.StatusBadRequest, "cannot delete the last admin")
		return
	}

	if _, err := h.db.Exec("DELETE FROM users WHERE id=?", id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	utils.Message(w, "deleted")
}

// ResetPassword sets a new password and forces a change on next login.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limitBody(w, r, maxAuthBodyBytes)
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			utils.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		utils.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > maxPasswordBytes {
		utils.Error(w, http.StatusBadRequest, "password too long")
		return
	}

	var exists int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id=?", id).Scan(&exists); err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if exists == 0 {
		utils.Error(w, http.StatusNotFound, "user not found")
		return
	}

	hash, err := h.auth.HashPassword(req.NewPassword)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "hash failed")
		return
	}
	if _, err := h.db.Exec("UPDATE users SET password_hash=?, password_changed=0 WHERE id=?", hash, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	utils.Message(w, "password reset")
}

func isLastEnabledAdmin(db *sql.DB, id string) bool {
	var total int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin' AND enabled=1").Scan(&total)
	return total <= 1
}
