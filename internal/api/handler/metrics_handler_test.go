// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"net/http"
	"testing"

	"github.com/netberth/netberth/internal/engine/forward"
	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/version"
)

type mockMetricsForwardDB struct{}

func (mockMetricsForwardDB) GetRules() ([]model.ForwardRule, error) { return nil, nil }

func TestMetricsHandler(t *testing.T) {
	db := setupFullTestDB(t)
	db.Exec("INSERT INTO forward_rules (id, name, protocol, listen_port, target_addr, target_port, enabled) VALUES ('f1','fwd','tcp',24001,'192.0.2.1',80,0)")
	db.Exec("INSERT INTO storage_mounts (id, name, type, source, enabled) VALUES ('m1','mnt','local','/tmp/x',1)")

	h := NewMetricsHandler(db, forward.New(mockMetricsForwardDB{}))
	w := doJSON(t, h.Metrics, http.MethodGet, "/api/v1/system/metrics", nil)
	expectStatus(t, w, http.StatusOK)

	var resp struct {
		Data struct {
			Version      string         `json:"version"`
			DBDriver     string         `json:"db_driver"`
			Modules      map[string]int `json:"modules"`
			ForwardRules []interface{}  `json:"forward_rules"`
			Storage      int            `json:"storage_mounts"`
		} `json:"data"`
	}
	decodeResponse(t, w, &resp)
	if resp.Data.Version != version.Version {
		t.Fatalf("unexpected version: %s", resp.Data.Version)
	}
	if resp.Data.DBDriver != "sqlite" {
		t.Fatalf("unexpected db driver: %s", resp.Data.DBDriver)
	}
	if resp.Data.Modules["forward_rules"] != 1 {
		t.Fatalf("expected 1 forward rule in modules, got %+v", resp.Data.Modules)
	}
	if resp.Data.Storage != 1 {
		t.Fatalf("expected 1 enabled storage mount, got %d", resp.Data.Storage)
	}
	if resp.Data.ForwardRules == nil {
		t.Fatal("expected forward_rules field")
	}
}

func TestDBDriverLabel(t *testing.T) {
	db := setupFullTestDB(t)
	if got := dbDriverLabel(db); got != "sqlite" {
		t.Fatalf("expected sqlite, got %s", got)
	}
}
