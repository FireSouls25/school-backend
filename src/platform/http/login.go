package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"grade/src/core/roles"
	"grade/src/core/throttle"
)

// loginRequest carries the credentials. Identifier is a username or an
// email; lookup rules match users.Authenticate.
type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// loginResponse is what an authenticated caller stores: the short access
// bearer for every later call, its expiry, the rotating refresh token
// (transitional JSON copy; the HttpOnly cookie is authoritative) and
// where the SPA should land.
type loginResponse struct {
	Token            string       `json:"token"`
	ExpiresAt        string       `json:"expiresAt"`
	RefreshToken     string       `json:"refreshToken,omitempty"`
	RefreshExpiresAt string       `json:"refreshExpiresAt,omitempty"`
	Subject          string       `json:"subject"`
	Roles            []roles.Role `json:"roles"`
	Home             string       `json:"home"`
}

// refreshRequest rotates an access bearer. The token travels in the
// HttpOnly cookie or, during the SPA transition, in JSON.
type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// refreshResponse returns the rotated pair.
type refreshResponse struct {
	Token            string `json:"token"`
	ExpiresAt        string `json:"expiresAt"`
	RefreshToken     string `json:"refreshToken"`
	RefreshExpiresAt string `json:"refreshExpiresAt"`
}

// logoutRequest revokes refresh tokens. Empty revokes the presented one;
// all=true revokes every session of the caller (needs access bearer).
type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
	All          bool   `json:"all"`
}

// refreshCookieName is the HttpOnly carrier for rotation. The JSON copy
// stays during the SPA transition to localStorage; the cookie lets the
// backend go cookie-only later without a flag day.
const refreshCookieName = "refresh_token"

// loginLimit caps password guesses per key per window: brute force gets
// expensive while normal typos never notice.
const (
	loginLimitPerMinute   = 10
	loginWindow           = time.Minute
	refreshLimitPerMinute = 30
	recoverLimitPerMinute = 5
)

// loginLimiter caps guesses per key and window. Backed by the shared
// throttle service (persistent across restarts/replicas) when wired;
// otherwise a per-Router memory map (tests). Denied attempts are never
// recorded, so they cannot extend the window.
type loginLimiter struct {
	svc     *throttle.Service
	trusted *throttle.Nets
	max     int
	window  time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLoginLimiter(svc *throttle.Service, trusted *throttle.Nets, max int, window time.Duration) *loginLimiter {
	return &loginLimiter{svc: svc, trusted: trusted, max: max, window: window, hits: make(map[string][]time.Time)}
}

// clientIP resolves the caller IP, trusting X-Forwarded-For only from
// configured proxies.
func (l *loginLimiter) clientIP(r *http.Request) string {
	return throttle.ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), l.trusted)
}

// check reports whether key may try once more, without recording.
func (l *loginLimiter) check(ctx context.Context, key string, now time.Time) bool {
	if l.svc != nil {
		return l.svc.Check(ctx, key, l.max, l.window, now)
	}
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, ts := range l.hits[key] {
		if ts.After(cutoff) {
			n++
		}
	}
	return n < l.max
}

// record persists one attempt (success or failure) for future windows
// and the audit trail.
func (l *loginLimiter) record(ctx context.Context, key, ip string, success bool, now time.Time) {
	if l.svc != nil {
		l.svc.Record(ctx, key, ip, success, now)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.hits[key], now)
}

// errRateLimited renders 429 when the login throttle bites.
var errRateLimited = codedError{"http.err_rate_limited"}

// loginKey scopes throttling to one identifier from one address. Shared
// NATs share a budget per identifier: the standard tradeoff between
// brute-force cost and lockout blast radius at school scale.
func loginKey(l *loginLimiter, r *http.Request, identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier)) + "|" + l.clientIP(r)
}

// refreshKey scopes rotation throttling per address.
func refreshKey(l *loginLimiter, r *http.Request) string {
	return "refresh|" + l.clientIP(r)
}

// setRefreshCookie stores the rotation token HttpOnly. Secure follows the
// connection (dev over http keeps working); SameSite=Lax keeps top-level
// navigation working while blocking third-party sends.
func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/v1",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearRefreshCookie drops the rotation cookie (logout).
func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/v1",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// refreshFromRequest prefers the HttpOnly cookie, falling back to JSON
// during the SPA transition.
func refreshFromRequest(r *http.Request, body refreshRequest) string {
	if c, err := r.Cookie(refreshCookieName); err == nil && strings.TrimSpace(c.Value) != "" {
		return strings.TrimSpace(c.Value)
	}
	return strings.TrimSpace(body.RefreshToken)
}

