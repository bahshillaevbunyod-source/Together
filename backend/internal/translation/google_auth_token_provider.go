package translation

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const GoogleCloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

var (
	errGoogleADCUnavailable   = errors.New("google ADC token source unavailable")
	errGoogleTokenUnavailable = errors.New("google ADC access token unavailable")
)

type defaultTokenSourceFunc func(context.Context, ...string) (oauth2.TokenSource, error)

// GoogleADCTokenProvider obtains bearer tokens through Application Default
// Credentials. It never reads credential files directly or logs tokens.
type GoogleADCTokenProvider struct {
	tokenSource oauth2.TokenSource
}

// NewGoogleADCTokenProvider resolves ADC using the supplied context and the
// Cloud Platform scope. GOOGLE_APPLICATION_CREDENTIALS is handled by ADC.
func NewGoogleADCTokenProvider(ctx context.Context) (*GoogleADCTokenProvider, error) {
	return newGoogleADCTokenProvider(ctx, google.DefaultTokenSource)
}

func newGoogleADCTokenProvider(ctx context.Context, defaultTokenSource defaultTokenSourceFunc) (*GoogleADCTokenProvider, error) {
	if ctx == nil || defaultTokenSource == nil {
		return nil, errGoogleADCUnavailable
	}

	tokenSource, err := defaultTokenSource(ctx, GoogleCloudPlatformScope)
	if err != nil || tokenSource == nil {
		return nil, errGoogleADCUnavailable
	}
	return &GoogleADCTokenProvider{tokenSource: tokenSource}, nil
}

// Token implements TokenProvider for the isolated Google v3 detector.
// The ADC token source receives its context during construction.
func (p *GoogleADCTokenProvider) Token(_ context.Context) (string, error) {
	if p == nil || p.tokenSource == nil {
		return "", errGoogleTokenUnavailable
	}

	token, err := p.tokenSource.Token()
	if err != nil || token == nil {
		return "", errGoogleTokenUnavailable
	}
	accessToken := strings.TrimSpace(token.AccessToken)
	if accessToken == "" {
		return "", errGoogleTokenUnavailable
	}
	return accessToken, nil
}

var _ TokenProvider = (*GoogleADCTokenProvider)(nil)
