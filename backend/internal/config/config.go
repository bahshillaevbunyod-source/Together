// Package config loads runtime configuration from the environment.
package config

import (
	"errors"
	"os"
)

// devFallbackDatabaseURL is a convenience DSN used only in dev/test when
// DATABASE_URL is unset. It contains no real secrets.
const devFallbackDatabaseURL = "postgres://postgres:postgres@localhost:5432/together_dev?sslmode=disable"

// Config holds process-wide settings. Extend this as new subsystems
// (auth, translation, …) are added.
type Config struct {
	Env                string
	Port               string
	DatabaseURL        string
	AppOrigin          string
	MediaPublicBaseURL string

	// Storage (S3/R2) — configuration only for now; no SDK/client yet.
	StorageEndpoint        string
	StorageRegion          string
	StorageBucket          string
	StorageAccessKeyID     string
	StorageSecretAccessKey string

	// Translation provider selection. Provider is "stub" (default; no external
	// calls) or a real provider like "google". Credentials live only in env and
	// are never logged. A real provider requires its API key.
	TranslationProvider string
	TranslationAPIKey   string
	TranslationEndpoint string
}

// Load reads configuration from environment variables, applying fallbacks.
// DATABASE_URL is required unless APP_ENV is "development" or "test", where a
// local fallback DSN is used instead.
func Load() (Config, error) {
	cfg := Config{
		Env:                getEnv("APP_ENV", "development"),
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AppOrigin:          getEnv("APP_ORIGIN", "http://localhost:3000"),
		MediaPublicBaseURL: getEnv("MEDIA_PUBLIC_BASE_URL", "http://localhost:8080/media"),

		StorageEndpoint:        os.Getenv("STORAGE_ENDPOINT"),
		StorageRegion:          os.Getenv("STORAGE_REGION"),
		StorageBucket:          os.Getenv("STORAGE_BUCKET"),
		StorageAccessKeyID:     os.Getenv("STORAGE_ACCESS_KEY_ID"),
		StorageSecretAccessKey: os.Getenv("STORAGE_SECRET_ACCESS_KEY"),

		TranslationProvider: getEnv("TRANSLATION_PROVIDER", "stub"),
		TranslationAPIKey:   os.Getenv("TRANSLATION_API_KEY"),
		TranslationEndpoint: os.Getenv("TRANSLATION_ENDPOINT"),
	}

	if cfg.DatabaseURL == "" {
		if cfg.Env == "development" || cfg.Env == "test" {
			cfg.DatabaseURL = devFallbackDatabaseURL
		} else {
			return Config{}, errors.New("DATABASE_URL is required")
		}
	}

	if err := cfg.validateTranslation(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validateTranslation checks the selected translation provider and that a real
// provider has the credentials it needs. The error names the missing variable
// but never echoes any secret value.
func (c *Config) validateTranslation() error {
	switch c.TranslationProvider {
	case "", "stub":
		c.TranslationProvider = "stub"
		return nil
	case "google":
		if c.TranslationAPIKey == "" {
			return errors.New("TRANSLATION_API_KEY is required for TRANSLATION_PROVIDER=google")
		}
		return nil
	default:
		return errors.New("unknown TRANSLATION_PROVIDER (expected: stub, google)")
	}
}

// IsProduction reports whether the app runs in the production environment.
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
