// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/pkg/logger"
)

func AuditMiddleware(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			method := r.Method
			if method == "GET" || method == "HEAD" || method == "OPTIONS" {
				return
			}

			resourceType, resourceID, action := extractAuditInfo(r.URL.Path, method)
			if resourceType == "" {
				return
			}

			tenantID := ""
			userID := ""
			username := ""
			if claims, ok := r.Context().Value(auth.ClaimsKey).(*auth.Claims); ok {
				userID = claims.UserID
				username = claims.Username
			}

			go func() {
				_, err := db.Exec(
					`INSERT INTO audit_events (tenant_id, user_id, username, action, resource_type, resource_id, changes, remote_addr, created_at)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					tenantID, userID, username, action, resourceType, resourceID, "", r.RemoteAddr, time.Now(),
				)
				if err != nil {
					logger.Log.Warn().Err(err).Msg("audit log write failed")
				}
			}()
		})
	}
}

func extractAuditInfo(path, method string) (resourceType, resourceID, action string) {
	if !strings.HasPrefix(path, "/api/v1/") {
		return "", "", ""
	}
	path = strings.TrimPrefix(path, "/api/v1/")

	switch method {
	case "POST":
		action = "created"
	case "PUT", "PATCH":
		action = "updated"
	case "DELETE":
		action = "deleted"
	default:
		return "", "", ""
	}

	// /forward-rules              → resourceType=forward_rule, action=created
	// /forward-rules/{id}         → resourceType=forward_rule, action=updated/deleted
	// /proxy-rules/{id}           → resourceType=proxy_rule
	// /ddns/{id}                  → resourceType=ddns_config
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "", "", ""
	}

	typeMap := map[string]string{
		"forward-rules": "forward_rule",
		"proxy-rules":   "proxy_rule",
		"ddns":          "ddns_config",
		"stun":          "stun_tunnel",
		"wol":           "wol_device",
		"cron":          "cron_job",
		"acme":          "acme_certificate",
		"storage":       "storage_mount",
	}

	if t, ok := typeMap[parts[0]]; ok {
		resourceType = t
	}
	if len(parts) >= 2 {
		resourceID = parts[1]
	}
	if resourceType == "" || action == "" {
		return "", "", ""
	}
	return
}
