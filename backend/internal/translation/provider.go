package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Supported provider identifiers.
const (
	ProviderStub   = "stub"
	ProviderGoogle = "google"
)

// googleDefaultEndpoint is used when TRANSLATION_ENDPOINT is unset.
const googleDefaultEndpoint = "https://translation.googleapis.com/language/translate/v2"

// googleRequestTimeout bounds a single translation HTTP call.
const googleRequestTimeout = 10 * time.Second

var (
	// ErrProviderNotConfigured is returned when a real provider is selected
	// without the credentials it needs.
	ErrProviderNotConfigured = errors.New("translation: provider not configured")
	// ErrProviderRequestFailed is a safe, opaque error for any provider-side
	// failure (network, non-2xx, malformed response). It never carries the API
	// key or raw upstream detail. Callers already treat translation failures as
	// "leave original content".
	ErrProviderRequestFailed = errors.New("translation: provider request failed")
)

// NewService builds a Service for the given provider. "stub" (or empty) always
// returns the offline stub; a real provider requires its API key. Credentials
// come only from the caller (backend env) and are never logged here.
func NewService(provider, apiKey, endpoint string) (Service, error) {
	switch provider {
	case "", ProviderStub:
		return NewStubService(), nil
	case ProviderGoogle:
		if apiKey == "" {
			return nil, ErrProviderNotConfigured
		}
		if endpoint == "" {
			endpoint = googleDefaultEndpoint
		}
		return &googleService{
			apiKey:   apiKey,
			endpoint: endpoint,
			client:   &http.Client{Timeout: googleRequestTimeout},
		}, nil
	default:
		return nil, fmt.Errorf("translation: unknown provider %q", provider)
	}
}

// googleService calls the Google Cloud Translation Basic (v2) REST API. The API
// key is sent only via the X-Goog-Api-Key header, never in the URL or logs.
type googleService struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

var _ Service = (*googleService)(nil)

// googleRequest is the JSON body for a v2 translate call.
type googleRequest struct {
	Q      string `json:"q"`
	Target string `json:"target"`
	Source string `json:"source,omitempty"`
	Format string `json:"format"`
}

// googleResponse mirrors the relevant fields of the v2 response envelope.
type googleResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText         string `json:"translatedText"`
			DetectedSourceLanguage string `json:"detectedSourceLanguage"`
		} `json:"translations"`
	} `json:"data"`
}

func (g *googleService) Translate(ctx context.Context, req Request) (*Result, error) {
	text, source, target, err := req.Validate()
	if err != nil {
		return nil, err
	}

	body := googleRequest{Q: text, Target: target, Format: "text"}
	// Send an explicit source only when the caller pinned one (not auto-detect).
	if source != "" && source != "auto" {
		body.Source = source
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, ErrProviderRequestFailed
	}

	reqCtx, cancel := context.WithTimeout(ctx, googleRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, g.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrProviderRequestFailed
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Goog-Api-Key", g.apiKey)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, ErrProviderRequestFailed // never surface the raw error (may echo the URL)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrProviderRequestFailed
	}

	var parsed googleResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, ErrProviderRequestFailed
	}
	if len(parsed.Data.Translations) == 0 {
		return nil, ErrProviderRequestFailed
	}

	tr := parsed.Data.Translations[0]
	resolvedSource := source
	if resolvedSource == "" || resolvedSource == "auto" {
		resolvedSource = tr.DetectedSourceLanguage
		if resolvedSource == "" {
			resolvedSource = "auto"
		}
	}

	return &Result{
		TranslatedText: tr.TranslatedText,
		SourceLang:     resolvedSource,
		TargetLang:     target,
	}, nil
}
