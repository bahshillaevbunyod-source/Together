package story

import (
	"strings"
	"testing"
)

// allActiveReadQueries are the read paths that must represent an ACTIVE, VISIBLE
// story: each must enforce both the 24-hour window and the access rule.
var allActiveReadQueries = map[string]string{
	"getActiveVisible": getActiveVisibleQuery,
	"listFeed":         listFeedQuery,
	"listAuthorActive": listAuthorActiveQuery,
}

// allQueries is every SQL constant in the package, used for global invariants
// (e.g. nothing ever consults follow_requests).
var allQueries = map[string]string{
	"create":           createQuery,
	"getByID":          getByIDQuery,
	"getActiveVisible": getActiveVisibleQuery,
	"listFeed":         listFeedQuery,
	"listAuthorActive": listAuthorActiveQuery,
	"canView":          canViewQuery,
	"deleteOwn":        deleteOwnQuery,
	"recordView":       recordViewQuery,
	"hasViewed":        hasViewedQuery,
}

// CRITICAL: a pending follow request must never grant story access. No query in
// this package may reference follow_requests.
func TestPendingRequestNeverGrantsAccess(t *testing.T) {
	for name, q := range allQueries {
		if strings.Contains(q, "follow_requests") {
			t.Fatalf("%s query references follow_requests — pending requests must not grant access", name)
		}
	}
}

// Self can access own story: the access predicate grants the author directly.
func TestSelfCanAccessOwnStory(t *testing.T) {
	if !strings.Contains(sqlViewerCanSeeAuthor, "s.author_id = $1") {
		t.Fatal("access predicate must grant the author (self) access")
	}
	// The standalone relationship check must also treat self as allowed.
	if !strings.Contains(canViewQuery, "$1 = $2") {
		t.Fatal("CanView must treat viewer == author as allowed")
	}
}

// Accepted follower can access: access flows through the canonical follows edge.
func TestAcceptedFollowerCanAccess(t *testing.T) {
	if !strings.Contains(
		sqlViewerCanSeeAuthor,
		"s.author_id IN (SELECT following_id FROM follows WHERE follower_id = $1)",
	) {
		t.Fatal("access predicate must grant accepted followers via the follows table")
	}
	if !strings.Contains(
		canViewQuery,
		"EXISTS (SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2)",
	) {
		t.Fatal("CanView must grant accepted followers via the follows table")
	}
}

// Non-follower cannot access: the ONLY access grants are self OR the follows
// edge. There must be no other membership/relationship source in the predicate.
func TestNonFollowerHasNoOtherGrant(t *testing.T) {
	// follows is the only relationship table consulted for a grant.
	for _, forbidden := range []string{"follow_requests", "post_likes", "conversations"} {
		if strings.Contains(sqlViewerCanSeeAuthor, forbidden) {
			t.Fatalf("access predicate must not consult %q as an access source", forbidden)
		}
	}
	// The grant is exactly self OR follows; blocks only ever REMOVE access.
	if !strings.Contains(sqlViewerCanSeeAuthor, "FROM follows") {
		t.Fatal("access predicate must use follows")
	}
}

// Either-direction block denies access on every active read path.
func TestBlockDeniesEitherDirection(t *testing.T) {
	for _, frag := range []string{
		"bl.blocker_id = $1 AND bl.blocked_id = s.author_id",
		"bl.blocker_id = s.author_id AND bl.blocked_id = $1",
	} {
		if !strings.Contains(sqlViewerCanSeeAuthor, frag) {
			t.Fatalf("access predicate missing block clause %q", frag)
		}
	}
	if !strings.Contains(sqlViewerCanSeeAuthor, "NOT EXISTS") {
		t.Fatal("blocks must remove access (NOT EXISTS)")
	}
	// CanView applies the same bidirectional block check.
	for _, frag := range []string{
		"bl.blocker_id = $1 AND bl.blocked_id = $2",
		"bl.blocker_id = $2 AND bl.blocked_id = $1",
	} {
		if !strings.Contains(canViewQuery, frag) {
			t.Fatalf("CanView missing block clause %q", frag)
		}
	}
}

