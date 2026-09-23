package translation

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

type fakeLanguageDetector struct {
	results map[string]Detection
	errors  map[string]error
	calls   []string
}

func (f *fakeLanguageDetector) Detect(_ context.Context, text string) (Detection, error) {
	f.calls = append(f.calls, text)
	if err := f.errors[text]; err != nil {
		return Detection{}, err
	}
	return f.results[text], nil
}

func newTestResolver(t *testing.T, detector *fakeLanguageDetector, threshold float64) *LanguageResolver {
	t.Helper()
	resolver, err := NewLanguageResolver(detector, threshold)
	if err != nil {
		t.Fatalf("NewLanguageResolver() error = %v", err)
	}
	return resolver
}

func TestLanguageResolverConfidentCurrentWinsWithoutContext(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "ru", Confidence: 0.98},
			"context": {LanguageCode: "en", Confidence: 0.99},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := (LanguageResolution{LanguageCode: "ru", Confidence: 0.98, Source: ResolutionCurrent}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
	if !reflect.DeepEqual(detector.calls, []string{"current"}) {
		t.Fatalf("detector calls = %v, want only current message", detector.calls)
	}
}

func TestLanguageResolverLowConfidenceCurrentUsesContext(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "fr", Confidence: 0.3},
			"context": {LanguageCode: "ru", Confidence: 0.94},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := (LanguageResolution{LanguageCode: "ru", Confidence: 0.94, Source: ResolutionContext}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
	if !reflect.DeepEqual(detector.calls, []string{"current", "context"}) {
		t.Fatalf("detector calls = %v, want current then context", detector.calls)
	}
}

func TestLanguageResolverLowConfidenceBothIsUnresolved(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "ru", Confidence: 0.3},
			"context": {LanguageCode: "en", Confidence: 0.4},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := (LanguageResolution{Source: ResolutionUnresolved}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
}

func TestLanguageResolverLowConfidenceWithoutContextIsUnresolved(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "ru", Confidence: 0.3},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := (LanguageResolution{Source: ResolutionUnresolved}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
	if !reflect.DeepEqual(detector.calls, []string{"current"}) {
		t.Fatalf("detector calls = %v, want only current message", detector.calls)
	}
}

func TestLanguageResolverCurrentLanguageCanDifferFromContext(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "uz", Confidence: 0.2},
			"context": {LanguageCode: "ru", Confidence: 0.91},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.LanguageCode != "ru" || resolution.Source != ResolutionContext {
		t.Fatalf("Resolve() = %+v, want contextual ru resolution", resolution)
	}
}

func TestLanguageResolverConfidentCurrentWinsWhenSenderChangesLanguage(t *testing.T) {
	detector := &fakeLanguageDetector{
		results: map[string]Detection{
			"current": {LanguageCode: "uz", Confidence: 0.9},
			"context": {LanguageCode: "ru", Confidence: 0.99},
		},
	}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := (LanguageResolution{LanguageCode: "uz", Confidence: 0.9, Source: ResolutionCurrent}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
}

func TestLanguageResolverDetectorErrorDoesNotInventLanguage(t *testing.T) {
	wantErr := errors.New("detector unavailable")
	detector := &fakeLanguageDetector{errors: map[string]error{"current": wantErr}}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "context")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Resolve() error = %v, want %v", err, wantErr)
	}
	if want := (LanguageResolution{Source: ResolutionUnresolved}); !reflect.DeepEqual(resolution, want) {
		t.Fatalf("Resolve() = %+v, want %+v", resolution, want)
	}
	if !reflect.DeepEqual(detector.calls, []string{"current"}) {
		t.Fatalf("detector calls = %v, want no context fallback after error", detector.calls)
	}
}

func TestLanguageResolverUsesInjectedThreshold(t *testing.T) {
	detector := &fakeLanguageDetector{results: map[string]Detection{
		"current": {LanguageCode: "en", Confidence: 0.75},
	}}
	resolver := newTestResolver(t, detector, 0.7)

	resolution, err := resolver.Resolve(context.Background(), "current", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.LanguageCode != "en" || resolution.Source != ResolutionCurrent {
		t.Fatalf("Resolve() = %+v, want current en resolution", resolution)
	}
}

func TestLanguageResolverPropagatesCurrentConfidence(t *testing.T) {
	detector := &fakeLanguageDetector{results: map[string]Detection{
		"current": {LanguageCode: "en", Confidence: 0.87},
	}}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "current", "")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.LanguageCode != "en" || resolution.Confidence != 0.87 || resolution.Source != ResolutionCurrent {
		t.Fatalf("Resolve() = %+v, want en/0.87/current", resolution)
	}
}

func TestLanguageResolverInvalidDetectionDoesNotBecomeFakeLanguage(t *testing.T) {
	detector := &fakeLanguageDetector{results: map[string]Detection{
		"empty": {LanguageCode: "", Confidence: 1},
		"nan":   {LanguageCode: "ru", Confidence: math.NaN()},
	}}
	resolver := newTestResolver(t, detector, 0.8)

	for _, text := range []string{"empty", "nan"} {
		resolution, err := resolver.Resolve(context.Background(), text, "")
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", text, err)
		}
		if resolution.Source != ResolutionUnresolved || resolution.LanguageCode != "" {
			t.Fatalf("Resolve(%q) = %+v, want unresolved without a language", text, resolution)
		}
	}
}

func TestLanguageResolverEmptyCurrentDoesNotCallDetector(t *testing.T) {
	detector := &fakeLanguageDetector{}
	resolver := newTestResolver(t, detector, 0.8)

	resolution, err := resolver.Resolve(context.Background(), "   ", "context")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.Source != ResolutionUnresolved || len(detector.calls) != 0 {
		t.Fatalf("Resolve() = %+v with calls %v, want unresolved without detector calls", resolution, detector.calls)
	}
}

func TestNewLanguageResolverRejectsInvalidThreshold(t *testing.T) {
	detector := &fakeLanguageDetector{}
	for _, threshold := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if _, err := NewLanguageResolver(detector, threshold); err == nil {
			t.Fatalf("NewLanguageResolver(%v) error = nil, want validation error", threshold)
		}
	}
}
