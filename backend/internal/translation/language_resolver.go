package translation

import (
	"context"
	"errors"
	"math"
	"strings"
)

// Detection is a language detector's result for one piece of text.
type Detection struct {
	LanguageCode string
	Confidence   float64
}

// LanguageDetector abstracts a source-language detector such as Google Cloud
// Translation Advanced detection. Implementations must not translate text.
type LanguageDetector interface {
	Detect(ctx context.Context, text string) (Detection, error)
}

type ResolutionSource string

const (
	ResolutionCurrent    ResolutionSource = "current"
	ResolutionContext    ResolutionSource = "context"
	ResolutionUnresolved ResolutionSource = "unresolved"
)

// LanguageResolution is the resolver's conservative result. An unresolved
// result always has an empty LanguageCode and must not be treated as a guess.
type LanguageResolution struct {
	LanguageCode string
	Confidence   float64
	Source       ResolutionSource
}

// LanguageResolver resolves the source language of a current message, with an
// optional bounded same-sender context sample as a fallback.
type LanguageResolver struct {
	detector  LanguageDetector
	threshold float64
}

func NewLanguageResolver(detector LanguageDetector, threshold float64) (*LanguageResolver, error) {
	if detector == nil {
		return nil, errors.New("language detector is required")
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return nil, errors.New("language confidence threshold must be between 0 and 1")
	}
	return &LanguageResolver{detector: detector, threshold: threshold}, nil
}

func (r *LanguageResolver) Resolve(ctx context.Context, currentText, contextText string) (LanguageResolution, error) {
	if strings.TrimSpace(currentText) == "" {
		return unresolvedLanguage(), nil
	}

	current, err := r.detector.Detect(ctx, currentText)
	if err != nil {
		return unresolvedLanguage(), err
	}
	if usableDetection(current, r.threshold) {
		return LanguageResolution{
			LanguageCode: strings.TrimSpace(current.LanguageCode),
			Confidence:   current.Confidence,
			Source:       ResolutionCurrent,
		}, nil
	}

	if strings.TrimSpace(contextText) == "" {
		return unresolvedLanguage(), nil
	}

	contextDetection, err := r.detector.Detect(ctx, contextText)
	if err != nil {
		return unresolvedLanguage(), err
	}
	if usableDetection(contextDetection, r.threshold) {
		return LanguageResolution{
			LanguageCode: strings.TrimSpace(contextDetection.LanguageCode),
			Confidence:   contextDetection.Confidence,
			Source:       ResolutionContext,
		}, nil
	}

	return unresolvedLanguage(), nil
}

func usableDetection(detection Detection, threshold float64) bool {
	code := strings.TrimSpace(detection.LanguageCode)
	return code != "" &&
		detection.Confidence >= threshold &&
		detection.Confidence <= 1 &&
		!math.IsNaN(detection.Confidence) &&
		!math.IsInf(detection.Confidence, 0)
}

func unresolvedLanguage() LanguageResolution {
	return LanguageResolution{Source: ResolutionUnresolved}
}
