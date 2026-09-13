package server

import (
	"strings"

	"together/backend/internal/media"
)

// mediaResponse is the public shape of a media item. storage_key is never
// exposed; only a derived public URL is returned.
type mediaResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	URL        string `json:"url"`
	MimeType   string `json:"mimeType"`
	Width      *int   `json:"width"`
	Height     *int   `json:"height"`
	DurationMs *int64 `json:"durationMs"`
	SortOrder  int    `json:"sortOrder"`
}

// mediaURL converts a storage_key into a public URL using the configured base.
func mediaURL(base, storageKey string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(storageKey, "/")
}

// toMediaResponses maps media rows to their public shape. It always returns a
// non-nil slice so JSON serializes an empty list as [] (never null).
func (s *Server) toMediaResponses(items []media.Media) []mediaResponse {
	out := make([]mediaResponse, 0, len(items))
	for _, m := range items {
		out = append(out, mediaResponse{
			ID:         m.ID,
			Type:       m.Type,
			URL:        mediaURL(s.cfg.MediaPublicBaseURL, m.StorageKey),
			MimeType:   m.MimeType,
			Width:      m.Width,
			Height:     m.Height,
			DurationMs: m.DurationMs,
			SortOrder:  m.SortOrder,
		})
	}
	return out
}
