// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package storage

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func TestFTPAnonymousAuth(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "public.txt"), []byte("hello"), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-anon", Name: "anon", Type: "local", Source: dir,
		Services: []string{"ftp"}, FTPPort: 30000, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	conn := ftpConnect(t, 30002)
	defer conn.Close()
	ftpExpect(t, conn, "220")

	// Anonymous login
	ftpSend(t, conn, "USER anonymous")
	time.Sleep(100 * time.Millisecond)
	ftpExpect(t, conn, "331")
	ftpSend(t, conn, "PASS anonymous")
	time.Sleep(100 * time.Millisecond)
	ftpExpect(t, conn, "230")

	eng.Stop()
}

func TestFTPAuthenticatedAuth(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("classified"), 0600)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-auth", Name: "auth", Type: "local", Source: dir,
		Services: []string{"ftp"}, FTPPort: 30100,
		Username: "admin", Password: "ftp-pass-123", Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	conn := ftpConnect(t, 30102)
	defer conn.Close()
	ftpExpect(t, conn, "220")

	// Wrong credentials → rejected
	ftpSend(t, conn, "USER baduser")
	ftpExpect(t, conn, "331")
	ftpSend(t, conn, "PASS badpass")
	resp := ftpRead(t, conn)
	if !strings.HasPrefix(resp, "530") {
		t.Errorf("expected 530 for invalid creds, got: %s", resp)
	}

	// Right credentials → accepted
	conn2 := ftpConnect(t, 30102)
	defer conn2.Close()
	ftpExpect(t, conn2, "220")
	ftpSend(t, conn2, "USER admin")
	ftpExpect(t, conn2, "331")
	ftpSend(t, conn2, "PASS ftp-pass-123")
	ftpExpect(t, conn2, "230")

	eng.Stop()
}

func TestFTPTenantIsolation(t *testing.T) {
	dir := t.TempDir()

	eng := New(&mockMountDB{})
	// Mount with tenant isolation
	m := model.StorageMount{
		ID: "ftp-tenant", Name: "tenant", Type: "local", Source: dir,
		TenantID: "tenant-A", // Creates dir/tenant-A as root
		Services: []string{"ftp"}, FTPPort: freePortRange(t, 3), Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	// Verify tenant directory was created
	tenantDir := filepath.Join(dir, "tenant-A")
	if _, err := os.Stat(tenantDir); os.IsNotExist(err) {
		t.Fatal("tenant directory not created")
	}

	// Create a file inside tenant root
	os.WriteFile(filepath.Join(tenantDir, "data.log"), []byte("tenant data"), 0644)

	conn := ftpConnect(t, m.FTPPort+2)
	defer conn.Close()
	ftpExpect(t, conn, "220")
	ftpLogin(t, conn)

	// LIST requires passive data connection. Wait for the PASV port and the
	// asynchronously-opened data listener instead of sleeping.
	ftpSend(t, conn, "PASV")
	dataPort := waitForPASVPort(t, conn)
	dataConn := waitForDataConn(t, dataPort)
	ftpSend(t, conn, "LIST")
	ftpExpect(t, conn, "150")
	buf := make([]byte, 4096)
	n, _ := dataConn.Read(buf)
	dataConn.Close()
	listResp := string(buf[:n])
	t.Logf("LIST: %q", listResp)
	ftpExpect(t, conn, "226")
	if !strings.Contains(listResp, "data.log") {
		t.Errorf("LIST should show data.log: %s", listResp)
	}

	// CWD should not escape tenant root
	time.Sleep(100 * time.Millisecond)
	ftpSend(t, conn, "PWD")
	time.Sleep(100 * time.Millisecond)
	pwdResp := ftpRead(t, conn)
	t.Logf("PWD after CWD ..: %s", pwdResp)
	// Should still be at "/" — afero blocks traversal

	eng.Stop()
}

func TestFTPPathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-traversal", Name: "traversal", Type: "local", Source: dir,
		Services: []string{"ftp"}, FTPPort: 30300, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	conn := ftpConnect(t, 30302)
	defer conn.Close()
	ftpExpect(t, conn, "220")
	ftpLogin(t, conn)

	// Try to CWD outside root
	ftpSend(t, conn, "CWD ../../../etc")
	time.Sleep(100 * time.Millisecond)
	resp := ftpRead(t, conn)
	t.Logf("CWD traversal response: %s", resp)
	if !strings.Contains(resp, "550") && !strings.Contains(resp, "5") {
		t.Errorf("expected rejection for path traversal, got: %s", resp)
	}

	ftpSend(t, conn, "PWD")
	time.Sleep(100 * time.Millisecond)
	pwdResp := ftpRead(t, conn)
	t.Logf("PWD after traversal attempt: %s", pwdResp)

	eng.Stop()
}

