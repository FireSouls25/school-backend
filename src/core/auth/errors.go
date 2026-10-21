package auth

// Error is a domain error carrying a stable message key. The i18n layer
// translates Code into a user-facing message; Error is internal English text.
type Error struct {
	code    string
	message string
}

// Error implements the error interface with internal (English) text.
func (e *Error) Error() string { return e.message }

// Code is the stable i18n catalog key for this error.
func (e *Error) Code() string { return e.code }

func newError(code, message string) *Error {
	return &Error{code: code, message: message}
}

var (
	// ErrInvalidSecret is returned when the token secret is missing.
	ErrInvalidSecret = newError("auth.err_invalid_secret", "auth: invalid secret")
	// ErrInvalidTTL is returned when the token lifetime is not positive.
	ErrInvalidTTL = newError("auth.err_invalid_ttl", "auth: invalid ttl")
	// ErrInvalidSubject is returned when minting for an empty subject.
	ErrInvalidSubject = newError("auth.err_invalid_subject", "auth: invalid subject")
	// ErrInvalidToken is returned when a token is malformed, mistyped or
	// signed by another secret. One code on purpose: callers cannot
	// distinguish "forged" from "expired" by message (see ErrExpiredToken
	// for the only intentional distinction, expiry).
	ErrInvalidToken = newError("auth.err_invalid_token", "auth: invalid token")
	// ErrExpiredToken is returned when a well-formed token is past expiry.
	ErrExpiredToken = newError("auth.err_expired_token", "auth: expired token")
	// ErrRevokedToken is returned when a refresh token was rotated or
	// revoked (logout, password change, disable). Login again.
	ErrRevokedToken = newError("auth.err_revoked_token", "auth: revoked token")
)
