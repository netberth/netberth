// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/netberth/netberth/internal/licensing"
	"github.com/netberth/netberth/pkg/utils"
)

type LicenseHandler struct {
	db      *sql.DB
	checker licensing.Checker
}

// keyProvider reads the stored license key from settings table.
func keyProvider(db *sql.DB) func() string {
	return func() string {
		var v string
		db.QueryRow("SELECT value FROM settings WHERE key='license_key'").Scan(&v)
		return v
	}
}

func NewLicenseHandler(db *sql.DB) *LicenseHandler {
	return &LicenseHandler{db: db, checker: licensing.NewChecker(keyProvider(db))}
}

func (h *LicenseHandler) Status(w http.ResponseWriter, r *http.Request) {
	tid := extractTenant(r)
	valid, _ := h.checker.IsValid(tid)
	tier := h.checker.Tier(tid)
	maxRules := h.checker.MaxRules(tier)
	utils.Success(w, map[string]interface{}{
		"tier": tier, "max_rules": maxRules, "valid": valid,
	})
}

func (h *LicenseHandler) Activate(w http.ResponseWriter, r *http.Request) {
	var body struct{ Key string `json:"license_key"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('license_key', ?)", body.Key)
	utils.Success(w, map[string]interface{}{
		"tier": "community",
		"note": "license activation requires enterprise build",
	})
}

func extractTenant(r *http.Request) string {
	if v := r.Context().Value(contextKey("tenant_id")); v != nil {
		if s, ok := v.(string); ok { return s }
	}
	return "system_default"
}

type contextKey string
