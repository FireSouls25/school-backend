// Package i18n provides multilanguage support. All user-facing text lives in
// embedded catalog files (src/platform/i18n/catalogs); code keeps English.
// Only Spanish (es) ships today; adding a locale means adding a catalog file.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// DefaultLanguage is used when a request does not negotiate a language.
const DefaultLanguage = "es"

// Language is a base language tag (e.g. "es", "en").
type Language string

// String returns the language tag.
func (l Language) String() string { return string(l) }

// ParseLanguage parses a tolerant language tag and reduces it to its base
// language (es-CO -> es). It accepts the forms used in Accept-Language.
func ParseLanguage(s string) (Language, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("i18n: empty language tag")
	}
	tag, err := language.Parse(s)
	if err != nil {
		return "", fmt.Errorf("i18n: parse %q: %w", s, err)
	}
	base, _ := tag.Base()
	return Language(base.String()), nil
}

// Coded is implemented by domain errors carrying a stable catalog key.
type Coded interface {
	Code() string
}

// MessageKey returns the catalog key carried by a coded error.
func MessageKey(err error) (string, bool) {
	var coded Coded
	if errors.As(err, &coded) {
		return coded.Code(), true
	}
	return "", false
}

//go:embed catalogs/*.json
var catalogsFS embed.FS

// Service holds the message catalogs and builds per-request Translators.
type Service struct {
	bundle *i18n.Bundle
}

// NewService loads the embedded message catalogs.
func NewService() (*Service, error) {
	bundle := i18n.NewBundle(language.Spanish)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	entries, err := fs.Glob(catalogsFS, "catalogs/*.json")
	if err != nil {
		return nil, fmt.Errorf("i18n: list catalogs: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("i18n: no catalogs embedded")
	}
	for _, path := range entries {
		if _, err := bundle.LoadMessageFileFS(catalogsFS, path); err != nil {
			return nil, fmt.Errorf("i18n: load %s: %w", path, err)
		}
	}
	return &Service{bundle: bundle}, nil
}

// Translator renders catalog messages for a single language.
type Translator interface {
	// Language returns the resolved language of this translator.
	Language() Language
	// Translate returns the message for key. When the message is missing it
	// returns the key itself. data provides template variables.
	Translate(key string, data map[string]any) string
	// TranslateError renders a coded error's user-facing message. Non-coded
	// errors fall back to the generic internal-error message.
	TranslateError(err error) string
}

// TranslatorFor returns a Translator matching the best supported language
// among the preferences. Unsupported or empty tags fall back to the default.
func (s *Service) TranslatorFor(preferred ...string) Translator {
	loc := i18n.NewLocalizer(s.bundle, preferred...)
	return &translator{lang: s.resolveLanguage(preferred), loc: loc}
}

func (s *Service) resolveLanguage(preferred []string) Language {
	for _, p := range preferred {
		lang, err := ParseLanguage(p)
		if err != nil {
			continue
		}
		for _, tag := range s.bundle.LanguageTags() {
			base, _ := tag.Base()
			if base.String() == lang.String() {
				return lang
			}
		}
	}
	return Language(DefaultLanguage)
}

type translator struct {
	lang Language
	loc  *i18n.Localizer
}

func (t *translator) Language() Language { return t.lang }

func (t *translator) Translate(key string, data map[string]any) string {
	cfg := &i18n.LocalizeConfig{MessageID: key}
	if len(data) > 0 {
		cfg.TemplateData = data
	}
	msg, err := t.loc.Localize(cfg)
	if err != nil || msg == "" {
		return key
	}
	return msg
}

func (t *translator) TranslateError(err error) string {
	if key, ok := MessageKey(err); ok {
		return t.Translate(key, nil)
	}
	return t.Translate("http.err_internal", nil)
}
