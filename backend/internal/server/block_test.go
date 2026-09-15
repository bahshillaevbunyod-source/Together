package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/session"
	"together/backend/internal/user"
)

// fakeBlockRepo is a test double for block.Repository. When onBlock is set it
// runs on Block, letting tests observe the "remove follows on block" contract.
type fakeBlockRepo struct {
	blockErr     error
	unblockErr   error
	blocked      [][2]string
	unblocked    [][2]string
	onBlock      func(blocker, blocked string)
	hasBetween   bool
	betweenErr   error
	isBlocked    bool
	isBlockedErr error
}

func (f *fakeBlockRepo) Block(_ context.Context, a, b string) error {
	if f.blockErr != nil {
		return f.blockErr
	}
	f.blocked = append(f.blocked, [2]string{a, b})
	if f.onBlock != nil {
		f.onBlock(a, b)
	}
	return nil
}

func (f *fakeBlockRepo) Unblock(_ context.Context, a, b string) error {
	if f.unblockErr != nil {
		return f.unblockErr
	}
	f.unblocked = append(f.unblocked, [2]string{a, b})
	return nil
}

func (f *fakeBlockRepo) IsBlocked(_ context.Context, _, _ string) (bool, error) {
	return f.isBlocked, f.isBlockedErr
}

func (f *fakeBlockRepo) HasBlockBetween(_ context.Context, _, _ string) (bool, error) {
	return f.hasBetween, f.betweenErr
}

func blockSetup(t *testing.T, target *user.User, blocks *fakeBlockRepo) *http.Server {
	t.Helper()
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: target}
	return buildServerWithBlocks(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		blocks,
	)
}

func blockReq(srv *http.Server, method, username string, withCookie, withOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/users/"+username+"/block", nil)
	if withCookie {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	}
	if withOrigin {
		req.Header.Set("Origin", testOrigin)
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestBlockSuccess(t *testing.T) {
	blocks := &fakeBlockRepo{}
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), blocks), http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"blocked":true`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if len(blocks.blocked) != 1 || blocks.blocked[0] != [2]string{"me-id", "target-id"} {
		t.Fatalf("block not recorded correctly: %v", blocks.blocked)
	}
}

func TestBlockDuplicateIdempotent(t *testing.T) {
	blocks := &fakeBlockRepo{}
	srv := blockSetup(t, mkUser("target-id", "target_user"), blocks)
	_ = blockReq(srv, http.MethodPost, "target_user", true, true)
	rec := blockReq(srv, http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate block should be 200, got %d", rec.Code)
	}
}

func TestUnblockSuccess(t *testing.T) {
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), &fakeBlockRepo{}), http.MethodDelete, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"blocked":false`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestBlockSelf(t *testing.T) {
	me := mkUser("me-id", "me_user")
	rec := blockReq(blockSetup(t, me, &fakeBlockRepo{}), http.MethodPost, "me_user", true, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self-block should be 400, got %d", rec.Code)
	}
}

func TestBlockUnknownTarget(t *testing.T) {
	rec := blockReq(blockSetup(t, nil, &fakeBlockRepo{}), http.MethodPost, "ghost", true, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown target should be 404, got %d", rec.Code)
	}
}

func TestBlockUnauthenticated(t *testing.T) {
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), &fakeBlockRepo{}), http.MethodPost, "target_user", false, true)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBlockMissingOrigin(t *testing.T) {
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), &fakeBlockRepo{}), http.MethodPost, "target_user", true, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestBlockRemovesFollows(t *testing.T) {
	// The production repo removes follow edges inside the same transaction.
	// Here we assert the block triggers that removal contract via onBlock.
	removed := false
	blocks := &fakeBlockRepo{onBlock: func(_, _ string) { removed = true }}
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), blocks), http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !removed {
		t.Fatal("block did not trigger follow-removal contract")
	}
}

func TestBlockRepositoryError(t *testing.T) {
	blocks := &fakeBlockRepo{blockErr: errForTest}
	rec := blockReq(blockSetup(t, mkUser("target-id", "target_user"), blocks), http.MethodPost, "target_user", true, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// TestPublicProfileReportsBlocked verifies the public profile carries the
// viewer's block state so the UI can show Unblock and survive a refresh.
func TestPublicProfileReportsBlocked(t *testing.T) {
	me := mkUser("me-id", "me_user")
	target := mkUser("target-id", "target_user")
	users := &fakeUserRepo{byIDUser: me, usernameUser: target}
	srv := buildServerFull(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		users,
		&fakeSessionRepo{active: &session.Session{UserID: "me-id", ExpiresAt: time.Now().Add(time.Hour)}},
		&fakeFollowRepo{},
		&fakeBlockRepo{isBlocked: true},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/target_user", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "raw"})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"isBlocked":true`) {
		t.Fatalf("expected isBlocked true, got %s", rec.Body.String())
	}
}
