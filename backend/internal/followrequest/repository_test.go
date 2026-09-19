package followrequest

import (
	"strings"
	"testing"
)

// The pending-request table must be independent of the follows graph. A request
// row is inserted into follow_requests only, never into follows.
func TestCreateWritesFollowRequestsNotFollows(t *testing.T) {
	if !strings.Contains(createQuery, "INSERT INTO follow_requests") {
		t.Fatal("create must insert into follow_requests")
	}
	if strings.Contains(createQuery, "follows") {
		t.Fatal("create must never touch the follows table")
	}
	if !strings.Contains(createQuery, "ON CONFLICT DO NOTHING") {
		t.Fatal("create must be idempotent (ON CONFLICT DO NOTHING)")
	}
}

// Cancel (by requester) and decline (by target) both delete the same row keyed
// on (requester_id, target_id), and never delete a follows edge.
func TestDeleteTargetsExactPairInFollowRequests(t *testing.T) {
	for _, frag := range []string{
		"DELETE FROM follow_requests",
		"requester_id = $1",
		"target_id = $2",
	} {
		if !strings.Contains(deleteQuery, frag) {
			t.Fatalf("delete query missing %q", frag)
		}
	}
	if strings.Contains(deleteQuery, "follows ") || strings.Contains(deleteQuery, "FROM follows") {
		t.Fatal("delete must never touch the follows table")
	}
}

// The incoming-request list must apply the same viewer-aware bidirectional
// block filter as the followers/following lists, and paginate by keyset.
func TestListIncomingHasBlockFilterAndKeyset(t *testing.T) {
	for _, frag := range []string{
		"FROM follow_requests fr",
		"JOIN users u ON u.id = fr.requester_id",
		"fr.target_id = $1",
		"bl.blocker_id = $1 AND bl.blocked_id = u.id",
		"bl.blocker_id = u.id AND bl.blocked_id = $1",
		"fr.created_at < $2",
		"fr.created_at = $2 AND fr.requester_id < $3::uuid",
		"ORDER BY fr.created_at DESC, fr.requester_id DESC",
		"LIMIT $4",
	} {
		if !strings.Contains(listIncomingQuery, frag) {
			t.Fatalf("list incoming query missing %q", frag)
		}
	}
}

// Counting incoming requests reads follow_requests, never follows, so a pending
// request can never inflate follower counts.
func TestCountIncomingReadsFollowRequestsOnly(t *testing.T) {
	if !strings.Contains(countIncomingQuery, "FROM follow_requests") {
		t.Fatal("count must read follow_requests")
	}
	if strings.Contains(countIncomingQuery, "follows") {
		t.Fatal("count must never read the follows table")
	}
}