// Expired excluded / active included: every active read path enforces the
// 24-hour window, so correctness never depends on a cleanup job.
func TestActiveWindowEnforcedOnEveryReadPath(t *testing.T) {
	if !strings.Contains(sqlActiveWindow, "now() - interval '24 hours'") {
		t.Fatal("active window must be created_at > now() - interval '24 hours'")
	}
	for name, q := range allActiveReadQueries {
		if !strings.Contains(q, sqlActiveWindow) {
			t.Fatalf("%s query must enforce the 24-hour active window", name)
		}
	}
}

// Access predicate is enforced on every active read path (get + both lists).
func TestAccessPredicateEnforcedOnEveryReadPath(t *testing.T) {
	for name, q := range allActiveReadQueries {
		if !strings.Contains(q, sqlViewerCanSeeAuthor) {
			t.Fatalf("%s query must enforce the viewer access predicate", name)
		}
	}
}

// Record-view is idempotent: repeat views never create duplicate rows.
func TestRecordViewIdempotent(t *testing.T) {
	if !strings.Contains(recordViewQuery, "INSERT INTO story_views") {
		t.Fatal("recordView must insert into story_views")
	}
	if !strings.Contains(recordViewQuery, "ON CONFLICT DO NOTHING") {
		t.Fatal("recordView must be idempotent (ON CONFLICT DO NOTHING)")
	}
}

// Viewed/unviewed state is derived per-viewer from story_views.
func TestViewedStateDerivedFromStoryViews(t *testing.T) {
	if !strings.Contains(sqlItemColumns, "FROM story_views sv") ||
		!strings.Contains(sqlItemColumns, "sv.viewer_id = $1") ||
		!strings.Contains(sqlItemColumns, "AS viewed") {
		t.Fatal("item projection must derive `viewed` per-viewer from story_views")
	}
	if !strings.Contains(hasViewedQuery, "FROM story_views") {
		t.Fatal("HasViewed must read story_views")
	}
}

// Own delete is scoped to the owner; another user's delete affects no rows and
// is therefore never permitted.
func TestDeleteScopedToOwner(t *testing.T) {
	if !strings.Contains(deleteOwnQuery, "DELETE FROM stories") {
		t.Fatal("deleteOwn must delete from stories")
	}
	if !strings.Contains(deleteOwnQuery, "author_id = $2") {
		t.Fatal("deleteOwn must be scoped to the owner (author_id = $2)")
	}
	if !strings.Contains(deleteOwnQuery, "id = $1") {
		t.Fatal("deleteOwn must target the specific story id")
	}
}

// Chronological ordering: lists are newest-first with a stable id tie-break.
func TestListsOrderedChronologically(t *testing.T) {
	for _, name := range []string{"listFeed", "listAuthorActive"} {
		if !strings.Contains(allActiveReadQueries[name], "ORDER BY s.created_at DESC, s.id DESC") {
			t.Fatalf("%s must order newest-first with a stable id tie-break", name)
		}
	}
}

// The feed keyset cursor is applied so pages never duplicate or skip rows.
func TestFeedKeysetCursor(t *testing.T) {
	for _, frag := range []string{
		"$2::timestamptz IS NULL",
		"s.created_at < $2",
		"s.created_at = $2 AND s.id < $3::uuid",
		"LIMIT $4",
	} {
		if !strings.Contains(listFeedQuery, frag) {
			t.Fatalf("listFeed missing keyset fragment %q", frag)
		}
	}
}

// GetByID is a raw internal lookup: it must NOT apply the access predicate or
// active window (it exists for ownership checks, not for serving content).
func TestGetByIDIsRawInternalLookup(t *testing.T) {
	if strings.Contains(getByIDQuery, sqlActiveWindow) ||
		strings.Contains(getByIDQuery, "follows") ||
		strings.Contains(getByIDQuery, "blocks") {
		t.Fatal("getByID must be a raw lookup without access/expiry checks")
	}
}
