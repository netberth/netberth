// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package ddns

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netberth/netberth/internal/model"
)

func providerEngine(t *testing.T, h http.HandlerFunc) (*Engine, *mockDDNSDB) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	db := &mockDDNSDB{}
	e := New(db)
	e.baseURL = srv.URL
	return e, db
}

func TestProviderMissingCredentials(t *testing.T) {
	e := New(&mockDDNSDB{})
	cfgs := []model.DDNSConfig{
		{Provider: "cloudflare"},
		{Provider: "aliyun"},
		{Provider: "dnspod"},
		{Provider: "godaddy"},
		{Provider: "duckdns"},
		{Provider: "noip"},
		{Provider: "dynv6"},
		{Provider: "namecheap"},
		{Provider: "cloudns"},
	}
	for _, cfg := range cfgs {
		if err := e.updateDNS(cfg, "1.2.3.4"); err == nil {
			t.Errorf("provider %s: expected missing-credentials error", cfg.Provider)
		}
	}
}

func TestProviderCloudflareSuccessAndError(t *testing.T) {
	putFailed := false
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "dns_records") {
			w.Write([]byte(`{"result":[{"id":"rec1"}]}`))
			return
		}
		if putFailed {
			w.WriteHeader(400)
			w.Write([]byte(`{"success":false}`))
			return
		}
		w.WriteHeader(200)
	})
	cfg := model.DDNSConfig{
		Provider: "cloudflare", Domain: "example.com", SubDomain: "@",
		RecordType: "A", TTL: 600, Credentials: map[string]string{"api_token": "tok", "zone_id": "zone1"},
	}
	if err := e.updateCloudflare(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("cloudflare success: %v", err)
	}
	putFailed = true
	if err := e.updateCloudflare(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected cloudflare API error")
	}
}

func TestProviderAliyunUpdateAndCreate(t *testing.T) {
	// Update path: record exists, then update succeeds.
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			w.Write([]byte(`{"DomainRecords":{"Record":[{"RecordId":"rec1"}]}}`))
		default:
			w.WriteHeader(200)
		}
	})
	cfg := model.DDNSConfig{
		Provider: "aliyun", Domain: "example.com", SubDomain: "@", RecordType: "A", TTL: 600,
		Credentials: map[string]string{"access_key_id": "id", "access_key_secret": "sec"},
	}
	if err := e.updateAliyun(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("aliyun update: %v", err)
	}

	// Create path: no existing record, creation returns an id.
	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("Action") {
		case "DescribeDomainRecords":
			w.Write([]byte(`{"DomainRecords":{"Record":[]}}`))
		case "AddDomainRecord":
			w.Write([]byte(`{"RecordId":"rec2"}`))
		default:
			w.WriteHeader(200)
		}
	})
	if err := e2.updateAliyun(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("aliyun create: %v", err)
	}
}

func TestProviderDNSPodUpdateCreateAndError(t *testing.T) {
	// Update path.
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			w.Write([]byte(`{"Response":{"RecordList":[{"RecordId":1}]}}`))
		case "ModifyRecord":
			w.Write([]byte(`{"Response":{}}`))
		default:
			w.WriteHeader(200)
		}
	})
	cfg := model.DDNSConfig{
		Provider: "dnspod", Domain: "example.com", SubDomain: "www", RecordType: "A", TTL: 600,
		Credentials: map[string]string{"secret_id": "id", "secret_key": "sec"},
	}
	if err := e.updateDNSPod(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("dnspod update: %v", err)
	}

	// Create path.
	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TC-Action") == "DescribeRecordList" {
			w.Write([]byte(`{"Response":{"RecordList":[]}}`))
			return
		}
		w.Write([]byte(`{"Response":{}}`))
	})
	if err := e2.updateDNSPod(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("dnspod create: %v", err)
	}

	// Provider error path.
	e3, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-TC-Action") == "DescribeRecordList" {
			w.Write([]byte(`{"Response":{"RecordList":[{"RecordId":1}]}}`))
			return
		}
		w.Write([]byte(`{"Response":{"Error":{"Code":"X","Message":"boom"}}}`))
	})
	if err := e3.updateDNSPod(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected dnspod update error")
	}
}

