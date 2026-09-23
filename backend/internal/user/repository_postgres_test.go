package user

import (
	"strings"
	"testing"
)

func TestDiscoverCandidateExcludesOutgoingFollowRequests(t *testing.T) {
	for _, want := range []string{
		"FROM follow_requests fr",
		"fr.requester_id = $1",
		"fr.target_id = u.id",
	} {
		if !strings.Contains(discoverCandidate, want) {
			t.Fatalf("discoverCandidate must exclude outgoing follow requests; missing %q", want)
		}
	}
}
