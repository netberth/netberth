// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"net/http"
	"testing"
	"time"
)

func seedAuditRows(t *testing.T, db *sql.DB) {
	t.Helper()
	base := time.Now()
	for i := 1; i <= 5; i++ {
		action := "created"
		resource := "forward_rule"
		if i%2 == 0 {
			action = "updated"
			resource = "user"
		}
		if _, err := db.Exec(
			`INSERT INTO audit_events (tenant_id, user_id, username, action, resource_type, resource_id, changes, remote_addr, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"tenant-1", "u1", "admin", action, resource, "res-"+string(rune('0'+i)), "{}", "127.0.0.1:1234",
			base.Add(time.Duration(i)*time.Second).Format("2006-01-02 15:04:05"),
		); err != nil {
			t.Fatalf("seed audit %d: %v", i, err)
		}
	}
}

func TestAuditListPaginationAndFilters(t *testing.T) {
	db := setupFullTestDB(t)
	seedAuditRows(t, db)
	h := NewAuditHandler(db)

	// Default page.
	w := doJSON(t, h.List, http.MethodGet, "/api/v1/audit", nil)
	expectStatus(t, w, http.StatusOK)
	var resp struct {
		Data  []map[string]interface{} `json:"data"`
		Total int                      `json:"total"`
		Page  int                      `json:"page"`
	}
	decodeResponse(t, w, &resp)
	if resp.Total != 5 || len(resp.Data) != 5 {
		t.Fatalf("expected 5 events, got total=%d len=%d", resp.Total, len(resp.Data))
	}

	// Page size + action filter.
	w2 := doJSON(t, h.List, http.MethodGet, "/api/v1/audit?page=1&page_size=2&action=created", nil)
	expectStatus(t, w2, http.StatusOK)
	decodeResponse(t, w2, &resp)
	if resp.Total != 3 || len(resp.Data) != 2 {
		t.Fatalf("expected total=3 page=2 rows, got total=%d len=%d", resp.Total, len(resp.Data))
	}

	// Resource filter + username filter.
	w3 := doJSON(t, h.List, http.MethodGet, "/api/v1/audit?resource_type=user&username=admin", nil)
	expectStatus(t, w3, http.StatusOK)
	decodeResponse(t, w3, &resp)
	if resp.Total != 2 {
		t.Fatalf("expected 2 user events, got %d", resp.Total)
	}

	// Empty filter result.
	w4 := doJSON(t, h.List, http.MethodGet, "/api/v1/audit?action=deleted", nil)
	expectStatus(t, w4, http.StatusOK)
	decodeResponse(t, w4, &resp)
	if resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("expected empty result, got total=%d len=%d", resp.Total, len(resp.Data))
	}

	// Out-of-range page returns empty data with correct total.
	w5 := doJSON(t, h.List, http.MethodGet, "/api/v1/audit?page=99&page_size=100", nil)
	expectStatus(t, w5, http.StatusOK)
	decodeResponse(t, w5, &resp)
	if resp.Total != 5 || len(resp.Data) != 0 {
		t.Fatalf("expected empty page 99, got total=%d len=%d", resp.Total, len(resp.Data))
	}
}

func TestAuditListScanError(t *testing.T) {
	db := setupFullTestDB(t)
	db.Exec("DROP TABLE audit_events")
	db.Exec(`CREATE TABLE audit_events (id INTEGER PRIMARY KEY, created_at TEXT)`)
	db.Exec(`INSERT INTO audit_events (id, created_at) VALUES (1, 'x')`)

	h := NewAuditHandler(db)
	w := doJSON(t, h.List, http.MethodGet, "/api/v1/audit", nil)
	expectStatus(t, w, http.StatusInternalServerError)
}
