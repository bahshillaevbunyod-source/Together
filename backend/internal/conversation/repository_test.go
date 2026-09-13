package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeRow struct{ scan func(dest ...any) error }

func (r fakeRow) Scan(dest ...any) error { return r.scan(dest...) }

// fakeTx records the statements it runs and replays scripted rows. QueryRow
// dispatches on the SQL so it serves the conversation upsert, the membership
// EXISTS check, and the message insert.
type fakeTx struct {
	queryRowSQL  string // last conversation-upsert query
	queryRowArgs []any  // last conversation-upsert args
	msgSQL       string // message-insert query
	msgArgs      []any  // message-insert args
	execSQL      string
	execArgs     []any
	recipient    *string // other-participant lookup result
	recipientErr error   // fails the other-participant lookup (e.g. pgx.ErrNoRows)
	scanErr      error   // fails the conversation upsert scan
	msgScanErr   error   // fails the message insert scan
	execErr      error
	commitErr    error
	committed    bool
	rolledBack   bool
}

func (f *fakeTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "WHEN user_low"):
		return fakeRow{scan: func(dest ...any) error {
			if f.recipientErr != nil {
				return f.recipientErr
			}
			if p, ok := dest[0].(**string); ok {
				*p = f.recipient
			}
			return nil
		}}
	case strings.Contains(sql, "INSERT INTO messages"):
		f.msgSQL = sql
		f.msgArgs = args
		return fakeRow{scan: func(dest ...any) error {
			if f.msgScanErr != nil {
				return f.msgScanErr
			}
			// id, conversation_id, sender_id, content, created_at
			vals := []any{"msg-1", args[0], args[1], args[2], time.Now()}
			for i, d := range dest {
				switch p := d.(type) {
				case *string:
					*p = vals[i].(string)
				case *time.Time:
					*p = vals[i].(time.Time)
				}
			}
			return nil
		}}
	default: // conversation upsert
		f.queryRowSQL = sql
		f.queryRowArgs = args
		return fakeRow{scan: func(dest ...any) error {
			if f.scanErr != nil {
				return f.scanErr
			}
			// id, user_low, user_high, created_at, updated_at
			now := time.Now()
			vals := []any{"conv-1", args[0], args[1], now, now}
			for i, d := range dest {
				switch p := d.(type) {
				case *string:
					*p = vals[i].(string)
				case *time.Time:
					*p = vals[i].(time.Time)
				}
			}
			return nil
		}}
	}
}

func (f *fakeTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execSQL = sql
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = true
	return nil
}

type fakeBeginner struct {
	tx       *fakeTx
	beginErr error
}

func (b *fakeBeginner) Begin(context.Context) (Tx, error) {
	if b.beginErr != nil {
		return nil, b.beginErr
	}
	return b.tx, nil
}

func newRepo(tx *fakeTx) (*PostgresRepository, *fakeBeginner) {
	b := &fakeBeginner{tx: tx}
	return &PostgresRepository{db: b}, b
}

// fakeRows is a minimal pgx.Rows over scripted message rows.
type fakeRows struct {
	rows [][]any // each row: id, sender_id, content, created_at
	i    int
	err  error
}

func (r *fakeRows) Next() bool {
	if r.i < len(r.rows) {
		r.i++
		return true
	}
	return false
}

func (r *fakeRows) Scan(dest ...any) error {
	row := r.rows[r.i-1]
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = row[i].(string)
		case *time.Time:
			*p = row[i].(time.Time)
		case *int64:
			*p = row[i].(int64)
		case **string:
			if row[i] == nil {
				*p = nil
			} else {
				v := row[i].(string)
				*p = &v
			}
		case **time.Time:
			if row[i] == nil {
				*p = nil
			} else {
				v := row[i].(time.Time)
				*p = &v
			}
		}
	}
	return nil
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) TypeMap() *pgtype.Map                         { return nil }

// fakeQueryer serves the membership EXISTS check, the list query and Exec.
type fakeQueryer struct {
	isParticipant bool
	membershipErr error
	rows          *fakeRows
	queryErr      error
	lastListArgs  []any
	execTag       pgconn.CommandTag
	execErr       error
	lastExecArgs  []any
}

func (q *fakeQueryer) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return fakeRow{scan: func(dest ...any) error {
		if q.membershipErr != nil {
			return q.membershipErr
		}
		if p, ok := dest[0].(*bool); ok {
			*p = q.isParticipant
		}
		return nil
	}}
}

func (q *fakeQueryer) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	q.lastListArgs = args
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	return q.rows, nil
}

func (q *fakeQueryer) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	q.lastExecArgs = args
	return q.execTag, q.execErr
}

