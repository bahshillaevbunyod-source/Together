package community

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool   *pgxpool.Pool
	blocks BlockChecker
}

func NewPostgresRepository(pool *pgxpool.Pool, blocks ...BlockChecker) *PostgresRepository {
	r := &PostgresRepository{pool: pool}
	if len(blocks) > 0 {
		r.blocks = blocks[0]
	}
	return r
}

const communityProjection = `
SELECT c.id, c.type, c.name, c.description, c.avatar_url, c.created_at, c.updated_at,
       me.role, me.muted_at IS NOT NULL,
       (SELECT count(*) FROM conversation_participants p WHERE p.conversation_id=c.id),
       (SELECT count(*) FROM messages m WHERE m.conversation_id=c.id AND m.sender_id <> $2
        AND (me.last_read_at IS NULL OR m.created_at > me.last_read_at)),
       lm.id, lm.sender_id, lm.content, lm.created_at
FROM conversations c
JOIN conversation_participants me ON me.conversation_id=c.id AND me.user_id=$2
LEFT JOIN LATERAL (SELECT id, sender_id, content, created_at FROM messages WHERE conversation_id=c.id ORDER BY created_at DESC,id DESC LIMIT 1) lm ON true
WHERE c.type=$1 AND ($3::timestamptz IS NULL OR c.updated_at < $3 OR (c.updated_at=$3 AND c.id < $4::uuid))
ORDER BY c.updated_at DESC,c.id DESC LIMIT $5`

type rowScanner interface{ Scan(...any) error }

