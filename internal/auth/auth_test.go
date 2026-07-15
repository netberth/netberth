// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package auth

import (
	"testing"
	"time"

	"github.com/netberth/netberth/internal/model"
)

func mkUser(id, name, role string) *model.User {
	return &model.User{ID: id, Username: name, Role: role}
}

func TestHashAndVerifyPassword(t *testing.T) {
	s := NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	hash, err := s.HashPassword("mypassword123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	valid, err := s.VerifyPassword("mypassword123", hash)
	if err != nil {
		t.Fatalf("VerifyPassword failed: %v", err)
	}
	if !valid {
		t.Fatal("expected password to be valid")
	}
	valid, _ = s.VerifyPassword("wrongpassword", hash)
	if valid {
		t.Fatal("expected wrong password to be invalid")
	}
}

func TestHashPasswordUniqueness(t *testing.T) {
	s := NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	h1, _ := s.HashPassword("samepassword")
	h2, _ := s.HashPassword("samepassword")
	if h1 == h2 {
		t.Fatal("expected unique hashes due to random salt")
	}
	ok1, _ := s.VerifyPassword("samepassword", h1)
	ok2, _ := s.VerifyPassword("samepassword", h2)
	if !ok1 || !ok2 {
		t.Fatal("both hashes should verify the same password")
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	s := NewService("my-jwt-secret-key", 1*time.Hour, 24*time.Hour)
	user := mkUser("user-1", "admin", "admin")
	tokens, err := s.GenerateTokens(user)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected non-empty tokens")
	}
	claims, err := s.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID != "user-1" || claims.Username != "admin" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestValidateTokenInvalid(t *testing.T) {
	s := NewService("secret-a", 1*time.Hour, 24*time.Hour)
	_, err := s.ValidateToken("invalid.token.here")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	s2 := NewService("secret-b", 1*time.Hour, 24*time.Hour)
	user := mkUser("u1", "admin", "admin")
	tokens, _ := s.GenerateTokens(user)
	_, err = s2.ValidateToken(tokens.AccessToken)
	if err == nil {
		t.Fatal("expected error when validating token with wrong secret")
	}
}

func TestArgon2HashDecode(t *testing.T) {
	s := NewService("test", 15*time.Minute, 7*24*time.Hour)
	hash, _ := s.HashPassword("test123")
	valid, err := s.VerifyPassword("test123", hash)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid")
	}
}

func TestOTPSecretGeneration(t *testing.T) {
	s := NewService("test", 15*time.Minute, 7*24*time.Hour)
	secret, err := s.GenerateOTPSecret()
	if err != nil {
		t.Fatalf("GenerateOTPSecret failed: %v", err)
	}
	if len(secret) < 16 {
		t.Fatalf("OTP secret too short: %d", len(secret))
	}
	s2, _ := s.GenerateOTPSecret()
	if secret == s2 {
		t.Fatal("expected unique OTP secrets")
	}
}
