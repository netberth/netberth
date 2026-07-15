// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/netberth/netberth/internal/auth"
)

// ForcePasswordChange blocks all API requests except login and password change
// if the user is still using the initial auto-generated password.
func ForcePasswordChange(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			path := r.URL.Path
			// Always allow login, password change, and status endpoints
			if strings.HasPrefix(path, "/api/v1/auth/login") ||
				strings.HasPrefix(path, "/api/v1/auth/change-password") ||
				strings.HasPrefix(path, "/api/v1/auth/2fa/") ||
				strings.HasPrefix(path, "/api/v1/auth/me") ||
				strings.HasPrefix(path, "/api/v1/system/status") {
				next.ServeHTTP(w, r)
				return
			}

			// Check if password was changed
			var changed int
			db.QueryRow("SELECT COUNT(*) FROM users WHERE id=? AND password_changed=1", claims.UserID).Scan(&changed)
			if changed == 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error":"password change required","redirect":"/settings"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
