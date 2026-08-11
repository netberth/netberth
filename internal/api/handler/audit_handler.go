// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/utils"
)

type AuditHandler struct{ db *sql.DB }

func NewAuditHandler(db *sql.DB) *AuditHandler { return &AuditHandler{db: db} }

// List returns paginated audit events with optional filters.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	action := strings.TrimSpace(r.URL.Query().Get("action"))
	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	username := strings.TrimSpace(r.URL.Query().Get("username"))

	where := make([]string, 0, 3)
	args := make([]interface{}, 0, 5)
	if action != "" {
		where = append(where, "action = ?")
		args = append(args, action)
	}
	if resourceType != "" {
		where = append(where, "resource_type = ?")
		args = append(args, resourceType)
	}
	if username != "" {
		where = append(where, "username = ?")
		args = append(args, username)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM audit_events"+whereSQL, args...).Scan(&total); err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}

	query := `SELECT id, tenant_id, user_id, username, action, resource_type, resource_id, changes, remote_addr, created_at
	          FROM audit_events` + whereSQL + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := h.db.Query(query, queryArgs...)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	events := make([]model.AuditEvent, 0)
	for rows.Next() {
		var e model.AuditEvent
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.UserID, &e.Username, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.Changes, &e.RemoteAddr, &e.CreatedAt,
		); err != nil {
			utils.Error(w, http.StatusInternalServerError, "scan failed")
			return
		}
		events = append(events, e)
	}
	utils.Paginated(w, events, total, page, pageSize)
}