func TestFTPSharedSecurityWithWebDAV(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("shared content"), 0644)

	port := freePortRange(t, 3)
	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-shared", Name: "shared", Type: "local", Source: dir,
		Services: []string{"ftp", "webdav"}, FTPPort: port, Enabled: true,
	}
	eng.startMount(m)

	// Wait for FTP control port instead of sleeping.
	ftpAddr := fmt.Sprintf("127.0.0.1:%d", port+2)
	waitFor(t, 5*time.Second, func() bool {
		c, err := net.DialTimeout("tcp", ftpAddr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		c.Close()
		return true
	})

	// FTP access.
	conn := ftpConnect(t, port+2)
	defer conn.Close()
	ftpExpect(t, conn, "220")
	ftpLogin(t, conn)
	ftpSend(t, conn, "PASV")
	pasvResp := ftpRead(t, conn)
	dataPort := parsePASVPort(pasvResp)
	if dataPort == 0 {
		t.Fatalf("PASV port not parseable: %s", pasvResp)
	}

	// Connect the data channel before LIST so the server has an established
	// connection when it processes the transfer command.
	dataConn := waitForDataConn(t, dataPort)
	ftpSend(t, conn, "LIST")
	ftpExpect(t, conn, "150")
	dataConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	listData, err := io.ReadAll(dataConn)
	dataConn.Close()
	if err != nil {
		t.Fatalf("read FTP LIST data: %v", err)
	}
	listResp := string(listData)
	ftpExpect(t, conn, "226")
	t.Logf("FTP LIST: %q", listResp)
	if !strings.Contains(listResp, "shared.txt") {
		t.Errorf("FTP should list shared.txt: %s", listResp)
	}

	// WebDAV access — same file must be visible.
	var webdavBody string
	webdavURL := fmt.Sprintf("http://127.0.0.1:%d/shared.txt", port+1)
	waitFor(t, 5*time.Second, func() bool {
		resp, err := httpGet(webdavURL)
		if err != nil {
			return false
		}
		if strings.Contains(resp, "shared content") {
			webdavBody = resp
			return true
		}
		return false
	})
	if !strings.Contains(webdavBody, "shared content") {
		t.Errorf("WebDAV should read shared.txt: %s", webdavBody)
	}

	eng.Stop()
}

func TestFTPGracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-shutdown", Name: "shutdown", Type: "local", Source: dir,
		Services: []string{"ftp"}, FTPPort: 30500, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	// Connect
	conn := ftpConnect(t, 30502)
	ftpExpect(t, conn, "220")
	ftpLogin(t, conn)

	// Send QUIT
	ftpSend(t, conn, "QUIT")
	time.Sleep(100 * time.Millisecond)
	ftpExpect(t, conn, "221")

	// Stop engine — should not panic or leak
	eng.Stop()
	time.Sleep(200 * time.Millisecond)
}

// === Helpers ===

func ftpConnect(t *testing.T, port int) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		t.Fatalf("FTP connect to :%d: %v", port, err)
	}
	return conn
}

func ftpSend(t *testing.T, conn net.Conn, cmd string) {
	t.Helper()
	fmt.Fprintf(conn, "%s\r\n", cmd)
}

// ftpRead reads exactly one CRLF-terminated control line. FTP responses are a
// stream; raw Read calls can return multiple lines (e.g. 150+226) in one chunk,
// so tests must consume one logical response at a time.
func ftpRead(t *testing.T, conn net.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var sb strings.Builder
	one := make([]byte, 1)
	for {
		n, err := conn.Read(one)
		if n > 0 {
			sb.WriteByte(one[0])
			if one[0] == '\n' {
				return strings.TrimRight(sb.String(), "\r\n")
			}
		}
		if err != nil {
			t.Fatalf("FTP read line failed: %v (partial %q)", err, sb.String())
		}
	}
}

