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

func TestDDNSHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewDDNSHandler(db)

	body, _ := json.Marshal(model.DDNSConfig{
		Name: "dd", Provider: "cloudflare", Domain: "example.com", SubDomain: "@",
		RecordType: "A", TTL: 600, Credentials: map[string]string{"api_token": "tok"},
		GetIPURL: "https://api.ipify.org", GetIPType: "url", Interval: 300, Enabled: true,
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/ddns", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.DDNSConfig `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/ddns?page=1&page_size=10", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data  []model.DDNSConfig `json:"data"`
		Total int                `json:"total"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Data[0].Credentials["api_token"] != "tok" {
		t.Fatalf("ddns list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.DDNSConfig{Name: "dd2", Provider: "duckdns", Domain: "example.net", SubDomain: "@", RecordType: "A", Credentials: map[string]string{"token": "x"}, Interval: 600})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/ddns/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)
	var name string
	db.QueryRow("SELECT name FROM ddns_configs WHERE id=?", id).Scan(&name)
	if name != "dd2" {
		t.Fatalf("expected dd2, got %s", name)
	}

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/ddns/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	var createdN, updatedN, deletedN int32
	h.SetNotifier(&Notifier{
		OnCreate: func(resource, id string) { atomic.AddInt32(&createdN, 1) },
		OnUpdate: func(resource, id string) { atomic.AddInt32(&updatedN, 1) },
		OnDelete: func(resource, id string) { atomic.AddInt32(&deletedN, 1) },
	})
	body2, _ := json.Marshal(model.DDNSConfig{Name: "n", Provider: "noip", Domain: "x.example.com", SubDomain: "@", Credentials: map[string]string{}, Interval: 300})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/ddns", body2)
	expectStatus(t, w2, http.StatusCreated)
	var c2 struct {
		Data model.DDNSConfig `json:"data"`
	}
	decodeResponse(t, w2, &c2)
	doPathJSON(t, h.Update, http.MethodPut, "/api/v1/ddns/"+c2.Data.ID, c2.Data.ID, body2)
	doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/ddns/"+c2.Data.ID, c2.Data.ID, nil)
	if atomic.LoadInt32(&createdN) != 1 || atomic.LoadInt32(&updatedN) != 1 || atomic.LoadInt32(&deletedN) != 1 {
		t.Fatalf("notifier counters: %d/%d/%d", createdN, updatedN, deletedN)
	}

	w3 := doJSON(t, h.Create, http.MethodPost, "/api/v1/ddns", []byte("{"))
	expectStatus(t, w3, http.StatusBadRequest)
	w4 := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/ddns/x", "x", []byte("{"))
	expectStatus(t, w4, http.StatusBadRequest)
}

func TestSTUNHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewSTUNHandler(db)

	body, _ := json.Marshal(model.STUNTunnel{
		Name: "st", Protocol: "tcp", LocalPort: 30001, RemotePort: 30002,
		STUNServer: "stun.example.com:19302", TargetAddr: "192.0.2.30", TargetPort: 80, Enabled: true,
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/stun", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.STUNTunnel `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/stun?page=1", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.STUNTunnel `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Data[0].STUNServer != "stun.example.com:19302" {
		t.Fatalf("stun list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.STUNTunnel{Name: "st2", Protocol: "udp", LocalPort: 30003, RemotePort: 30004, STUNServer: "stun2.example.com:19302", TargetAddr: "192.0.2.31", TargetPort: 81})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/stun/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)

	var updatedN, deletedN int32
	h.SetNotifier(&Notifier{
		OnUpdate: func(resource, id string) { atomic.AddInt32(&updatedN, 1) },
		OnDelete: func(resource, id string) { atomic.AddInt32(&deletedN, 1) },
	})
	doPathJSON(t, h.Update, http.MethodPut, "/api/v1/stun/"+id, id, upd)
	doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/stun/"+id, id, nil)
	if atomic.LoadInt32(&updatedN) != 1 || atomic.LoadInt32(&deletedN) != 1 {
		t.Fatalf("notifier counters: %d/%d", updatedN, deletedN)
	}

	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/stun", []byte("{"))
	expectStatus(t, w2, http.StatusBadRequest)
}

func TestWOLHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewWOLHandler(db)

	body, _ := json.Marshal(model.WOLDevice{Name: "pc", MAC: "AA:BB:CC:DD:EE:FF", Broadcast: "255.255.255.255", Port: 9})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/wol", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.WOLDevice `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/wol", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.WOLDevice `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Data[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("wol list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.WOLDevice{Name: "pc2", MAC: "AA:BB:CC:DD:EE:00", Broadcast: "255.255.255.255", Port: 9})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/wol/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)

	wk := doPathJSON(t, h.Wake, http.MethodPost, "/api/v1/wol/"+id+"/wake", id, nil)
	expectStatus(t, wk, http.StatusOK)

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/wol/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/wol", []byte("{"))
	expectStatus(t, w2, http.StatusBadRequest)
}

func TestCronHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewCronHandler(db)

	body, _ := json.Marshal(model.CronJob{Name: "job", Schedule: "*/5 * * * *", Type: "shell", Command: "echo hi", Enabled: true})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/cron", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.CronJob `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/cron", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.CronJob `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || listed.Data[0].Schedule != "*/5 * * * *" {
		t.Fatalf("cron list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.CronJob{Name: "job2", Schedule: "0 * * * *", Type: "module", ModuleID: "m1", ModuleType: "forward"})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/cron/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/cron/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/cron", []byte("{"))
	expectStatus(t, w2, http.StatusBadRequest)
}

func TestACMEHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewACMEHandler(db)

	body, _ := json.Marshal(model.ACMECertificate{
		Name: "cert", Domains: []string{"example.com"}, Provider: "letsencrypt",
		DNSProvider: "cloudflare", DNSConfig: map[string]string{"token": "x"},
		Email: "admin@example.com", AutoRenew: true, RenewDays: 30,
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/acme", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.ACMECertificate `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/acme", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.ACMECertificate `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || len(listed.Data[0].Domains) != 1 || listed.Data[0].Status != "pending" {
		t.Fatalf("acme list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.ACMECertificate{Name: "cert2", Domains: []string{"example.net"}, Email: "ops@example.com", AutoRenew: false, RenewDays: 15})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/acme/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/acme/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	badDomain, _ := json.Marshal(model.ACMECertificate{Name: "x", Domains: []string{"bad domain"}, Email: "a@b.com"})
	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/acme", badDomain)
	expectStatus(t, w2, http.StatusBadRequest)

	badEmail, _ := json.Marshal(model.ACMECertificate{Name: "x", Domains: []string{"example.com"}, Email: "not-an-email"})
	w3 := doJSON(t, h.Create, http.MethodPost, "/api/v1/acme", badEmail)
	expectStatus(t, w3, http.StatusBadRequest)

	w4 := doJSON(t, h.Create, http.MethodPost, "/api/v1/acme", []byte("{"))
	expectStatus(t, w4, http.StatusBadRequest)
}

func TestStorageHandlerFullCRUD(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewStorageHandler(db)

	body, _ := json.Marshal(model.StorageMount{
		Name: "mnt", Type: "local", Source: "/mnt/user/media", Services: []string{"webdav", "ftp"}, FTPPort: 2121, Enabled: true,
	})
	w := doJSON(t, h.Create, http.MethodPost, "/api/v1/storage", body)
	expectStatus(t, w, http.StatusCreated)
	var created struct {
		Data model.StorageMount `json:"data"`
	}
	decodeResponse(t, w, &created)
	id := created.Data.ID

	lw := doJSON(t, h.List, http.MethodGet, "/api/v1/storage", nil)
	expectStatus(t, lw, http.StatusOK)
	var listed struct {
		Data []model.StorageMount `json:"data"`
	}
	decodeResponse(t, lw, &listed)
	if len(listed.Data) != 1 || len(listed.Data[0].Services) != 2 {
		t.Fatalf("storage list mismatch: %+v", listed.Data)
	}

	upd, _ := json.Marshal(model.StorageMount{Name: "mnt2", Type: "local", Source: "/mnt/user/other", Services: []string{"filebrowser"}})
	uw := doPathJSON(t, h.Update, http.MethodPut, "/api/v1/storage/"+id, id, upd)
	expectStatus(t, uw, http.StatusOK)

	dw := doPathJSON(t, h.Delete, http.MethodDelete, "/api/v1/storage/"+id, id, nil)
	expectStatus(t, dw, http.StatusOK)

	w2 := doJSON(t, h.Create, http.MethodPost, "/api/v1/storage", []byte("{"))
	expectStatus(t, w2, http.StatusBadRequest)
}

func TestACMEListScanError(t *testing.T) {
	db := setupFullTestDB(t)
	h := NewACMEHandler(db)
	db.Exec("DROP TABLE acme_certificates")
	db.Exec(`CREATE TABLE acme_certificates (id TEXT PRIMARY KEY, tenant_id TEXT, name TEXT, domains TEXT, provider TEXT, dns_provider TEXT, dns_config TEXT, email TEXT, auto_renew INTEGER, renew_days INTEGER, cert_path TEXT, key_path TEXT, expires_at DATETIME, status INTEGER, error TEXT, created_at DATETIME, updated_at DATETIME)`)
	db.Exec(`INSERT INTO acme_certificates (id, name, status) VALUES ('a1','x',0)`)

	w := doJSON(t, h.List, http.MethodGet, "/api/v1/acme", nil)
	expectStatus(t, w, http.StatusInternalServerError)
}
