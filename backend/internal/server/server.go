// Package server wires HTTP routing and the configured http.Server.
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"together/backend/internal/block"
	"together/backend/internal/bookmark"
	"together/backend/internal/comment"
	"together/backend/internal/config"
	"together/backend/internal/conversation"
	"together/backend/internal/follow"
	"together/backend/internal/followrequest"
	"together/backend/internal/like"
	"together/backend/internal/media"
	"together/backend/internal/notification"
	"together/backend/internal/post"
	"together/backend/internal/ratelimit"
	"together/backend/internal/realtime"
	"together/backend/internal/registration"
	"together/backend/internal/session"
	"together/backend/internal/storage"
	"together/backend/internal/story"
	"together/backend/internal/topic"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

// Pinger is the minimal dependency the readiness probe needs. *pgxpool.Pool
// satisfies it, so the server stays decoupled from the concrete driver.
type Pinger interface {
	Ping(ctx context.Context) error
}

type languageResolver interface {
	Resolve(ctx context.Context, currentText, contextText string) (translation.LanguageResolution, error)
}

// Server holds handler dependencies.
type Server struct {
	cfg            config.Config
	db             Pinger
	users          user.Repository
	sessions       session.Repository
	follows        follow.Repository
	followRequests followrequest.Repository
	followReq      followRequestService
	blocks         block.Repository
	stories        story.Repository
	posts          post.Repository
	topics         topic.Repository
	likes          like.Repository
	comments       comment.Repository
	media          media.Repository
	storage        storage.Repository
	bookmarks      bookmark.Repository
	notifications  notification.Repository
	postCreate     postWithMediaCreator
	followNotify   followNotifier
	likeNotify     likeNotifier
	commentNotify  commentNotifier
	conversations  conversation.Repository
	registrar      registration.Creator
	translator     translation.Service

	languageResolver languageResolver

	hub             *realtime.Hub
	wsUpgrader      websocket.Upgrader
	registerLimiter *ratelimit.Limiter
	loginLimiter    *ratelimit.Limiter
}

// postWithMediaCreator creates a post and its media atomically.
// *postservice.Service satisfies it.
type postWithMediaCreator interface {
	CreatePostWithMedia(ctx context.Context, in post.CreateInput, mediaItems []media.CreateInput) (*post.Post, error)
}

// followNotifier creates a follow edge and its notification atomically.
// *followservice.Service satisfies it.
type followNotifier interface {
	Follow(ctx context.Context, followerID, followingID string) error
}

// followRequestService creates and accepts pending follow requests atomically.
// Request inserts a pending request and (only when newly created) its
// follow_request notification; Accept deletes the request, inserts the accepted
// follows edge, and records the follow notification. *followrequestservice.Service
// satisfies it.
type followRequestService interface {
	Request(ctx context.Context, requesterID, targetID string) (bool, error)
	Accept(ctx context.Context, requesterID, targetID string) error
}

// likeNotifier creates a post like and its notification atomically.
// *likeservice.Service satisfies it.
type likeNotifier interface {
	Like(ctx context.Context, actorID, postID, authorID string) error
}

// commentNotifier creates a comment and its notification atomically.
// *commentservice.Service satisfies it.
type commentNotifier interface {
	CreateComment(ctx context.Context, in comment.CreateInput, postAuthorID string) (*comment.Comment, error)
}

