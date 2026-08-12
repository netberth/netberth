// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/internal/service"
	"github.com/netberth/netberth/pkg/utils"
)

type WebhookHandler struct {
	db        *sql.DB
	deliverer *service.WebhookDispatcher
}

func NewWebhookHandler(db *sql.DB, deliverer *service.WebhookDispatcher) *WebhookHandler {
	return &WebhookHandler{db: db, deliverer: deliverer}
}

type webhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

type webhookCreatedResponse struct {
	model.WebhookEndpoint
	Secret string `json:"secret,omitempty"`
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, name, url, secret, events, enabled, created_at, updated_at
		 FROM webhook_endpoints ORDER BY created_at DESC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	endpoints := make([]model.WebhookEndpoint, 0)
	for rows.Next() {
		var ep model.WebhookEndpoint
		var eventsJSON string
		var enabled int
		if err := rows.Scan(&ep.ID, &ep.Name, &ep.URL, &ep.Secret, &eventsJSON,
			&enabled, &ep.CreatedAt, &ep.UpdatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "scan failed")
			return
		}
		ep.Enabled = enabled == 1
		ep.HasSecret = ep.Secret != ""
		ep.Secret = ""
		if err := json.Unmarshal([]byte(eventsJSON), &ep.Events); err != nil {
			ep.Events = nil
		}
		endpoints = append(endpoints, ep)
	}
	utils.Success(w, endpoints)
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxAuthBodyBytes)
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateWebhookRequest(&req); msg != "" {
		utils.Error(w, http.StatusBadRequest, msg)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	eventsJSON, _ := json.Marshal(req.Events)
	ep := model.WebhookEndpoint{
		ID:        generateUUID(),
		Name:      strings.TrimSpace(req.Name),
		URL:       strings.TrimSpace(req.URL),
		Secret:    req.Secret,
		Events:    req.Events,
		Enabled:   enabled,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if _, err := h.db.Exec(
		`INSERT INTO webhook_endpoints (id, name, url, secret, events, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.Name, ep.URL, ep.Secret, string(eventsJSON), boolInt(enabled), ep.CreatedAt, ep.UpdatedAt); err != nil {
		utils.Error(w, http.StatusInternalServerError, "create failed")
		return
	}
	ep.HasSecret = ep.Secret != ""
	utils.Created(w, webhookCreatedResponse{WebhookEndpoint: ep, Secret: req.Secret})
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limitBody(w, r, maxAuthBodyBytes)
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateWebhookRequest(&req); msg != "" {
		utils.Error(w, http.StatusBadRequest, msg)
		return
	}
	var current model.WebhookEndpoint
	var eventsJSON string
	var enabled int
	err := h.db.QueryRow(
		`SELECT id, name, url, secret, events, enabled FROM webhook_endpoints WHERE id = ?`, id).
		Scan(&current.ID, &current.Name, &current.URL, &current.Secret, &eventsJSON, &enabled)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "webhook endpoint not found")
		return
	}
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	secret := current.Secret
	if req.Secret != "" {
		secret = req.Secret
	}
	newEnabled := enabled == 1
	if req.Enabled != nil {
		newEnabled = *req.Enabled
	}
	newEventsJSON, _ := json.Marshal(req.Events)
	if _, err := h.db.Exec(
		`UPDATE webhook_endpoints SET name=?, url=?, secret=?, events=?, enabled=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(req.Name), strings.TrimSpace(req.URL), secret, string(newEventsJSON),
		boolInt(newEnabled), time.Now(), id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	utils.Message(w, "updated")
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	res, err := h.db.Exec("DELETE FROM webhook_endpoints WHERE id = ?", r.PathValue("id"))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		utils.Error(w, http.StatusNotFound, "webhook endpoint not found")
		return
	}
	utils.Message(w, "deleted")
}

func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	if h.deliverer == nil {
		utils.Error(w, http.StatusServiceUnavailable, "webhook delivery not configured")
		return
	}
	var ep model.WebhookEndpoint
	var eventsJSON string
	var enabled int
	err := h.db.QueryRow(
		`SELECT id, name, url, secret, events, enabled FROM webhook_endpoints WHERE id = ?`, r.PathValue("id")).
		Scan(&ep.ID, &ep.Name, &ep.URL, &ep.Secret, &eventsJSON, &enabled)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "webhook endpoint not found")
		return
	}
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	ep.Enabled = enabled == 1
	json.Unmarshal([]byte(eventsJSON), &ep.Events)
	if err := h.deliverer.SendTest(r.Context(), ep); err != nil {
		utils.Error(w, http.StatusBadGateway, "delivery failed: "+err.Error())
		return
	}
	utils.Message(w, "delivery ok")
}

func validateWebhookRequest(req *webhookRequest) string {
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	if req.Name == "" {
		return "name required"
	}
	if len(req.Name) > 64 {
		return "name too long"
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "url must be a valid http(s) URL"
	}
	if len(req.URL) > 1024 {
		return "url too long"
	}
	if req.Secret != "" && len(req.Secret) > 128 {
		return "secret too long"
	}
	known := make(map[string]bool, 32)
	for _, e := range service.KnownEventTypes() {
		known[e] = true
	}
	for _, e := range req.Events {
		if !known[e] {
			return "unknown event type: " + e
		}
	}
	return ""
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
