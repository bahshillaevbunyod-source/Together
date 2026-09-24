package conversation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// DirectPeer verifies membership and returns the conversation kind and the
// other participant. It is intentionally optional on Repository so existing
// test doubles remain source-compatible.
func (r *PostgresRepository) DirectPeer(ctx context.Context, conversationID, userID string) (string, string, error) {
	var kind, peer string
	err := r.q.QueryRow(ctx, `SELECT c.type, CASE WHEN c.user_low=$2 THEN c.user_high ELSE c.user_low END FROM conversations c JOIN conversation_participants p ON p.conversation_id=c.id AND p.user_id=$2 WHERE c.id=$1 AND c.type='direct'`, conversationID, userID).Scan(&kind, &peer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotParticipant
	}
	return kind, peer, err
}