// handleLogin exchanges credentials for an access bearer plus a rotating
// refresh token. Unknown identifier, wrong password and inactive account
// all answer 401 with the same code (no enumeration); over-limit callers
// get 429.
func handleLogin(deps Dependencies, limit *loginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body loginRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Identifier) == "" || body.Password == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		now := time.Now()
		key := loginKey(limit, r, body.Identifier)
		ip := limit.clientIP(r)
		if !limit.check(r.Context(), key, now) {
			slog.Warn("login throttled", "identifier", strings.ToLower(strings.TrimSpace(body.Identifier)), "ip", ip)
			WriteError(w, r, errRateLimited)
			return
		}
		u, err := deps.Users.Authenticate(r.Context(), body.Identifier, body.Password)
		if err != nil {
			limit.record(r.Context(), key, ip, false, now)
			if err == errRateLimited {
				slog.Warn("login throttled", "ip", ip)
			}
			WriteError(w, r, err)
			return
		}
		limit.record(r.Context(), key, ip, true, now)
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if deps.Tokens == nil {
			WriteError(w, r, errInternal)
			return
		}
		if rs == nil {
			rs = []roles.Role{}
		}
		roleNames := make([]string, 0, len(rs))
		for _, role := range rs {
			roleNames = append(roleNames, string(role))
		}
		token, expiresAt, err := deps.Tokens.MintAccess(u.ID, roleNames, u.TokenVersion, now)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		refresh, refreshExp, err := deps.Tokens.IssueRefresh(r.Context(), u.ID, u.TokenVersion, now)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		_ = deps.Tokens.DeleteExpired(r.Context(), now)
		setRefreshCookie(w, r, refresh, refreshExp)
		writeJSON(w, http.StatusOK, loginResponse{
			Token:            token,
			ExpiresAt:        expiresAt.UTC().Format(time.RFC3339),
			RefreshToken:     refresh,
			RefreshExpiresAt: refreshExp.UTC().Format(time.RFC3339),
			Subject:          u.ID,
			Roles:            rs,
			Home:             homeFor(rs),
		})
	}
}

// handleRefresh rotates a refresh token into a fresh access+refresh pair.
// Reuse of a rotated token fails closed (revoked): login again. The old
// version check also kills rotations after password changes/disables.
func handleRefresh(deps Dependencies, limit *loginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body refreshRequest
		if r.ContentLength != 0 {
			if !decodeJSON(w, r, &body) {
				return
			}
		}
		now := time.Now()
		key := refreshKey(limit, r)
		ip := limit.clientIP(r)
		if !limit.check(r.Context(), key, now) {
			slog.Warn("refresh throttled", "ip", ip)
			WriteError(w, r, errRateLimited)
			return
		}
		presented := refreshFromRequest(r, body)
		if presented == "" || deps.Tokens == nil || deps.Users == nil {
			limit.record(r.Context(), key, ip, false, now)
			WriteError(w, r, errUnauthorized)
			return
		}
		claims, err := deps.Tokens.CheckRefresh(r.Context(), presented, now)
		if err != nil {
			limit.record(r.Context(), key, ip, false, now)
			WriteError(w, r, err)
			return
		}
		u, err := deps.Users.ByID(r.Context(), claims.Subject)
		if err != nil {
			limit.record(r.Context(), key, ip, false, now)
			WriteError(w, r, errUnauthorized)
			return
		}
		if !u.Active || u.DeletedAt != nil || u.TokenVersion != claims.Ver {
			_ = deps.Tokens.Revoke(r.Context(), claims.Jti)
			limit.record(r.Context(), key, ip, false, now)
			WriteError(w, r, errUnauthorized)
			return
		}
		limit.record(r.Context(), key, ip, true, now)
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if rs == nil {
			rs = []roles.Role{}
		}
		roleNames := make([]string, 0, len(rs))
		for _, role := range rs {
			roleNames = append(roleNames, string(role))
		}
		access, accessExp, err := deps.Tokens.MintAccess(u.ID, roleNames, u.TokenVersion, now)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		// Rotation: revoke the presented token before issuing the next,
		// so reuse of either fails closed.
		_ = deps.Tokens.Revoke(r.Context(), claims.Jti)
		refresh, refreshExp, err := deps.Tokens.IssueRefresh(r.Context(), u.ID, u.TokenVersion, now)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		_ = deps.Tokens.DeleteExpired(r.Context(), now)
		setRefreshCookie(w, r, refresh, refreshExp)
		writeJSON(w, http.StatusOK, refreshResponse{
			Token:            access,
			ExpiresAt:        accessExp.UTC().Format(time.RFC3339),
			RefreshToken:     refresh,
			RefreshExpiresAt: refreshExp.UTC().Format(time.RFC3339),
		})
	}
}

// handleLogout revokes refresh tokens. Single (cookie/body) by default;
// all=true with an access bearer revokes every session of the caller.
func handleLogout(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body logoutRequest
		if r.ContentLength != 0 {
			if !decodeJSON(w, r, &body) {
				return
			}
		}
		if deps.Tokens == nil {
			clearRefreshCookie(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if body.All {
			// all=true needs an access bearer (logout route sits outside
			// the subject middleware so single logout stays cookied).
			subject, ok := SubjectFrom(r.Context())
			if !ok {
				if deps.Tokens == nil {
					WriteError(w, r, errUnauthorized)
					return
				}
				if raw, hasBearer := bearerToken(r); hasBearer {
					if claims, err := deps.Tokens.VerifyAccess(raw, time.Now()); err == nil {
						subject, ok = claims.Subject, claims.Subject != ""
					}
				}
			}
			if !ok {
				WriteError(w, r, errUnauthorized)
				return
			}
			_ = deps.Tokens.RevokeAllForUser(r.Context(), subject)
			clearRefreshCookie(w)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		presented := refreshFromRequest(r, refreshRequest{RefreshToken: body.RefreshToken})
		if presented != "" {
			if claims, err := deps.Tokens.VerifyRefresh(presented, time.Now()); err == nil {
				_ = deps.Tokens.Revoke(r.Context(), claims.Jti)
			} else {
				slog.Warn("logout with invalid refresh", "error", err)
			}
		}
		clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}