// New builds the HTTP server with sensible timeouts and registered routes.
func New(cfg config.Config, db Pinger, users user.Repository, sessions session.Repository, follows follow.Repository, blocks block.Repository, posts post.Repository, likes like.Repository, comments comment.Repository, media media.Repository, storageRepo storage.Repository, bookmarks bookmark.Repository, notifications notification.Repository, postCreate postWithMediaCreator, followNotify followNotifier, likeNotify likeNotifier, commentNotify commentNotifier, conversations conversation.Repository, dependencies ...any) *http.Server {
	s := newServer(cfg, db, users, sessions, follows, blocks, posts, likes, comments, media, storageRepo, bookmarks, notifications, postCreate, followNotify, likeNotify, commentNotify, conversations, dependencies...)

	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           s.routes(),
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// newServer assembles the Server with all dependencies. New wraps it to build
// the *http.Server; tests use it directly for white-box access (e.g. the hub).
func newServer(cfg config.Config, db Pinger, users user.Repository, sessions session.Repository, follows follow.Repository, blocks block.Repository, posts post.Repository, likes like.Repository, comments comment.Repository, media media.Repository, storageRepo storage.Repository, bookmarks bookmark.Repository, notifications notification.Repository, postCreate postWithMediaCreator, followNotify followNotifier, likeNotify likeNotifier, commentNotify commentNotifier, conversations conversation.Repository, dependencies ...any) *Server {
	var registrar registration.Creator
	var topics topic.Repository
	var followRequests followrequest.Repository
	var followReq followRequestService
	var stories story.Repository
	var resolver languageResolver
	for _, dependency := range dependencies {
		switch dependency := dependency.(type) {
		case registration.Creator:
			registrar = dependency
		case topic.Repository:
			topics = dependency
		case followrequest.Repository:
			followRequests = dependency
		case followRequestService:
			followReq = dependency
		case story.Repository:
			stories = dependency
		case languageResolver:
			resolver = dependency
		}
	}
	s := &Server{
		cfg:            cfg,
		db:             db,
		users:          users,
		sessions:       sessions,
		follows:        follows,
		followRequests: followRequests,
		followReq:      followReq,
		blocks:         blocks,
		stories:        stories,
		posts:          posts,
		topics:         topics,
		likes:          likes,
		comments:       comments,
		media:          media,
		storage:        storageRepo,
		bookmarks:      bookmarks,
		notifications:  notifications,
		postCreate:     postCreate,
		followNotify:   followNotify,
		likeNotify:     likeNotify,
		commentNotify:  commentNotify,
		conversations:  conversations,
		registrar:      registrar,
		translator:     newTranslator(cfg),

		languageResolver: resolver,

		hub: realtime.NewHub(),
		wsUpgrader: websocket.Upgrader{
			// Only accept handshakes from the configured app origin.
			CheckOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == cfg.AppOrigin
			},
		},
		// register: 5 req / 10 min / IP; login: 10 req / 10 min / IP.
		registerLimiter: ratelimit.New(5, 10*time.Minute),
		loginLimiter:    ratelimit.New(10, 10*time.Minute),
	}
	return s
}

// newTranslator selects the translation provider from config. Config.Load has
// already validated the provider/credentials at startup; if construction still
// fails we fall back to the offline stub so the server stays functional and
// never calls an unconfigured external API.
func newTranslator(cfg config.Config) translation.Service {
	svc, err := translation.NewService(cfg.TranslationProvider, cfg.TranslationAPIKey, cfg.TranslationEndpoint)
	if err != nil {
		return translation.NewStubService()
	}
	return svc
}

// routes mounts all handlers and returns the server's HTTP handler.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return s.withCORS(mux)
}