func TestProviderGoDaddy(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	cfg := model.DDNSConfig{
		Provider: "godaddy", Domain: "example.com", SubDomain: "@", RecordType: "A", TTL: 600,
		Credentials: map[string]string{"api_key": "k", "api_secret": "s"},
	}
	if err := e.godaddyUpdate(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("godaddy success: %v", err)
	}

	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
	})
	if err := e2.godaddyUpdate(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected godaddy error")
	}
}

func TestProviderDuckDNS(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	cfg := model.DDNSConfig{Provider: "duckdns", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"token": "t"}}
	if err := e.duckdnsUpdate(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("duckdns success: %v", err)
	}

	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("KO"))
	})
	if err := e2.duckdnsUpdate(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected duckdns error")
	}
}

func TestProviderNoIP(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("good 1.2.3.4"))
	})
	cfg := model.DDNSConfig{Provider: "noip", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"username": "u", "password": "p"}}
	if err := e.noipUpdate(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("noip success: %v", err)
	}

	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("911"))
	})
	if err := e2.noipUpdate(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected noip error")
	}
}

func TestProviderDynv6(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("addresses updated"))
	})
	cfg := model.DDNSConfig{Provider: "dynv6", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"token": "t"}}
	if err := e.dynv6Update(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("dynv6 success: %v", err)
	}

	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid token"))
	})
	if err := e2.dynv6Update(cfg, "1.2.3.4"); err == nil {
		t.Fatal("expected dynv6 error")
	}
}

func TestProviderNamecheap(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	cfg := model.DDNSConfig{Provider: "namecheap", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"password": "p"}}
	if err := e.namecheapUpdate(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("namecheap: %v", err)
	}
}

func TestProviderCloudns(t *testing.T) {
	e, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	cfg := model.DDNSConfig{Provider: "cloudns", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"auth_id": "1", "auth_password": "p"}}
	if err := e.cloudnsUpdate(cfg, "1.2.3.4"); err != nil {
		t.Fatalf("cloudns auth-id path: %v", err)
	}

	e2, _ := providerEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	cfg2 := model.DDNSConfig{Provider: "cloudns", Domain: "example.com", SubDomain: "@", Credentials: map[string]string{"auth_id": "1", "auth_password": "p", "sub_auth_id": "sub1"}}
	if err := e2.cloudnsUpdate(cfg2, "1.2.3.4"); err != nil {
		t.Fatalf("cloudns sub-auth path: %v", err)
	}
}

func TestAliyunHelpers(t *testing.T) {
	params := map[string]string{"z": "2", "a": "1", "m": "3"}
	q := aliyunBuildQuery(params)
	if !strings.HasPrefix(q, "a=1&m=3&z=2") {
		t.Fatalf("unexpected sorted query: %s", q)
	}
	sig := aliyunSign(params, "secret", "GET")
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	sorted := aliyunSortedQuery(params)
	if !strings.HasPrefix(sorted, "a=1&m=3&z=2") {
		t.Fatalf("unexpected sorted query: %s", sorted)
	}
}

func TestDNSPodHelpersAndStringUtils(t *testing.T) {
	if got := sha256Hex([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("unexpected sha256: %s", got)
	}
	if len(hmacSHA256([]byte("k"), []byte("data"))) != 32 {
		t.Fatal("expected 32-byte hmac")
	}
	if stringsIndex("hello", "ll") != 2 || stringsIndex("hello", "zz") != -1 {
		t.Fatal("stringsIndex mismatch")
	}
	r := readerFromString("abc")
	buf := make([]byte, 2)
	n, _ := r.Read(buf)
	if n != 2 || string(buf) != "ab" {
		t.Fatalf("reader mismatch: %d %q", n, buf)
	}
}
