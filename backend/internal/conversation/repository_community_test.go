package conversation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Regression: the community branch used a bare "SELECT $2", which Postgres
// types as text; combined with the uuid comparisons the statement failed to
// prepare (SQLSTATE 42P08) and every message send returned 500.
func TestOtherParticipantQueryTypesCommunitySenderAsUUID(t *testing.T) {
	if !strings.Contains(otherParticipantQuery, "SELECT $2::uuid WHERE EXISTS") ||
		!strings.Contains(otherParticipantQuery, "p.user_id=$2::uuid") {
		t.Fatalf("community sender parameter must be cast to uuid:\n%s", otherParticipantQuery)
	}
	if strings.Contains(otherParticipantQuery, "SELECT $2 WHERE") {
		t.Fatal("untyped SELECT $2 makes Postgres infer text and breaks prepare")
	}
}

// TestOtherParticipantQueryPreparesOnPostgres exercises the real SQL against a
// local database when TOGETHER_TEST_DATABASE_URL is set. It is read-only: it
// prepares the statement and looks up a random (non-existent) conversation.
func TestOtherParticipantQueryPreparesOnPostgres(t *testing.T) {
	url := os.Getenv("TOGETHER_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TOGETHER_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	// Every statement on the text and attachment send paths must prepare
	// (parse/type-check only; nothing is executed or written).
	for name, sql := range map[string]string{
		"other_participant":     otherParticipantQuery,
		"has_block_between":     hasBlockBetweenQuery,
		"insert_message":        insertMessageQuery,
		"insert_attachment":     insertMessageAttachmentQuery,
		"touch_conversation":    touchConversationQuery,
		"recent_sender_context": recentSenderContextQuery,
		"set_language_metadata": setMessageLanguageMetadataQuery,
		"update_message":        updateMessageQuery,
		"delete_message":        deleteMessageQuery,
	} {
		if _, err := conn.Prepare(ctx, name, sql); err != nil {
			t.Fatalf("%s must prepare: %v", name, err)
		}
	}
	var recipient *string
	err = conn.QueryRow(ctx, otherParticipantQuery,
		"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002").Scan(&recipient)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown conversation should yield no row, got %v", err)
	}
}

// For group/channel conversations the lookup yields the sender itself, so the
// message path must accept recipient == sender (solo communities included).
func TestCreateMessageWithVoiceAttachmentInGroup(t *testing.T) {
	voice := Attachment{StorageKey: "users/u1/private/voice.webm", Filename: "voice.webm", Type: "voice", MimeType: "audio/webm", SizeBytes: 42}
	for _, content := range []string{"", "11"} {
		tx := &fakeTx{recipient: ptr("u1")}
		repo, beginner := newRepo(tx)
		m, recipient, err := repo.CreateMessageWithAttachment(context.Background(), "group-1", "u1", content, voice)
		if err != nil {
			t.Fatalf("content %q: unexpected error: %v", content, err)
		}
		if recipient != "u1" || m.Content != content || m.Attachment == nil || m.Attachment.Type != "voice" {
			t.Fatalf("content %q: unexpected group voice message: %+v", content, m)
		}
		if !beginner.tx.committed {
			t.Fatalf("content %q: group voice message must commit", content)
		}
	}
}

func TestCreateMessageWithVoiceAttachmentRejectsNonMember(t *testing.T) {
	tx := &fakeTx{recipient: nil} // community lookup yields NULL for non-members
	repo, _ := newRepo(tx)
	_, _, err := repo.CreateMessageWithAttachment(context.Background(), "group-1", "outsider", "",
		Attachment{StorageKey: "users/outsider/private/voice.webm", Filename: "voice.webm", Type: "voice", MimeType: "audio/webm", SizeBytes: 42})
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant, got %v", err)
	}
	if tx.msgSQL != "" {
		t.Fatal("non-member must be rejected before any insert")
	}
}
