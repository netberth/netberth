// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"database/sql"
	"net/http"
	"runtime"
	"time"

	"github.com/netberth/netberth/internal/engine/forward"
	"github.com/netberth/netberth/pkg/utils"
	"github.com/netberth/netberth/pkg/version"
)

// MetricsHandler exposes machine-readable runtime and module metrics.
type MetricsHandler struct {
	db        *sql.DB
	forward   *forward.Engine
	startTime time.Time
}

func NewMetricsHandler(db *sql.DB, forwardEng *forward.Engine) *MetricsHandler {
	return &MetricsHandler{db: db, forward: forwardEng, startTime: time.Now()}
}

func (h *MetricsHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	forwardStatus := h.forward.Status()
	forwardInfo := make([]map[string]interface{}, 0, len(forwardStatus))
	for _, s := range forwardStatus {
		var name string
		h.db.QueryRow("SELECT name FROM forward_rules WHERE id=?", s.ID).Scan(&name)
		forwardInfo = append(forwardInfo, map[string]interface{}{
			"id":          s.ID,
			"name":        name,
			"active":      s.Active,
			"connections": s.Connections,
			"bytes_in":    s.BytesIn,
			"bytes_out":   s.BytesOut,
		})
	}

	modules := make(map[string]int)
	for _, t := range []string{"forward_rules", "proxy_rules", "ddns_configs", "stun_tunnels",
		"wol_devices", "cron_jobs", "acme_certificates", "storage_mounts"} {
		var count int
		h.db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&count)
		modules[t] = count
	}

	utils.Success(w, map[string]interface{}{
		"version":        version.Version,
		"go_version":     runtime.Version(),
		"uptime_seconds": int64(time.Since(h.startTime).Seconds()),
		"goroutines":     runtime.NumGoroutine(),
		"memory_mb":      mem.Alloc / 1024 / 1024,
		"db_driver":      dbDriverLabel(h.db),
		"modules":        modules,
		"forward_rules":  forwardInfo,
		"storage_mounts": countEnabledStorageMounts(h.db),
	})
}

func dbDriverLabel(db *sql.DB) string {
	var v string
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&v); err == nil {
		return "sqlite"
	}
	return "postgres"
}

func countEnabledStorageMounts(db *sql.DB) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM storage_mounts WHERE enabled=1").Scan(&n)
	return n
}
