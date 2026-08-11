// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/netberth/netberth/internal/model"
)

func TestForwardHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewForwardHandler(db)

	// Create with whitelist entry.
	body, _ := json.Marshal(model.ForwardRule{
		Name: "fwd", Protocol: "tcp", ListenPort: 22001, TargetAddr: "192.0.2.10",
		TargetPort: 80, Enabled: true,
		Whitelist: []model.ACLEntry{{Value: "192.0.2.0/24"}},
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.ForwardRule `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID
	if id == "" {
		t.Fatal("expected generated id")
	}

	// List returns the created rule with ACL loaded.
	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/forward-rules?page=1&page_size=10", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data  []model.ForwardRule `json:"data"`
		Total int                 `json:"total"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Total != 1 {
		t.Fatalf("expected 1 rule, got %d (total %d)", len(listed.Data), listed.Total)
	}
	if len(listed.Data[0].Whitelist) != 1 || listed.Data[0].Whitelist[0].Value != "192.0.2.0/24" {
		t.Fatalf("whitelist not persisted: %+v", listed.Data[0].Whitelist)
	}

	// Update.
	upd, _ := json.Marshal(model.ForwardRule{
		Name: "fwd2", Protocol: "tcp", ListenPort: 22003, TargetAddr: "192.0.2.11", TargetPort: 81,
	})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/forward-rules/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)
	var name string
	if err := db.QueryRow("SELECT name FROM forward_rules WHERE id=?", id).Scan(&name); err != nil {
		t.Fatalf("query updated rule: %v", err)
	}
	if name != "fwd2" {
		t.Fatalf("expected name fwd2, got %s", name)
	}

	// Delete.
	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/forward-rules/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)
	var n int
	db.QueryRow("SELECT COUNT(*) FROM forward_rules WHERE id=?", id).Scan(&n)
	if n != 0 {
		t.Fatalf("rule still present after delete")
	}

	// Notifier callbacks.
	var createdN, updatedN, deletedN int32
	h.SetNotifier(&Notifier{
		OnCreate: func(resource, id string) { atomic.AddInt32(&createdN, 1) },
		OnUpdate: func(resource, id string) { atomic.AddInt32(&updatedN, 1) },
		OnDelete: func(resource, id string) { atomic.AddInt32(&deletedN, 1) },
	})
	body2, _ := json.Marshal(model.ForwardRule{
		Name: "n", Protocol: "tcp", ListenPort: 22004, TargetAddr: "192.0.2.12", TargetPort: 82,
	})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", body2)
	expectStatus(t, w2, http.StatusCreated)
	var c2 struct {
		Data model.ForwardRule `json:"data"`
	}
	decodeResponse(t, w2, &c2)
	doPathJSON(t, h.Update, http.MethodPut, "/api/v1/forward-rules/"+c2.Data.ID, c2.Data.ID, body2)
	doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/forward-rules/"+c2.Data.ID, c2.Data.ID, nil)
	if atomic.LoadInt32(&createdN) != 1 || atomic.LoadInt32(&updatedN) != 1 || atomic.LoadInt32(&deletedN) != 1 {
		t.Fatalf("notifier counters: created=%d updated=%d deleted=%d", createdN, updatedN, deletedN)
	}
}

