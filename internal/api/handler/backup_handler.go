// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/netberth/netberth/internal/backupcrypto"
	"github.com/netberth/netberth/pkg/logger"
	"github.com/netberth/netberth/pkg/utils"
)

const backupPasswordHeader = "X-NetBerth-Backup-Password"

type BackupHandler struct{ db *sql.DB }

func NewBackupHandler(db *sql.DB) *BackupHandler { return &BackupHandler{db: db} }

func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	pass := r.Header.Get(backupPasswordHeader)
	if pass != "" && (len(pass) < 8 || len(pass) > 128) {
		utils.Error(w, http.StatusBadRequest, "backup password must be 8-128 characters")
		return
	}

	// Lock DB briefly for consistent backup
	if _, err := h.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		utils.Error(w, http.StatusInternalServerError, "checkpoint failed")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	ext := ".db"
	if pass != "" {
		ext = ".nbbk"
	}
	w.Header().Set("Content-Disposition", "attachment; filename=netberth-backup-"+time.Now().Format("20060102-150405")+ext)

	// Read and stream the DB file
	dbPath := dbPathFromEnv()
	f, err := os.Open(dbPath)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "cannot open database")
		return
	}
	defer f.Close()

	if pass == "" {
		io.Copy(w, f)
		return
	}
	if err := backupcrypto.EncryptStream(w, f, pass); err != nil {
		logger.Log.Warn().Err(err).Msg("encrypted backup stream failed")
	}
}

func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	pass := r.Header.Get(backupPasswordHeader)
	if pass != "" && (len(pass) < 8 || len(pass) > 128) {
		utils.Error(w, http.StatusBadRequest, "backup password must be 8-128 characters")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100 MB max
	defer r.Body.Close()

	dbPath := dbPathFromEnv()

	// Write to temp file first
	tmp, err := os.CreateTemp(filepath.Dir(dbPath), ".netberth-restore-*")
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "cannot create temp file")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		utils.Error(w, http.StatusInternalServerError, "cannot secure temp file")
		return
	}
	if _, err := io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		utils.Error(w, http.StatusBadRequest, "upload failed")
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		utils.Error(w, http.StatusInternalServerError, "cannot flush upload")
		return
	}
	tmp.Close()

	encrypted, err := hasNBBKMagic(tmpName)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "cannot inspect upload")
		return
	}
	if encrypted && pass == "" {
		utils.Error(w, http.StatusBadRequest, "encrypted backup requires "+backupPasswordHeader)
		return
	}
	if !encrypted && pass != "" {
		utils.Error(w, http.StatusBadRequest, "password provided but backup is not encrypted")
		return
	}

	final := tmpName
	if encrypted {
		tmp2, err := os.CreateTemp(filepath.Dir(dbPath), ".netberth-decrypt-*")
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "cannot create temp file")
			return
		}
		tmp2Name := tmp2.Name()
		defer os.Remove(tmp2Name)
		if err := tmp2.Chmod(0600); err != nil {
			tmp2.Close()
			utils.Error(w, http.StatusInternalServerError, "cannot secure temp file")
			return
		}
		in, err := os.Open(tmpName)
		if err != nil {
			tmp2.Close()
			utils.Error(w, http.StatusInternalServerError, "cannot open upload")
			return
		}
		decErr := backupcrypto.DecryptStream(tmp2, in, pass)
		in.Close()
		if decErr != nil {
			tmp2.Close()
			utils.Error(w, http.StatusBadRequest, "invalid encrypted backup")
			return
		}
		if err := tmp2.Sync(); err != nil {
			tmp2.Close()
			utils.Error(w, http.StatusInternalServerError, "cannot flush decrypted backup")
			return
		}
		tmp2.Close()
		final = tmp2Name
	}

	// Validate the uploaded file is a valid SQLite database
	if !validSQLiteFile(final) {
		utils.Error(w, http.StatusBadRequest, "corrupt database file")
		return
	}

	// Backup current, replace with uploaded
	backup := dbPath + ".bak"
	os.Rename(dbPath, backup)
	if err := os.Rename(final, dbPath); err != nil {
		os.Rename(backup, dbPath)
		utils.Error(w, http.StatusInternalServerError, "restore failed")
		return
	}
	os.Remove(backup)

	utils.Message(w, "database restored. restart the service to apply changes.")
}

func dbPathFromEnv() string {
	if p := os.Getenv("NB_DB_PATH"); p != "" {
		return p
	}
	return "./data/netberth.db"
}

func hasNBBKMagic(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false, err
	}
	return string(magic[:]) == "NBBK", nil
}

func validSQLiteFile(path string) bool {
	validateDB, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return false
	}
	defer validateDB.Close()
	return validateDB.Ping() == nil
}
