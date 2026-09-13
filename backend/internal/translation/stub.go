package translation

import "context"

// StubService is a minimal, provider-free Service used until a real provider is
// wired in. It validates the request and echoes the input text back as the
// "translation", resolving an omitted source language to "auto". It exists so
// the rest of the system can depend on Service today and swap in Google/Azure/
// DeepL later without changing callers.
type StubService struct{}

// NewStubService returns a ready-to-use stub Service.
func NewStubService() *StubService { return &StubService{} }

// Compile-time assurance that StubService satisfies Service.
var _ Service = (*StubService)(nil)

// Translate validates the request and returns the (unchanged) text. A real
// provider would send text to its API here.
func (s *StubService) Translate(_ context.Context, req Request) (*Result, error) {
	text, source, target, err := req.Validate()
	if err != nil {
		return nil, err
	}
	if source == "" {
		source = "auto"
	}
	return &Result{
		TranslatedText: text,
		SourceLang:     source,
		TargetLang:     target,
	}, nil
}
