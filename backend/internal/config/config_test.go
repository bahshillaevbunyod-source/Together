package config

import "testing"

func TestLoadTranslationDefaultsToStub(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TranslationProvider != "stub" {
		t.Fatalf("expected stub by default, got %q", cfg.TranslationProvider)
	}
}

func TestLoadGoogleRequiresAPIKey(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("TRANSLATION_PROVIDER", "google")
	t.Setenv("TRANSLATION_API_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when google provider has no API key")
	}
}

func TestLoadGoogleWithKeyOK(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("TRANSLATION_PROVIDER", "google")
	t.Setenv("TRANSLATION_API_KEY", "secret-key")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TranslationProvider != "google" || cfg.TranslationAPIKey != "secret-key" {
		t.Fatalf("unexpected config: %+v", cfg.TranslationProvider)
	}
}

func TestLoadUnknownProvider(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("TRANSLATION_PROVIDER", "made-up")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
