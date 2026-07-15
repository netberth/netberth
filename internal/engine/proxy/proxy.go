// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
)

type routeEntry struct {
	rule    model.ProxyRule
	handler http.Handler
}

type Engine struct {
	mu       sync.RWMutex
	routes   map[string]*routeEntry // domain -> entry
	server   *http.Server
	listener net.Listener
	stopCh   chan struct{}
	db       interface {
		GetRules() ([]model.ProxyRule, error)
	}
	port string
}

func New(db interface {
	GetRules() ([]model.ProxyRule, error)
}) *Engine {
	return &Engine{
		routes: make(map[string]*routeEntry),
		stopCh: make(chan struct{}),
		db:     db,
	}
}

func (e *Engine) Start(addr string) error {
	_, port, _ := net.SplitHostPort(addr)
	if port == "" {
		port = "80"
	}
	e.port = port

	rules, err := e.db.GetRules()
	if err != nil {
		return fmt.Errorf("load proxy rules: %w", err)
	}
	for _, rule := range rules {
		if rule.Enabled {
			e.addRoute(rule)
		}
	}

	e.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("proxy listen: %w", err)
	}

	e.server = &http.Server{
		Handler:      http.HandlerFunc(e.serveHTTP),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		logger.Log.Info().Str("addr", addr).Msg("reverse proxy started")
		if err := e.server.Serve(e.listener); err != nil && err != http.ErrServerClosed {
			logger.Log.Error().Err(err).Msg("proxy server error")
		}
	}()
	return nil
}

func (e *Engine) Stop() {
	if e.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.server.Shutdown(ctx)
	}
	if e.listener != nil {
		e.listener.Close()
	}
}

func (e *Engine) Reload(rule model.ProxyRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.routes, ruleKey(rule))
	if rule.Enabled {
		e.addRoute(rule)
	}
	logger.Log.Info().Str("name", rule.Name).Bool("enabled", rule.Enabled).Msg("proxy rule reloaded")
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range e.routes {
		if v.rule.ID == id {
			delete(e.routes, k)
			logger.Log.Info().Str("id", id).Msg("proxy rule removed")
			return
		}
	}
}

func (e *Engine) serveHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.Split(r.Host, ":")[0]

	e.mu.RLock()
	entry, ok := e.routes[host]
	// Fallback: try wildcard match
	if !ok {
		for domain, ent := range e.routes {
			if strings.HasPrefix(domain, "*.") {
				suffix := domain[1:] // .example.com
				if strings.HasSuffix(host, suffix) {
					entry = ent
					break
				}
			}
		}
	}
	e.mu.RUnlock()

	if entry == nil {
		http.Error(w, "502 Bad Gateway — no upstream configured for this host", http.StatusBadGateway)
		return
	}
	entry.handler.ServeHTTP(w, r)
}

func (e *Engine) addRoute(rule model.ProxyRule) {
	entry := &routeEntry{rule: rule, handler: buildHandler(rule)}
	for _, domain := range rule.Domains {
		e.routes[domain] = entry
	}
}

func ruleKey(rule model.ProxyRule) string {
	if len(rule.Domains) > 0 {
		return rule.Domains[0]
	}
	return rule.ID
}

func buildHandler(rule model.ProxyRule) http.Handler {
	target, err := url.Parse(rule.TargetURL)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Invalid upstream URL", http.StatusInternalServerError)
		})
	}

	if rule.Websocket {
		return &wsProxy{target: target}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: false,
	}

	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Forwarded-For", clientIP(req))
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Real-IP", clientIP(req))
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-Powered-By", "NetBerth")
		return nil
	}

	wl := make([]string, len(rule.IPWhitelist))
	for i, e := range rule.IPWhitelist {
		wl[i] = e.Value
	}
	bl := make([]string, len(rule.IPBlacklist))
	for i, e := range rule.IPBlacklist {
		bl[i] = e.Value
	}

	return &aclHandler{
		proxy:       proxy,
		ipWhitelist: wl,
		ipBlacklist: bl,
		basicUser:   rule.BasicAuthUser,
		basicPass:   rule.BasicAuthHash,
	}
}

type aclHandler struct {
	proxy                    http.Handler
	ipWhitelist, ipBlacklist []string
	basicUser, basicPass     string
}

func (h *aclHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	for _, blocked := range h.ipBlacklist {
		if ip == blocked {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}
	if len(h.ipWhitelist) > 0 {
		allowed := false
		for _, a := range h.ipWhitelist {
			if ip == a {
				allowed = true
				break
			}
		}
		if !allowed {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}
	if h.basicUser != "" {
		u, p, ok := r.BasicAuth()
		if !ok || u != h.basicUser || p != h.basicPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="NetBerth"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	h.proxy.ServeHTTP(w, r)
}

type wsProxy struct {
	target *url.URL
}

func (h *wsProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy := httputil.NewSingleHostReverseProxy(h.target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	proxy.ServeHTTP(w, r)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
