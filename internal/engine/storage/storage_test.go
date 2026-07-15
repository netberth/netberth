// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package storage

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func TestFTPServerStartStop(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID:       "test-ftp",
		Name:     "test",
		Type:     "local",
		Source:   dir,
		Services: []string{"ftp"},
		FTPPort: osPort(t), // FTP actual port = 29000+2 = 29002
		Enabled:  true,
	}
	eng.startMount(m)
	time.Sleep(500 * time.Millisecond)

	// Connect to FTP server
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.FTPPort+2), 2*time.Second)
	if err != nil {
		t.Fatalf("cannot connect to FTP: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("FTP read greeting: %v", err)
	}
	greeting := string(buf[:n])
	t.Logf("FTP greeting: %s", greeting)
	if len(greeting) == 0 {
		t.Fatal("expected non-empty FTP greeting")
	}

	// Graceful shutdown
	eng.mu.RLock()
	inst := eng.mounts["test-ftp"]
	eng.mu.RUnlock()
	if inst == nil {
		t.Fatal("FTP mount not found")
	}
	eng.Stop()
	time.Sleep(200 * time.Millisecond)
}

func TestFTPReloadCycle(t *testing.T) {
	dir := t.TempDir()
	eng := New(&mockMountDB{})

	m := model.StorageMount{
		ID:       "reload",
		Name:     "reload",
		Type:     "local",
		Source:   dir,
		Services: []string{"ftp"},
		FTPPort: osPort(t),
		Enabled:  true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	eng.mu.RLock()
	_, exists := eng.mounts["reload"]
	eng.mu.RUnlock()
	if !exists {
		t.Fatal("mount should exist")
	}

	// Disable
	m.Enabled = false
	eng.Reload(m)
	time.Sleep(200 * time.Millisecond)

	eng.mu.RLock()
	_, exists = eng.mounts["reload"]
	eng.mu.RUnlock()
	if exists {
		t.Fatal("mount should be gone after disable")
	}

	eng.Stop()
}

func TestFileBrowser(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hello</h1>"), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID:       "fb",
		Name:     "fb",
		Type:     "local",
		Source:   dir,
		Services: []string{"filebrowser"},
		FTPPort: osPort(t),
		Enabled:  true,
	}
	eng.startMount(m)
	time.Sleep(100 * time.Millisecond)

	body, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d/index.html", m.FTPPort))
	if err != nil {
		t.Fatalf("filebrowser: %v", err)
	}
	if body != "<h1>hello</h1>" {
		t.Errorf("got %q", body)
	}
	eng.Stop()
}

func TestWebDAV(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("webdav content"), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID:       "dav",
		Name:     "dav",
		Type:     "local",
		Source:   dir,
		Services: []string{"webdav"},
		FTPPort: osPort(t),
		Enabled:  true,
	}
	eng.startMount(m)
	time.Sleep(100 * time.Millisecond)

	// WebDAV port = FTPPort + 1 = 29301
	body, err := httpGet(fmt.Sprintf("http://127.0.0.1:%d/test.txt", m.FTPPort+1))
	if err != nil {
		t.Fatalf("webdav: %v", err)
	}
	if body != "webdav content" {
		t.Errorf("got %q", body)
	}
	eng.Stop()
}

func TestEngineStartStop(t *testing.T) {
	eng := New(&mockMountDB{})
	if err := eng.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	eng.Stop()
}

func httpGet(url string) (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

type mockMountDB struct{}

func (m *mockMountDB) GetMounts() ([]model.StorageMount, error) { return nil, nil }

func osPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatalf("os port: %v", err) }
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestWebDAVPathIsolationReal(t *testing.T) {
	base := t.TempDir()
	port := 29000 + int(time.Now().UnixNano()%20000)

	// Tenant A mount: Source=base, TenantID=tenantA → real root = base/tenantA/
	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "tenant-a-mount", Name: "tenant-a", Type: "local", Source: base,
		TenantID: "tenantA", Services: []string{"webdav"}, FTPPort: port, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(200 * time.Millisecond)

	webdavPort := port + 1
	tenantARoot := filepath.Join(base, "tenantA")
	os.WriteFile(filepath.Join(tenantARoot, "a.txt"), []byte("tenant-A-file"), 0644)
	// Tenant B's file OUTSIDE A's root
	os.MkdirAll(filepath.Join(base, "tenantB"), 0755)
	os.WriteFile(filepath.Join(base, "tenantB", "b.txt"), []byte("tenant-B-file"), 0644)

	addr := fmt.Sprintf("127.0.0.1:%d", webdavPort)

	// Valid: tenant A reads its own file
	resp, err := httpGet("http://" + addr + "/a.txt")
	if err != nil { t.Fatalf("valid access: %v", err) }
	if resp != "tenant-A-file" { t.Errorf("expected tenant-A-file, got %q", resp) }

	// Raw HTTP traversal: Go http.Get normalizes ../ before sending.
	// Use raw TCP to send request with literal ../ path.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil { t.Fatalf("raw dial: %v", err) }
	fmt.Fprintf(conn, "GET /../tenantB/b.txt HTTP/1.0\r\nHost: localhost\r\n\r\n")
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(buf)
	conn.Close()
	rawResp := string(buf[:n])
	if strings.Contains(rawResp, "tenant-B-file") {
		t.Error("raw ../ traversal should be blocked — tenant A must not read tenant B files")
	}
	t.Logf("raw traversal response: %s", strings.SplitN(rawResp, "\r\n", 2)[0])

	// Parse and follow 307 redirect
	if strings.Contains(rawResp, "307") {
		loc := ""
		for _, line := range strings.Split(rawResp, "\r\n") {
			if strings.HasPrefix(strings.ToLower(line), "location:") {
				loc = strings.TrimSpace(line[len("location:"):])
				break
			}
		}
		t.Logf("307 redirect to: %s", loc)
		// Follow redirect — should NOT serve tenant B file
		if loc != "" {
			followResp, _ := httpGet("http://" + addr + loc)
			if strings.Contains(followResp, "tenant-B-file") {
				t.Error("redirected path should NOT expose tenant B file")
			}
			t.Logf("redirect follow response: %q", strings.TrimSpace(followResp))
		}
	}

	// URL-encoded traversal: ..%2F (Go client sends as-is, server must decode + reject)
	resp3, _ := httpGet("http://" + addr + "/..%2FtenantB%2Fb.txt")
	if strings.Contains(resp3, "tenant-B-file") {
		t.Error("URL-encoded traversal should be blocked")
	}
	t.Logf("encoded traversal: %q", resp3)

	// Absolute path
	resp4, _ := httpGet("http://" + addr + "/etc/passwd")
	if resp4 != "" { t.Logf("absolute path: %q", resp4) }

	eng.Stop()
}



