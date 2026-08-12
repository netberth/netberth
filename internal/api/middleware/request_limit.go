// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package middleware

import "net/http"

const (
	// maxJSONBodyBytes bounds every JSON API payload (login, CRUD, settings).
	// Legitimate payloads are a few KB; anything larger is an attack or a bug.
	maxJSONBodyBytes = 64 << 10
	// maxRestoreBodyBytes mirrors the restore handler's own 100 MB cap so the
	// route-level guard never breaks legitimate database restores.
	maxRestoreBodyBytes = 100 << 20
)

// LimitRequestBody caps request bodies for the API surface. The login,
// refresh and password endpoints are additionally capped inside their
// handlers so unit tests exercise the same protection without the router.
func LimitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxJSONBodyBytes)
		if r.URL.Path == "/api/v1/system/restore" && r.Method == http.MethodPost {
			limit = maxRestoreBodyBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
