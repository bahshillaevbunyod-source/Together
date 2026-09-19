package block

import (
	"strings"
	"testing"
)

// Blocking must clear both accepted follow edges and pending follow requests in
// BOTH directions, so a blocked pair has no lingering path to gain access.
func TestBlockCleanupQueriesCoverBothDirections(t *testing.T) {
	t.Run("follows both ways", func(t *testing.T) {
		for _, frag := range []string{
			"DELETE FROM follows",
			"follower_id = $1 AND following_id = $2",
			"follower_id = $2 AND following_id = $1",
		} {
			if !strings.Contains(deleteFollowsBothWays, frag) {
				t.Fatalf("follows cleanup missing %q", frag)
			}
		}
	})

	t.Run("follow_requests both ways", func(t *testing.T) {
		for _, frag := range []string{
			"DELETE FROM follow_requests",
			"requester_id = $1 AND target_id = $2",
			"requester_id = $2 AND target_id = $1",
		} {
			if !strings.Contains(deleteFollowRequestsBothWays, frag) {
				t.Fatalf("follow_requests cleanup missing %q", frag)
			}
		}
	})
}
