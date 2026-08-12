// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netberth/netberth/internal/db"
)

func newWebhookHandler(t *testing.T) (*WebhookHandler, *sql.DB) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "webhook-handler.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewWebhookHandler(database, nil), database
}

func webhookDo(h *WebhookHandler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if strings.Contains(path, "/webhooks/") {
		parts := strings.Split(path, "/")
		id := parts[len(parts)-1]
		if id == "test" {
			id = parts[len(parts)-2]
		}
		req.SetPathValue("id", id)
	}
	w := httptest.NewRecorder()
	switch {
	case method == http.MethodPost && strings.HasSuffix(path, "/test"):
		h.Test(w, req)
	case method == http.MethodPost:
		h.Create(w, req)
	case method == http.MethodPut:
		h.Update(w, req)
	case method == http.MethodDelete:
		h.Delete(w, req)
	default:
		h.List(w, req)
	}
	return w
}

func TestWebhookCreateListUpdateDelete(t *testing.T) {
	h, _ := newWebhookHandler(t)

	create := webhookDo(h, http.MethodPost, "/api/v1/webhooks",
		`{"name":"ops","url":"https://hooks.example.com/ingest","secret":"topsecret","events":["forward:created","proxy:updated"],"enabled":true}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			ID        string   `json:"id"`
			Secret    string   `json:"secret"`
			HasSecret bool     `json:"has_secret"`
			Events    []string `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.ID == "" || created.Data.Secret != "topsecret" || !created.Data.HasSecret {
		t.Fatalf("unexpected create response: %+v", created.Data)
	}
	if len(created.Data.Events) != 2 {
		t.Fatalf("unexpected events: %+v", created.Data.Events)
	}

	list := webhookDo(h, http.MethodGet, "/api/v1/webhooks", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "topsecret") {
		t.Fatal("list leaked webhook secret")
	}
	if !strings.Contains(list.Body.String(), `"has_secret":true`) {
		t.Fatalf("list should report has_secret: %s", list.Body.String())
	}

	// Empty secret in update keeps the stored secret.
	upd := webhookDo(h, http.MethodPut, "/api/v1/webhooks/"+created.Data.ID,
		`{"name":"ops2","url":"https://hooks.example.com/ingest2","events":[],"enabled":false}`)
	if upd.Code != http.StatusOK {
		t.Fatalf("update: %d %s", upd.Code, upd.Body.String())
	}

	del := webhookDo(h, http.MethodDelete, "/api/v1/webhooks/"+created.Data.ID, "")
	if del.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", del.Code, del.Body.String())
	}
	del2 := webhookDo(h, http.MethodDelete, "/api/v1/webhooks/"+created.Data.ID, "")
	if del2.Code != http.StatusNotFound {
		t.Fatalf("second delete should 404, got %d", del2.Code)
	}
}

func TestWebhookValidation(t *testing.T) {
	h, _ := newWebhookHandler(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{"url":"https://x.example.com/hook"}`},
		{"bad scheme", `{"name":"x","url":"ftp://x.example.com/hook"}`},
		{"unknown event", `{"name":"x","url":"https://x.example.com/hook","events":["bogus:created"]}`},
		{"no host", `{"name":"x","url":"http:///hook"}`},
	}
	for _, c := range cases {
		w := webhookDo(h, http.MethodPost, "/api/v1/webhooks", c.body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", c.name, w.Code, w.Body.String())
		}
	}
}

func TestWebhookTestUnconfigured(t *testing.T) {
	h, _ := newWebhookHandler(t)
	w := webhookDo(h, http.MethodPost, "/api/v1/webhooks/nope/test", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without deliverer, got %d", w.Code)
	}
}
