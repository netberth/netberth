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

type ForwardHandler struct {
	db       *sql.DB
	notifier *Notifier
}

func NewForwardHandler(db *sql.DB) *ForwardHandler {
	return &ForwardHandler{db: db, notifier: noopNotifier()}
}
func (h *ForwardHandler) SetNotifier(n *Notifier) { h.notifier = n }

func (h *ForwardHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int
	h.db.QueryRow("SELECT COUNT(*) FROM forward_rules").Scan(&total)

	rows, err := h.db.Query(
		`SELECT id, tenant_id, owner_id, name, protocol, listen_addr, listen_port,
		 target_addr, target_port, enable_ipv6, max_conns, enabled,
		 schedule_on, schedule_off, created_at, updated_at
		 FROM forward_rules ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	rules := make([]model.ForwardRule, 0)
	for rows.Next() {
		var r model.ForwardRule
		rows.Scan(&r.ID, &r.TenantID, &r.OwnerID, &r.Name, &r.Protocol,
			&r.ListenAddr, &r.ListenPort, &r.TargetAddr, &r.TargetPort,
			&r.EnableIPv6, &r.MaxConns, &r.Enabled,
			&r.ScheduleOn, &r.ScheduleOff, &r.CreatedAt, &r.UpdatedAt)

		r.Whitelist = loadACLEntries(h.db, "forward_whitelist", r.ID)
		r.Blacklist = loadACLEntries(h.db, "forward_blacklist", r.ID)
		rules = append(rules, r)
	}
	utils.Paginated(w, rules, total, page, pageSize)
}

func (h *ForwardHandler) Create(w http.ResponseWriter, r *http.Request) {
	var rule model.ForwardRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validator.Port(rule.ListenPort) || !validator.Port(rule.TargetPort) {
		utils.Error(w, http.StatusBadRequest, "port must be 1-65535")
		return
	}
	for _, e := range rule.Whitelist {
		if !validator.IPOrCIDR(e.Value) {
			utils.Error(w, http.StatusBadRequest, "invalid whitelist: "+e.Value)
			return
		}
	}
	for _, e := range rule.Blacklist {
		if !validator.IPOrCIDR(e.Value) {
			utils.Error(w, http.StatusBadRequest, "invalid blacklist: "+e.Value)
			return
		}
	}

	rule.ID = generateUUID()
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	_, err := h.db.Exec(
		`INSERT INTO forward_rules (id, tenant_id, owner_id, name, protocol, listen_addr, listen_port,
		 target_addr, target_port, enable_ipv6, max_conns, enabled, schedule_on, schedule_off, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rule.ID, rule.TenantID, rule.OwnerID, rule.Name, rule.Protocol,
		rule.ListenAddr, rule.ListenPort, rule.TargetAddr, rule.TargetPort,
		rule.EnableIPv6, rule.MaxConns, rule.Enabled,
		rule.ScheduleOn, rule.ScheduleOff, rule.CreatedAt, rule.UpdatedAt,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "create failed")
		return
	}

	saveACLEntries(h.db, "forward_whitelist", rule.ID, rule.Whitelist)
	saveACLEntries(h.db, "forward_blacklist", rule.ID, rule.Blacklist)

	utils.Created(w, rule)
	h.notifier.OnCreate("forward", rule.ID)
}

func (h *ForwardHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var rule model.ForwardRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, err := h.db.Exec(
		`UPDATE forward_rules SET name=?, protocol=?, listen_addr=?, listen_port=?, target_addr=?,
		 target_port=?, enable_ipv6=?, max_conns=?, enabled=?, schedule_on=?,
		 schedule_off=?, updated_at=? WHERE id=?`,
		rule.Name, rule.Protocol, rule.ListenAddr, rule.ListenPort, rule.TargetAddr,
		rule.TargetPort, rule.EnableIPv6, rule.MaxConns, rule.Enabled,
		rule.ScheduleOn, rule.ScheduleOff, time.Now(), id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}

	h.db.Exec("DELETE FROM forward_whitelist WHERE rule_id = ?", id)
	h.db.Exec("DELETE FROM forward_blacklist WHERE rule_id = ?", id)
	saveACLEntries(h.db, "forward_whitelist", id, rule.Whitelist)
	saveACLEntries(h.db, "forward_blacklist", id, rule.Blacklist)

	utils.Message(w, "updated")
	h.notifier.OnUpdate("forward", id)
}

func (h *ForwardHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.db.Exec("DELETE FROM forward_rules WHERE id = ?", id)
	utils.Message(w, "deleted")
	h.notifier.OnDelete("forward", id)
}

func loadACLEntries(db *sql.DB, table, ruleID string) []model.ACLEntry {
	rows, err := db.Query("SELECT id, value FROM "+table+" WHERE rule_id = ?", ruleID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var entries []model.ACLEntry
	for rows.Next() {
		var e model.ACLEntry
		rows.Scan(&e.ID, &e.Value)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []model.ACLEntry{}
	}
	return entries
}

func saveACLEntries(db *sql.DB, table, ruleID string, entries []model.ACLEntry) {
	for _, e := range entries {
		if e.ID == "" {
			e.ID = generateUUID()
		}
		db.Exec("INSERT INTO "+table+" (id, rule_id, value) VALUES (?,?,?)", e.ID, ruleID, e.Value)
	}
}
