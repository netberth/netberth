// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package cron

import (
	"errors"
	"testing"

	"github.com/netberth/netberth/internal/model"
)

type mockCronDB struct {
	jobs []model.CronJob
	err  error
}

func (m *mockCronDB) GetJobs() ([]model.CronJob, error) { return m.jobs, m.err }

func TestStartWithEnabledJobs(t *testing.T) {
	e := New(&mockCronDB{jobs: []model.CronJob{
		{ID: "j1", Name: "cmd", Schedule: "*/5 * * * * *", Type: "command", Command: "echo hi", Enabled: true},
		{ID: "j2", Name: "disabled", Schedule: "*/5 * * * * *", Type: "command", Command: "true", Enabled: false},
	}})
	if err := e.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	e.Stop()
}

func TestStartDBError(t *testing.T) {
	e := New(&mockCronDB{err: errors.New("boom")})
	if err := e.Start(); err == nil {
		t.Fatal("expected start error")
	}
}

func TestStartInvalidSchedule(t *testing.T) {
	e := New(&mockCronDB{jobs: []model.CronJob{
		{ID: "bad", Name: "bad", Schedule: "not-a-schedule", Enabled: true},
	}})
	if err := e.Start(); err != nil {
		t.Fatalf("start with invalid schedule should not fail: %v", err)
	}
	e.Stop()
}

func TestReloadAndRemove(t *testing.T) {
	e := New(&mockCronDB{})
	job := model.CronJob{ID: "j1", Name: "cmd", Schedule: "*/5 * * * * *", Type: "command", Command: "true", Enabled: true}

	e.Reload(job)                       // add
	e.Reload(model.CronJob{ID: "j1", Schedule: "*/5 * * * * *", Enabled: false}) // remove
	e.Reload(job)                       // re-add
	e.Remove("j1")                      // remove
	e.Remove("missing")                 // no-op
}

func TestExecuteBranches(t *testing.T) {
	e := New(&mockCronDB{})
	e.execute(model.CronJob{Name: "cmd", Type: "command", Command: "echo ok"})
	e.execute(model.CronJob{Name: "empty-cmd", Type: "command"})
	e.execute(model.CronJob{Name: "toggle", Type: "module_toggle", ModuleType: "forward", ModuleID: "m1"})
	e.execute(model.CronJob{Name: "unknown", Type: "weird"})
}
