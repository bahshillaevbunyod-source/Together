package translation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type fakeOAuthTokenSource struct {
	token *oauth2.Token
	err   error
}

func (f fakeOAuthTokenSource) Token() (*oauth2.Token, error) {
	return f.token, f.err
}

func TestNewGoogleADCTokenProviderUsesCloudPlatformScope(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "test")
	called := false
	provider, err := newGoogleADCTokenProvider(ctx, func(gotCtx context.Context, scopes ...string) (oauth2.TokenSource, error) {
		called = true
		if gotCtx != ctx {
			t.Fatal("ADC context was not propagated")
		}
		if len(scopes) != 1 || scopes[0] != GoogleCloudPlatformScope {
			t.Fatalf("scopes = %v, want cloud platform scope", scopes)
		}
		return fakeOAuthTokenSource{token: &oauth2.Token{AccessToken: "test-token"}}, nil
	})
	if err != nil {
		t.Fatalf("newGoogleADCTokenProvider() error = %v", err)
	}
	if !called || provider == nil {
		t.Fatal("ADC provider was not created")
	}
}

func TestGoogleADCTokenProviderReturnsToken(t *testing.T) {
	provider := &GoogleADCTokenProvider{tokenSource: fakeOAuthTokenSource{token: &oauth2.Token{AccessToken: "test-token"}}}
	got, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != "test-token" {
		t.Fatalf("Token() = %q, want test token", got)
	}
}

func TestGoogleADCTokenProviderSanitizesSourceError(t *testing.T) {
	secret := "sensitive-token-text"
	provider := &GoogleADCTokenProvider{tokenSource: fakeOAuthTokenSource{err: errors.New(secret)}}
	_, err := provider.Token(context.Background())
	if !errors.Is(err, errGoogleTokenUnavailable) {
		t.Fatalf("Token() error = %v, want sanitized token error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("Token() error exposed source error text")
	}
}

func TestGoogleADCTokenProviderRejectsEmptyToken(t *testing.T) {
	provider := &GoogleADCTokenProvider{tokenSource: fakeOAuthTokenSource{token: &oauth2.Token{AccessToken: "   "}}}
	_, err := provider.Token(context.Background())
	if !errors.Is(err, errGoogleTokenUnavailable) {
		t.Fatalf("Token() error = %v, want empty-token error", err)
	}
}

func TestNewGoogleADCTokenProviderSanitizesADCError(t *testing.T) {
	secret := "credential-path-should-not-appear"
	_, err := newGoogleADCTokenProvider(context.Background(), func(context.Context, ...string) (oauth2.TokenSource, error) {
		return nil, errors.New(secret)
	})
	if !errors.Is(err, errGoogleADCUnavailable) {
		t.Fatalf("newGoogleADCTokenProvider() error = %v, want sanitized ADC error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("ADC error exposed source error text")
	}
}

func TestNewGoogleADCTokenProviderRejectsNilInputs(t *testing.T) {
	if _, err := newGoogleADCTokenProvider(nil, func(context.Context, ...string) (oauth2.TokenSource, error) {
		return fakeOAuthTokenSource{}, nil
	}); !errors.Is(err, errGoogleADCUnavailable) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := newGoogleADCTokenProvider(context.Background(), nil); !errors.Is(err, errGoogleADCUnavailable) {
		t.Fatalf("nil factory error = %v", err)
	}
}
