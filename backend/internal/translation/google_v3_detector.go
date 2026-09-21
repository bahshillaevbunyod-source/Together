package translation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const defaultEndpoint = "https://translation.googleapis.com"

// TokenProvider supplies an OAuth bearer token for a single request.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// Detector is an isolated Google Cloud Translation Advanced language detector.
// It only detects source language; it does not translate content.
type Detector struct {
	client        *http.Client
	endpoint      string
	projectID     string
	location      string
	tokenProvider TokenProvider
}

func NewDetector(projectID, location, endpoint string, client *http.Client, tokenProvider TokenProvider) (*Detector, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, errors.New("google v3 project ID is required")
	}
	if tokenProvider == nil {
		return nil, errors.New("google v3 token provider is required")
	}
	if strings.TrimSpace(location) == "" {
		location = "global"
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultEndpoint
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Detector{
		client:        client,
		endpoint:      strings.TrimRight(endpoint, "/"),
		projectID:     projectID,
		location:      strings.TrimSpace(location),
		tokenProvider: tokenProvider,
	}, nil
}

type detectLanguageResponse struct {
	Languages []detectedLanguage `json:"languages"`
}

type detectedLanguage struct {
	LanguageCode string  `json:"languageCode"`
	Confidence   float64 `json:"confidence"`
}

func (d *Detector) Detect(ctx context.Context, text string) (Detection, error) {
	if strings.TrimSpace(text) == "" {
		return Detection{}, errors.New("text is required for language detection")
	}

	token, err := d.tokenProvider.Token(ctx)
	if err != nil {
		return Detection{}, fmt.Errorf("get google v3 access token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return Detection{}, errors.New("google v3 access token is empty")
	}

	requestBody := struct {
		Content  string `json:"content"`
		MimeType string `json:"mimeType"`
	}{
		Content:  text,
		MimeType: "text/plain",
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return Detection{}, fmt.Errorf("encode google v3 detect request: %w", err)
	}

	detectURL := fmt.Sprintf(
		"%s/v3/projects/%s/locations/%s:detectLanguage",
		d.endpoint,
		url.PathEscape(d.projectID),
		url.PathEscape(d.location),
	)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, detectURL, strings.NewReader(string(body)))
	if err != nil {
		return Detection{}, fmt.Errorf("create google v3 detect request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := d.client.Do(request)
	if err != nil {
		return Detection{}, fmt.Errorf("call google v3 detect language: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Detection{}, fmt.Errorf("google v3 detect language returned %s", response.Status)
	}

	var decoded detectLanguageResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return Detection{}, fmt.Errorf("decode google v3 detect response: %w", err)
	}

	best, ok := bestDetection(decoded.Languages)
	if !ok {
		return Detection{}, errors.New("google v3 detect language returned no usable languages")
	}
	return Detection{
		LanguageCode: best.LanguageCode,
		Confidence:   best.Confidence,
	}, nil
}

func bestDetection(candidates []detectedLanguage) (detectedLanguage, bool) {
	var best detectedLanguage
	found := false
	for _, candidate := range candidates {
		candidate.LanguageCode = strings.TrimSpace(candidate.LanguageCode)
		if candidate.LanguageCode == "" ||
			candidate.Confidence < 0 ||
			candidate.Confidence > 1 ||
			math.IsNaN(candidate.Confidence) ||
			math.IsInf(candidate.Confidence, 0) {
			continue
		}
		if !found || candidate.Confidence > best.Confidence {
			best = candidate
			found = true
		}
	}
	return best, found
}
