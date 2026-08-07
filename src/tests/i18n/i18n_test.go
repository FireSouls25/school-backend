package i18n_test

import (
	"errors"
	"testing"

	"grade/src/core/roles"
	"grade/src/platform/i18n"
)

func newService(t *testing.T) *i18n.Service {
	t.Helper()
	svc, err := i18n.NewService()
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestParseLanguage(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{name: "spanish", input: "es", want: "es"},
		{name: "spanish colombia", input: "es-CO", want: "es"},
		{name: "spanish colombia lowercase", input: "es-co", want: "es"},
		{name: "english", input: "en", want: "en"},
		{name: "english us", input: "en-US", want: "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := i18n.ParseLanguage(tt.input)
			if err != nil {
				t.Fatalf("ParseLanguage(%q): %v", tt.input, err)
			}
			if got.String() != tt.want {
				t.Errorf("ParseLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseLanguageInvalid(t *testing.T) {
	for _, input := range []string{"", "   ", "123", "zzzz", "es@#$"} {
		if _, err := i18n.ParseLanguage(input); err == nil {
			t.Errorf("ParseLanguage(%q) should fail", input)
		}
	}
}

func TestParseLanguageWellFormedUnknownAccepted(t *testing.T) {
	// BCP-47 well-formed but unregistered tags parse fine; unsupported
	// languages simply fall back to the default catalog at translation time.
	if _, err := i18n.ParseLanguage("not-a-tag"); err != nil {
		t.Errorf("ParseLanguage(not-a-tag) should succeed: %v", err)
	}
}

func TestTranslateKnownKey(t *testing.T) {
	tr := newService(t).TranslatorFor()
	if got := tr.Language().String(); got != i18n.DefaultLanguage {
		t.Fatalf("default language = %q, want %q", got, i18n.DefaultLanguage)
	}
	got := tr.Translate("roles.err_unknown_role", nil)
	if want := "Rol desconocido"; got != want {
		t.Errorf("Translate = %q, want %q", got, want)
	}
}

func TestTranslateFallsBackToDefaultLanguage(t *testing.T) {
	tr := newService(t).TranslatorFor("fr-FR", "en")
	if got := tr.Language().String(); got != "es" {
		t.Errorf("language = %q, want es (unsupported falls back)", got)
	}
	if got := tr.Translate("roles.err_unknown_role", nil); got != "Rol desconocido" {
		t.Errorf("Translate = %q, want Spanish fallback", got)
	}
}

func TestTranslateMissingKeyReturnsKey(t *testing.T) {
	tr := newService(t).TranslatorFor()
	if got := tr.Translate("no.such.key", nil); got != "no.such.key" {
		t.Errorf("Translate = %q, want the key itself", got)
	}
}

func TestTranslateErrorCoded(t *testing.T) {
	tr := newService(t).TranslatorFor()
	if got := tr.TranslateError(roles.ErrUnknownRole); got != "Rol desconocido" {
		t.Errorf("TranslateError = %q, want Spanish", got)
	}
}

func TestTranslateErrorNonCoded(t *testing.T) {
	tr := newService(t).TranslatorFor()
	if got := tr.TranslateError(errors.New("boom")); got != "Error interno del servidor" {
		t.Errorf("TranslateError = %q, want generic internal message", got)
	}
}

func TestMessageKey(t *testing.T) {
	key, ok := i18n.MessageKey(roles.ErrUnknownRole)
	if !ok || key != "roles.err_unknown_role" {
		t.Errorf("MessageKey = (%q, %v)", key, ok)
	}
	if _, ok := i18n.MessageKey(errors.New("boom")); ok {
		t.Error("non-coded error should not yield a key")
	}
}
