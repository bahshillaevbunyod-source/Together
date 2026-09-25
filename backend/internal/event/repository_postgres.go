package event

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
type PostgresRepository struct{ db DBTX }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: pool}
}

var _ Repository = (*PostgresRepository)(nil)

const eventProjection = `id,creator_user_id,title,description,starts_at,ends_at,timezone,event_type,location_name,location_address,online_url,visibility,created_at,updated_at,username,display_name,avatar_url`
const eventProjectionJoined = `e.id,e.creator_user_id,e.title,e.description,e.starts_at,e.ends_at,e.timezone,e.event_type,e.location_name,e.location_address,e.online_url,e.visibility,e.created_at,e.updated_at,u.username,u.display_name,u.avatar_url`

func scanEvent(row pgx.Row, e *Event) error {
	return row.Scan(&e.ID, &e.CreatorID, &e.Title, &e.Description, &e.StartsAt, &e.EndsAt, &e.Timezone, &e.EventType, &e.LocationName, &e.LocationAddress, &e.OnlineURL, &e.Visibility, &e.CreatedAt, &e.UpdatedAt, &e.CreatorUsername, &e.CreatorDisplayName, &e.CreatorAvatarURL)
}

func (r *PostgresRepository) Create(ctx context.Context, in CreateInput) (*Event, error) {
	var id string
	err := r.db.QueryRow(ctx, `INSERT INTO events (creator_user_id,title,description,starts_at,ends_at,timezone,event_type,location_name,location_address,online_url,visibility) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, in.CreatorID, in.Title, in.Description, in.StartsAt, in.EndsAt, in.Timezone, in.EventType, in.LocationName, in.LocationAddress, in.OnlineURL, in.Visibility).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id, in.CreatorID)
}

func (r *PostgresRepository) Get(ctx context.Context, id, viewer string) (*Event, error) {
	e := new(Event)
	q := `SELECT ` + eventProjectionJoined + `, (SELECT status FROM event_rsvps WHERE event_id=e.id AND user_id=$2), (SELECT count(*) FROM event_rsvps WHERE event_id=e.id AND status='going'), (SELECT count(*) FROM event_rsvps WHERE event_id=e.id AND status='interested') FROM events e JOIN users u ON u.id=e.creator_user_id WHERE e.id=$1 AND (e.creator_user_id=$2 OR e.visibility='public' OR (e.visibility='followers' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_id=$2 AND f.following_id=e.creator_user_id)) ) AND NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id=$2 AND b.blocked_id=e.creator_user_id) OR (b.blocker_id=e.creator_user_id AND b.blocked_id=$2))`
	var status *string
	if err := r.db.QueryRow(ctx, q, id, viewer).Scan(&e.ID, &e.CreatorID, &e.Title, &e.Description, &e.StartsAt, &e.EndsAt, &e.Timezone, &e.EventType, &e.LocationName, &e.LocationAddress, &e.OnlineURL, &e.Visibility, &e.CreatedAt, &e.UpdatedAt, &e.CreatorUsername, &e.CreatorDisplayName, &e.CreatorAvatarURL, &status, &e.GoingCount, &e.InterestedCount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.ViewerRSVP = status
	e.CanEdit = e.CreatorID == viewer
	e.CanDelete = e.CreatorID == viewer
	return e, nil
}

func (r *PostgresRepository) List(ctx context.Context, viewer string, cur *Cursor, limit int, creator, rsvp string) ([]Event, error) {
	args := []any{viewer}
	n := 1
	where := []string{`(e.visibility='public' OR e.creator_user_id=$1 OR (e.visibility='followers' AND EXISTS (SELECT 1 FROM follows f WHERE f.follower_id=$1 AND f.following_id=e.creator_user_id)))`, `NOT EXISTS (SELECT 1 FROM blocks b WHERE (b.blocker_id=$1 AND b.blocked_id=e.creator_user_id) OR (b.blocker_id=e.creator_user_id AND b.blocked_id=$1))`, `e.starts_at >= now()`}
	if creator != "" {
		n++
		where = append(where, "e.creator_user_id=$"+itoa(n))
		args = append(args, creator)
	}
	if rsvp != "" {
		n++
		where = append(where, "EXISTS (SELECT 1 FROM event_rsvps rx WHERE rx.event_id=e.id AND rx.user_id=$"+itoa(n)+" AND rx.status=$"+itoa(n+1)+")")
		args = append(args, viewer, rsvp)
		n++
	}
	if cur != nil {
		n++
		where = append(where, "(e.starts_at>$"+itoa(n)+" OR (e.starts_at=$"+itoa(n)+" AND e.id>$"+itoa(n+1)+"))")
		args = append(args, cur.StartsAt, cur.ID)
		n++
	}
	q := `SELECT ` + eventProjectionJoined + ` FROM events e JOIN users u ON u.id=e.creator_user_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY e.starts_at ASC,e.id ASC LIMIT $` + itoa(n+1)
	args = append(args, limit)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.CreatorID, &e.Title, &e.Description, &e.StartsAt, &e.EndsAt, &e.Timezone, &e.EventType, &e.LocationName, &e.LocationAddress, &e.OnlineURL, &e.Visibility, &e.CreatedAt, &e.UpdatedAt, &e.CreatorUsername, &e.CreatorDisplayName, &e.CreatorAvatarURL); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func itoa(n int) string {
	const d = "0123456789"
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 3)
	for n > 0 {
		b = append([]byte{d[n%10]}, b...)
		n /= 10
	}
	return string(b)
}

func (r *PostgresRepository) Update(ctx context.Context, id, viewer string, in UpdateInput) (*Event, error) {
	sets := []string{}
	args := []any{}
	add := func(col string, v any) { args = append(args, v); sets = append(sets, col+"=$"+itoa(len(args))) }
	if in.Title != nil {
		add("title", *in.Title)
	}
	if in.Description.Set {
		add("description", in.Description.Value)
	}
	if in.StartsAt != nil {
		add("starts_at", *in.StartsAt)
	}
	if in.EndsAt.Set {
		add("ends_at", in.EndsAt.Value)
	}
	if in.Timezone != nil {
		add("timezone", *in.Timezone)
	}
	if in.EventType != nil {
		add("event_type", *in.EventType)
	}
	if in.LocationName.Set {
		add("location_name", in.LocationName.Value)
	}
	if in.LocationAddress.Set {
		add("location_address", in.LocationAddress.Value)
	}
	if in.OnlineURL.Set {
		add("online_url", in.OnlineURL.Value)
	}
	if in.Visibility != nil {
		add("visibility", *in.Visibility)
	}
	if len(sets) == 0 {
		return r.Get(ctx, id, viewer)
	}
	args = append(args, id, viewer)
	q := `UPDATE events SET ` + strings.Join(sets, ",") + `,updated_at=now() WHERE id=$` + itoa(len(args)-1) + ` AND creator_user_id=$` + itoa(len(args))
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrForbidden
	}
	return r.Get(ctx, id, viewer)
}
func (r *PostgresRepository) Delete(ctx context.Context, id, viewer string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM events WHERE id=$1 AND creator_user_id=$2", id, viewer)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}
func (r *PostgresRepository) SetRSVP(ctx context.Context, id, viewer, status string) (*Event, error) {
	_, err := r.db.Exec(ctx, `INSERT INTO event_rsvps(event_id,user_id,status) VALUES($1,$2,$3) ON CONFLICT(event_id,user_id) DO UPDATE SET status=EXCLUDED.status,updated_at=now()`, id, viewer, status)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id, viewer)
}
func (r *PostgresRepository) RemoveRSVP(ctx context.Context, id, viewer string) (*Event, error) {
	_, err := r.db.Exec(ctx, "DELETE FROM event_rsvps WHERE event_id=$1 AND user_id=$2", id, viewer)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id, viewer)
}
func (r *PostgresRepository) ListAttendees(ctx context.Context, id, viewer string, cur *Cursor, status string, limit int) ([]Attendee, error) {
	args := []any{id, viewer, status}
	q := `SELECT u.id,u.username,u.display_name,u.avatar_url,r.status,r.created_at FROM event_rsvps r JOIN users u ON u.id=r.user_id JOIN events e ON e.id=r.event_id WHERE r.event_id=$1 AND r.status=$3 AND (e.creator_user_id=$2 OR e.visibility='public' OR (e.visibility='followers' AND EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=$2 AND f.following_id=e.creator_user_id)))`
	if cur != nil {
		q += " AND (r.created_at>$4 OR (r.created_at=$4 AND r.user_id>$5))"
		args = append(args, cur.StartsAt, cur.ID)
	}
	q += " ORDER BY r.created_at ASC,r.user_id ASC LIMIT $" + itoa(len(args)+1)
	args = append(args, limit)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Attendee, 0, limit)
	for rows.Next() {
		var a Attendee
		if err := rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.AvatarURL, &a.Status, &a.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
