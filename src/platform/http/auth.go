package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"grade/src/core/auth"
	"grade/src/core/roles"
	"grade/src/core/users"
)

// SubjectHeader carries the caller identity in development and tests.
// Production callers use Authorization: Bearer <token>; the header is
// ignored there, so a raw UUID never authenticates outside development.
const SubjectHeader = "X-Subject-ID"

// codedError is a transport-level error with a stable catalog key.
type codedError struct{ code string }

func (e codedError) Error() string { return e.code }

// Code returns the stable i18n catalog key.
func (e codedError) Code() string { return e.code }

var (
	// errUnauthorized renders 401 when the request carries no valid subject.
	errUnauthorized = codedError{"http.err_unauthorized"}
	// errForbidden renders 403 when the subject lacks the permission.
	errForbidden = codedError{"http.err_forbidden"}
	// errInternal renders 500 for server misconfiguration the caller
	// cannot fix (e.g. login wired without a token service).
	errInternal = codedError{"http.err_internal"}
)

// MiddlewareSubject resolves the caller from the Authorization bearer
// token and stores the subject in the request context. Requests without
// a valid token are rejected; in development (and tests) a valid
// X-Subject-ID still authenticates when no bearer is present, so local
// tooling keeps working without a login round-trip.
// rejected with 401; nothing downstream ever sees an empty subject.
func MiddlewareSubject(next http.Handler) http.Handler {
	return MiddlewareSubjectWith(next, nil, true)
}

// TokenVerifier validates login tokens. It mirrors the method set the
// middleware needs so tests can substitute fakes.
type TokenVerifier interface {
	Verify(token string, now time.Time) (auth.Claims, error)
}

// MiddlewareSubjectWith resolves the caller from the Authorization bearer
// token. When no bearer is present and allowDevIdentity holds, a valid
// X-Subject-ID still authenticates (development and tests only).
func MiddlewareSubjectWith(next http.Handler, tokens TokenVerifier, allowDevIdentity bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tokens != nil {
			if token, ok := bearerToken(r); ok {
				claims, err := tokens.Verify(token, time.Now())
				if err != nil {
					WriteError(w, r, err)
					return
				}
				// Only access bearers open API routes. Refresh tokens
				// (and pre-typ tokens, read as access) go through the
				// typed verifiers in login/refresh handlers.
				if claims.Typ != "" && claims.Typ != auth.AccessTokenType {
					WriteError(w, r, errUnauthorized)
					return
				}
				ctx := WithSubject(r.Context(), claims.Subject)
				ctx = WithClaims(ctx, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if allowDevIdentity {
			if id := strings.TrimSpace(r.Header.Get(SubjectHeader)); id != "" {
				if _, err := uuid.Parse(id); err != nil {
					WriteError(w, r, errUnauthorized)
					return
				}
				next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), id)))
				return
			}
		}
		WriteError(w, r, errUnauthorized)
	})
}

// bearerToken extracts the token from `Authorization: Bearer <token>`.
func bearerToken(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return "", false
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	return token, token != ""
}

// WithSubject stores a subject id in the context.
func WithSubject(ctx context.Context, subjectID string) context.Context {
	return context.WithValue(ctx, contextKeySubject, subjectID)
}

// SubjectFrom extracts the request subject id, reporting whether present.
func SubjectFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKeySubject).(string)
	return id, ok && id != ""
}

// WithClaims stores verified token claims in the context. Dev identities
// (X-Subject-ID) carry no claims; version checks skip them.
func WithClaims(ctx context.Context, claims auth.Claims) context.Context {
	return context.WithValue(ctx, contextKeyClaims, claims)
}

// ClaimsFrom extracts verified token claims, reporting whether present.
func ClaimsFrom(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(contextKeyClaims).(auth.Claims)
	return claims, ok
}

// RequireTokenFresh rejects bearers whose version no longer matches the
// stored account (password rotation, disable) or whose account is now
// inactive. Dev identities without a users row pass through; unknown
// subjects with a bearer fail closed. Missing Users service disables
// the check (tests without auth setup).
func RequireTokenFresh(usersSvc *users.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, hasClaims := ClaimsFrom(r.Context())
			if !hasClaims {
				next.ServeHTTP(w, r)
				return
			}
			if usersSvc == nil {
				next.ServeHTTP(w, r)
				return
			}
			u, err := usersSvc.ByID(r.Context(), claims.Subject)
			if err != nil {
				if errors.Is(err, users.ErrNotFound) {
					next.ServeHTTP(w, r)
					return
				}
				WriteError(w, r, err)
				return
			}
			if !u.Active || u.TokenVersion != claims.Ver {
				WriteError(w, r, errUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission denies with 403 unless the subject holds p. Missing
// subjects yield 401; authorizer failures fail closed with 500 and leak
// nothing.
func RequirePermission(auth roles.Authorizer, p roles.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, ok := SubjectFrom(r.Context())
			if !ok {
				WriteError(w, r, errUnauthorized)
				return
			}
			allowed, err := auth.Can(r.Context(), subject, p)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if !allowed {
				WriteError(w, r, errForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
