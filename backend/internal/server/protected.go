package server

import "net/http"

// handlePing is a minimal protected endpoint used to verify requireAuth.
func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
