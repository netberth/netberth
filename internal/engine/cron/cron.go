// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package cron

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
	robfigcron "github.com/robfig/cron/v3"
)

type Engine struct {
	mu   sync.RWMutex
	cron *robfigcron.Cron
	jobs map[string]robfigcron.EntryID
	db   interface {
		GetJobs() ([]model.CronJob, error)
	}
	stopCh chan struct{}
}

func New(db interface {
	GetJobs() ([]model.CronJob, error)
}) *Engine {
	return &Engine{
		cron:   robfigcron.New(robfigcron.WithSeconds()),
		jobs:   make(map[string]robfigcron.EntryID),
		db:     db,
		stopCh: make(chan struct{}),
	}
}

func (e *Engine) Start() error {
	jobs, err := e.db.GetJobs()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Enabled {
			e.addJob(job)
		}
	}
	e.cron.Start()
	logger.Log.Info().Msg("cron engine started")
	return nil
}

func (e *Engine) Stop() {
	close(e.stopCh)
	e.cron.Stop()
}

func (e *Engine) addJob(job model.CronJob) {
	id, err := e.cron.AddFunc(job.Schedule, func() {
		e.execute(job)
	})
	if err != nil {
		logger.Log.Error().Err(err).Str("name", job.Name).Msg("cron add job failed")
		return
	}
	e.jobs[job.ID] = id
}

func (e *Engine) Reload(job model.CronJob) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if entryID, exists := e.jobs[job.ID]; exists {
		e.cron.Remove(entryID)
		delete(e.jobs, job.ID)
	}
	if job.Enabled {
		e.addJob(job)
	}
}

func (e *Engine) Remove(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if entryID, exists := e.jobs[id]; exists {
		e.cron.Remove(entryID)
		delete(e.jobs, id)
	}
}

func (e *Engine) execute(job model.CronJob) {
	now := time.Now()
	logger.Log.Info().Str("name", job.Name).Msg("cron job executing")
	switch job.Type {
	case "command":
		if job.Command != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
			output, err := cmd.CombinedOutput()
			if err != nil {
				logger.Log.Error().Err(err).Str("name", job.Name).Str("output", string(output)).Msg("cron command failed")
			}
		}
	case "module_toggle":
		logger.Log.Info().Str("name", job.Name).Str("module", job.ModuleType).Str("id", job.ModuleID).Msg("module toggle triggered")
	default:
		logger.Log.Warn().Str("name", job.Name).Str("type", job.Type).Msg("unknown cron job type")
	}
	logger.Log.Info().Str("name", job.Name).Dur("since", time.Since(now)).Msg("cron job completed")
}
