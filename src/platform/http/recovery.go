package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// recoverRequest carries the identifier (username or email) that forgot
// its password. The response is always 204: unknown identifiers and
// inactive accounts answer exactly like valid ones (no enumeration).
type recoverRequest struct {
	Identifier string `json:"identifier"`
}

// recoverReset consumes a single-use token and sets the new password.
type recoverReset struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// setActiveRequest flips a login without deleting it (lockout response).
type setActiveRequest struct {
	Active bool `json:"active"`
}

// handleRecoverRequest mints a reset token (when the account exists and
// is active) and always answers 204. Throttled callers get 429.
func handleRecoverRequest(deps Dependencies, limit *loginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body recoverRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Identifier) == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		now := time.Now()
		key := "recover|" + limit.clientIP(r)
		ip := limit.clientIP(r)
		if !limit.check(r.Context(), key, now) {
			slog.Warn("recover throttled", "ip", ip)
			WriteError(w, r, errRateLimited)
			return
		}
		limit.record(r.Context(), key, ip, true, now)
		if deps.Recovery == nil {
			WriteError(w, r, errInternal)
			return
		}
		token, err := deps.Recovery.Request(r.Context(), body.Identifier)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		// Delivery is out-of-band until mail/WhatsApp lands: log the raw
		// token in development only, never in production.
		if token != "" && deps.RecoveryLogTokens {
			slog.Info("password reset token (development only)",
				"identifier", strings.ToLower(strings.TrimSpace(body.Identifier)), "token", token)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleRecoverReset consumes a reset token and sets the new password.
// Invalid, expired and reused tokens share 401 (no distinction); weak
// passwords answer 400. Success revokes every refresh row server-side.
func handleRecoverReset(deps Dependencies, limit *loginLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body recoverReset
		if !decodeJSON(w, r, &body) {
			return
		}
		if strings.TrimSpace(body.Token) == "" || body.NewPassword == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		now := time.Now()
		key := "recover|" + limit.clientIP(r)
		ip := limit.clientIP(r)
		if !limit.check(r.Context(), key, now) {
			slog.Warn("recover throttled", "ip", ip)
			WriteError(w, r, errRateLimited)
			return
		}
		limit.record(r.Context(), key, ip, true, now)
		if deps.Recovery == nil {
			WriteError(w, r, errInternal)
			return
		}
		userID, err := deps.Recovery.Reset(r.Context(), body.Token, body.NewPassword)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if deps.Tokens != nil {
			if err := deps.Tokens.RevokeAllForUser(r.Context(), userID); err != nil {
				slog.Warn("revoke refresh after recovery failed", "error", err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSetUserActive enables or disables a login (lockout response).
// Only admins reach it (manage-users); the version bump kills bearers
// and the Active check rejects refreshes, so the cut is immediate.
func handleSetUserActive(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body setActiveRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		u, err := deps.Users.SetActive(r.Context(), id, body.Active)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, userAccount{
			ID: u.ID, Username: u.Username, Email: u.Email, Active: u.Active,
		})
	}
}
