package server

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"

	"together/backend/internal/ratelimit"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

type contextKey int

const userContextKey contextKey = iota

// CurrentUser returns the authenticated user that requireAuth placed in the
// context. ok is false when the request was not authenticated.
func CurrentUser(ctx context.Context) (*user.User, bool) {
	u, ok := ctx.Value(userContextKey).(*user.User)
	return u, ok
}

// requireAuth wraps a handler so it runs only for authenticated requests. The
// authenticated user is available via CurrentUser(r.Context()). Reusable for
// future Profile / Messages / Groups / Posts endpoints.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, status := s.authenticate(r)
		if status != 0 {
			if status == http.StatusUnauthorized {
				unauthorized(w)
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, u)
		next(w, r.WithContext(ctx))
	}
}

// csrfProtect guards state-changing requests against CSRF by requiring the
// request Origin to exactly match the configured frontend origin. Safe methods
// (GET/HEAD/OPTIONS) are never checked. Reusable alongside requireAuth, e.g.
// requireAuth(csrfProtect(handler)) for authenticated mutations.
func (s *Server) csrfProtect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isSafeMethod(r.Method) {
			origin := r.Header.Get("Origin")
			if origin == "" || origin != s.cfg.AppOrigin {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		next(w, r)
	}
}

// rateLimit wraps a handler with the given limiter, keyed by client IP. On
// exceed it responds 429 with a Retry-After header.
func (s *Server) rateLimit(l *ratelimit.Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := l.Allow(clientIP(r))
		if !ok {
			secs := int(math.Ceil(retryAfter.Seconds()))
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}

// clientIP identifies the client by RemoteAddr only. X-Forwarded-For is
// intentionally ignored until a trusted-proxy layer exists.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// optionalUser resolves the session cookie to a user for public endpoints
// where auth is optional. It returns nil (never an error) when the request is
// unauthenticated or the session cannot be resolved — anonymous access must
// not break. Reuses authenticate, so session parsing is not duplicated.
func (s *Server) optionalUser(r *http.Request) *user.User {
	u, status := s.authenticate(r)
	if status != 0 {
		return nil
	}
	return u
}

// authenticate resolves the session cookie to a user. It returns (user, 0) on
// success, or (nil, 401/500). The 401 is identical for missing / invalid /
// expired session and unknown user — the reason is never disclosed. Raw tokens
// and hashes are never logged.
func (s *Server) authenticate(r *http.Request) (*user.User, int) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, http.StatusUnauthorized
	}

	sess, err := s.sessions.GetActiveByTokenHash(r.Context(), session.HashToken(c.Value))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, http.StatusUnauthorized
		}
		return nil, http.StatusInternalServerError
	}

	u, err := s.users.GetByID(r.Context(), sess.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, http.StatusUnauthorized
		}
		return nil, http.StatusInternalServerError
	}

	return u, 0
}
