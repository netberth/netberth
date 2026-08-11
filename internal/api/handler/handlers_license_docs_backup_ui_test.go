// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestLicenseHandlerStatusAndActivate(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewLicenseHandler(db)

	w := doJSON(t, h.Status, http.MethodGet, "/api/v1/license/status", nil)
	expectStatus(t, w, http.StatusOK)
	var resp struct {
		Data struct {
			Tier     string `json:"tier"`
			Valid    bool   `json:"valid"`
			MaxRules int    `json:"max_rules"`
		} `json:"data"`
	}
	decodeResponse(t, w, &resp)
	if resp.Data.Tier != "community" || !resp.Data.Valid || resp.Data.MaxRules != 5 {
		t.Fatalf("unexpected license status: %+v", resp.Data)
	}

	// Tenant context path.
	req := httptest.NewRequest("GET", "/api/v1/license/status", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextKey("tenant_id"), "tenant-1"))
	w2 := httptest.NewRecorder()
	h.Status(w2, req)
	expectStatus(t, w2, http.StatusOK)

	// Activate stores the key.
	body, _ := json.Marshal(map[string]string{"license_key": "k-123"})
	w3 := doJSON(t, h.Activate, http.MethodPost, "/api/v1/license/activate", body)
	expectStatus(t, w3, http.StatusOK)
	var stored string
	if err := db.QueryRow("SELECT value FROM settings WHERE key='license_key'").Scan(&stored); err != nil {
		t.Fatalf("query settings: %v", err)
	}
	if stored != "k-123" {
		t.Fatalf("expected k-123, got %s", stored)
	}

	w4 := doJSON(t, h.Activate, http.MethodPost, "/api/v1/license/activate", []byte("{"))
	expectStatus(t, w4, http.StatusBadRequest)
}

func TestDocsHandler(t *testing.T) {
	w := doJSON(t, DocsHandler(), http.MethodGet, "/api/v1/docs", nil)
	expectStatus(t, w, http.StatusOK)
	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	decodeResponse(t, w, &resp)
	if !resp.Success || resp.Data["title"] != "NetBerth Documentation" {
		t.Fatalf("docs response mismatch: %+v", resp)
	}
}

func TestSystemDashboard(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewSystemHandler(db)

	req := httptest.NewRequest("GET", "/api/v1/system/dashboard", nil)
	w := httptest.NewRecorder()
	h.Dashboard(w, req)
	expectStatus(t, w, http.StatusOK)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Modules map[string]int `json:"modules"`
		} `json:"data"`
	}
	decodeResponse(t, w, &resp)
	if !resp.Success || resp.Data.Modules == nil || len(resp.Data.Modules) == 0 {
		t.Fatalf("dashboard response mismatch: %+v", resp)
	}
}

func TestBackupDownloadAndRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open backup db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (name) VALUES ('x')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	t.Setenv("NB_DB_PATH", dbPath)
	h := NewBackupHandler(db)

	// Download streams the file.
	w := doJSON(t, h.Download, http.MethodGet, "/api/v1/system/backup", nil)
	expectStatus(t, w, http.StatusOK)
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty backup body")
	}

	// Restore with a valid SQLite file.
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	w2 := doJSON(t, h.Restore, http.MethodPost, "/api/v1/system/backup/restore", data)
	expectStatus(t, w2, http.StatusOK)

	// Restore with garbage is rejected.
	w3 := doJSON(t, h.Restore, http.MethodPost, "/api/v1/system/backup/restore", []byte("this is not a sqlite database"))
	expectStatus(t, w3, http.StatusBadRequest)
}

func TestUIHandlerServing(t *testing.T) {
	h := UIHandler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/anything", nil))
	expectStatus(t, w, http.StatusNotFound)

	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, httptest.NewRequest("GET", "/", nil))
	expectStatus(t, w2, http.StatusOK)

	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, httptest.NewRequest("GET", "/some/spa/route", nil))
	expectStatus(t, w3, http.StatusOK)
}
