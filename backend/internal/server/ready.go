package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"together/backend/internal/schema"
)

// handleReady is a readiness probe: 200 when the database is reachable AND the
// live schema is compatible with this backend; 503 otherwise. Reporting
// not-ready on schema drift prevents the API from looking healthy while every
// real request fails with an opaque 500 (the failure mode that took the app
// down when migrations 000017–000022 were unapplied).
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
		})
		return
	}

	// Schema compatibility check. Only runs when the underlying db handle can
	// execute queries (the production *pgxpool.Pool); test doubles that only
	// implement Ping skip it. Probe-only endpoint, so this is not a per-request
	// cost on user traffic.
	if q, ok := s.db.(schema.Queryer); ok {
		missing, err := schema.Verify(ctx, q)
		if err != nil {
			// Could not determine compatibility (e.g. transient). Do not claim
			// ready; report unavailable without leaking internals to the client.
			log.Printf("ready: schema check failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
			})
			return
		}
		if len(missing) > 0 {
			log.Printf("ready: schema incompatible — missing %d object(s): %v", len(missing), missing)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "schema_incompatible",
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
