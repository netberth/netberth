// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

// Package security provides small, shared cryptographic helpers used across
// NetBerth. Keeping HMAC signing in one place avoids subtle drift between
// call sites (webhooks, CSRF, ...).
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignHMACSHA256 returns the lowercase hex-encoded HMAC-SHA256 of message
// keyed by secret. The caller decides whether an empty secret is acceptable
// for its use case.
func SignHMACSHA256(secret string, message []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHMACSHA256 reports whether sig is a valid HMAC-SHA256 of message keyed
// by secret. The comparison is constant-time; malformed hex input is rejected
// without comparing.
func VerifyHMACSHA256(secret string, message []byte, sig string) bool {
	want := SignHMACSHA256(secret, message)
	if len(want) != len(sig) {
		return false
	}
	return hmac.Equal([]byte(want), []byte(sig))
}
