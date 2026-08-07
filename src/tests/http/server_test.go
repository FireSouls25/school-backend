package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"grade/src/core/roles"
	"grade/src/platform/http"
	"grade/src/platform/i18n"
)

func newI18n(t *testing.T) *i18n.Service {
	t.Helper()
	svc, err := i18n.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func writeTestJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestHealthz(t *testing.T) {
	handler := httpapi.Router(newI18n(t))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v, want status ok", body)
	}
}

func TestLocaleMiddleware(t *testing.T) {
	svc := newI18n(t)
	r := chi.NewRouter()
	r.Use(httpapi.MiddlewareLocale(svc))
	r.Get("/lang", func(w http.ResponseWriter, r *http.Request) {
		tr, ok := httpapi.TranslatorFrom(r.Context())
		if !ok {
			http.Error(w, "missing translator", http.StatusInternalServerError)
			return
		}
		writeTestJSON(w, http.StatusOK, map[string]string{"lang": tr.Language().String()})
	})

	tests := []struct {
		name   string
		path   string
		header string
		want   string
	}{
		{name: "default is spanish", path: "/lang", header: "", want: "es"},
		{name: "accept-language header", path: "/lang", header: "es-CO, es;q=0.8", want: "es"},
		{name: "unsupported language falls back", path: "/lang", header: "en-US", want: "es"},
		{name: "query lang override", path: "/lang?lang=en", header: "es", want: "es"},
		{name: "query wins over header", path: "/lang?lang=es", header: "fr", want: "es"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Accept-Language", tt.header)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Lang string `json:"lang"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Lang != tt.want {
				t.Errorf("lang = %q, want %q", body.Lang, tt.want)
			}
		})
	}
}

func TestWriteErrorLocalized(t *testing.T) {
	svc := newI18n(t)
	ctx := httpapi.WithTranslator(context.Background(), svc.TranslatorFor())
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	httpapi.WriteError(w, r, roles.ErrUnknownRole)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var body httpapi.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "roles.err_unknown_role" {
		t.Errorf("code = %q, want roles.err_unknown_role", body.Error.Code)
	}
	if body.Error.Message != "Rol desconocido" {
		t.Errorf("message = %q, want Spanish", body.Error.Message)
	}
}

func TestWriteErrorInternal(t *testing.T) {
	svc := newI18n(t)
	ctx := httpapi.WithTranslator(context.Background(), svc.TranslatorFor())
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	httpapi.WriteError(w, r, errors.New("boom"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var body httpapi.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "http.err_internal" {
		t.Errorf("code = %q, want http.err_internal", body.Error.Code)
	}
	if body.Error.Message != "Error interno del servidor" {
		t.Errorf("message = %q, want Spanish", body.Error.Message)
	}
}
