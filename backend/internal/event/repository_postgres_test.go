package event

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresEventsRoundTripAndCursor(t *testing.T) {
	dsn := os.Getenv("TOGETHER_TEST_DATABASE_URL")
	if dsn == "" { t.Skip("TOGETHER_TEST_DATABASE_URL not set") }
	u, err := url.Parse(dsn); if err != nil { t.Fatal(err) }
	host := strings.ToLower(u.Hostname()); if host != "localhost" && host != "127.0.0.1" && host != "::1" { t.Skip("integration tests require a local PostgreSQL URL") }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second); defer cancel()
	pool, err := pgxpool.New(ctx, dsn); if err != nil { t.Fatal(err) }; defer pool.Close()
	var creator string; if err := pool.QueryRow(ctx, "SELECT id FROM users ORDER BY id LIMIT 1").Scan(&creator); err != nil { t.Skip("local database has no test user") }
	repo := NewPostgresRepository(pool)
	prefix := "events-repo-test-" + time.Now().UTC().Format("20060102150405.000000000")
	cleanup := func() { _, _ = pool.Exec(context.Background(), "DELETE FROM events WHERE title LIKE $1", prefix+"%") }; defer cleanup()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	inPerson, err := repo.Create(ctx, CreateInput{CreatorID: creator, Title: prefix+"-in-person", StartsAt: start, Timezone: "Asia/Tashkent", EventType: TypeInPerson, Visibility: VisibilityPublic}); if err != nil { t.Fatal(err) }
	onlineURL := "https://example.com/events/test"
	online, err := repo.Create(ctx, CreateInput{CreatorID: creator, Title: prefix+"-online", StartsAt: start.Add(time.Hour), Timezone: "UTC", EventType: TypeOnline, OnlineURL: &onlineURL, Visibility: VisibilityPublic}); if err != nil { t.Fatal(err) }
	if inPerson.Description != nil || inPerson.EndsAt != nil || online.LocationName != nil { t.Fatal("nullable event fields were not preserved") }
	got, err := repo.Get(ctx, inPerson.ID, creator); if err != nil || got.ID != inPerson.ID { t.Fatalf("get event: %v", err) }
	items, err := repo.List(ctx, creator, nil, 1, "", ""); if err != nil || len(items) != 1 { t.Fatalf("list first page: err=%v items=%d", err, len(items)) }
	page2, err := repo.List(ctx, creator, &Cursor{StartsAt: items[0].StartsAt, ID: items[0].ID}, 2, "", ""); if err != nil || len(page2) != 1 || page2[0].ID != online.ID { t.Fatalf("cursor page: err=%v items=%v", err, page2) }
}
