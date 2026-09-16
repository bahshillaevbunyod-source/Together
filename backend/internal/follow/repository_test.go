package follow

import (
	"strings"
	"testing"
)

func TestFollowListQueriesFilterBlockedViewerRelationships(t *testing.T) {
	for name, query := range map[string]string{
		"followers": listFollowersQuery,
		"following": listFollowingQuery,
	} {
		t.Run(name, func(t *testing.T) {
			for _, fragment := range []string{
				"$2::uuid IS NULL OR NOT EXISTS",
				"bl.blocker_id = $2::uuid AND bl.blocked_id = u.id",
				"bl.blocker_id = u.id AND bl.blocked_id = $2::uuid",
			} {
				if !strings.Contains(query, fragment) {
					t.Fatalf("query missing viewer block filter %q", fragment)
				}
			}
		})
	}
}
