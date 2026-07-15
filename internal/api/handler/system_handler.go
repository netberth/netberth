// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/netberth/netberth/pkg/utils"
)

// StartTime is the process start timestamp. Set from main.go during initialization.
var StartTime = time.Now()

type SystemHandler struct{ db *sql.DB }

func NewSystemHandler(db *sql.DB) *SystemHandler { return &SystemHandler{db: db} }

func (h *SystemHandler) Status(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	status := map[string]interface{}{
		"hostname":   hostname,
		"version":    "1.0.0-rc1",
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpu_count":  runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"memory_mb":  memStats.Alloc / 1024 / 1024,
		"uptime":     int64(time.Since(StartTime).Seconds()),
	}
	utils.Success(w, status)
}

func (h *SystemHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	modules := make(map[string]int)
	tables := []string{"forward_rules", "proxy_rules", "ddns_configs", "stun_tunnels",
		"wol_devices", "cron_jobs", "acme_certificates", "storage_mounts"}
	for _, t := range tables {
		var count int
		h.db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&count)
		modules[t] = count
	}

	storageMounts := make([]map[string]interface{}, 0)
	rows, err := h.db.Query("SELECT name, enabled, source FROM storage_mounts WHERE enabled=1")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, source string
			var enabled bool
			rows.Scan(&name, &enabled, &source)
			storageMounts = append(storageMounts, map[string]interface{}{
				"name": name, "enabled": enabled, "source": source,
			})
		}
	}
	if storageMounts == nil { storageMounts = make([]map[string]interface{}, 0) }

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	payload := map[string]interface{}{
		"modules":        modules,
		"storage_mounts": storageMounts,
		"version":        "1.0.0-rc1",
		"uptime":         int64(time.Since(StartTime).Seconds()),
		"goroutines":     runtime.NumGoroutine(),
		"memory_mb":      m.Alloc / 1024 / 1024,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": payload})
}
