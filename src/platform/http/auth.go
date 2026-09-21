package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"grade/src/core/roles"
)

// SubjectHeader carries the caller identity until users/auth lands. The
// backend is agnostic to the request origin as long as the id is valid;
// a real token scheme replaces this header without touching handlers.
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
)

// MiddlewareSubject resolves the caller from the SubjectHeader (UUID) and
// stores it in the request context. Requests without a valid id are
// rejected with 401; nothing downstream ever sees an empty subject.
func MiddlewareSubject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(SubjectHeader))
		if _, err := uuid.Parse(id); err != nil {
			WriteError(w, r, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), id)))
	})
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
