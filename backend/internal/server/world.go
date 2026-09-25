package server

import "net/http"

type worldCountryItem struct {
	CountryCode string `json:"countryCode"`
	People      int64  `json:"people"`
}

// handleWorldCountries serves GET /api/v1/world/countries. Auth required. It
// returns, per stored ISO country code, how many people the viewer can
// discover there (same exclusions as /users/discover: self, blocks in either
// direction, already followed / requested). Only coarse country-level counts
// are exposed; no individual locations.
func (s *Server) handleWorldCountries(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}
	counts, err := s.users.DiscoverCountries(r.Context(), me.ID)
	if err != nil {
		s.internalError(w, "world countries", err)
		return
	}
	items := make([]worldCountryItem, 0, len(counts))
	for _, c := range counts {
		items = append(items, worldCountryItem{CountryCode: c.CountryCode, People: c.People})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
