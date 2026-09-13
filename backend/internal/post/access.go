package post

// Shared SQL WHERE fragments for post access rules. They assume the posts table
// is aliased as `p` and that $1 is the viewer's user id. Composing queries from
// these keeps the block / follow logic identical across feed and bookmarks.
const (
	// SQLFollowsAuthor is true when the viewer ($1) follows the post's author.
	SQLFollowsAuthor = `p.author_id IN (SELECT following_id FROM follows WHERE follower_id = $1)`

	// SQLNotBlocked excludes posts where a block exists in either direction
	// between the viewer ($1) and the post's author.
	SQLNotBlocked = `NOT EXISTS (
        SELECT 1 FROM blocks bl
        WHERE (bl.blocker_id = $1 AND bl.blocked_id = p.author_id)
           OR (bl.blocker_id = p.author_id AND bl.blocked_id = $1)
    )`
)
