package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"together/backend/internal/auth"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

const maxRegisterBodyBytes = 1 << 20 // 1 MiB

const (
	sessionCookieName = "together_session"
	sessionTTL        = 7 * 24 * time.Hour
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type registerRequest struct {
	Email          string `json:"email"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	NativeLanguage string `json:"nativeLanguage"`
	Password       string `json:"password"`
}

// userResponse is the public shape of a user. It never includes password_hash.
type userResponse struct {
	ID             string  `json:"id"`
	Email          *string `json:"email"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"displayName"`
	NativeLanguage string  `json:"nativeLanguage"`
	CreatedAt      string  `json:"createdAt"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRegisterBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req registerRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	username := strings.ToLower(strings.TrimSpace(req.Username))
	displayName := strings.TrimSpace(req.DisplayName)
	nativeLanguage := strings.TrimSpace(req.NativeLanguage)

	if !isValidEmail(email) {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if !isValidUsername(username) {
		writeError(w, http.StatusBadRequest, "username must be 3-30 chars of a-z, 0-9, _")
		return
	}
	if displayName == "" {
		writeError(w, http.StatusBadRequest, "displayName is required")
		return
	}
	if nativeLanguage == "" {
		writeError(w, http.StatusBadRequest, "nativeLanguage is required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooShort) || errors.Is(err, auth.ErrPasswordTooLong) {
			writeError(w, http.StatusBadRequest, "password does not meet the policy")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	created, err := s.users.Create(r.Context(), user.CreateInput{
		Email:          email,
		Username:       username,
		DisplayName:    displayName,
		NativeLanguage: nativeLanguage,
		PasswordHash:   hash,
	})
	if err != nil {
		switch {
		case errors.Is(err, user.ErrDuplicateEmail), errors.Is(err, user.ErrDuplicateUsername):
			writeError(w, http.StatusConflict, "email or username already in use")
		default:
			// Never leak SQL / driver / credential details.
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	// Registration also logs the user in: create a session and set the cookie
	// exactly like handleLogin, so the client is authenticated immediately.
	rawToken, err := session.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	expiresAt := time.Now().Add(sessionTTL)
	if _, err := s.sessions.Create(r.Context(), created.ID, session.HashToken(rawToken), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, rawToken, expiresAt)

	writeJSON(w, http.StatusCreated, toUserResponse(created))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRegisterBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req loginRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	u, err := s.users.GetByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			unauthorized(w)
			return
		}
		// Never leak SQL / driver / credential details.
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Same 401 whether the account has no password or the password is wrong —
	// never reveal which case occurred.
	if u.PasswordHash == nil {
		unauthorized(w)
		return
	}
	if err := auth.CheckPassword(*u.PasswordHash, req.Password); err != nil {
		unauthorized(w)
		return
	}

	// Establish a server-side session. Only the token hash is stored; the raw
	// token goes to the client in an HttpOnly cookie.
	rawToken, err := session.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	expiresAt := time.Now().Add(sessionTTL)
	if _, err := s.sessions.Create(r.Context(), u.ID, session.HashToken(rawToken), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSessionCookie(w, rawToken, expiresAt)

	writeJSON(w, http.StatusOK, toUserResponse(u))
}

// handleMe returns the authenticated user. requireAuth has already resolved
// and validated the session and placed the user in the context.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(u))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Idempotent: no cookie means nothing to delete — still clear and return OK.
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if err := s.sessions.DeleteByTokenHash(r.Context(), session.HashToken(c.Value)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, rawToken string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
}

// unauthorized writes the single, generic auth-failure response. It must be
// identical for unknown email, wrong password, and missing password hash.
func unauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "invalid email or password")
}

func toUserResponse(u *user.User) userResponse {
	return userResponse{
		ID:             u.ID,
		Email:          u.Email,
		Username:       u.Username,
		DisplayName:    u.DisplayName,
		NativeLanguage: u.NativeLanguage,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339),
	}
}

func isValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 30 {
		return false
	}
	return usernamePattern.MatchString(username)
}

func isValidEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	// require a dot in the domain part
	return strings.Contains(email[at+1:], ".")
}
