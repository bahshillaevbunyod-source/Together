package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeTokenProvider struct {
	token string
	err   error
	calls int
}

func (f *fakeTokenProvider) Token(context.Context) (string, error) {
	f.calls++
	return f.token, f.err
}

func newTestDetector(t *testing.T, serverURL string, provider TokenProvider) *Detector {
	t.Helper()
	detector, err := NewDetector("test-project", "global", serverURL, http.DefaultClient, provider)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	return detector
}

func TestDetectorRequestAndHighestConfidence(t *testing.T) {
	provider := &fakeTokenProvider{token: "test-token"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v3/projects/test-project/locations/global:detectLanguage" {
			t.Errorf("path = %s, want detectLanguage endpoint", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var requestBody struct {
			Content  string `json:"content"`
			MimeType string `json:"mimeType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if requestBody.Content != "холодный" {
			t.Errorf("content = %q, want original text", requestBody.Content)
		}
		if requestBody.MimeType != "text/plain" {
			t.Errorf("mimeType = %q, want text/plain", requestBody.MimeType)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"languages":[{"languageCode":"ru","confidence":0.42},{"languageCode":"mk","confidence":0.61},{"languageCode":"uz","confidence":0.89}]}`))
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, provider)
	got, err := detector.Detect(context.Background(), "холодный")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	want := Detection{LanguageCode: "uz", Confidence: 0.89}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Detect() = %+v, want %+v", got, want)
	}
	if provider.calls != 1 {
		t.Fatalf("token calls = %d, want 1", provider.calls)
	}
}

func TestDetectorRejectsNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, &fakeTokenProvider{token: "token"})
	_, err := detector.Detect(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Detect() error = %v, want HTTP 401 error", err)
	}
}

func TestDetectorRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, &fakeTokenProvider{token: "token"})
	_, err := detector.Detect(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "decode google v3 detect response") {
		t.Fatalf("Detect() error = %v, want malformed JSON error", err)
	}
}

func TestDetectorRejectsEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"languages":[]}`))
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, &fakeTokenProvider{token: "token"})
	_, err := detector.Detect(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "no usable languages") {
		t.Fatalf("Detect() error = %v, want empty-result error", err)
	}
}

func TestDetectorPropagatesTokenError(t *testing.T) {
	wantErr := errors.New("token unavailable")
	provider := &fakeTokenProvider{err: wantErr}
	detector, err := NewDetector("test-project", "global", "http://unused", http.DefaultClient, provider)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	_, err = detector.Detect(context.Background(), "hello")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Detect() error = %v, want %v", err, wantErr)
	}
}

func TestDetectorRejectsBlankInputBeforeTokenRequest(t *testing.T) {
	provider := &fakeTokenProvider{token: "token"}
	detector, err := NewDetector("test-project", "global", "http://unused", http.DefaultClient, provider)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}

	_, err = detector.Detect(context.Background(), " \t\n")
	if err == nil || !strings.Contains(err.Error(), "text is required") {
		t.Fatalf("Detect() error = %v, want blank-input error", err)
	}
	if provider.calls != 0 {
		t.Fatalf("token calls = %d, want 0", provider.calls)
	}
}

func TestDetectorHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, &fakeTokenProvider{token: "token"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := detector.Detect(ctx, "hello")
	if err == nil {
		t.Fatal("Detect() error = nil, want context cancellation error")
	}
}

func TestDetectorUsesDefaultLocationWhenBlank(t *testing.T) {
	provider := &fakeTokenProvider{token: "token"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/projects/test-project/locations/global:detectLanguage" {
			t.Errorf("path = %s, want global location", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"languages":[{"languageCode":"en","confidence":1}]}`))
	}))
	defer server.Close()

	detector, err := NewDetector("test-project", "", server.URL, http.DefaultClient, provider)
	if err != nil {
		t.Fatalf("NewDetector() error = %v", err)
	}
	if _, err := detector.Detect(context.Background(), "hello"); err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
}

func TestDetectorDoesNotTranslate(t *testing.T) {
	provider := &fakeTokenProvider{token: "token"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"languages":[{"languageCode":"en","confidence":1}]}`))
	}))
	defer server.Close()

	detector := newTestDetector(t, server.URL, provider)
	got, err := detector.Detect(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if got.LanguageCode != "en" || got.Confidence != 1 {
		t.Fatalf("Detect() = %+v, want only detection metadata", got)
	}
	if got.LanguageCode == "hello" {
		t.Fatal("detector returned translated content as language code")
	}
}

var _ LanguageDetector = (*Detector)(nil)

var _ = time.Second