func TestOpenRejectsSameUser(t *testing.T) {
	tx := &fakeTx{}
	repo, b := newRepo(tx)
	_, err := repo.OpenPrivateConversation(context.Background(), "u1", "u1")
	if !errors.Is(err, ErrSameUser) {
		t.Fatalf("expected ErrSameUser, got %v", err)
	}
	if b.tx.committed || b.tx.queryRowSQL != "" {
		t.Fatal("must not touch the database for a self-conversation")
	}
}

func TestOpenCanonicalizesPair(t *testing.T) {
	// Pass the higher id first; it must be stored as user_high.
	tx := &fakeTx{}
	repo, _ := newRepo(tx)
	c, err := repo.OpenPrivateConversation(context.Background(), "zzz", "aaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tx.queryRowArgs) != 2 || tx.queryRowArgs[0] != "aaa" || tx.queryRowArgs[1] != "zzz" {
		t.Fatalf("pair not canonicalized: %v", tx.queryRowArgs)
	}
	if c.UserLow != "aaa" || c.UserHigh != "zzz" {
		t.Fatalf("unexpected conversation pair: %+v", c)
	}
}

func TestOpenUpsertAndParticipants(t *testing.T) {
	tx := &fakeTx{}
	repo, _ := newRepo(tx)
	c, err := repo.OpenPrivateConversation(context.Background(), "aaa", "bbb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID != "conv-1" {
		t.Fatalf("expected conv-1, got %q", c.ID)
	}
	// Upsert returns the existing row on duplicate pairs.
	if !strings.Contains(tx.queryRowSQL, "ON CONFLICT") || !strings.Contains(tx.queryRowSQL, "RETURNING") {
		t.Fatalf("upsert query missing conflict/returning: %s", tx.queryRowSQL)
	}
	// Both participant rows created for the conversation.
	if !strings.Contains(tx.execSQL, "conversation_participants") {
		t.Fatalf("participants not inserted: %s", tx.execSQL)
	}
	if len(tx.execArgs) != 3 || tx.execArgs[0] != "conv-1" || tx.execArgs[1] != "aaa" || tx.execArgs[2] != "bbb" {
		t.Fatalf("unexpected participant args: %v", tx.execArgs)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestOpenBeginErrorSafe(t *testing.T) {
	repo := &PostgresRepository{db: &fakeBeginner{beginErr: errors.New("down")}}
	if _, err := repo.OpenPrivateConversation(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}

func TestOpenParticipantErrorRollsBack(t *testing.T) {
	tx := &fakeTx{execErr: errors.New("boom")}
	repo, _ := newRepo(tx)
	if _, err := repo.OpenPrivateConversation(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestOpenUpsertErrorRollsBack(t *testing.T) {
	tx := &fakeTx{scanErr: errors.New("boom")}
	repo, _ := newRepo(tx)
	if _, err := repo.OpenPrivateConversation(context.Background(), "a", "b"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

// ---- CreateMessage ----

func ptr(s string) *string { return &s }

func TestCreateMessageSuccess(t *testing.T) {
	tx := &fakeTx{recipient: ptr("other-1")}
	repo, _ := newRepo(tx)
	m, recipient, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil || m.ID != "msg-1" || m.ConversationID != "conv-1" || m.SenderID != "sender-1" {
		t.Fatalf("unexpected message: %+v", m)
	}
	if recipient != "other-1" {
		t.Fatalf("recipient not resolved server-side: %q", recipient)
	}
	if m.Content != "hello" {
		t.Fatalf("content not trimmed: %q", m.Content)
	}
	if len(tx.msgArgs) != 3 || tx.msgArgs[2] != "hello" {
		t.Fatalf("trimmed content not passed to insert: %v", tx.msgArgs)
	}
	// conversation updated_at bumped
	if !strings.Contains(tx.execSQL, "UPDATE conversations") || len(tx.execArgs) != 1 || tx.execArgs[0] != "conv-1" {
		t.Fatalf("conversation not touched: %s %v", tx.execSQL, tx.execArgs)
	}
	if !tx.committed {
		t.Fatal("expected commit")
	}
}

func TestCreateMessageEmpty(t *testing.T) {
	tx := &fakeTx{recipient: ptr("other-1")}
	repo, _ := newRepo(tx)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", "   "); !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
	if tx.committed || tx.msgSQL != "" {
		t.Fatal("must not touch the database for empty content")
	}
}

func TestCreateMessageTooLong(t *testing.T) {
	tx := &fakeTx{recipient: ptr("other-1")}
	repo, _ := newRepo(tx)
	long := strings.Repeat("a", 2001)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", long); !errors.Is(err, ErrContentTooLong) {
		t.Fatalf("expected ErrContentTooLong, got %v", err)
	}
	if tx.msgSQL != "" {
		t.Fatal("must not insert an over-length message")
	}
}

func TestCreateMessageNotParticipant(t *testing.T) {
	// sender not in the pair -> CASE yields NULL -> nil recipient.
	tx := &fakeTx{recipient: nil}
	repo, _ := newRepo(tx)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "stranger", "hi"); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant, got %v", err)
	}
	if tx.msgSQL != "" {
		t.Fatal("must not insert when the sender is not a participant")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreateMessageUnknownConversation(t *testing.T) {
	// no conversation row -> ErrNoRows -> not a participant.
	tx := &fakeTx{recipientErr: pgx.ErrNoRows}
	repo, _ := newRepo(tx)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-x", "sender-1", "hi"); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant, got %v", err)
	}
}

func TestCreateMessageInsertErrorRollsBack(t *testing.T) {
	tx := &fakeTx{recipient: ptr("other-1"), msgScanErr: errors.New("boom")}
	repo, _ := newRepo(tx)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", "hi"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreateMessageTouchErrorRollsBack(t *testing.T) {
	tx := &fakeTx{recipient: ptr("other-1"), execErr: errors.New("boom")}
	repo, _ := newRepo(tx)
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", "hi"); err == nil {
		t.Fatal("expected error")
	}
	if tx.committed || !tx.rolledBack {
		t.Fatalf("expected rollback, committed=%v rolledBack=%v", tx.committed, tx.rolledBack)
	}
}

func TestCreateMessageBeginErrorSafe(t *testing.T) {
	repo := &PostgresRepository{db: &fakeBeginner{beginErr: errors.New("down")}}
	if _, _, err := repo.CreateMessage(context.Background(), "conv-1", "sender-1", "hi"); err == nil {
		t.Fatal("expected error when the transaction cannot begin")
	}
}

// ---- ListMessages ----

func msgRow(id, sender, content string, t time.Time) []any {
	return []any{id, sender, content, t}
}

func TestListMessagesSuccess(t *testing.T) {
	t0 := time.Now()
	q := &fakeQueryer{
		isParticipant: true,
		rows: &fakeRows{rows: [][]any{
			msgRow("m2", "u2", "newer", t0),
			msgRow("m1", "u1", "older", t0.Add(-time.Minute)),
		}},
	}
	repo := &PostgresRepository{q: q}
	got, err := repo.ListMessages(context.Background(), "conv-1", "u1", nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	// newest first, conversation id filled in, fields mapped
	if got[0].ID != "m2" || got[0].Content != "newer" || got[0].SenderID != "u2" {
		t.Fatalf("unexpected first message: %+v", got[0])
	}
	if got[0].ConversationID != "conv-1" {
		t.Fatalf("conversation id not set: %+v", got[0])
	}
	// first page -> nil cursor args forwarded
	if len(q.lastListArgs) != 4 || q.lastListArgs[0] != "conv-1" || q.lastListArgs[1] != nil || q.lastListArgs[2] != nil || q.lastListArgs[3] != 20 {
		t.Fatalf("unexpected list args: %v", q.lastListArgs)
	}
}

func TestListMessagesWithCursor(t *testing.T) {
	cur := &MessageCursor{CreatedAt: time.Now(), ID: "m5"}
	q := &fakeQueryer{isParticipant: true, rows: &fakeRows{}}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListMessages(context.Background(), "conv-1", "u1", cur, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.lastListArgs[1] != cur.CreatedAt || q.lastListArgs[2] != "m5" || q.lastListArgs[3] != 10 {
		t.Fatalf("cursor not forwarded: %v", q.lastListArgs)
	}
}

func TestListMessagesNotParticipant(t *testing.T) {
	q := &fakeQueryer{isParticipant: false, rows: &fakeRows{}}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListMessages(context.Background(), "conv-1", "stranger", nil, 20); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant, got %v", err)
	}
	if q.lastListArgs != nil {
		t.Fatal("must not run the list query for a non-participant")
	}
}

func TestListMessagesMembershipError(t *testing.T) {
	q := &fakeQueryer{membershipErr: errors.New("boom"), rows: &fakeRows{}}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListMessages(context.Background(), "conv-1", "u1", nil, 20); err == nil {
		t.Fatal("expected error")
	}
}

func TestListMessagesQueryError(t *testing.T) {
	q := &fakeQueryer{isParticipant: true, queryErr: errors.New("boom")}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListMessages(context.Background(), "conv-1", "u1", nil, 20); err == nil {
		t.Fatal("expected error")
	}
}

func TestListMessagesRowsError(t *testing.T) {
	q := &fakeQueryer{isParticipant: true, rows: &fakeRows{err: errors.New("boom")}}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListMessages(context.Background(), "conv-1", "u1", nil, 20); err == nil {
		t.Fatal("expected rows.Err() to surface")
	}
}

// ---- ListConversations ----

// convRow builds a scripted conversation-list row. Pass lastID == nil for a
// conversation with no messages yet.
func convRow(id string, updated time.Time, otherID, otherUser, otherName string, otherAvatar any, lastID, lastSender, lastContent any, lastAt any, unread int64) []any {
	return []any{id, updated, otherID, otherUser, otherName, otherAvatar, lastID, lastSender, lastContent, lastAt, unread}
}

func TestListConversationsSuccess(t *testing.T) {
	t0 := time.Now()
	q := &fakeQueryer{rows: &fakeRows{rows: [][]any{
		convRow("c2", t0, "u2", "bob", "Bob", nil, "m9", "u2", "latest", t0, int64(2)),
		convRow("c1", t0.Add(-time.Hour), "u3", "cara", "Cara", "http://a/x.png", nil, nil, nil, nil, int64(0)),
	}}}
	repo := &PostgresRepository{q: q}
	got, err := repo.ListConversations(context.Background(), "u1", nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	// newest activity first
	if got[0].ID != "c2" || got[0].OtherID != "u2" || got[0].OtherUsername != "bob" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	// latest message present
	if got[0].LastMessageID == nil || *got[0].LastMessageID != "m9" || got[0].LastMessageContent == nil || *got[0].LastMessageContent != "latest" {
		t.Fatalf("latest message not mapped: %+v", got[0])
	}
	if got[0].UnreadCount != 2 {
		t.Fatalf("unread count not mapped: %+v", got[0])
	}
	if got[0].OtherAvatarURL != nil {
		t.Fatalf("expected nil avatar, got %v", *got[0].OtherAvatarURL)
	}
	// conversation with no messages -> nil message fields, avatar present
	if got[1].LastMessageID != nil || got[1].LastMessageCreatedAt != nil {
		t.Fatalf("expected nil latest message: %+v", got[1])
	}
	if got[1].OtherAvatarURL == nil || *got[1].OtherAvatarURL != "http://a/x.png" {
		t.Fatalf("avatar not mapped: %+v", got[1])
	}
	// first page -> nil cursor args forwarded
	if len(q.lastListArgs) != 4 || q.lastListArgs[0] != "u1" || q.lastListArgs[1] != nil || q.lastListArgs[2] != nil || q.lastListArgs[3] != 20 {
		t.Fatalf("unexpected list args: %v", q.lastListArgs)
	}
}

func TestListConversationsEmpty(t *testing.T) {
	q := &fakeQueryer{rows: &fakeRows{}}
	repo := &PostgresRepository{q: q}
	got, err := repo.ListConversations(context.Background(), "u1", nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestListConversationsWithCursor(t *testing.T) {
	cur := &ConversationCursor{UpdatedAt: time.Now(), ID: "c5"}
	q := &fakeQueryer{rows: &fakeRows{}}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListConversations(context.Background(), "u1", cur, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.lastListArgs[1] != cur.UpdatedAt || q.lastListArgs[2] != "c5" || q.lastListArgs[3] != 10 {
		t.Fatalf("cursor not forwarded: %v", q.lastListArgs)
	}
}

func TestListConversationsQueryError(t *testing.T) {
	q := &fakeQueryer{queryErr: errors.New("boom")}
	repo := &PostgresRepository{q: q}
	if _, err := repo.ListConversations(context.Background(), "u1", nil, 20); err == nil {
		t.Fatal("expected error")
	}
}

// ---- MarkConversationRead ----

func TestMarkConversationReadFound(t *testing.T) {
	q := &fakeQueryer{execTag: pgconn.NewCommandTag("UPDATE 1")}
	repo := &PostgresRepository{q: q}
	ok, err := repo.MarkConversationRead(context.Background(), "conv-1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected found=true when the participant row was updated")
	}
	if len(q.lastExecArgs) != 2 || q.lastExecArgs[0] != "conv-1" || q.lastExecArgs[1] != "u1" {
		t.Fatalf("unexpected exec args: %v", q.lastExecArgs)
	}
}

func TestMarkConversationReadNotParticipant(t *testing.T) {
	q := &fakeQueryer{execTag: pgconn.NewCommandTag("UPDATE 0")}
	repo := &PostgresRepository{q: q}
	ok, err := repo.MarkConversationRead(context.Background(), "conv-1", "stranger")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected found=false when no participant row matched")
	}
}

func TestMarkConversationReadError(t *testing.T) {
	q := &fakeQueryer{execErr: errors.New("boom")}
	repo := &PostgresRepository{q: q}
	if _, err := repo.MarkConversationRead(context.Background(), "conv-1", "u1"); err == nil {
		t.Fatal("expected error")
	}
}
