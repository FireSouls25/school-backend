package httpapi

import (
	"context"
	"net/http"
	"strings"

	"grade/src/platform/i18n"
)

// contextKey is an unexported type for request context values.
type contextKey int

const (
	contextKeyTranslator contextKey = iota
)

// MiddlewareLocale resolves the request language from the Accept-Language
// header, allows an explicit ?lang= query override (which wins), and stores
// the resulting Translator in the request context.
func MiddlewareLocale(svc *i18n.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			preferred := make([]string, 0, 2)
			if q := r.URL.Query().Get("lang"); q != "" {
				preferred = append(preferred, q)
			}
			preferred = append(preferred, parseAcceptLanguage(r.Header.Get("Accept-Language"))...)
			ctx := WithTranslator(r.Context(), svc.TranslatorFor(preferred...))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithTranslator stores a Translator in the context. Handlers and tests use
// it to render user-facing messages.
func WithTranslator(ctx context.Context, tr i18n.Translator) context.Context {
	return context.WithValue(ctx, contextKeyTranslator, tr)
}

// TranslatorFrom extracts the request Translator, reporting whether present.
func TranslatorFrom(ctx context.Context) (i18n.Translator, bool) {
	tr, ok := ctx.Value(contextKeyTranslator).(i18n.Translator)
	return tr, ok
}

// parseAcceptLanguage splits an Accept-Language header into tags in order,
// ignoring quality values.
func parseAcceptLanguage(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		tag := strings.TrimSpace(strings.SplitN(p, ";", 2)[0])
		if tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
