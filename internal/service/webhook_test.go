// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netberth/netberth/internal/db"
	"github.com/netberth/netberth/internal/model"
)

func newWebhookTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func insertTestEndpoint(t *testing.T, database *sql.DB, ep model.WebhookEndpoint) {
	t.Helper()
	events, _ := json.Marshal(ep.Events)
	enabled := 0
	if ep.Enabled {
		enabled = 1
	}
	if _, err := database.Exec(
		`INSERT INTO webhook_endpoints (id, name, url, secret, events, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.Name, ep.URL, ep.Secret, string(events), enabled, time.Now(), time.Now()); err != nil {
		t.Fatalf("insert endpoint: %v", err)
	}
}

func TestWebhookDeliversWithSignature(t *testing.T) {
	var gotBody []byte
	var gotSig string
	hit := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-NetBerth-Signature")
		hit <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	database := newWebhookTestDB(t)
	insertTestEndpoint(t, database, model.WebhookEndpoint{
		ID: "w1", Name: "test", URL: srv.URL + "/hook",
		Secret: "s3cret", Events: []string{"forward:created"}, Enabled: true,
	})

	bus := NewBus()
	d := NewWebhookDispatcher(database, bus)
	d.maxAttempts = 1
	d.backoffBase = 0
	bus.Publish(Event{Type: EventForwardCreated, ID: "rule-1"})

	select {
	case <-hit:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for webhook delivery")
	}
	d.Stop()

	var payload WebhookPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Event != "forward:created" || payload.ResourceID != "rule-1" || payload.Resource != "forward" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Fatalf("signature mismatch: got %q want %q", gotSig, want)
	}
}

func TestWebhookSkipsDisabledAndNonMatching(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	database := newWebhookTestDB(t)
	insertTestEndpoint(t, database, model.WebhookEndpoint{
		ID: "w-disabled", Name: "disabled", URL: srv.URL + "/disabled",
		Enabled: false,
	})
	insertTestEndpoint(t, database, model.WebhookEndpoint{
		ID: "w-other", Name: "other", URL: srv.URL + "/other",
		Events: []string{"proxy:created"}, Enabled: true,
	})
	insertTestEndpoint(t, database, model.WebhookEndpoint{
		ID: "w-all", Name: "all", URL: srv.URL + "/all",
		Enabled: true,
	})

	bus := NewBus()
	d := NewWebhookDispatcher(database, bus)
	d.maxAttempts = 1
	d.backoffBase = 0
	bus.Publish(Event{Type: EventForwardCreated, ID: "rule-2"})

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&hits) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	d.Stop()
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected exactly 1 delivery (all-events endpoint), got %d", got)
	}
}

func TestWebhookRetriesOn5xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	database := newWebhookTestDB(t)
	insertTestEndpoint(t, database, model.WebhookEndpoint{
		ID: "w-retry", Name: "retry", URL: srv.URL + "/retry",
		Enabled: true,
	})

	bus := NewBus()
	d := NewWebhookDispatcher(database, bus)
	d.maxAttempts = 3
	d.backoffBase = time.Millisecond
	bus.Publish(Event{Type: EventForwardCreated, ID: "rule-3"})

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&attempts) < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	d.Stop()
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestWebhookKnownEventTypes(t *testing.T) {
	types := KnownEventTypes()
	if len(types) == 0 {
		t.Fatal("expected at least one known event type")
	}
	seen := map[string]bool{}
	for _, e := range types {
		if seen[e] {
			t.Fatalf("duplicate event type %q", e)
		}
		seen[e] = true
	}
}
