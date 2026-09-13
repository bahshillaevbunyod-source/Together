package server

import (
	"context"
	"net/http"
	"time"
)

// handleReady is a readiness probe: 200 when the database is reachable,
// 503 otherwise.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
