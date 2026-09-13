package server

import "net/http"

// handleHealth is a liveness probe. It never touches the database.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
