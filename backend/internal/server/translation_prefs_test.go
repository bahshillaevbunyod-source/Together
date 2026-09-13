package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodeProfileResp(t *testing.T, rec *httptest.ResponseRecorder) profileResponse {
	t.Helper()
	var p profileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("invalid profile json: %v", err)
	}
	return p
}

func TestProfileLanguagePrefsDefaults(t *testing.T) {
	rec := getProfile(profileServer(profileUser(t)), true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	p := decodeProfileResp(t, rec)
	if p.PreferredLanguage != nil {
		t.Fatalf("expected nil preferredLanguage by default, got %v", *p.PreferredLanguage)
	}
	if p.AutoTranslateEnabled {
		t.Fatal("expected autoTranslateEnabled=false by default")
	}
}

func TestProfileReturnsStoredLanguagePrefs(t *testing.T) {
	u := userWithPassword(t, "strongpass")
	lang := "es"
	u.PreferredLanguage = &lang
	u.AutoTranslateEnabled = true
	rec := getProfile(profileServer(&fakeUserRepo{byIDUser: u}), true)
	p := decodeProfileResp(t, rec)
	if p.PreferredLanguage == nil || *p.PreferredLanguage != "es" {
		t.Fatalf("preferredLanguage not returned: %+v", p.PreferredLanguage)
	}
	if !p.AutoTranslateEnabled {
		t.Fatal("autoTranslateEnabled not returned")
	}
}

func TestPatchLanguagePrefs(t *testing.T) {
	users := profileUser(t)
	rec := patchProfile(profileServer(users), `{"preferredLanguage":"es","autoTranslateEnabled":true}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !users.lastUpdate.PreferredLanguage.Set || users.lastUpdate.PreferredLanguage.Value == nil || *users.lastUpdate.PreferredLanguage.Value != "es" {
		t.Fatalf("preferredLanguage not persisted: %+v", users.lastUpdate.PreferredLanguage)
	}
	if users.lastUpdate.AutoTranslateEnabled == nil || !*users.lastUpdate.AutoTranslateEnabled {
		t.Fatalf("autoTranslateEnabled not persisted: %v", users.lastUpdate.AutoTranslateEnabled)
	}
}

func TestPatchPreferredLanguageClear(t *testing.T) {
	users := profileUser(t)
	rec := patchProfile(profileServer(users), `{"preferredLanguage":null}`, true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !users.lastUpdate.PreferredLanguage.Set || users.lastUpdate.PreferredLanguage.Value != nil {
		t.Fatalf("expected preferredLanguage cleared, got %+v", users.lastUpdate.PreferredLanguage)
	}
}

func TestPatchPreferredLanguageInvalid(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"preferredLanguage":"english!"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPatchAutoTranslateInvalidType(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"autoTranslateEnabled":"yes"}`, true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPatchLanguagePrefsUnauthenticated(t *testing.T) {
	rec := patchProfile(profileServer(profileUser(t)), `{"autoTranslateEnabled":true}`, false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
