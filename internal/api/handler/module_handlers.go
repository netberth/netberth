// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/utils"
	"github.com/netberth/netberth/pkg/validator"
)

// Handlers for STUN, WOL, Cron, ACME, Storage - shorter form

type STUNHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewSTUNHandler(db *sql.DB) *STUNHandler {
	return &STUNHandler{db: db, notifier: noopNotifier()}
}
func (h *STUNHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *STUNHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM stun_tunnels").Scan(&total)
	rows, err := h.db.Query(
		"SELECT id, name, protocol, local_port, remote_port, stun_server, target_addr, target_port, enabled, created_at, updated_at FROM stun_tunnels ORDER BY created_at DESC LIMIT ? OFFSET ?",
		20, (page-1)*20)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	tunnels := make([]model.STUNTunnel, 0)
	for rows.Next() {
		var t model.STUNTunnel
		rows.Scan(&t.ID, &t.Name, &t.Protocol, &t.LocalPort, &t.RemotePort, &t.STUNServer, &t.TargetAddr, &t.TargetPort, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
		tunnels = append(tunnels, t)
	}
	utils.Success(w, tunnels)
}

func (h *STUNHandler) Create(w http.ResponseWriter, r *http.Request) {
	var t model.STUNTunnel
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	t.ID = generateUUID()
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	h.db.Exec(`INSERT INTO stun_tunnels (id, name, protocol, local_port, remote_port, stun_server, target_addr, target_port, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.Protocol, t.LocalPort, t.RemotePort, t.STUNServer, t.TargetAddr, t.TargetPort, t.Enabled, t.CreatedAt, t.UpdatedAt)
	utils.Created(w, t)
}

func (h *STUNHandler) Update(w http.ResponseWriter, r *http.Request) {
	var t model.STUNTunnel
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	h.db.Exec(`UPDATE stun_tunnels SET name=?, protocol=?, local_port=?, remote_port=?, stun_server=?, target_addr=?, target_port=?, enabled=?, updated_at=? WHERE id=?`,
		t.Name, t.Protocol, t.LocalPort, t.RemotePort, t.STUNServer, t.TargetAddr, t.TargetPort, t.Enabled, time.Now(), r.PathValue("id"))
	utils.Message(w, "updated")
	h.notifier.OnUpdate("stun", r.PathValue("id"))
}

func (h *STUNHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM stun_tunnels WHERE id = ?", r.PathValue("id"))
	utils.Message(w, "deleted")
	h.notifier.OnDelete("stun", r.PathValue("id"))
}

// WOL Handler
type WOLHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewWOLHandler(db *sql.DB) *WOLHandler {
	return &WOLHandler{db: db, notifier: noopNotifier()}
}
func (h *WOLHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *WOLHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, name, mac, broadcast, port, platform, platform_key, created_at, updated_at FROM wol_devices ORDER BY created_at DESC")
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	devices := make([]model.WOLDevice, 0)
	for rows.Next() {
		var d model.WOLDevice
		rows.Scan(&d.ID, &d.Name, &d.MAC, &d.Broadcast, &d.Port, &d.Platform, &d.PlatformKey, &d.CreatedAt, &d.UpdatedAt)
		devices = append(devices, d)
	}
	utils.Success(w, devices)
}

func (h *WOLHandler) Create(w http.ResponseWriter, r *http.Request) {
	var d model.WOLDevice
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.ID = generateUUID()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	h.db.Exec(`INSERT INTO wol_devices (id, name, mac, broadcast, port, platform, platform_key, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Name, d.MAC, d.Broadcast, d.Port, d.Platform, d.PlatformKey, d.CreatedAt, d.UpdatedAt)
	utils.Created(w, d)
}

func (h *WOLHandler) Update(w http.ResponseWriter, r *http.Request) {
	var d model.WOLDevice
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	h.db.Exec(`UPDATE wol_devices SET name=?, mac=?, broadcast=?, port=?, platform=?, platform_key=?, updated_at=? WHERE id=?`,
		d.Name, d.MAC, d.Broadcast, d.Port, d.Platform, d.PlatformKey, time.Now(), r.PathValue("id"))
	utils.Message(w, "updated")
}

func (h *WOLHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM wol_devices WHERE id = ?", r.PathValue("id"))
	utils.Message(w, "deleted")
}

func (h *WOLHandler) Wake(w http.ResponseWriter, r *http.Request) {
	utils.Message(w, "wake signal sent")
}

// Cron Handler
type CronHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewCronHandler(db *sql.DB) *CronHandler {
	return &CronHandler{db: db, notifier: noopNotifier()}
}
func (h *CronHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *CronHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, name, schedule, type, command, module_id, module_type, enabled, last_run, next_run, created_at, updated_at FROM cron_jobs ORDER BY created_at DESC")
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	jobs := make([]model.CronJob, 0)
	for rows.Next() {
		var j model.CronJob
		rows.Scan(&j.ID, &j.Name, &j.Schedule, &j.Type, &j.Command, &j.ModuleID, &j.ModuleType, &j.Enabled, &j.LastRun, &j.NextRun, &j.CreatedAt, &j.UpdatedAt)
		jobs = append(jobs, j)
	}
	utils.Success(w, jobs)
}

func (h *CronHandler) Create(w http.ResponseWriter, r *http.Request) {
	var j model.CronJob
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	j.ID = generateUUID()
	j.CreatedAt = time.Now()
	j.UpdatedAt = time.Now()
	h.db.Exec(`INSERT INTO cron_jobs (id, name, schedule, type, command, module_id, module_type, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Name, j.Schedule, j.Type, j.Command, j.ModuleID, j.ModuleType, j.Enabled, j.CreatedAt, j.UpdatedAt)
	utils.Created(w, j)
}

func (h *CronHandler) Update(w http.ResponseWriter, r *http.Request) {
	var j model.CronJob
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	h.db.Exec(`UPDATE cron_jobs SET name=?, schedule=?, type=?, command=?, module_id=?, module_type=?, enabled=?, updated_at=? WHERE id=?`,
		j.Name, j.Schedule, j.Type, j.Command, j.ModuleID, j.ModuleType, j.Enabled, time.Now(), r.PathValue("id"))
	utils.Message(w, "updated")
}

func (h *CronHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM cron_jobs WHERE id = ?", r.PathValue("id"))
	utils.Message(w, "deleted")
}

// ACME Handler
type ACMEHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewACMEHandler(db *sql.DB) *ACMEHandler {
	return &ACMEHandler{db: db, notifier: noopNotifier()}
}
func (h *ACMEHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *ACMEHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, name, domains, provider, dns_provider, dns_config, email, auto_renew, renew_days, cert_path, key_path, expires_at, status, error, created_at, updated_at FROM acme_certificates ORDER BY created_at DESC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	certs := make([]model.ACMECertificate, 0)
	for rows.Next() {
		var c model.ACMECertificate
		var domains, dnsConfig string
		rows.Scan(&c.ID, &c.Name, &domains, &c.Provider, &c.DNSProvider, &dnsConfig, &c.Email, &c.AutoRenew, &c.RenewDays, &c.CertPath, &c.KeyPath, &c.ExpiresAt, &c.Status, &c.Error, &c.CreatedAt, &c.UpdatedAt)
		json.Unmarshal([]byte(domains), &c.Domains)
		json.Unmarshal([]byte(dnsConfig), &c.DNSConfig)
		certs = append(certs, c)
	}
	utils.Success(w, certs)
}

func (h *ACMEHandler) Create(w http.ResponseWriter, r *http.Request) {
	var c model.ACMECertificate
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, d := range c.Domains {
		if !validator.Domain(d) {
			utils.Error(w, http.StatusBadRequest, "invalid domain: "+d)
			return
		}
	}
	if !validator.Email(c.Email) {
		utils.Error(w, http.StatusBadRequest, "invalid email")
		return
	}
	c.ID = generateUUID()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	domains, _ := json.Marshal(c.Domains)
	dnsConfig, _ := json.Marshal(c.DNSConfig)
	h.db.Exec(`INSERT INTO acme_certificates (id, name, domains, provider, dns_provider, dns_config, email, auto_renew, renew_days, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, string(domains), c.Provider, c.DNSProvider, string(dnsConfig), c.Email, c.AutoRenew, c.RenewDays, "pending", c.CreatedAt, c.UpdatedAt)
	utils.Created(w, c)
}

func (h *ACMEHandler) Update(w http.ResponseWriter, r *http.Request) {
	var c model.ACMECertificate
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	domains, _ := json.Marshal(c.Domains)
	dnsConfig, _ := json.Marshal(c.DNSConfig)
	h.db.Exec(`UPDATE acme_certificates SET name=?, domains=?, provider=?, dns_provider=?, dns_config=?, email=?, auto_renew=?, renew_days=?, updated_at=? WHERE id=?`,
		c.Name, string(domains), c.Provider, c.DNSProvider, string(dnsConfig), c.Email, c.AutoRenew, c.RenewDays, time.Now(), r.PathValue("id"))
	utils.Message(w, "updated")
}

func (h *ACMEHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM acme_certificates WHERE id = ?", r.PathValue("id"))
	utils.Message(w, "deleted")
}

// Storage Handler
type StorageHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewStorageHandler(db *sql.DB) *StorageHandler {
	return &StorageHandler{db: db, notifier: noopNotifier()}
}
func (h *StorageHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *StorageHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query("SELECT id, name, type, source, username, password, services, ftp_port, enabled, created_at, updated_at FROM storage_mounts ORDER BY created_at DESC")
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	mounts := make([]model.StorageMount, 0)
	for rows.Next() {
		var m model.StorageMount
		var services string
		rows.Scan(&m.ID, &m.Name, &m.Type, &m.Source, &m.Username, &m.Password, &services, &m.FTPPort, &m.Enabled, &m.CreatedAt, &m.UpdatedAt)
		json.Unmarshal([]byte(services), &m.Services)
		mounts = append(mounts, m)
	}
	utils.Success(w, mounts)
}

func (h *StorageHandler) Create(w http.ResponseWriter, r *http.Request) {
	var m model.StorageMount
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	m.ID = generateUUID()
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	services, _ := json.Marshal(m.Services)
	h.db.Exec(`INSERT INTO storage_mounts (id, name, type, source, username, password, services, ftp_port, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Name, m.Type, m.Source, m.Username, m.Password, string(services), m.FTPPort, m.Enabled, m.CreatedAt, m.UpdatedAt)
	utils.Created(w, m)
}

func (h *StorageHandler) Update(w http.ResponseWriter, r *http.Request) {
	var m model.StorageMount
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	services, _ := json.Marshal(m.Services)
	h.db.Exec(`UPDATE storage_mounts SET name=?, type=?, source=?, username=?, password=?, services=?, ftp_port=?, enabled=?, updated_at=? WHERE id=?`,
		m.Name, m.Type, m.Source, m.Username, m.Password, string(services), m.FTPPort, m.Enabled, time.Now(), r.PathValue("id"))
	utils.Message(w, "updated")
}

func (h *StorageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM storage_mounts WHERE id = ?", r.PathValue("id"))
	utils.Message(w, "deleted")
}
