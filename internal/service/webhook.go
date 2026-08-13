// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/netberth/netberth/internal/model"
	"github.com/netberth/netberth/pkg/logger"
	"github.com/netberth/netberth/pkg/security"
)

const (
	webhookMaxAttempts = 3
	webhookQueueSize   = 16
	webhookTimeout     = 5 * time.Second
)

// WebhookPayload is the JSON body delivered to a webhook endpoint.
type WebhookPayload struct {
	ID         string    `json:"id"`
	Event      string    `json:"event"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resource_id"`
	Timestamp  time.Time `json:"timestamp"`
}

// WebhookDispatcher delivers bus events to configured webhook endpoints with
// bounded concurrency, retries and HMAC-SHA256 signing.
type WebhookDispatcher struct {
	db          *sql.DB
	bus         *Bus
	client      *http.Client
	maxAttempts int
	backoffBase time.Duration
	sem         chan struct{}
	wg          sync.WaitGroup
}

func NewWebhookDispatcher(db *sql.DB, bus *Bus) *WebhookDispatcher {
	d := &WebhookDispatcher{
		db:          db,
		bus:         bus,
		client:      &http.Client{Timeout: webhookTimeout},
		maxAttempts: webhookMaxAttempts,
		backoffBase: 100 * time.Millisecond,
		sem:         make(chan struct{}, webhookQueueSize),
	}
	d.bus.SubscribeAll(d.dispatch)
	return d
}

// Stop waits for in-flight deliveries. Call during graceful shutdown.
func (d *WebhookDispatcher) Stop() { d.wg.Wait() }

func (d *WebhookDispatcher) dispatch(event Event) {
	endpoints, err := d.loadEndpoints()
	if err != nil {
		logger.Log.Warn().Err(err).Msg("webhook: load endpoints failed")
		return
	}
	for _, ep := range endpoints {
		if !ep.Enabled || !webhookMatches(ep, event.Type) {
			continue
		}
		ep := ep
		d.wg.Add(1)
		select {
		case d.sem <- struct{}{}:
			go func() {
				defer d.wg.Done()
				defer func() { <-d.sem }()
				d.sendWithRetry(ep, event)
			}()
		default:
			d.wg.Done()
			logger.Log.Warn().
				Str("endpoint", ep.Name).
				Str("event", string(event.Type)).
				Msg("webhook: queue full, event dropped")
		}
	}
}

func webhookMatches(ep model.WebhookEndpoint, t EventType) bool {
	if len(ep.Events) == 0 {
		return true
	}
	for _, e := range ep.Events {
		if EventType(e) == t {
			return true
		}
	}
	return false
}

func (d *WebhookDispatcher) loadEndpoints() ([]model.WebhookEndpoint, error) {
	rows, err := d.db.Query(
		`SELECT id, name, url, secret, events, enabled, created_at, updated_at FROM webhook_endpoints`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WebhookEndpoint
	for rows.Next() {
		var ep model.WebhookEndpoint
		var eventsJSON string
		var enabled int
		if err := rows.Scan(&ep.ID, &ep.Name, &ep.URL, &ep.Secret, &eventsJSON,
			&enabled, &ep.CreatedAt, &ep.UpdatedAt); err != nil {
			return nil, err
		}
		ep.Enabled = enabled == 1
		ep.HasSecret = ep.Secret != ""
		if err := json.Unmarshal([]byte(eventsJSON), &ep.Events); err != nil {
			ep.Events = nil
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (d *WebhookDispatcher) sendWithRetry(ep model.WebhookEndpoint, event Event) {
	payload := WebhookPayload{
		ID:         fmt.Sprintf("%s/%s", event.Type, event.ID),
		Event:      string(event.Type),
		Resource:   resourceFromEvent(event.Type),
		ResourceID: event.ID,
		Timestamp:  time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("webhook: marshal payload failed")
		return
	}
	for attempt := 1; attempt <= d.maxAttempts; attempt++ {
		if attempt > 1 && d.backoffBase > 0 {
			time.Sleep(d.backoffBase * time.Duration(1<<(attempt-2)))
		}
		if err := d.send(ep, body); err == nil {
			return
		} else {
			logger.Log.Warn().
				Err(err).
				Str("endpoint", ep.Name).
				Int("attempt", attempt).
				Msg("webhook: delivery failed")
		}
	}
}

func (d *WebhookDispatcher) send(ep model.WebhookEndpoint, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetBerth/1")
	signWebhook(req, ep.Secret, body)
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// SendTest delivers a synthetic ping so the API/UI can verify connectivity.
func (d *WebhookDispatcher) SendTest(ctx context.Context, ep model.WebhookEndpoint) error {
	payload := WebhookPayload{
		ID:         "test/" + ep.ID,
		Event:      "test:ping",
		Resource:   "webhook",
		ResourceID: ep.ID,
		Timestamp:  time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NetBerth/1")
	signWebhook(req, ep.Secret, body)
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func signWebhook(req *http.Request, secret string, body []byte) {
	if secret == "" {
		return
	}
	req.Header.Set("X-NetBerth-Signature", "sha256="+security.SignHMACSHA256(secret, body))
}

func resourceFromEvent(t EventType) string {
	part, _, _ := strings.Cut(string(t), ":")
	return part
}

// KnownEventTypes returns all dispatchable event types for validation.
func KnownEventTypes() []string {
	out := make([]string, 0, len(allEventTypes))
	for _, t := range allEventTypes {
		out = append(out, string(t))
	}
	return out
}