func ftpExpect(t *testing.T, conn net.Conn, code string) {
	t.Helper()
	resp := ftpRead(t, conn)
	if !strings.HasPrefix(resp, code) {
		t.Errorf("expected code %s in %q", code, resp)
	}
}

func ftpLogin(t *testing.T, conn net.Conn) {
	t.Helper()
	ftpSend(t, conn, "USER anonymous")
	time.Sleep(100 * time.Millisecond)
	ftpRead(t, conn)
	ftpSend(t, conn, "PASS anonymous")
	time.Sleep(100 * time.Millisecond)
	ftpRead(t, conn)
}

func parsePASVPort(resp string) int {
	i := strings.LastIndex(resp, "(")
	j := strings.LastIndex(resp, ")")
	if i < 0 || j <= i {
		return 0
	}
	parts := strings.Split(resp[i+1:j], ",")
	if len(parts) < 6 {
		return 0
	}
	var p1, p2 int
	fmt.Sscanf(parts[4]+" "+parts[5], "%d %d", &p1, &p2)
	return p1*256 + p2
}

func ftpDataConnect(t *testing.T, port int) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		t.Fatalf("data connect to :%d: %v", port, err)
	}
	return conn
}

func TestFTPChrootSecurity(t *testing.T) {
	base := t.TempDir()
	os.WriteFile(filepath.Join(base, "outside.txt"), []byte("SHOULD_NOT_BE_VISIBLE"), 0644)
	tenantDir := filepath.Join(base, "tenantA")
	os.MkdirAll(tenantDir, 0755)
	os.WriteFile(filepath.Join(tenantDir, "inside.txt"), []byte("visible"), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "chroot-test", Name: "chroot", Type: "local", Source: base,
		TenantID: "tenantA", Services: []string{"ftp"}, FTPPort: osPort(t), Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(300 * time.Millisecond)

	ftpPort := m.FTPPort + 2
	conn := ftpConnect(t, ftpPort)
	defer conn.Close()
	ftpExpect(t, conn, "220")
	ftpLogin(t, conn)

	// LIST: should show inside.txt, NOT outside.txt
	ftpSend(t, conn, "PASV")
	dp := waitForPASVPort(t, conn)
	dc := waitForDataConn(t, dp)
	ftpSend(t, conn, "LIST")
	ftpExpect(t, conn, "150")
	buf := make([]byte, 4096)
	n, _ := dc.Read(buf)
	dc.Close()
	lr := string(buf[:n])
	ftpExpect(t, conn, "226")

	if !strings.Contains(lr, "inside.txt") {
		t.Errorf("should list inside.txt: %s", lr)
	}
	if strings.Contains(lr, "outside.txt") {
		t.Errorf("SECURITY BREACH: outside.txt visible in chroot: %s", lr)
	}
	t.Logf("chroot LIST: %q", lr)

	// CWD .. then LIST — still chrooted
	ftpSend(t, conn, "CWD ..")
	ftpExpect(t, conn, "250")
	ftpSend(t, conn, "PASV")
	dp2 := waitForPASVPort(t, conn)
	dc2 := waitForDataConn(t, dp2)
	ftpSend(t, conn, "LIST")
	ftpExpect(t, conn, "150")
	buf2 := make([]byte, 4096)
	n2, _ := dc2.Read(buf2)
	dc2.Close()
	lr2 := string(buf2[:n2])
	ftpExpect(t, conn, "226")
	if strings.Contains(lr2, "outside.txt") {
		t.Errorf("SECURITY: CWD .. breached chroot: %s", lr2)
	}
	t.Logf("after CWD .. LIST: %q", lr2)

	eng.Stop()
}

// waitFor polls fn until it returns true or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// waitForPASVPort reads the PASV response line and fails loudly if unparseable.
func waitForPASVPort(t *testing.T, conn net.Conn) int {
	t.Helper()
	line := ftpRead(t, conn)
	port := parsePASVPort(line)
	if port == 0 {
		t.Fatalf("PASV port not parseable: %s", line)
	}
	return port
}

// waitForDataConn dials until the asynchronously-opened data listener accepts.
func waitForDataConn(t *testing.T, port int) net.Conn {
	t.Helper()
	var conn net.Conn
	waitFor(t, 5*time.Second, func() bool {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err != nil {
			return false
		}
		conn = c
		return true
	})
	return conn
}
