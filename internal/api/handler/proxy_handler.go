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

type ProxyHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewProxyHandler(db *sql.DB) *ProxyHandler {
	return &ProxyHandler{db: db, notifier: noopNotifier()}
}
func (h *ProxyHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM proxy_rules").Scan(&total)

	rows, _ := h.db.Query(
		`SELECT id, tenant_id, owner_id, name, target_url, tls_enabled, cert_id,
		 force_https, http2, websocket, url_rewrite, basic_auth_user, basic_auth_hash,
		 max_conns, enabled, created_at, updated_at
		 FROM proxy_rules ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize,
	)
	defer rows.Close()

	rules := make([]model.ProxyRule, 0)
	for rows.Next() {
		var r model.ProxyRule
		rows.Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.TargetURL,
			&r.TLSEnabled, &r.CertID, &r.ForceHTTPS, &r.HTTP2, &r.Websocket,
			&r.URLRewrite, &r.BasicAuthUser, &r.BasicAuthHash, &r.MaxConns,
			&r.Enabled, &r.CreatedAt, &r.UpdatedAt)

		r.Domains = loadStringList(h.db, "proxy_domains", r.ID)
		r.IPWhitelist = loadACLEntries(h.db, "proxy_ip_whitelist", r.ID)
		r.IPBlacklist = loadACLEntries(h.db, "proxy_ip_blacklist", r.ID)
		r.UAWhitelist = loadStringList(h.db, "proxy_ua_whitelist", r.ID)
		r.UABlacklist = loadStringList(h.db, "proxy_ua_blacklist", r.ID)
		rules = append(rules, r)
	}
	utils.Paginated(w, rules, total, page, pageSize)
}

func (h *ProxyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var rule model.ProxyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Validate inputs
	for _, d := range rule.Domains {
		if !validator.Domain(d) {
			utils.Error(w, http.StatusBadRequest, "invalid domain: "+d)
			return
		}
	}
	if !validator.URL(rule.TargetURL) {
		utils.Error(w, http.StatusBadRequest, "invalid target URL")
		return
	}
	for _, e := range rule.IPWhitelist {
		if !validator.IPOrCIDR(e.Value) {
			utils.Error(w, http.StatusBadRequest, "invalid whitelist entry: "+e.Value)
			return
		}
	}
	for _, e := range rule.IPBlacklist {
		if !validator.IPOrCIDR(e.Value) {
			utils.Error(w, http.StatusBadRequest, "invalid blacklist entry: "+e.Value)
			return
		}
	}

	rule.ID = generateUUID()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	_, err := h.db.Exec(
		`INSERT INTO proxy_rules (id, tenant_id, owner_id, name, target_url, tls_enabled, cert_id,
		 force_https, http2, websocket, url_rewrite, basic_auth_user, basic_auth_hash,
		 max_conns, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rule.ID, rule.TenantID, rule.OwnerID, rule.Name, rule.TargetURL,
		rule.TLSEnabled, rule.CertID, rule.ForceHTTPS, rule.HTTP2, rule.Websocket,
		rule.URLRewrite, rule.BasicAuthUser, rule.BasicAuthHash, rule.MaxConns,
		rule.Enabled, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "create failed")
		return
	}

	saveStringList(h.db, "proxy_domains", rule.ID, rule.Domains)
	saveACLEntries(h.db, "proxy_ip_whitelist", rule.ID, rule.IPWhitelist)
	saveACLEntries(h.db, "proxy_ip_blacklist", rule.ID, rule.IPBlacklist)
	saveStringList(h.db, "proxy_ua_whitelist", rule.ID, rule.UAWhitelist)
	saveStringList(h.db, "proxy_ua_blacklist", rule.ID, rule.UABlacklist)

	utils.Created(w, rule)
	h.notifier.OnCreate("proxy", rule.ID)
}

func (h *ProxyHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule model.ProxyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, err := h.db.Exec(
		`UPDATE proxy_rules SET name=?, target_url=?, tls_enabled=?, cert_id=?,
		 force_https=?, http2=?, websocket=?, url_rewrite=?,
		 basic_auth_user=?, basic_auth_hash=?, max_conns=?, enabled=?, updated_at=?
		 WHERE id=?`,
		rule.Name, rule.TargetURL, rule.TLSEnabled, rule.CertID,
		rule.ForceHTTPS, rule.HTTP2, rule.Websocket, rule.URLRewrite,
		rule.BasicAuthUser, rule.BasicAuthHash, rule.MaxConns, rule.Enabled,
		time.Now(), id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Replace relational entries
	for _, t := range []string{"proxy_domains", "proxy_ip_whitelist", "proxy_ip_blacklist", "proxy_ua_whitelist", "proxy_ua_blacklist"} {
		h.db.Exec("DELETE FROM "+t+" WHERE rule_id = ?", id)
	}
	saveStringList(h.db, "proxy_domains", id, rule.Domains)
	saveACLEntries(h.db, "proxy_ip_whitelist", id, rule.IPWhitelist)
	saveACLEntries(h.db, "proxy_ip_blacklist", id, rule.IPBlacklist)
	saveStringList(h.db, "proxy_ua_whitelist", id, rule.UAWhitelist)
	saveStringList(h.db, "proxy_ua_blacklist", id, rule.UABlacklist)

	utils.Message(w, "updated")
	h.notifier.OnUpdate("proxy", id)
}

func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.db.Exec("DELETE FROM proxy_rules WHERE id = ?", id)
	utils.Message(w, "deleted")
	h.notifier.OnDelete("proxy", id)
}

func loadStringList(db *sql.DB, table, ruleID string) []string {
	rows, err := db.Query("SELECT value FROM "+table+" WHERE rule_id = ?", ruleID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var v string
		rows.Scan(&v)
		list = append(list, v)
	}
	if list == nil {
		list = []string{}
	}
	return list
}

func saveStringList(db *sql.DB, table, ruleID string, list []string) {
	for _, v := range list {
		db.Exec("INSERT INTO "+table+" (id, rule_id, value) VALUES (?,?,?)", generateUUID(), ruleID, v)
	}
}
