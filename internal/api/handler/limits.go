// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import "net/http"

const (
	// maxAuthBodyBytes covers login/refresh/password payloads. 8 KB is far
	// more than any legitimate request and keeps password hashing input tiny.
	maxAuthBodyBytes = 8 << 10
	// maxPasswordBytes prevents multi-megabyte passwords from paying the full
	// argon2 CPU cost (the QA devil test sent a 5 MB password).
	maxPasswordBytes = 128
	// maxUsernameBytes keeps usernames well within schema limits.
	maxUsernameBytes = 64
)

// limitBody installs http.MaxBytesReader so oversized bodies fail fast.
// Handlers call this before decoding; the router-level guard is a second
// layer for endpoints that do not set their own limit.
func limitBody(w http.ResponseWriter, r *http.Request, n int64) {
	r.Body = http.MaxBytesReader(w, r.Body, n)
}
