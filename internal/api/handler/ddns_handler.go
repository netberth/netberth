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
)

type DDNSHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewDDNSHandler(db *sql.DB) *DDNSHandler {
	return &DDNSHandler{db: db, notifier: noopNotifier()}
}
func (h *DDNSHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *DDNSHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM ddns_configs").Scan(&total)
	rows, err := h.db.Query(
		`SELECT id, name, provider, domain, sub_domain, record_type, ttl, credentials,
		 get_ip_url, get_ip_type, net_interface, interval_seconds, enabled, created_at, updated_at
		 FROM ddns_configs ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	configs := make([]model.DDNSConfig, 0)
	for rows.Next() {
		var c model.DDNSConfig
		var creds string
		rows.Scan(&c.ID, &c.Name, &c.Provider, &c.Domain, &c.SubDomain, &c.RecordType,
			&c.TTL, &creds, &c.GetIPURL, &c.GetIPType, &c.NetInterface, &c.Interval,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt)
		json.Unmarshal([]byte(creds), &c.Credentials)
		configs = append(configs, c)
	}
	utils.Paginated(w, configs, total, page, pageSize)
}

func (h *DDNSHandler) Create(w http.ResponseWriter, r *http.Request) {
	var c model.DDNSConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c.ID = generateUUID()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	creds, _ := json.Marshal(c.Credentials)
	_, err := h.db.Exec(
		`INSERT INTO ddns_configs (id, name, provider, domain, sub_domain, record_type, ttl,
		 credentials, get_ip_url, get_ip_type, net_interface, interval_seconds, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Provider, c.Domain, c.SubDomain, c.RecordType, c.TTL,
		string(creds), c.GetIPURL, c.GetIPType, c.NetInterface, c.Interval,
		c.Enabled, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "create failed")
		return
	}
	utils.Created(w, c)
	h.notifier.OnCreate("ddns", c.ID)
}

func (h *DDNSHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var c model.DDNSConfig
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	creds, _ := json.Marshal(c.Credentials)
	_, err := h.db.Exec(
		`UPDATE ddns_configs SET name=?, provider=?, domain=?, sub_domain=?, record_type=?,
		 ttl=?, credentials=?, get_ip_url=?, get_ip_type=?, net_interface=?,
		 interval_seconds=?, enabled=?, updated_at=? WHERE id=?`,
		c.Name, c.Provider, c.Domain, c.SubDomain, c.RecordType, c.TTL, string(creds),
		c.GetIPURL, c.GetIPType, c.NetInterface, c.Interval, c.Enabled, time.Now(), id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	h.notifier.OnUpdate("ddns", id)
	utils.Message(w, "updated")
}

func (h *DDNSHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.db.Exec("DELETE FROM ddns_configs WHERE id = ?", r.PathValue("id"))
	h.notifier.OnDelete("ddns", id)
	utils.Message(w, "deleted")
}
