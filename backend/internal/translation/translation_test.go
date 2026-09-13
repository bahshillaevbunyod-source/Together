package translation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTranslateSuccess(t *testing.T) {
	svc := NewStubService()
	res, err := svc.Translate(context.Background(), Request{Text: "  hello  ", SourceLang: "EN", TargetLang: "ES"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TranslatedText != "hello" {
		t.Fatalf("text not trimmed/returned: %q", res.TranslatedText)
	}
	if res.SourceLang != "en" || res.TargetLang != "es" {
		t.Fatalf("languages not normalized: %+v", res)
	}
}

func TestTranslateOptionalSourceDefaultsAuto(t *testing.T) {
	svc := NewStubService()
	res, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "fr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SourceLang != "auto" {
		t.Fatalf("expected auto source, got %q", res.SourceLang)
	}
}

func TestTranslateRegionalTarget(t *testing.T) {
	svc := NewStubService()
	res, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "pt-BR"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetLang != "pt-br" {
		t.Fatalf("expected pt-br, got %q", res.TargetLang)
	}
}

func TestTranslateEmptyText(t *testing.T) {
	svc := NewStubService()
	if _, err := svc.Translate(context.Background(), Request{Text: "   ", TargetLang: "es"}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}
}

func TestTranslateTextTooLong(t *testing.T) {
	svc := NewStubService()
	long := strings.Repeat("a", maxTextRunes+1)
	if _, err := svc.Translate(context.Background(), Request{Text: long, TargetLang: "es"}); !errors.Is(err, ErrTextTooLong) {
		t.Fatalf("expected ErrTextTooLong, got %v", err)
	}
}

func TestTranslateMissingTarget(t *testing.T) {
	svc := NewStubService()
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "  "}); !errors.Is(err, ErrMissingTargetLang) {
		t.Fatalf("expected ErrMissingTargetLang, got %v", err)
	}
}

func TestTranslateInvalidTarget(t *testing.T) {
	svc := NewStubService()
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "english!"}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("expected ErrInvalidLanguage, got %v", err)
	}
}

func TestTranslateInvalidSource(t *testing.T) {
	svc := NewStubService()
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", SourceLang: "123", TargetLang: "es"}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("expected ErrInvalidLanguage, got %v", err)
	}
}

func TestTranslateMaxLengthBoundary(t *testing.T) {
	svc := NewStubService()
	exact := strings.Repeat("a", maxTextRunes)
	if _, err := svc.Translate(context.Background(), Request{Text: exact, TargetLang: "es"}); err != nil {
		t.Fatalf("text at the limit should be valid, got %v", err)
	}
}
