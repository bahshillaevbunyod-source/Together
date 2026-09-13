// Package translation defines a provider-agnostic text translation abstraction.
// Concrete providers (Google, Azure, DeepL, …) implement Service; callers depend
// only on the interface so the backend stays swappable.
package translation

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxTextRunes bounds a single translation request.
const maxTextRunes = 5000

// Errors returned by request validation.
var (
	// ErrEmptyText is returned when the input text is blank after trimming.
	ErrEmptyText = errors.New("translation: text is required")
	// ErrTextTooLong is returned when the input exceeds the length limit.
	ErrTextTooLong = errors.New("translation: text is too long")
	// ErrMissingTargetLang is returned when no target language is given.
	ErrMissingTargetLang = errors.New("translation: target language is required")
	// ErrInvalidLanguage is returned when a language code is malformed.
	ErrInvalidLanguage = errors.New("translation: invalid language code")
)

// languagePattern matches a lenient BCP-47-ish code, e.g. "en", "pt-BR".
var languagePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// ValidLanguage reports whether code is a syntactically valid language code by
// the same rules the translation package uses for requests. Callers that store
// language preferences validate with this so rules stay in one place.
func ValidLanguage(code string) bool {
	return languagePattern.MatchString(code)
}

// Request is a translation ask. SourceLang is optional ("" means auto-detect);
// TargetLang is required. Language codes are case-insensitive.
type Request struct {
	Text       string
	SourceLang string
	TargetLang string
}

// Result is a completed translation. SourceLang is the resolved source
// ("auto" when the caller let the provider detect it).
type Result struct {
	TranslatedText string
	SourceLang     string
	TargetLang     string
}

// Service translates text from an optional source language into a required
// target language. Implementations must validate input (see Request.Validate).
type Service interface {
	Translate(ctx context.Context, req Request) (*Result, error)
}

// Validate checks the request and returns the normalized text, source and
// target. Target is required and validated; a non-empty source is validated;
// both codes are lower-cased. Text is trimmed and length-checked.
func (r Request) Validate() (text, source, target string, err error) {
	text = strings.TrimSpace(r.Text)
	if text == "" {
		return "", "", "", ErrEmptyText
	}
	if utf8.RuneCountInString(text) > maxTextRunes {
		return "", "", "", ErrTextTooLong
	}

	target = strings.ToLower(strings.TrimSpace(r.TargetLang))
	if target == "" {
		return "", "", "", ErrMissingTargetLang
	}
	if !languagePattern.MatchString(target) {
		return "", "", "", ErrInvalidLanguage
	}

	source = strings.ToLower(strings.TrimSpace(r.SourceLang))
	if source == "auto" {
		source = "" // sentinel: let the provider auto-detect
	}
	if source != "" && !languagePattern.MatchString(source) {
		return "", "", "", ErrInvalidLanguage
	}

	return text, source, target, nil
}
