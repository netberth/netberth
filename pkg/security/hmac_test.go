// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package security

import (
	"strings"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	secret := "s3cret"
	msg := []byte("hello netberth")
	sig := SignHMACSHA256(secret, msg)
	if len(sig) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(sig))
	}
	if !VerifyHMACSHA256(secret, msg, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	secret := "s3cret"
	msg := []byte("hello netberth")
	sig := SignHMACSHA256(secret, msg)

	cases := map[string]struct {
		secret string
		msg    []byte
		sig    string
	}{
		"wrong secret":  {"other", msg, sig},
		"tampered msg":  {secret, []byte("hello netberth!"), sig},
		"tampered sig":  {secret, msg, "0" + sig[1:]},
		"uppercase sig": {secret, msg, strings.ToUpper(sig)},
		"empty sig":     {secret, msg, ""},
		"truncated sig": {secret, msg, sig[:63]},
		"extra sig":     {secret, msg, sig + "00"},
		"non-hex sig":   {secret, msg, strings.Repeat("g", 64)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if VerifyHMACSHA256(tc.secret, tc.msg, tc.sig) {
				t.Fatal("expected verification to fail")
			}
		})
	}
}

func TestVerifyEmptyMessage(t *testing.T) {
	sig := SignHMACSHA256("k", nil)
	if !VerifyHMACSHA256("k", nil, sig) {
		t.Fatal("empty message signature must verify")
	}
}

func TestSignDeterministic(t *testing.T) {
	if SignHMACSHA256("k", []byte("m")) != SignHMACSHA256("k", []byte("m")) {
		t.Fatal("signature must be deterministic")
	}
	if SignHMACSHA256("k1", []byte("m")) == SignHMACSHA256("k2", []byte("m")) {
		t.Fatal("different keys must produce different signatures")
	}
}