func TestForwardHandlerValidationErrors(t *testing.T) {
	h := NewForwardHandler(setupFullTestDB(t))

	// Invalid ports.
	bad, _ := json.Marshal(model.ForwardRule{Name: "bad", ListenPort: 0, TargetPort: 80})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", bad)
	expectStatus(t, w, http.StatusBadRequest)

	// Invalid whitelist entry.
	bad2, _ := json.Marshal(model.ForwardRule{
		Name: "bad2", ListenPort: 22005, TargetPort: 80, Whitelist: []model.ACLEntry{{Value: "not-an-ip"}},
	})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", bad2)
	expectStatus(t, w2, http.StatusBadRequest)

	// Invalid blacklist entry.
	bad3, _ := json.Marshal(model.ForwardRule{
		Name: "bad3", ListenPort: 22006, TargetPort: 80, Blacklist: []model.ACLEntry{{Value: "nope"}},
	})
	w3 := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", bad3)
	expectStatus(t, w3, http.StatusBadRequest)

	// Malformed JSON.
	w4 := doJSON(t, h.Create, http.MethodPost, "/api/v1/forward-rules", []byte("{"))
	expectStatus(t, w4, http.StatusBadRequest)
	w5 := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/forward-rules/x", "x", []byte("{"))
	expectStatus(t, w5, http.StatusBadRequest)
}

func TestProxyHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewProxyHandler(db)

	body, _ := json.Marshal(model.ProxyRule{
		Name: "prx", Domains: []string{"example.com", "*.example.com"},
		TargetURL: "http://192.0.2.20:8080", Enabled: true,
		IPWhitelist: []model.ACLEntry{{Value: "192.0.2.0/24"}},
		UAWhitelist: []string{"Mozilla/5.0"},
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.ProxyRule `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/proxy-rules?page=1&page_size=10", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data  []model.ProxyRule `json:"data"`
		Total int               `json:"total"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Total != 1 {
		t.Fatalf("expected 1 proxy rule, got %d", len(listed.Data))
	}
	if len(listed.Data[0].Domains) != 2 || listed.Data[0].Domains[0] != "example.com" {
		t.Fatalf("proxy domains not persisted: %+v", listed.Data[0].Domains)
	}
	if len(listed.Data[0].IPWhitelist) != 1 || len(listed.Data[0].UAWhitelist) != 1 {
		t.Fatalf("proxy lists not persisted: %+v", listed.Data[0])
	}

	upd, _ := json.Marshal(model.ProxyRule{
		Name: "prx2", Domains: []string{"example.net"}, TargetURL: "http://192.0.2.21:8081",
	})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/proxy-rules/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)
	var name string
	if err := db.QueryRow("SELECT name FROM proxy_rules WHERE id=?", id).Scan(&name); err != nil {
		t.Fatalf("query updated proxy: %v", err)
	}
	if name != "prx2" {
		t.Fatalf("expected prx2, got %s", name)
	}
	var domains int
	db.QueryRow("SELECT COUNT(*) FROM proxy_domains WHERE rule_id=?", id).Scan(&domains)
	if domains != 1 {
		t.Fatalf("expected 1 domain after update, got %d", domains)
	}

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/proxy-rules/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	var createdN, updatedN, deletedN int32
	h.SetNotifier(&Notifier{
		OnCreate: func(resource, id string) { atomic.AddInt32(&createdN, 1) },
		OnUpdate: func(resource, id string) { atomic.AddInt32(&updatedN, 1) },
		OnDelete: func(resource, id string) { atomic.AddInt32(&deletedN, 1) },
	})
	body2, _ := json.Marshal(model.ProxyRule{Name: "n", Domains: []string{"a.example.com"}, TargetURL: "http://192.0.2.22:80"})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", body2)
	expectStatus(t, w2, http.StatusCreated)
	var c2 struct {
		Data model.ProxyRule `json:"data"`
	}
	decodeResponse(t, w2, &c2)
	doPathJSON(t, h.Update, http.MethodPut, "/api/v1/proxy-rules/"+c2.Data.ID, c2.Data.ID, body2)
	doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/proxy-rules/"+c2.Data.ID, c2.Data.ID, nil)
	if atomic.LoadInt32(&createdN) != 1 || atomic.LoadInt32(&updatedN) != 1 || atomic.LoadInt32(&deletedN) != 1 {
		t.Fatalf("notifier counters: created=%d updated=%d deleted=%d", createdN, updatedN, deletedN)
	}
}

func TestProxyHandlerValidationErrors(t *testing.T) {
	h := NewProxyHandler(setupFullTestDB(t))

	badDomain, _ := json.Marshal(model.ProxyRule{Name: "x", Domains: []string{"not a domain"}, TargetURL: "http://192.0.2.1:80"})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", badDomain)
	expectStatus(t, w, http.StatusBadRequest)

	badURL, _ := json.Marshal(model.ProxyRule{Name: "x", Domains: []string{"example.com"}, TargetURL: "not a url"})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", badURL)
	expectStatus(t, w2, http.StatusBadRequest)

	badIP, _ := json.Marshal(model.ProxyRule{Name: "x", Domains: []string{"example.com"}, TargetURL: "http://192.0.2.1:80", IPWhitelist: []model.ACLEntry{{Value: "nope"}}})
	w3 := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", badIP)
	expectStatus(t, w3, http.StatusBadRequest)

	w4 := doJSON(t, h.Create, http.MethodPost, "/api/v1/proxy-rules", []byte("{"))
	expectStatus(t, w4, http.StatusBadRequest)
	w5 := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/proxy-rules/x", "x", []byte("{"))
	expectStatus(t, w5, http.StatusBadRequest)
}

func TestForwardListScanError(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewForwardHandler(db)
	db.Exec("DROP TABLE forward_whitelist")
	db.Exec("DROP TABLE forward_blacklist")
	db.Exec("DROP TABLE forward_rules")
	db.Exec(`CREATE TABLE forward_rules (id TEXT PRIMARY KEY, tenant_id TEXT, owner_id TEXT, name TEXT, protocol TEXT, listen_addr TEXT, listen_port TEXT, target_addr TEXT, target_port INTEGER, enable_ipv6 INTEGER, max_conns INTEGER, enabled INTEGER, schedule_on TEXT, schedule_off TEXT, created_at DATETIME, updated_at DATETIME)`)
	db.Exec(`INSERT INTO forward_rules (id, name, listen_port) VALUES ('f1','x','not-a-port')`)

	w := doJSON(t, h.List, http.MethodGet, "/api/v1/forward-rules", nil)
	expectStatus(t, w, http.StatusInternalServerError)
}

func TestProxyListScanError(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewProxyHandler(db)
	db.Exec("DROP TABLE proxy_ua_blacklist")
	db.Exec("DROP TABLE proxy_ua_whitelist")
	db.Exec("DROP TABLE proxy_ip_blacklist")
	db.Exec("DROP TABLE proxy_ip_whitelist")
	db.Exec("DROP TABLE proxy_domains")
	db.Exec("DROP TABLE proxy_rules")
	db.Exec(`CREATE TABLE proxy_rules (id TEXT PRIMARY KEY, tenant_id TEXT, owner_id TEXT, name TEXT, target_url INTEGER, tls_enabled INTEGER, cert_id TEXT, force_https INTEGER, http2 INTEGER, websocket INTEGER, url_rewrite TEXT, basic_auth_user TEXT, basic_auth_hash TEXT, max_conns INTEGER, enabled INTEGER, created_at DATETIME, updated_at DATETIME)`)
	db.Exec(`INSERT INTO proxy_rules (id, name, target_url) VALUES ('p1','x',123)`)

	w := doJSON(t, h.List, http.MethodGet, "/api/v1/proxy-rules", nil)
	expectStatus(t, w, http.StatusInternalServerError)
}
