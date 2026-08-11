// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	return m
}

func TestSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	Success(w, map[string]string{"k": "v"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected json content type, got %q", ct)
	}
	m := decode(t, w)
	if m["success"] != true {
		t.Fatalf("expected success=true: %v", m)
	}
	if _, ok := m["data"]; !ok {
		t.Fatalf("expected data field: %v", m)
	}
}

func TestCreated(t *testing.T) {
	w := httptest.NewRecorder()
	Created(w, "thing")
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if m := decode(t, w); m["success"] != true || m["data"] != "thing" {
		t.Fatalf("unexpected created body: %v", m)
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusBadRequest, "bad input")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	m := decode(t, w)
	if m["success"] != false || m["error"] != "bad input" {
		t.Fatalf("unexpected error body: %v", m)
	}
}

func TestMessage(t *testing.T) {
	w := httptest.NewRecorder()
	Message(w, "done")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	m := decode(t, w)
	if m["success"] != true || m["message"] != "done" {
		t.Fatalf("unexpected message body: %v", m)
	}
}

func TestPaginated(t *testing.T) {
	w := httptest.NewRecorder()
	Paginated(w, []int{1, 2, 3}, 23, 2, 10)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	m := decode(t, w)
	if m["success"] != true {
		t.Fatalf("expected success=true: %v", m)
	}
	if m["total"] != float64(23) || m["page"] != float64(2) || m["page_size"] != float64(10) || m["total_pages"] != float64(3) {
		t.Fatalf("unexpected pagination fields: %v", m)
	}
	data, ok := m["data"].([]interface{})
	if !ok || len(data) != 3 {
		t.Fatalf("expected 3 data items, got %v", m["data"])
	}
}

func TestPaginatedExactDivision(t *testing.T) {
	w := httptest.NewRecorder()
	Paginated(w, []int{}, 20, 1, 10)
	m := decode(t, w)
	if m["total_pages"] != float64(2) {
		t.Fatalf("expected total_pages=2, got %v", m["total_pages"])
	}
}

func TestJSONCustomStatus(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, http.StatusTeapot, map[string]interface{}{"success": false, "error": "teapot"})
	if w.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", w.Code)
	}
	m := decode(t, w)
	if m["error"] != "teapot" {
		t.Fatalf("unexpected body: %v", m)
	}
}
