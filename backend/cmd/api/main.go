// Command api starts the Together HTTP API server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"together/backend/internal/block"
	"together/backend/internal/bookmark"
	"together/backend/internal/comment"
	"together/backend/internal/commentservice"
	"together/backend/internal/config"
	"together/backend/internal/conversation"
	"together/backend/internal/db"
	"together/backend/internal/follow"
	"together/backend/internal/followservice"
	"together/backend/internal/like"
	"together/backend/internal/likeservice"
	"together/backend/internal/media"
	"together/backend/internal/notification"
	"together/backend/internal/post"
	"together/backend/internal/postservice"
	"together/backend/internal/server"
	"together/backend/internal/session"
	"together/backend/internal/storage"
	"together/backend/internal/user"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Create the connection pool (connects lazily). Never log the DSN.
	pool, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to create database pool: %v", err)
	}
	defer pool.Close()

	// Verify connectivity, but do not block startup on it — /ready reports
	// live DB status, so the API stays observable even if the DB is down.
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := pool.Ping(pingCtx); err != nil {
		log.Printf("warning: database not reachable at startup: %v", err)
	} else {
		log.Println("database connection OK")
	}
	cancelPing()

	users := user.NewPostgresRepository(pool)
	sessions := session.NewPostgresRepository(pool)
	follows := follow.NewPostgresRepository(pool)
	blocks := block.NewPostgresRepository(pool)
	posts := post.NewPostgresRepository(pool)
	likes := like.NewPostgresRepository(pool)
	comments := comment.NewPostgresRepository(pool)
	mediaRepo := media.NewPostgresRepository(pool)
	storageRepo := storage.NewS3Repository(cfg)
	bookmarks := bookmark.NewPostgresRepository(pool)
	notifications := notification.NewPostgresRepository(pool)
	postCreator := postservice.New(pool, posts, mediaRepo)
	followNotifier := followservice.New(pool, follows, notifications)
	likeNotifier := likeservice.New(pool, likes, notifications)
	commentNotifier := commentservice.New(pool, comments, notifications)
	conversations := conversation.NewPostgresRepository(pool)
	srv := server.New(cfg, pool, users, sessions, follows, blocks, posts, likes, comments, mediaRepo, storageRepo, bookmarks, notifications, postCreator, followNotifier, likeNotifier, commentNotifier, conversations)

	// Start the server in the background.
	go func() {
		log.Printf("Together API listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Block until an interrupt/terminate signal arrives.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Graceful shutdown with a bounded timeout.
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}
