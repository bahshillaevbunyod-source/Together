package schema

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeRow returns a preset bool from Scan.
type fakeRow struct {
	exists bool
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) == 1 {
		if p, ok := dest[0].(*bool); ok {
			*p = r.exists
		}
	}
	return nil
}

// fakeQueryer answers existence based on a set of "present" object keys. Keys:
// column checks -> "col:<table>.<column>", table checks -> "tbl:<table>".
type fakeQueryer struct {
	present map[string]bool
	err     error
}

func (q fakeQueryer) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if q.err != nil {
		return fakeRow{err: q.err}
	}
	var key string
	if strings.Contains(sql, "information_schema.columns") {
		key = "col:" + args[0].(string) + "." + args[1].(string)
	} else {
		key = "tbl:" + args[0].(string)
	}
	return fakeRow{exists: q.present[key]}
}

func allPresent() map[string]bool {
	return map[string]bool{
		"col:users.platform_language":             true,
		"col:users.preferred_language":            true,
		"col:users.auto_translate_enabled":        true,
		"col:users.is_private":                    true,
		"col:messages.source_language":            true,
		"col:messages.source_language_confidence": true,
		"col:messages.source_language_resolution": true,
		"col:messages.updated_at":                 true,
		"col:messages.deleted_at":                 true,
		"col:conversation_participants.muted_at":  true,
		"col:conversation_participants.role":      true,
		"col:conversations.type":                  true,
		"col:conversations.name":                  true,
		"col:conversations.description":           true,
		"col:posts.original_post_id":              true,
		"tbl:follow_requests":                     true,
		"tbl:stories":                             true,
		"tbl:story_views":                         true,
		"tbl:message_attachments":                 true,
	}
}

func TestVerifyAllPresent(t *testing.T) {
	missing, err := Verify(context.Background(), fakeQueryer{present: allPresent()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected no missing objects, got %v", missing)
	}
}

func TestVerifyDetectsDrift(t *testing.T) {
	// Reproduce the exact live drift that took auth down: platform_language +
	// is_private columns and the three new tables absent.
	p := allPresent()
	for _, k := range []string{
		"col:users.platform_language", "col:users.is_private",
		"tbl:follow_requests", "tbl:stories", "tbl:story_views",
	} {
		delete(p, k)
	}
	missing, err := Verify(context.Background(), fakeQueryer{present: p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 5 {
		t.Fatalf("expected 5 missing objects, got %d: %v", len(missing), missing)
	}
	joined := strings.Join(missing, ",")
	for _, want := range []string{
		"column users.platform_language", "column users.is_private",
		"table follow_requests", "table stories", "table story_views",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing list should include %q, got %v", want, missing)
		}
	}
}

func TestVerifyPropagatesQueryError(t *testing.T) {
	_, err := Verify(context.Background(), fakeQueryer{err: pgx.ErrTxClosed})
	if err == nil {
		t.Fatal("expected error when the catalog query fails")
	}
}
