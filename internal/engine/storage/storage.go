// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package storage

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	ftpserverlib "github.com/fclairamb/ftpserverlib"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
	"github.com/spf13/afero"
)

type Engine struct {
	mu     sync.RWMutex
	mounts map[string]*mountInstance
	db     interface {
		GetMounts() ([]model.StorageMount, error)
	}
}

type mountInstance struct {
	cfg     model.StorageMount
	servers []service
}

type service struct {
	kind   string
	ln     net.Listener
	srv    *http.Server
	ftpSrv *ftpserverlib.FtpServer
}

func New(db interface {
	GetMounts() ([]model.StorageMount, error)
}) *Engine {
	return &Engine{
		mounts: make(map[string]*mountInstance),
		db:     db,
	}
}

func (e *Engine) Start() error {
	mounts, err := e.db.GetMounts()
	if err != nil {
		return err
	}
	for _, m := range mounts {
		if m.Enabled {
			e.startMount(m)
		}
	}
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, inst := range e.mounts {
		e.stopMount(inst)
		delete(e.mounts, id)
	}
}

func (e *Engine) Reload(m model.StorageMount) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inst, exists := e.mounts[m.ID]; exists {
		e.stopMount(inst)
		delete(e.mounts, m.ID)
	}
	if m.Enabled {
		e.startMount(m)
	}
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inst, exists := e.mounts[id]; exists {
		e.stopMount(inst)
		delete(e.mounts, id)
	}
}

func (e *Engine) stopMount(inst *mountInstance) {
	for _, svc := range inst.servers {
		switch svc.kind {
		case "filebrowser", "webdav":
			if svc.ln != nil {
				svc.ln.Close()
			}
		case "ftp":
			if svc.ftpSrv != nil {
				svc.ftpSrv.Stop()
			}
		}
	}
}

func (e *Engine) startMount(m model.StorageMount) {
	inst := &mountInstance{cfg: m}
	root := m.Source
	// Tenant isolation: each tenant gets a subdirectory
	if m.TenantID != "" {
		root = filepath.Join(m.Source, m.TenantID)
	}
	if m.Type == "local" {
		if err := os.MkdirAll(root, 0755); err != nil {
			logger.Log.Error().Err(err).Str("path", root).Msg("storage mkdir failed")
			return
		}
	}
	m.Source = root
	for _, name := range m.Services {
		if s := e.startService(m, name); s != nil {
			inst.servers = append(inst.servers, *s)
		}
	}
	e.mounts[m.ID] = inst
	logger.Log.Info().Str("name", m.Name).Str("path", m.Source).Msg("storage mounted")
}

func (e *Engine) startService(m model.StorageMount, name string) *service {
	switch name {
	case "filebrowser":
		return e.fileBrowser(m)
	case "webdav":
		return e.webdav(m)
	case "ftp":
		return e.ftp(m)
	}
	return nil
}

func (e *Engine) fileBrowser(m model.StorageMount) *service {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", m.FTPPort))
	if err != nil {
		return nil
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(m.Source))}
	go safeGo(func() { srv.Serve(ln) }, "filebrowser")
	return &service{kind: "filebrowser", ln: ln, srv: srv}
}

func (e *Engine) webdav(m model.StorageMount) *service {
	root := m.Source
	fs := http.FileServer(http.Dir(root))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean(r.URL.Path))
		if !strings.HasPrefix(path, filepath.Clean(root)) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if info, err := os.Stat(path); err != nil {
			http.NotFound(w, r)
			return
		} else if info.IsDir() {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		fs.ServeHTTP(w, r)
	})
	port := m.FTPPort + 1
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil
	}
	srv := &http.Server{Handler: mux}
	go safeGo(func() { srv.Serve(ln) }, "webdav")
	return &service{kind: "webdav", ln: ln, srv: srv}
}

func (e *Engine) ftp(m model.StorageMount) *service {
	port := m.FTPPort + 2
	if port > 65535 {
		port = 2121
	}

	driver := &ftpMainDriver{
		root:     m.Source,
		username: m.Username,
		password: m.Password,
		addr:     fmt.Sprintf(":%d", port),
	}

	srv := ftpserverlib.NewFtpServer(driver)
	go safeGo(func() {
		if err := srv.ListenAndServe(); err != nil {
			logger.Log.Error().Err(err).Str("path", m.Source).Msg("FTP server error")
		}
	}, "ftp")
	logger.Log.Info().Int("port", port).Str("path", m.Source).Msg("FTP server started")
	return &service{kind: "ftp", ftpSrv: srv}
}

// ftpMainDriver implements ftpserverlib.MainDriver
type ftpMainDriver struct {
	root     string
	username string
	password string
	addr     string
}

func (d *ftpMainDriver) GetSettings() (*ftpserverlib.Settings, error) {
	return &ftpserverlib.Settings{
		ListenAddr: d.addr,
		Banner:     "NetBerth FTP Server — Ready",
	}, nil
}

func (d *ftpMainDriver) ClientConnected(cc ftpserverlib.ClientContext) (string, error) {
	return "NetBerth FTP ready", nil
}

func (d *ftpMainDriver) ClientDisconnected(cc ftpserverlib.ClientContext) {}
func (d *ftpMainDriver) GetTLSConfig() (*tls.Config, error)               { return nil, nil }

func (d *ftpMainDriver) AuthUser(cc ftpserverlib.ClientContext, user, pass string) (ftpserverlib.ClientDriver, error) {
	if d.username == "" && d.password == "" {
		return d.clientDriver(), nil
	}
	if user == d.username && pass == d.password {
		return d.clientDriver(), nil
	}
	return nil, fmt.Errorf("invalid credentials")
}

func (d *ftpMainDriver) clientDriver() ftpserverlib.ClientDriver {
	return &ftpClientDriver{Fs: afero.NewBasePathFs(afero.NewOsFs(), d.root)}
}

// ftpClientDriver wraps afero filesystem for ftpserverlib
type ftpClientDriver struct {
	afero.Fs
}

func (d *ftpClientDriver) MkdirAll(path string, perm os.FileMode) error {
	return d.Fs.MkdirAll(path, perm)
}

func safeGo(fn func(), name string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Log.Error().Str("component", name).Interface("panic", r).Str("stack", string(debug.Stack())).Msg("storage panic recovered")
		}
	}()
	fn()
}