func scanCommunity(row rowScanner) (*Community, error) {
	var c Community
	var kind, role string
	var desc, avatar *string
	var muted bool
	var count, unread int64
	var lid, lsender, lcontent *string
	var lat *time.Time
	if err := row.Scan(&c.ID, &kind, &c.Name, &desc, &avatar, &c.CreatedAt, &c.UpdatedAt, &role, &muted, &count, &unread, &lid, &lsender, &lcontent, &lat); err != nil {
		return nil, err
	}
	c.Type = Kind(kind)
	c.Description = desc
	c.AvatarURL = avatar
	c.Muted = muted
	c.MemberCount = &count
	c.UnreadCount = unread
	r := Role(role)
	c.Role = &r
	if lid != nil && lsender != nil && lcontent != nil && lat != nil {
		c.LastMessage = &LastMessage{ID: *lid, SenderID: *lsender, Content: *lcontent, CreatedAt: *lat}
	}
	c.Permissions = permissions(c.Type, r)
	return &c, nil
}
func permissions(k Kind, r Role) Permissions {
	p := Permissions{CanViewMembers: true, CanLeave: r != Owner}
	if k == Group {
		p.CanPost = true
		p.CanAddMembers = r == Owner || r == Admin
		p.CanRemoveMembers = r == Owner || r == Admin
		p.CanManageAdmins = r == Owner
	} else {
		p.CanPost = r == Owner || r == Admin
		p.CanAddMembers = false
		p.CanRemoveMembers = r == Owner || r == Admin
		p.CanManageAdmins = r == Owner
	}
	return p
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, in CreateInput) (*Community, error) {
	if (in.Type != Group && in.Type != Channel) || strings.TrimSpace(in.Name) == "" || len([]rune(in.Name)) > 120 {
		return nil, ErrInvalid
	}
	if err := r.validateMemberIDs(ctx, userID, in.MemberIDs); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO conversations (type,name,description) VALUES ($1,$2,$3) RETURNING id`, in.Type, strings.TrimSpace(in.Name), in.Description).Scan(&id); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO conversation_participants (conversation_id,user_id,role) VALUES ($1,$2,'owner')`, id, userID); err != nil {
		return nil, err
	}
	seen := map[string]bool{userID: true}
	for _, uid := range in.MemberIDs {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		if _, err = tx.Exec(ctx, `INSERT INTO conversation_participants (conversation_id,user_id,role) VALUES ($1,$2,'member')`, id, uid); err != nil {
			if strings.Contains(err.Error(), "foreign key") {
				return nil, ErrInvalid
			}
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.Get(ctx, id, in.Type, userID)
}
func (r *PostgresRepository) List(ctx context.Context, userID string, k Kind, after *time.Time, cursor string, limit int) (Page, error) {
	var out Page
	var id any
	if cursor != "" {
		id = cursor
	}
	rows, err := r.pool.Query(ctx, communityProjection, k, userID, after, id, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanCommunity(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, *c)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) Get(ctx context.Context, id string, k Kind, userID string) (*Community, error) {
	c, err := r.queryCommunity(ctx, id, k, userID)
	if err == nil {
		return c, nil
	}
	if k == Channel {
		var exists bool
		if e := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1 AND type='channel')`, id).Scan(&exists); e == nil && exists {
			return r.queryPublicChannel(ctx, id)
		}
	}
	return nil, ErrNotFound
}
func (r *PostgresRepository) queryCommunity(ctx context.Context, id string, k Kind, userID string) (*Community, error) {
	return scanCommunity(r.pool.QueryRow(ctx, `SELECT c.id,c.type,c.name,c.description,c.avatar_url,c.created_at,c.updated_at,me.role,me.muted_at IS NOT NULL,(SELECT count(*) FROM conversation_participants p WHERE p.conversation_id=c.id),(SELECT count(*) FROM messages m WHERE m.conversation_id=c.id AND m.sender_id<>$2 AND (me.last_read_at IS NULL OR m.created_at>me.last_read_at)),lm.id,lm.sender_id,lm.content,lm.created_at FROM conversations c JOIN conversation_participants me ON me.conversation_id=c.id AND me.user_id=$2 LEFT JOIN LATERAL (SELECT id,sender_id,content,created_at FROM messages WHERE conversation_id=c.id ORDER BY created_at DESC,id DESC LIMIT 1) lm ON true WHERE c.id=$1 AND c.type=$3`, id, userID, k))
}
func (r *PostgresRepository) queryPublicChannel(ctx context.Context, id string) (*Community, error) {
	var c Community
	var kind string
	var desc, avatar *string
	var count int64
	if err := r.pool.QueryRow(ctx, `SELECT c.id,c.type,c.name,c.description,c.avatar_url,c.created_at,c.updated_at,(SELECT count(*) FROM conversation_participants WHERE conversation_id=c.id) FROM conversations c WHERE c.id=$1 AND c.type='channel'`, id).Scan(&c.ID, &kind, &c.Name, &desc, &avatar, &c.CreatedAt, &c.UpdatedAt, &count); err != nil {
		return nil, ErrNotFound
	}
	c.Type = Channel
	c.Description = desc
	c.AvatarURL = avatar
	c.MemberCount = &count
	c.Permissions = Permissions{CanViewMembers: false}
	return &c, nil
}
func (r *PostgresRepository) SearchChannels(ctx context.Context, userID, q string, limit int) ([]Community, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM conversations WHERE type='channel' AND name ILIKE '%'||$1||'%' ORDER BY updated_at DESC,id DESC LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Community
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		c, err := r.Get(ctx, id, Channel, userID)
		if err == nil {
			out = append(out, *c)
		}
	}
	return out, rows.Err()
}
func (r *PostgresRepository) JoinChannel(ctx context.Context, id, userID string) (*Community, error) {
	tag, err := r.pool.Exec(ctx, `INSERT INTO conversation_participants(conversation_id,user_id,role) SELECT id,$2,'member' FROM conversations WHERE id=$1 AND type='channel' ON CONFLICT DO NOTHING`, id, userID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, id, Channel, userID)
}
func (r *PostgresRepository) Authorize(ctx context.Context, id, userID string) (Kind, Role, error) {
	var k, role string
	if err := r.pool.QueryRow(ctx, `SELECT c.type,p.role FROM conversations c JOIN conversation_participants p ON p.conversation_id=c.id WHERE c.id=$1 AND p.user_id=$2`, id, userID).Scan(&k, &role); err != nil {
		return "", "", ErrNotMember
	}
	return Kind(k), Role(role), nil
}
func (r *PostgresRepository) Leave(ctx context.Context, id string, k Kind, userID string) error {
	kind, role, err := r.Authorize(ctx, id, userID)
	if err != nil || kind != k {
		return ErrNotMember
	}
	if role == Owner {
		var n int
		if err = r.pool.QueryRow(ctx, `SELECT count(*) FROM conversation_participants WHERE conversation_id=$1`, id).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return ErrOwnerInvariant
		}
		return ErrForbidden
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM conversation_participants WHERE conversation_id=$1 AND user_id=$2`, id, userID)
	return err
}
func (r *PostgresRepository) ListMembers(ctx context.Context, id string, k Kind, userID string, after *time.Time, cursor string, limit int) (MemberPage, error) {
	kind, _, err := r.Authorize(ctx, id, userID)
	if err != nil || kind != k {
		return MemberPage{}, ErrNotMember
	}
	rows, err := r.pool.Query(ctx, `SELECT u.id,u.username,u.display_name,u.avatar_url,p.role,p.created_at FROM conversation_participants p JOIN users u ON u.id=p.user_id WHERE p.conversation_id=$1 AND ($2::timestamptz IS NULL OR p.created_at>$2 OR (p.created_at=$2 AND p.user_id>$3::uuid)) ORDER BY CASE p.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,p.created_at,u.id LIMIT $4`, id, after, nilIfEmpty(cursor), limit)
	if err != nil {
		return MemberPage{}, err
	}
	defer rows.Close()
	var out MemberPage
	for rows.Next() {
		var m Member
		var role string
		if err := rows.Scan(&m.User.ID, &m.User.Username, &m.User.DisplayName, &m.User.AvatarURL, &role, &m.JoinedAt); err != nil {
			return out, err
		}
		m.Role = Role(role)
		out.Items = append(out.Items, m)
	}
	return out, rows.Err()
}

func nilIfEmpty(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (r *PostgresRepository) AddMembers(ctx context.Context, id, userID string, ids []string) ([]Member, error) {
	k, role, err := r.Authorize(ctx, id, userID)
	if err != nil || k != Group || (role != Owner && role != Admin) {
		return nil, ErrForbidden
	}
	if err := r.validateMemberIDs(ctx, userID, ids); err != nil {
		return nil, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var out []Member
	seen := make(map[string]bool)
	for _, uid := range ids {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		var m Member
		err = tx.QueryRow(ctx, `INSERT INTO conversation_participants(conversation_id,user_id,role) VALUES($1,$2,'member') ON CONFLICT DO NOTHING RETURNING created_at`, id, uid).Scan(&m.JoinedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		m.Role = RoleMember
		if err = tx.QueryRow(ctx, `SELECT id,username,display_name,avatar_url FROM users WHERE id=$1`, uid).Scan(&m.User.ID, &m.User.Username, &m.User.DisplayName, &m.User.AvatarURL); err != nil {
			return nil, ErrInvalid
		}
		out = append(out, m)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) validateMemberIDs(ctx context.Context, actor string, ids []string) error {
	seen := map[string]bool{actor: true}
	for _, uid := range ids {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1)`, uid).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrInvalid
		}
		if r.blocks != nil {
			blocked, err := r.blocks.HasBlockBetween(ctx, actor, uid)
			if err != nil {
				return err
			}
			if blocked {
				return ErrForbidden
			}
		}
	}
	return nil
}

func (r *PostgresRepository) AuthorizeMessage(ctx context.Context, messageID, userID string) error {
	var kind string
	var member bool
	err := r.pool.QueryRow(ctx, `SELECT c.type, EXISTS(SELECT 1 FROM conversation_participants p WHERE p.conversation_id=m.conversation_id AND p.user_id=$2) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=$1`, messageID, userID).Scan(&kind, &member)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if kind == string(Group) || kind == string(Channel) {
		if !member {
			return ErrNotMember
		}
	}
	return nil
}
func (r *PostgresRepository) SetRole(ctx context.Context, id, userID, targetID, newRole string) (Member, error) {
	k, role, err := r.Authorize(ctx, id, userID)
	if err != nil || k == "" || role != Owner {
		return Member{}, ErrForbidden
	}
	if newRole != "admin" && newRole != "member" {
		return Member{}, ErrInvalid
	}
	var m Member
	err = r.pool.QueryRow(ctx, `UPDATE conversation_participants SET role=$3 WHERE conversation_id=$1 AND user_id=$2 AND role<>'owner' RETURNING created_at`, id, targetID, newRole).Scan(&m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrForbidden
	}
	if err != nil {
		return m, err
	}
	m.Role = Role(newRole)
	err = r.pool.QueryRow(ctx, `SELECT id,username,display_name,avatar_url FROM users WHERE id=$1`, targetID).Scan(&m.User.ID, &m.User.Username, &m.User.DisplayName, &m.User.AvatarURL)
	return m, err
}
func (r *PostgresRepository) RemoveMember(ctx context.Context, id, userID, targetID string) error {
	k, role, err := r.Authorize(ctx, id, userID)
	if err != nil || k == "" || (role != Owner && role != Admin) {
		return ErrForbidden
	}
	var target Role
	if err = r.pool.QueryRow(ctx, `SELECT role FROM conversation_participants WHERE conversation_id=$1 AND user_id=$2`, id, targetID).Scan(&target); err != nil {
		return ErrNotFound
	}
	if target == Owner || (role == Admin && target == Admin) {
		return ErrForbidden
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM conversation_participants WHERE conversation_id=$1 AND user_id=$2`, id, targetID)
	return err
}
func (r *PostgresRepository) MemberIDs(ctx context.Context, id string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM conversation_participants WHERE conversation_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
