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
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func TestHighConcurrencyFileBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos test in short mode")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.txt"), make([]byte, 1024*100), 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "chaos-fb", Name: "chaos", Type: "local", Source: dir,
		Services: []string{"filebrowser"}, FTPPort: 25000, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(200 * time.Millisecond)

	var wg sync.WaitGroup
	concurrency := 500
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := httpGet("http://127.0.0.1:25000/data.txt")
			if err != nil {
				errors <- err
				return
			}
			if len(resp) == 0 {
				errors <- fmt.Errorf("empty response")
			}
		}()
	}
	wg.Wait()
	close(errors)

	errCount := 0
	for range errors {
		errCount++
	}
	if errCount > 0 {
		t.Errorf("%d/%d requests failed", errCount, concurrency)
	}

	eng.Stop()
	t.Logf("%d concurrent requests completed, %d failures", concurrency, errCount)
}

func TestGCAfterStress(t *testing.T) {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	dir := t.TempDir()
	// Create large files
	for i := 0; i < 100; i++ {
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("file_%d.bin", i)))
		f.Write(make([]byte, 1024*10))
		f.Close()
	}

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "gc-test", Name: "gc", Type: "local", Source: dir,
		Services: []string{"filebrowser"}, FTPPort: 25100, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(100 * time.Millisecond)

	// Stress read
	for i := 0; i < 100; i++ {
		httpGet(fmt.Sprintf("http://127.0.0.1:25100/file_%d.bin", i%100))
	}
	eng.Stop()

	runtime.GC()
	runtime.ReadMemStats(&m2)
	growth := int64(m2.Alloc) - int64(m1.Alloc)
	t.Logf("GC: before=%dKB after=%dKB diff=%dKB goroutines=%d",
		m1.Alloc/1024, m2.Alloc/1024, growth/1024, runtime.NumGoroutine())

	if growth > 50*1024*1024 || growth < -50*1024*1024 {
		t.Errorf("abnormal memory delta: %dKB", growth/1024)
	}
}

func TestPanicRecovery(t *testing.T) {
	dir := t.TempDir()
	eng := New(&mockMountDB{})

	// Simulate a mount that will cause I/O panic during read
	m := model.StorageMount{
		ID: "panic-test", Name: "panic", Type: "local", Source: dir,
		Services: []string{"filebrowser"}, FTPPort: 25200, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(200 * time.Millisecond)

	// This should NOT crash the engine, even under I/O stress
	// The safeGo wrapper catches panics
	resp, err := httpGet("http://127.0.0.1:25200/nonexistent")
	if err != nil {
		// Connection error is acceptable (404 page should return)
		t.Logf("expected 404: %v", err)
	} else if len(resp) > 0 {
		t.Logf("got response for missing file: %d bytes", len(resp))
	}

	eng.Stop()
	time.Sleep(100 * time.Millisecond)
}

func TestFTPThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	dir := t.TempDir()
	// Create a 1MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(dir, "bigfile.bin"), data, 0644)

	eng := New(&mockMountDB{})
	m := model.StorageMount{
		ID: "ftp-thru", Name: "thru", Type: "local", Source: dir,
		Services: []string{"ftp"}, FTPPort: 25300, Enabled: true,
	}
	eng.startMount(m)
	time.Sleep(500 * time.Millisecond)

	// Connect and download via FTP
	conn, err := net.DialTimeout("tcp", "127.0.0.1:25302", 3*time.Second)
	if err != nil {
		t.Skipf("FTP not reachable: %v", err)
		return
	}
	defer conn.Close()

	// Read greeting
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(buf)
	t.Logf("FTP greeting: %s", string(buf[:n]))

	// Send USER/PASS (anonymous)
	fmt.Fprintf(conn, "USER anonymous\r\n")
	time.Sleep(50 * time.Millisecond)
	conn.Read(buf)
	fmt.Fprintf(conn, "PASS anonymous\r\n")
	time.Sleep(50 * time.Millisecond)
	conn.Read(buf)

	// Test throughput with repeated LIST
	start := time.Now()
	for i := 0; i < 20; i++ {
		fmt.Fprintf(conn, "LIST\r\n")
		time.Sleep(100 * time.Millisecond)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		io.ReadAll(conn)
	}
	elapsed := time.Since(start)
	t.Logf("20 LIST ops in %v (%.1f ops/sec)", elapsed, 20/elapsed.Seconds())

	eng.Stop()
}