// withCORS allows the browser SPA (served from AppOrigin, a different port) to
// make credentialed cross-origin requests. Only the configured AppOrigin is
// permitted, mirroring the CSRF Origin check, and credentials are allowed so the
// session cookie is sent. Preflight (OPTIONS) requests are answered here.
func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := origin != "" && origin == s.cfg.AppOrigin

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			if allowed {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// registerRoutes is the single place to mount handlers. Future groups
// (auth, posts, ws, translation) are registered here.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("POST /api/v1/auth/register", s.rateLimit(s.registerLimiter, s.handleRegister))
	mux.HandleFunc("POST /api/v1/auth/login", s.rateLimit(s.loginLimiter, s.handleLogin))
	mux.HandleFunc("POST /api/v1/auth/logout", s.csrfProtect(s.handleLogout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireAuth(s.handleMe))

	// Protected routes (require a valid session).
	mux.HandleFunc("GET /api/v1/protected/ping", s.requireAuth(s.handlePing))
	mux.HandleFunc("GET /api/v1/profile", s.requireAuth(s.handleGetProfile))
	mux.HandleFunc("PATCH /api/v1/profile", s.requireAuth(s.csrfProtect(s.handleUpdateProfile)))

	// User search (authenticated). The literal "search" segment takes precedence
	// over the {username} wildcard in the Go 1.22 mux, so it never collides.
	mux.HandleFunc("GET /api/v1/users/search", s.requireAuth(s.handleUserSearch))
	mux.HandleFunc("GET /api/v1/users/discover", s.requireAuth(s.handleUserDiscover))

	// Public routes (no auth required).
	mux.HandleFunc("GET /api/v1/users/{username}", s.handlePublicProfile)
	mux.HandleFunc("GET /api/v1/users/{username}/followers", s.handleFollowersList)
	mux.HandleFunc("GET /api/v1/users/{username}/following", s.handleFollowingList)

	// Topics are public reads with optional authentication. An authenticated
	// viewer receives the same visibility and block filtering as other post
	// reads; anonymous viewers see public posts only.
	mux.HandleFunc("GET /api/v1/topics", s.handleListTopics)
	mux.HandleFunc("GET /api/v1/topics/search", s.handleSearchTopics)
	mux.HandleFunc("GET /api/v1/topics/{slug}", s.handleGetTopic)
	mux.HandleFunc("GET /api/v1/topics/{slug}/posts", s.handleTopicPosts)

	// Follow graph (authenticated, state-changing).
	mux.HandleFunc("POST /api/v1/users/{username}/follow", s.requireAuth(s.csrfProtect(s.handleFollow)))
	mux.HandleFunc("DELETE /api/v1/users/{username}/follow", s.requireAuth(s.csrfProtect(s.handleUnfollow)))

	// Follow requests (private accounts).
	mux.HandleFunc("DELETE /api/v1/users/{username}/follow-request", s.requireAuth(s.csrfProtect(s.handleCancelFollowRequest)))
	mux.HandleFunc("GET /api/v1/follow-requests", s.requireAuth(s.handleListFollowRequests))
	mux.HandleFunc("GET /api/v1/blocks", s.requireAuth(s.handleListBlocked))
	mux.HandleFunc("POST /api/v1/follow-requests/{username}/accept", s.requireAuth(s.csrfProtect(s.handleAcceptFollowRequest)))
	mux.HandleFunc("POST /api/v1/follow-requests/{username}/decline", s.requireAuth(s.csrfProtect(s.handleDeclineFollowRequest)))

	// Block graph (authenticated, state-changing).
	mux.HandleFunc("POST /api/v1/users/{username}/block", s.requireAuth(s.csrfProtect(s.handleBlock)))
	mux.HandleFunc("DELETE /api/v1/users/{username}/block", s.requireAuth(s.csrfProtect(s.handleUnblock)))

	// Stories (private, ephemeral). All authenticated; mutations CSRF-protected.
	mux.HandleFunc("POST /api/v1/stories", s.requireAuth(s.csrfProtect(s.handleCreateStory)))
	mux.HandleFunc("GET /api/v1/stories", s.requireAuth(s.handleListStoriesFeed))
	mux.HandleFunc("GET /api/v1/stories/{id}", s.requireAuth(s.handleGetStory))
	mux.HandleFunc("POST /api/v1/stories/{id}/view", s.requireAuth(s.csrfProtect(s.handleViewStory)))
	mux.HandleFunc("DELETE /api/v1/stories/{id}", s.requireAuth(s.csrfProtect(s.handleDeleteStory)))
	mux.HandleFunc("GET /api/v1/users/{username}/stories", s.requireAuth(s.handleUserStories))

	// Posts.
	mux.HandleFunc("POST /api/v1/posts", s.requireAuth(s.csrfProtect(s.handleCreatePost)))
	mux.HandleFunc("POST /api/v1/posts/{id}/share", s.requireAuth(s.csrfProtect(s.handleSharePost)))
	// Literal "discover" takes precedence over the {id} wildcard in the Go mux.
	mux.HandleFunc("GET /api/v1/posts/discover", s.requireAuth(s.handleDiscoverPosts))
	mux.HandleFunc("GET /api/v1/posts/{id}", s.handleGetPost) // public, optional auth
	mux.HandleFunc("PATCH /api/v1/posts/{id}", s.requireAuth(s.csrfProtect(s.handleUpdatePost)))
	mux.HandleFunc("DELETE /api/v1/posts/{id}", s.requireAuth(s.csrfProtect(s.handleDeletePost)))
	mux.HandleFunc("GET /api/v1/feed", s.requireAuth(s.handleFeed))
	mux.HandleFunc("GET /api/v1/bookmarks", s.requireAuth(s.handleListBookmarks))
	mux.HandleFunc("GET /api/v1/ws", s.handleWebSocket)
	mux.HandleFunc("GET /api/v1/conversations", s.requireAuth(s.handleListConversations))
	mux.HandleFunc("GET /api/v1/messages/search", s.requireAuth(s.handleSearchMessages))
	mux.HandleFunc("POST /api/v1/conversations", s.requireAuth(s.csrfProtect(s.handleOpenConversation)))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", s.requireAuth(s.handleListMessages))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", s.requireAuth(s.csrfProtect(s.handleCreateMessage)))
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", s.requireAuth(s.csrfProtect(s.handleMarkConversationRead)))
	mux.HandleFunc("PUT /api/v1/messages/{id}", s.requireAuth(s.csrfProtect(s.handleUpdateMessage)))
	mux.HandleFunc("DELETE /api/v1/messages/{id}", s.requireAuth(s.csrfProtect(s.handleDeleteMessage)))
	mux.HandleFunc("PUT /api/v1/conversations/{id}/mute", s.requireAuth(s.csrfProtect(s.handleSetConversationMuted)))
	mux.HandleFunc("GET /api/v1/notifications", s.requireAuth(s.handleListNotifications))
	mux.HandleFunc("GET /api/v1/notifications/unread-count", s.requireAuth(s.handleUnreadNotificationCount))
	mux.HandleFunc("POST /api/v1/notifications/{id}/read", s.requireAuth(s.csrfProtect(s.handleMarkNotificationRead)))
	mux.HandleFunc("POST /api/v1/notifications/read-all", s.requireAuth(s.csrfProtect(s.handleMarkAllNotificationsRead)))
	mux.HandleFunc("POST /api/v1/posts/{id}/like", s.requireAuth(s.csrfProtect(s.handleLikePost)))
	mux.HandleFunc("DELETE /api/v1/posts/{id}/like", s.requireAuth(s.csrfProtect(s.handleUnlikePost)))
	mux.HandleFunc("POST /api/v1/posts/{id}/comments", s.requireAuth(s.csrfProtect(s.handleCreateComment)))
	mux.HandleFunc("GET /api/v1/posts/{id}/comments", s.handleListComments) // public, optional auth
	mux.HandleFunc("POST /api/v1/posts/{id}/bookmark", s.requireAuth(s.csrfProtect(s.handleSavePost)))
	mux.HandleFunc("DELETE /api/v1/posts/{id}/bookmark", s.requireAuth(s.csrfProtect(s.handleUnsavePost)))
	mux.HandleFunc("POST /api/v1/media/upload-url", s.requireAuth(s.csrfProtect(s.handleCreateUploadURL)))
	mux.HandleFunc("POST /api/v1/media/confirm", s.requireAuth(s.csrfProtect(s.handleConfirmUpload)))
	mux.HandleFunc("PATCH /api/v1/comments/{id}", s.requireAuth(s.csrfProtect(s.handleUpdateComment)))
	mux.HandleFunc("DELETE /api/v1/comments/{id}", s.requireAuth(s.csrfProtect(s.handleDeleteComment)))
}

// writeJSON is a small helper for JSON responses.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a safe JSON error response ({"error": message}).
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// internalError records the real cause of a 500 server-side and returns the
// generic, non-leaking error to the client. `op` is a short operation label
// (e.g. "auth login: GetByEmail"); the logged error must never contain secrets,
// tokens, cookies, the DSN, or raw request bodies. This keeps client responses
// safe while making schema/DB failures diagnosable in server logs instead of
// vanishing into an opaque "internal error".
func (s *Server) internalError(w http.ResponseWriter, op string, err error) {
	log.Printf("%s: %v", op, err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
