// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/netberth/netberth/pkg/utils"
)

type BackupHandler struct{ db *sql.DB }

func NewBackupHandler(db *sql.DB) *BackupHandler { return &BackupHandler{db: db} }

func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	// Lock DB briefly for consistent backup
	if _, err := h.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		utils.Error(w, http.StatusInternalServerError, "checkpoint failed")
		return
	}

	// Find DB path
	var pageCount int
	h.db.QueryRow("PRAGMA page_count").Scan(&pageCount)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=netberth-backup-"+time.Now().Format("20060102-150405")+".db")

	// Read and stream the DB file
	dbPath := "./data/netberth.db"
	if p := os.Getenv("NB_DB_PATH"); p != "" {
		dbPath = p
	}
	f, err := os.Open(dbPath)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "cannot open database")
		return
	}
	defer f.Close()
	io.Copy(w, f)
}

func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100 MB max
	defer r.Body.Close()

	dbPath := "./data/netberth.db"
	if p := os.Getenv("NB_DB_PATH"); p != "" {
		dbPath = p
	}

	// Write to temp file first
	tmp := dbPath + ".restore"
	f, err := os.Create(tmp)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "cannot create temp file")
		return
	}
	if _, err := io.Copy(f, r.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		utils.Error(w, http.StatusBadRequest, "upload failed")
		return
	}
	f.Close()

	// Validate the uploaded file is a valid SQLite database
	validateDB, err := sql.Open("sqlite3", tmp+"?_journal_mode=WAL")
	if err != nil {
		os.Remove(tmp)
		utils.Error(w, http.StatusBadRequest, "invalid database file")
		return
	}
	if err := validateDB.Ping(); err != nil {
		validateDB.Close()
		os.Remove(tmp)
		utils.Error(w, http.StatusBadRequest, "corrupt database file")
		return
	}
	validateDB.Close()

	// Backup current, replace with uploaded
	backup := dbPath + ".bak"
	os.Rename(dbPath, backup)
	if err := os.Rename(tmp, dbPath); err != nil {
		os.Rename(backup, dbPath)
		os.Remove(tmp)
		utils.Error(w, http.StatusInternalServerError, "restore failed")
		return
	}
	os.Remove(backup)

	utils.Message(w, "database restored. restart the service to apply changes.")
}
