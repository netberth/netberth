// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/netberth/netberth/internal/model"
	"golang.org/x/crypto/argon2"
)

type ContextKey string

const ClaimsKey ContextKey = "claims"

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Claims struct {
	jwt.RegisteredClaims
	UserID   string `json:"uid"`
	Username string `json:"uname"`
	Role     string `json:"role"`
}

type Service struct {
	jwtSecret      []byte
	accessExpiry   time.Duration
	refreshExpiry  time.Duration
	passwordPepper []byte
}

type Argon2Params struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

var DefaultArgon2Params = Argon2Params{
	Time:    3,
	Memory:  64 * 1024,
	Threads: 4,
	KeyLen:  32,
	SaltLen: 16,
}

func NewService(jwtSecret string, accessExpiry, refreshExpiry time.Duration) *Service {
	return &Service{
		jwtSecret:     []byte(jwtSecret),
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

func (s *Service) HashPassword(password string) (string, error) {
	salt := make([]byte, DefaultArgon2Params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		DefaultArgon2Params.Time,
		DefaultArgon2Params.Memory,
		DefaultArgon2Params.Threads,
		DefaultArgon2Params.KeyLen,
	)
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		DefaultArgon2Params.Memory,
		DefaultArgon2Params.Time,
		DefaultArgon2Params.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

func (s *Service) VerifyPassword(password, encoded string) (bool, error) {
	params, salt, hash, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, params.KeyLen)
	return subtle.ConstantTimeCompare(hash, computed) == 1, nil
}

func (s *Service) GenerateTokens(user *model.User) (*TokenPair, error) {
	now := time.Now()
	accessClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessExpiry)),
			ID:        generateID(),
		},
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}
	refreshClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpiry)),
			ID:        generateID(),
		},
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessExpiry.Seconds()),
	}, nil
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.jwtSecret, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func (s *Service) GenerateOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func (s *Service) ValidateTOTP(secret, code string) bool {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil || len(key) == 0 { return false }
	counter := time.Now().Unix() / 30
	// Check ±1 window for clock skew
	for offset := int64(-1); offset <= 1; offset++ {
		if totpCode(key, counter+offset) == code { return true }
	}
	return false
}

func totpCode(key []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)
	offset := hash[len(hash)-1] & 0x0f
	binary := int32(hash[offset]&0x7f)<<24 | int32(hash[offset+1])<<16 | int32(hash[offset+2])<<8 | int32(hash[offset+3])
	otp := int(binary) % 1000000
	return fmt.Sprintf("%06d", otp)
}

func decodeArgon2Hash(encoded string) (Argon2Params, []byte, []byte, error) {
	var params Argon2Params
	var version int
	_, err := fmt.Sscanf(encoded, "$argon2id$v=%d$m=%d,t=%d,p=%d$", &version, &params.Memory, &params.Time, &params.Threads)
	if err != nil {
		return params, nil, nil, fmt.Errorf("invalid hash format: %w", err)
	}
	params.KeyLen = 32
	saltB64 := extractField(encoded, 4)
	hashB64 := extractField(encoded, 5)
	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return params, nil, nil, fmt.Errorf("decode salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return params, nil, nil, fmt.Errorf("decode hash: %w", err)
	}
	return params, salt, hash, nil
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func extractField(s string, n int) string {
	count := 0
	start := -1
	for i, c := range s {
		if c == '$' {
			count++
			if count == n {
				start = i + 1
			} else if count == n+1 {
				return s[start:i]
			}
		}
	}
	if start >= 0 {
		return s[start:]
	}
	return ""
}
