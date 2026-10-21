// Package auth mints and verifies the login tokens that replace the
// development X-Subject-ID header. Tokens are HS256 JWTs signed with the
// deployment secret: stateless, so no session table to operate, and short
// lived, so a leaked token expires on its own. Revocation before expiry is
// a future login-phase concern (denylist); logout today means the client
// discards the token.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims is the token payload: who it was minted for and until when it
// is valid. Roles travel inside so /v1/me needs no extra lookup; the
// roles service stays authoritative on next login either way.
type Claims struct {
	Subject   string   `json:"sub"`
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

// Service mints and verifies tokens with one shared secret.
type Service struct {
	secret []byte
	ttl    time.Duration
}

// NewService wires token minting onto secret with the given lifetime.
// Empty secrets and non-positive lifetimes are rejected: failing closed
// beats minting forever-tokens by accident.
func NewService(secret string, ttl time.Duration) (*Service, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidSecret
	}
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}
	return &Service{secret: []byte(secret), ttl: ttl}, nil
}

// Mint issues a token for subject with roles, valid for the service TTL.
func (s *Service) Mint(subject string, roles []string, now time.Time) (token string, expiresAt time.Time, err error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", time.Time{}, ErrInvalidSubject
	}
	expiresAt = now.UTC().Add(s.ttl)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, err := json.Marshal(Claims{
		Subject:   subject,
		Roles:     append([]string(nil), roles...),
		IssuedAt:  now.UTC().Unix(),
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: encode claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, expiresAt, nil
}

// Verify checks signature, shape and expiry, returning the claims.
func (s *Service) Verify(token string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	if err := json.Unmarshal(rawHeader, &header); err != nil || header.Alg != "HS256" {
		return Claims{}, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	if subtle.ConstantTimeCompare(mac.Sum(nil), want) != 1 {
		return Claims{}, ErrInvalidToken
	}
	rawBody, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	dec := json.NewDecoder(strings.NewReader(string(rawBody)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return Claims{}, ErrInvalidToken
	}
	if now.UTC().Unix() >= claims.ExpiresAt {
		return Claims{}, ErrExpiredToken
	}
	return claims, nil
}
