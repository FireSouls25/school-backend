// Package auth mints and verifies the login tokens that replace the
// development X-Subject-ID header. Tokens are HS256 JWTs signed with the
// deployment secret: short lived, so a leaked token expires on its own.
// Revocation is version-based: every token carries the account token
// version (ver) and a unique id (jti); bumping the version (password
// change, disable) invalidates previously minted bearers at the HTTP
// layer, which compares claims against the stored user.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Claims is the token payload: who it was minted for and until when it
// is valid. Roles travel inside so /v1/me needs no extra lookup; the
// roles service stays authoritative on next login either way.
type Claims struct {
	Subject   string   `json:"sub"`
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	// Ver is the account token version at mint time. The HTTP layer
	// rejects bearers whose version no longer matches the stored user.
	Ver int64 `json:"ver"`
	// Jti is the unique token id, for audit and future denylists.
	Jti string `json:"jti"`
	// Typ is access or refresh. Empty (pre-typ era) reads as access.
	Typ string `json:"typ,omitempty"`
}

// AccessTokenType marks short-lived API bearers. RefreshTokenType marks
// long-lived rotation tokens: they never open API routes, only /v1/refresh.
const (
	AccessTokenType  = "access"
	RefreshTokenType = "refresh"
)

// Service mints and verifies access and refresh tokens with one shared
// secret. Access tokens stay stateless (version-checked against users);
// refresh tokens are stateful (hash stored, rotated on every use) so a
// stolen refresh dies on first reuse and logout/password rotation kill
// sessions server-side.
type Service struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	refresh    Store
}

// NewService wires token minting onto secret with separate access and
// refresh lifetimes. Empty secrets and non-positive lifetimes are
// rejected: failing closed beats minting forever-tokens by accident.
// A nil store gets an in-memory one (tests); production wires Postgres.
func NewService(secret string, accessTTL, refreshTTL time.Duration, store Store) (*Service, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidSecret
	}
	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, ErrInvalidTTL
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, refresh: store}, nil
}

// AccessTTL reports the access token lifetime.
func (s *Service) AccessTTL() time.Duration { return s.accessTTL }

// RefreshTTL reports the refresh token lifetime.
func (s *Service) RefreshTTL() time.Duration { return s.refreshTTL }

// TokenHash binds a stored refresh record to its token without keeping
// a usable bearer: SHA-256 hex over the full token string.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

// MintAccess issues a short-lived API bearer for subject with roles and
// token version. The version must be the current user TokenVersion;
// rotation bumps it and older bearers stop validating.
func (s *Service) MintAccess(subject string, roles []string, ver int64, now time.Time) (token string, expiresAt time.Time, err error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", time.Time{}, ErrInvalidSubject
	}
	if ver < 0 {
		return "", time.Time{}, ErrInvalidSubject
	}
	expiresAt = now.UTC().Add(s.accessTTL)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, err := json.Marshal(Claims{
		Subject:   subject,
		Roles:     append([]string(nil), roles...),
		IssuedAt:  now.UTC().Unix(),
		ExpiresAt: expiresAt.Unix(),
		Ver:       ver,
		Jti:       uuid.NewString(),
		Typ:       AccessTokenType,
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

// MintRefreshToken issues a long-lived rotation token for subject and
// version. It carries no roles: refresh mints fresh access with live roles.
// The caller persists TokenHash(token) via IssueRefresh (or manually).
func (s *Service) MintRefreshToken(subject string, ver int64, now time.Time) (token string, expiresAt time.Time, jti string, err error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", time.Time{}, "", ErrInvalidSubject
	}
	if ver < 0 {
		return "", time.Time{}, "", ErrInvalidSubject
	}
	expiresAt = now.UTC().Add(s.refreshTTL)
	jti = uuid.NewString()
	body, err := json.Marshal(Claims{
		Subject:   subject,
		IssuedAt:  now.UTC().Unix(),
		ExpiresAt: expiresAt.Unix(),
		Ver:       ver,
		Jti:       jti,
		Typ:       RefreshTokenType,
	})
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("auth: encode claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	unsigned := headerFor(payload) + "." + payload
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, expiresAt, jti, nil
}

// IssueRefresh mints a refresh token and persists its hash. Rotation and
// logout revoke it via Revoke/ RevokeAllForUser.
func (s *Service) IssueRefresh(ctx context.Context, subject string, ver int64, now time.Time) (token string, expiresAt time.Time, err error) {
	token, expiresAt, jti, err := s.MintRefreshToken(subject, ver, now)
	if err != nil {
		return "", time.Time{}, err
	}
	rec := RefreshRec{
		JTI:       jti,
		UserID:    strings.TrimSpace(subject),
		TokenHash: TokenHash(token),
		ExpiresAt: expiresAt,
		CreatedAt: now.UTC(),
	}
	if err := s.refresh.Save(ctx, rec); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// Revoke marks one refresh token revoked.
func (s *Service) Revoke(ctx context.Context, jti string) error {
	return s.refresh.Revoke(ctx, jti)
}

// RevokeAllForUser marks every refresh token of the user revoked.
func (s *Service) RevokeAllForUser(ctx context.Context, userID string) error {
	return s.refresh.RevokeAllForUser(ctx, userID)
}

// DeleteExpired purges expired refresh rows.
func (s *Service) DeleteExpired(ctx context.Context, now time.Time) error {
	return s.refresh.DeleteExpired(ctx, now)
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

// headerFor builds the JOSE header for a payload.
func headerFor(payload string) string {
	_ = payload
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
}

// VerifyAccess verifies and requires an access token.
func (s *Service) VerifyAccess(token string, now time.Time) (Claims, error) {
	claims, err := s.Verify(token, now)
	if err != nil {
		return Claims{}, err
	}
	if claims.Typ != "" && claims.Typ != AccessTokenType {
		return Claims{}, ErrInvalidToken
	}
	// Pre-typ tokens (ver-only era) count as access for rollout.
	return claims, nil
}

// VerifyRefresh verifies and requires a refresh token.
func (s *Service) VerifyRefresh(token string, now time.Time) (Claims, error) {
	claims, err := s.Verify(token, now)
	if err != nil {
		return Claims{}, err
	}
	if claims.Typ != RefreshTokenType {
		return Claims{}, ErrInvalidToken
	}
	if strings.TrimSpace(claims.Jti) == "" {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

// CheckRefresh validates a presented refresh token against storage:
// signature, type, expiry, stored hash match, not revoked, not expired.
func (s *Service) CheckRefresh(ctx context.Context, token string, now time.Time) (Claims, error) {
	claims, err := s.VerifyRefresh(token, now)
	if err != nil {
		return Claims{}, err
	}
	rec, err := s.refresh.Lookup(ctx, claims.Jti)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	if rec.RevokedAt != nil {
		return Claims{}, ErrRevokedToken
	}
	if !rec.ExpiresAt.After(now) {
		return Claims{}, ErrExpiredToken
	}
	if rec.UserID != claims.Subject || subtle.ConstantTimeCompare([]byte(rec.TokenHash), []byte(TokenHash(token))) != 1 {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}
