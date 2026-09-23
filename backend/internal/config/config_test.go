package config

import (
	"math"
	"os"
	"testing"
)

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q): %v", key, err)
	}
	t.Cleanup(func() {
		if hadOld {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestLoadTranslationDefaultsToStub(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	unsetEnvForTest(t, "SOURCE_LANGUAGE_DETECTION_CONFIDENCE_THRESHOLD")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TranslationProvider != "stub" {
		t.Fatalf("expected stub by default, got %q", cfg.TranslationProvider)
	}
	if cfg.SourceLanguageDetectionConfidenceThreshold != 0.80 || !cfg.SourceLanguageDetectionEnabled {
		t.Fatalf("unexpected source language detection defaults: threshold=%v enabled=%v", cfg.SourceLanguageDetectionConfidenceThreshold, cfg.SourceLanguageDetectionEnabled)
	}
}

func TestLoadSourceLanguageDetectionConfig(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "test-project")
	t.Setenv("SOURCE_LANGUAGE_DETECTION_CONFIDENCE_THRESHOLD", "0.65")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GoogleCloudProjectID != "test-project" || cfg.SourceLanguageDetectionConfidenceThreshold != 0.65 || !cfg.SourceLanguageDetectionEnabled {
		t.Fatalf("unexpected source language detection config: project=%q threshold=%v enabled=%v", cfg.GoogleCloudProjectID, cfg.SourceLanguageDetectionConfidenceThreshold, cfg.SourceLanguageDetectionEnabled)
	}
}

func TestLoadInvalidSourceLanguageDetectionThresholdDisablesDetection(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	for _, raw := range []string{"not-a-number", "NaN", "+Inf", "-Inf", "-0.01", "1.01"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("SOURCE_LANGUAGE_DETECTION_CONFIDENCE_THRESHOLD", raw)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v, want graceful disabled config", err)
			}
			if cfg.SourceLanguageDetectionEnabled {
				t.Fatalf("detection enabled for invalid threshold %q", raw)
			}
			if math.IsNaN(cfg.SourceLanguageDetectionConfidenceThreshold) || math.IsInf(cfg.SourceLanguageDetectionConfidenceThreshold, 0) {
				t.Fatalf("invalid threshold leaked into config: %v", cfg.SourceLanguageDetectionConfidenceThreshold)
			}
		})
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
