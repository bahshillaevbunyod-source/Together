package translation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServiceStubDefault(t *testing.T) {
	for _, p := range []string{"", "stub"} {
		svc, err := NewService(p, "", "")
		if err != nil {
			t.Fatalf("provider %q: unexpected error: %v", p, err)
		}
		if _, ok := svc.(*StubService); !ok {
			t.Fatalf("provider %q: expected StubService, got %T", p, svc)
		}
	}
}

func TestNewServiceGoogleRequiresKey(t *testing.T) {
	if _, err := NewService(ProviderGoogle, "", ""); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("expected ErrProviderNotConfigured, got %v", err)
	}
}

func TestNewServiceUnknownProvider(t *testing.T) {
	if _, err := NewService("made-up", "k", ""); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestGoogleValidatesRequest(t *testing.T) {
	svc, _ := NewService(ProviderGoogle, "k", "http://127.0.0.1:0")
	if _, err := svc.Translate(context.Background(), Request{Text: "", TargetLang: "es"}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("expected ErrEmptyText, got %v", err)
	}
}

// googleMock builds a mock Google Translation v2 endpoint that records the last
// request and returns the given translatedText/detectedSourceLanguage.
func googleMock(t *testing.T, translated, detected string, captured *googleRequest, capturedKey *string, capturedURL *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capturedKey != nil {
			*capturedKey = r.Header.Get("X-Goog-Api-Key")
		}
		if capturedURL != nil {
			*capturedURL = r.URL.String()
		}
		body, _ := io.ReadAll(r.Body)
		if captured != nil {
			_ = json.Unmarshal(body, captured)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := googleResponse{}
		resp.Data.Translations = append(resp.Data.Translations, struct {
			TranslatedText         string `json:"translatedText"`
			DetectedSourceLanguage string `json:"detectedSourceLanguage"`
		}{TranslatedText: translated, DetectedSourceLanguage: detected})
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestGoogleTranslateSuccessAutoDetect(t *testing.T) {
	var body googleRequest
	var key, url string
	srv := googleMock(t, "hola", "en", &body, &key, &url)
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "secret-key", srv.URL)
	res, err := svc.Translate(context.Background(), Request{Text: "hello", TargetLang: "es"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TranslatedText != "hola" {
		t.Fatalf("translatedText not parsed: %q", res.TranslatedText)
	}
	if res.SourceLang != "en" {
		t.Fatalf("detectedSourceLanguage not used: %q", res.SourceLang)
	}
	if res.TargetLang != "es" {
		t.Fatalf("unexpected target: %q", res.TargetLang)
	}
	// Request shape.
	if body.Q != "hello" || body.Target != "es" || body.Format != "text" {
		t.Fatalf("unexpected request body: %+v", body)
	}
	if body.Source != "" {
		t.Fatalf("source must be omitted for auto-detect, got %q", body.Source)
	}
	// Key only in the header, never the URL.
	if key != "secret-key" {
		t.Fatalf("API key not sent via X-Goog-Api-Key header: %q", key)
	}
	if strings.Contains(url, "secret-key") || strings.Contains(url, "key=") {
		t.Fatalf("API key leaked into URL: %q", url)
	}
}

func TestGoogleTranslateWithExplicitSource(t *testing.T) {
	var body googleRequest
	srv := googleMock(t, "hola", "", &body, nil, nil)
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "k", srv.URL)
	res, err := svc.Translate(context.Background(), Request{Text: "hello", SourceLang: "EN", TargetLang: "es"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body.Source != "en" {
		t.Fatalf("explicit source not sent (normalized): %q", body.Source)
	}
	// With a pinned source, the result reports that source.
	if res.SourceLang != "en" {
		t.Fatalf("expected source en, got %q", res.SourceLang)
	}
}

func TestGoogleTranslateAutoSourceOmitted(t *testing.T) {
	var body googleRequest
	srv := googleMock(t, "hola", "fr", &body, nil, nil)
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "k", srv.URL)
	if _, err := svc.Translate(context.Background(), Request{Text: "hello", SourceLang: "auto", TargetLang: "es"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body.Source != "" {
		t.Fatalf(`source "auto" must not be sent, got %q`, body.Source)
	}
}

func TestGoogleTranslateNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	}))
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "k", srv.URL)
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "es"}); !errors.Is(err, ErrProviderRequestFailed) {
		t.Fatalf("expected ErrProviderRequestFailed, got %v", err)
	}
}

func TestGoogleTranslateEmptyTranslations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"translations":[]}}`))
	}))
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "k", srv.URL)
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "es"}); !errors.Is(err, ErrProviderRequestFailed) {
		t.Fatalf("expected ErrProviderRequestFailed, got %v", err)
	}
}

func TestGoogleTranslateBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	svc, _ := NewService(ProviderGoogle, "k", srv.URL)
	if _, err := svc.Translate(context.Background(), Request{Text: "hi", TargetLang: "es"}); !errors.Is(err, ErrProviderRequestFailed) {
		t.Fatalf("expected ErrProviderRequestFailed, got %v", err)
	}
}
