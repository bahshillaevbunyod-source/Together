package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"together/backend/internal/config"
	"together/backend/internal/media"
	"together/backend/internal/post"
	"together/backend/internal/user"
)

// fakeMediaRepo is a test double for media.Repository.
type fakeMediaRepo struct {
	createErr     error
	last          media.CreateInput
	byPost        []media.Media
	listErr       error
	batch         map[string][]media.Media
	batchErr      error
	createdMany   []media.CreateInput
	createManyErr error
}

func (f *fakeMediaRepo) Create(_ context.Context, in media.CreateInput) (*media.Media, error) {
	f.last = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &media.Media{
		ID: "media-1", PostID: in.PostID, Type: in.Type, StorageKey: in.StorageKey,
		MimeType: in.MimeType, SizeBytes: in.SizeBytes, Width: in.Width, Height: in.Height,
		DurationMs: in.DurationMs, SortOrder: in.SortOrder, CreatedAt: time.Now(),
	}, nil
}

func (f *fakeMediaRepo) CreateMany(_ context.Context, items []media.CreateInput) error {
	f.createdMany = append(f.createdMany, items...)
	return f.createManyErr
}

func (f *fakeMediaRepo) ListByPost(_ context.Context, _ string) ([]media.Media, error) {
	return f.byPost, f.listErr
}

func (f *fakeMediaRepo) ListByPostIDs(_ context.Context, ids []string) ([]media.Media, error) {
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	var out []media.Media
	for _, id := range ids {
		out = append(out, f.batch[id]...)
	}
	return out, nil
}

func mkMedia(sortOrder int, key string) media.Media {
	w, h := 800, 600
	return media.Media{
		ID: "m-" + key, PostID: validPostID, Type: media.TypeImage, StorageKey: key,
		MimeType: "image/webp", SizeBytes: 1234, Width: &w, Height: &h, SortOrder: sortOrder,
	}
}

// server with a public post owned by author-id, and the given media repo.
func mediaServer(mediaRepo *fakeMediaRepo) *http.Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me, "author-id": mkUser("author-id", "author_user")}}
	return New(
		config.Config{Env: "test", Port: "8080", MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{getPost: mkPost(post.VisibilityPublic, "author-id")}, &fakeLikeRepo{}, &fakeCommentRepo{}, mediaRepo, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
}

func TestPostResponseIncludesMediaInOrder(t *testing.T) {
	mediaRepo := &fakeMediaRepo{byPost: []media.Media{mkMedia(0, "a.webp"), mkMedia(1, "b.webp")}}
	rec := getPost(mediaServer(mediaRepo), validPostID, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var m struct {
		Media []struct {
			URL       string `json:"url"`
			SortOrder int    `json:"sortOrder"`
		} `json:"media"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if len(m.Media) != 2 {
		t.Fatalf("expected 2 media, got %d", len(m.Media))
	}
	if m.Media[0].SortOrder != 0 || m.Media[1].SortOrder != 1 {
		t.Fatalf("media order not preserved: %+v", m.Media)
	}
	if m.Media[0].URL != "https://cdn.example.com/media/a.webp" {
		t.Fatalf("unexpected url: %q", m.Media[0].URL)
	}
	// storage_key must never appear in the response.
	if bodyHas(rec.Body.String(), "storage_key", "storageKey", "a.webp\",\"storage") {
		t.Fatalf("response leaked storage_key: %s", rec.Body.String())
	}
}

func bodyHas(body string, subs ...string) bool {
	for _, s := range subs {
		if contains2(body, s) {
			return true
		}
	}
	return false
}

func contains2(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestPostResponseEmptyMediaIsArray(t *testing.T) {
	rec := getPost(mediaServer(&fakeMediaRepo{}), validPostID, true)
	// media must serialize as [] not null
	if !contains2(rec.Body.String(), `"media":[]`) {
		t.Fatalf("empty media should be [], body: %s", rec.Body.String())
	}
}

func TestPostResponseMediaRepoError(t *testing.T) {
	rec := getPost(mediaServer(&fakeMediaRepo{listErr: errForTest}), validPostID, true)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestFeedIncludesMedia(t *testing.T) {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	feedItem := mkFeedItem("11111111-1111-1111-1111-111111111111")
	mediaRepo := &fakeMediaRepo{batch: map[string][]media.Media{
		"11111111-1111-1111-1111-111111111111": {{ID: "m1", PostID: "11111111-1111-1111-1111-111111111111", Type: "image", StorageKey: "x.webp", MimeType: "image/webp", SizeBytes: 1, SortOrder: 0}},
	}}
	srv := New(
		config.Config{Env: "test", Port: "8080", MediaPublicBaseURL: "https://cdn.example.com/media"},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{feed: []post.FeedItem{feedItem}}, &fakeLikeRepo{}, &fakeCommentRepo{}, mediaRepo, &fakeStorageRepo{}, &fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{}, &fakeLikeNotifier{}, &fakeCommentNotifier{}, &fakeConversationRepo{},
	)
	rec := getFeed(srv, "")
	var resp struct {
		Items []struct {
			Media []struct {
				URL string `json:"url"`
			} `json:"media"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 || len(resp.Items[0].Media) != 1 {
		t.Fatalf("feed item missing media: %s", rec.Body.String())
	}
	if resp.Items[0].Media[0].URL != "https://cdn.example.com/media/x.webp" {
		t.Fatalf("unexpected media url: %q", resp.Items[0].Media[0].URL)
	}
}

func TestFeedEmptyMediaIsArray(t *testing.T) {
	item := mkFeedItem("11111111-1111-1111-1111-111111111111")
	rec := getFeed(feedServer(&fakePostRepo{feed: []post.FeedItem{item}}), "")
	if !contains2(rec.Body.String(), `"media":[]`) {
		t.Fatalf("empty feed media should be [], body: %s", rec.Body.String())
	}
}

func TestMediaRepoCreateListShapes(t *testing.T) {
	// Exercises the fake for create/list shape (production SQL covered by DB).
	repo := &fakeMediaRepo{byPost: []media.Media{mkMedia(0, "a"), mkMedia(1, "b")}}
	created, err := repo.Create(context.Background(), media.CreateInput{PostID: validPostID, Type: media.TypeImage, StorageKey: "a", MimeType: "image/webp", SizeBytes: 1, SortOrder: 0})
	if err != nil || created.StorageKey != "a" {
		t.Fatalf("create failed: %v", err)
	}
	list, err := repo.ListByPost(context.Background(), validPostID)
	if err != nil || len(list) != 2 || list[0].SortOrder != 0 || list[1].SortOrder != 1 {
		t.Fatalf("list order wrong: %+v", list)
	}
}
