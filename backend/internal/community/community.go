package community

import (
	"context"
	"errors"
	"time"
)

type Kind string

const (
	Group   Kind = "group"
	Channel Kind = "channel"
)

type Role string

const (
	Owner      Role = "owner"
	Admin      Role = "admin"
	RoleMember Role = "member"
)

var (
	ErrNotFound       = errors.New("community: not found")
	ErrNotMember      = errors.New("community: not a member")
	ErrForbidden      = errors.New("community: forbidden")
	ErrInvalid        = errors.New("community: invalid request")
	ErrOwnerInvariant = errors.New("community: owner invariant")
	ErrDuplicate      = errors.New("community: duplicate membership")
)

type Permissions struct {
	CanPost          bool `json:"canPost"`
	CanAddMembers    bool `json:"canAddMembers"`
	CanRemoveMembers bool `json:"canRemoveMembers"`
	CanManageAdmins  bool `json:"canManageAdmins"`
	CanViewMembers   bool `json:"canViewMembers"`
	CanLeave         bool `json:"canLeave"`
}
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}
type Member struct {
	User     User
	Role     Role
	JoinedAt time.Time
}
type LastMessage struct {
	ID, SenderID, Content string
	CreatedAt             time.Time
}
type Community struct {
	ID                   string
	Type                 Kind
	Name                 string
	Description          *string
	AvatarURL            *string
	Role                 *Role
	MemberCount          *int64
	Muted                bool
	UnreadCount          int64
	LastMessage          *LastMessage
	CreatedAt, UpdatedAt time.Time
	Permissions          Permissions
}
type Page struct {
	Items      []Community
	NextCursor string
}
type MemberPage struct {
	Items      []Member
	NextCursor string
}
type CreateInput struct {
	Type        Kind
	Name        string
	Description *string
	MemberIDs   []string
}

type BlockChecker interface {
	HasBlockBetween(context.Context, string, string) (bool, error)
}

type Repository interface {
	Create(context.Context, string, CreateInput) (*Community, error)
	List(context.Context, string, Kind, *time.Time, string, int) (Page, error)
	Get(context.Context, string, Kind, string) (*Community, error)
	SearchChannels(context.Context, string, string, int) ([]Community, error)
	JoinChannel(context.Context, string, string) (*Community, error)
	Leave(context.Context, string, Kind, string) error
	ListMembers(context.Context, string, Kind, string, *time.Time, string, int) (MemberPage, error)
	AddMembers(context.Context, string, string, []string) ([]Member, error)
	SetRole(context.Context, string, string, string, string) (Member, error)
	RemoveMember(context.Context, string, string, string) error
	Authorize(context.Context, string, string) (Kind, Role, error)
	MemberIDs(context.Context, string) ([]string, error)
	AuthorizeMessage(context.Context, string, string) error
}
