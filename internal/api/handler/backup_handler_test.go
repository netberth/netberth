// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/netberth/netberth/internal/backupcrypto"
)

const backupPass = "backup-pass-123"

func createMarkerDB(t *testing.T, path, table, value string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO "+table+" (value) VALUES (?)", value); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return db
}

func doBackupRequest(t *testing.T, fn func(http.ResponseWriter, *http.Request), method string, body []byte, pass string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/system/backup", bytes.NewReader(body))
	if pass != "" {
		req.Header.Set(backupPasswordHeader, pass)
	}
	w := httptest.NewRecorder()
	fn(w, req)
	return w
}

func TestBackupDownloadPlaintext(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Download, "GET", nil, "")
	expectStatus(t, w, http.StatusOK)

	want, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatalf("plaintext download mismatch: got %d bytes, want %d", w.Body.Len(), len(want))
	}
	if ct := w.Header().Get("Content-Disposition"); !bytes.Contains([]byte(ct), []byte(".db")) {
		t.Fatalf("expected .db attachment, got %q", ct)
	}
}

func TestBackupDownloadEncrypted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Download, "GET", nil, backupPass)
	expectStatus(t, w, http.StatusOK)

	env := w.Body.Bytes()
	if !bytes.HasPrefix(env, []byte("NBBK")) {
		t.Fatalf("expected NBBK envelope, got prefix %q", env[:8])
	}
	plain, err := backupcrypto.Decrypt(env, backupPass)
	if err != nil {
		t.Fatalf("decrypt download: %v", err)
	}
	want, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if !bytes.Equal(plain, want) {
		t.Fatalf("encrypted download mismatch: got %d bytes, want %d", len(plain), len(want))
	}
	if ct := w.Header().Get("Content-Disposition"); !bytes.Contains([]byte(ct), []byte(".nbbk")) {
		t.Fatalf("expected .nbbk attachment, got %q", ct)
	}
}

func TestBackupRestoreEncrypted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	original := createMarkerDB(t, dbPath, "marker", "original")
	defer original.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	replacementPath := filepath.Join(dir, "replacement.db")
	replacement := createMarkerDB(t, replacementPath, "replacement", "restored")
	replacement.Close()
	replBytes, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}
	env, err := backupcrypto.Encrypt(replBytes, backupPass)
	if err != nil {
		t.Fatalf("encrypt replacement: %v", err)
	}

	h := NewBackupHandler(original)
	w := doBackupRequest(t, h.Restore, "POST", env, backupPass)
	expectStatus(t, w, http.StatusOK)

	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer check.Close()
	var value string
	if err := check.QueryRow("SELECT value FROM replacement WHERE id=1").Scan(&value); err != nil {
		t.Fatalf("restored table missing: %v", err)
	}
	if value != "restored" {
		t.Fatalf("unexpected restored value %q", value)
	}
}

func TestBackupRestorePlaintext(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	original := createMarkerDB(t, dbPath, "marker", "original")
	defer original.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	replacementPath := filepath.Join(dir, "replacement.db")
	replacement := createMarkerDB(t, replacementPath, "replacement", "restored")
	replacement.Close()
	replBytes, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatalf("read replacement: %v", err)
	}

	h := NewBackupHandler(original)
	w := doBackupRequest(t, h.Restore, "POST", replBytes, "")
	expectStatus(t, w, http.StatusOK)

	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer check.Close()
	if err := check.QueryRow("SELECT value FROM replacement WHERE id=1").Scan(new(string)); err != nil {
		t.Fatalf("restored table missing: %v", err)
	}
}

func TestBackupRestoreEncryptedWithoutPassword(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	env, err := backupcrypto.Encrypt([]byte("not a real db"), backupPass)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Restore, "POST", env, "")
	expectStatus(t, w, http.StatusBadRequest)
}

func TestBackupRestorePasswordWithPlaintext(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Restore, "POST", []byte("not a real db"), backupPass)
	expectStatus(t, w, http.StatusBadRequest)
}

func TestBackupRestoreCorruptEncrypted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	env, err := backupcrypto.Encrypt([]byte("not a real db"), backupPass)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	env[len(env)-1] ^= 0x01

	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Restore, "POST", env, backupPass)
	expectStatus(t, w, http.StatusBadRequest)

	// Original database must remain intact.
	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer check.Close()
	if err := check.QueryRow("SELECT value FROM marker WHERE id=1").Scan(new(string)); err != nil {
		t.Fatalf("original table damaged: %v", err)
	}
}

func TestBackupPasswordLengthValidation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "netberth.db")
	db := createMarkerDB(t, dbPath, "marker", "original")
	defer db.Close()
	t.Setenv("NB_DB_PATH", dbPath)

	h := NewBackupHandler(db)
	w := doBackupRequest(t, h.Download, "GET", nil, "short")
	expectStatus(t, w, http.StatusBadRequest)

	w2 := doBackupRequest(t, h.Restore, "POST", []byte("x"), "short")
	expectStatus(t, w2, http.StatusBadRequest)
}
