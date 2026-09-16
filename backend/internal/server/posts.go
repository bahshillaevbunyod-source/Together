package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/storage"
	"together/backend/internal/user"
)

const (
	maxPostBodyBytes    = 1 << 20 // 1 MiB
	maxPostContentRunes = 5000
	maxPostMedia        = 8 // product limit: at most 8 images per post
)

var validVisibility = map[string]bool{
	post.VisibilityPublic:    true,
	post.VisibilityFollowers: true,
	post.VisibilityPrivate:   true,
}

// storageClassForVisibility maps a post visibility to the storage class its
// media must live in: public posts use the public bucket; followers-only and
// private posts use the private bucket.
func storageClassForVisibility(visibility string) storage.Class {
	if visibility == post.VisibilityPublic {
		return storage.ClassPublic
	}
	return storage.ClassPrivate
}

type createPostRequest struct {
	Content     string   `json:"content"`
	Visibility  string   `json:"visibility"`
	StorageKeys []string `json:"storageKeys"`
}

type postAuthorResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type postResponse struct {
	ID            string             `json:"id"`
	Author        postAuthorResponse `json:"author"`
	Content       *string            `json:"content"`
	Visibility    string             `json:"visibility"`
	CreatedAt     string             `json:"createdAt"`
	UpdatedAt     string             `json:"updatedAt"`
	LikesCount    int64              `json:"likesCount"`
	LikedByMe     bool               `json:"likedByMe"`
	CommentsCount int64              `json:"commentsCount"`
	SavedByMe     bool               `json:"savedByMe"`
	Media         []mediaResponse    `json:"media"`

	// Translation fields are null unless the viewer opted in and a translation
	// succeeded. Content always holds the original, untranslated text.
	TranslatedContent *string `json:"translatedContent"`
	SourceLanguage    *string `json:"sourceLanguage"`
	TargetLanguage    *string `json:"targetLanguage"`
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	me, ok := CurrentUser(r.Context())
	if !ok {
		unauthorized(w)
		return
	}

	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPostBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req createPostRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = post.VisibilityPublic
	}
	if !validVisibility[visibility] {
		writeError(w, http.StatusBadRequest, "invalid visibility")
		return
	}

	// Resolve media from confirmed uploads. type/mime/size/sortOrder are decided
	// by the backend — the client only supplies storage keys. Media must live in
	// the storage class that matches the post's visibility (public post -> public
	// bucket; followers/private -> private bucket), so a restricted post can never
	// attach a permanently-public object and vice versa.
	mediaItems, ok := s.resolveMediaForPost(w, r, me.ID, req.StorageKeys, visibility)
	if !ok {
		return
	}

	content := strings.TrimSpace(req.Content)
	hasText := content != ""
	if hasText && utf8.RuneCountInString(content) > maxPostContentRunes {
		writeError(w, http.StatusBadRequest, "content is too long")
		return
	}
	if !hasText && len(mediaItems) == 0 {
		writeError(w, http.StatusBadRequest, "post must have text or media")
		return
	}

	var contentPtr *string
	if hasText {
		contentPtr = &content
	}

	// author_id always comes from the authenticated user, never the request.
	created, err := s.postCreate.CreatePostWithMedia(r.Context(), post.CreateInput{
		AuthorID:   me.ID,
		Content:    contentPtr,
		Visibility: visibility,
	}, mediaItems)
	if err != nil {
		if errors.Is(err, media.ErrMediaAlreadyAttached) {
			writeError(w, http.StatusConflict, "media already attached")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp, err := s.buildPostResponse(r.Context(), created, me, me)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// resolveMediaForPost validates the storage keys into media inputs. It enforces
// the count limit, rejects duplicates, and confirms each object via
// validateUploadedObject. On any failure it writes the response and returns
// ok=false. sortOrder follows the array order.
func (s *Server) resolveMediaForPost(w http.ResponseWriter, r *http.Request, userID string, keys []string, visibility string) ([]media.CreateInput, bool) {
	if len(keys) > maxPostMedia {
		writeError(w, http.StatusBadRequest, "too many media")
		return nil, false
	}

	// The post's visibility dictates which storage class its media must live in.
	wantClass := storageClassForVisibility(visibility)

	seen := make(map[string]bool, len(keys))
	items := make([]media.CreateInput, 0, len(keys))
	for i, key := range keys {
		if seen[key] {
			writeError(w, http.StatusBadRequest, "duplicate media")
			return nil, false
		}
		seen[key] = true

		// Post media may only come from the uploads/ (public) or private/
		// namespaces — never an avatar.
		v, err := s.validateUploadedObject(r.Context(), userID, key, "uploads", "private")
		if err != nil {
			switch {
			case errors.Is(err, errInvalidStorageKey),
				errors.Is(err, errUnsupportedMedia),
				errors.Is(err, errObjectMissing):
				writeError(w, http.StatusBadRequest, "invalid media")
			default:
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return nil, false
		}

		// Enforce that the object's storage class matches the post visibility so
		// restricted posts never carry a public-bucket object (which would be
		// permanently reachable) and public posts never carry a private object.
		if storageClassForKey(v.StorageKey) != wantClass {
			writeError(w, http.StatusBadRequest, "media does not match post visibility")
			return nil, false
		}

		items = append(items, media.CreateInput{
			Type:       v.Type,
			StorageKey: v.StorageKey,
			MimeType:   v.MimeType,
			SizeBytes:  v.SizeBytes,
			SortOrder:  i,
		})
	}
	return items, true
}

func toPostResponse(p *post.Post, author *user.User) postResponse {
	return postResponse{
		ID: p.ID,
		Author: postAuthorResponse{
			ID:          author.ID,
			Username:    author.Username,
			DisplayName: author.DisplayName,
			AvatarURL:   author.AvatarURL,
		},
		Content:    p.Content,
		Visibility: p.Visibility,
		CreatedAt:  p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  p.UpdatedAt.Format(time.RFC3339),
		Media:      []mediaResponse{},
	}
}
