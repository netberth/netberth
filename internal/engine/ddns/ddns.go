// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package ddns

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
)

type Engine struct {
	mu   sync.RWMutex
	jobs map[string]context.CancelFunc
	db   interface {
		GetConfigs() ([]model.DDNSConfig, error)
		UpdateIP(id, ip string) error
	}
	stopCh chan struct{}
}

func New(db interface {
	GetConfigs() ([]model.DDNSConfig, error)
	UpdateIP(id, ip string) error
}) *Engine {
	return &Engine{
		jobs:   make(map[string]context.CancelFunc),
		db:     db,
		stopCh: make(chan struct{}),
	}
}

func (e *Engine) Start() error {
	configs, err := e.db.GetConfigs()
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		if cfg.Enabled {
			e.startJob(cfg)
		}
	}
	return nil
}

func (e *Engine) Stop() {
	close(e.stopCh)
	e.mu.Lock()
	for _, cancel := range e.jobs {
		cancel()
	}
	e.mu.Unlock()
}

func (e *Engine) Reload(cfg model.DDNSConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel, exists := e.jobs[cfg.ID]; exists {
		cancel()
	}
	if cfg.Enabled {
		e.startJob(cfg)
	} else {
		delete(e.jobs, cfg.ID)
	}
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel, exists := e.jobs[id]; exists {
		cancel()
		delete(e.jobs, id)
	}
}

func (e *Engine) startJob(cfg model.DDNSConfig) {
	ctx, cancel := context.WithCancel(context.Background())
	e.jobs[cfg.ID] = cancel
	go e.run(ctx, cfg)
}

func (e *Engine) run(ctx context.Context, cfg model.DDNSConfig) {
	interval := time.Duration(cfg.Interval) * time.Second
	if interval < 60*time.Second {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	e.update(cfg)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.update(cfg)
		}
	}
}

func (e *Engine) update(cfg model.DDNSConfig) {
	ip, err := e.getPublicIP(cfg)
	if err != nil {
		logger.Log.Warn().Err(err).Str("name", cfg.Name).Msg("ddns get IP failed")
		return
	}
	if ip == "" {
		return
	}
	if err := e.updateDNS(cfg, ip); err != nil {
		logger.Log.Error().Err(err).Str("name", cfg.Name).Str("ip", ip).Msg("ddns update failed")
		return
	}
	e.db.UpdateIP(cfg.ID, ip)
	logger.Log.Info().Str("name", cfg.Name).Str("domain", cfg.Domain).Str("ip", ip).Msg("ddns updated")
}

func (e *Engine) getPublicIP(cfg model.DDNSConfig) (string, error) {
	if cfg.GetIPType == "interface" && cfg.NetInterface != "" {
		return e.getInterfaceIP(cfg.NetInterface)
	}
	url := cfg.GetIPURL
	if url == "" {
		url = "https://api.ipify.org"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (e *Engine) getInterfaceIP(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.IsGlobalUnicast() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no public IPv4 found on interface %s", name)
}

func (e *Engine) updateDNS(cfg model.DDNSConfig, ip string) error {
	switch cfg.Provider {
	case "cloudflare":
		return e.updateCloudflare(cfg, ip)
	case "aliyun":
		return e.updateAliyun(cfg, ip)
	case "dnspod", "tencent":
		return e.updateDNSPod(cfg, ip)
	case "godaddy":
		return godaddyUpdate(cfg, ip)
	case "duckdns":
		return duckdnsUpdate(cfg, ip)
	case "noip":
		return noipUpdate(cfg, ip)
	case "dynv6":
		return dynv6Update(cfg, ip)
	case "namecheap":
		return namecheapUpdate(cfg, ip)
	case "cloudns":
		return cloudnsUpdate(cfg, ip)
	default:
		return fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func (e *Engine) updateCloudflare(cfg model.DDNSConfig, ip string) error {
	apiToken := cfg.Credentials["api_token"]
	zoneID := cfg.Credentials["zone_id"]
	if apiToken == "" || zoneID == "" {
		return fmt.Errorf("missing cloudflare credentials: api_token, zone_id")
	}
	recordID, err := e.cloudflareGetRecordID(apiToken, zoneID, cfg.SubDomain, cfg.Domain)
	if err != nil {
		return fmt.Errorf("get record: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	body := fmt.Sprintf(`{"type":"%s","name":"%s.%s","content":"%s","ttl":%d,"proxied":false}`,
		cfg.RecordType, cfg.SubDomain, cfg.Domain, ip, cfg.TTL)
	req, _ := http.NewRequest("PUT", url, io.NopCloser(
		readerFromString(body),
	))
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloudflare API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (e *Engine) cloudflareGetRecordID(token, zoneID, subDomain, domain string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	name := subDomain
	if name != "@" {
		name = subDomain + "." + domain
	} else {
		name = domain
	}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?type=A&name=%s", zoneID, name)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	// Simple extraction — in production use proper JSON parsing
	// Find "id":" in the response
	body := string(respBody)
	idStart := stringsIndex(body, `"id":"`)
	if idStart < 0 {
		return "", fmt.Errorf("record not found in: %s", body)
	}
	idStart += 6
	idEnd := stringsIndex(body[idStart:], `"`)
	if idEnd < 0 {
		return "", fmt.Errorf("malformed response")
	}
	return body[idStart : idStart+idEnd], nil
}

func readerFromString(s string) io.Reader {
	return &stringReader{s: s, i: 0}
}

type stringReader struct {
	s string
	i int64
}

func (r *stringReader) Read(b []byte) (int, error) {
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n := copy(b, r.s[r.i:])
	r.i += int64(n)
	return n, nil
}

func stringsIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
