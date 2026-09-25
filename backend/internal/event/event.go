package event

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("event not found")
	ErrForbidden = errors.New("event access forbidden")
)

const (
	TypeInPerson        = "in_person"
	TypeOnline          = "online"
	VisibilityPublic    = "public"
	VisibilityFollowers = "followers"
	VisibilityPrivate   = "private"
	RSVPGoing           = "going"
	RSVPInterested      = "interested"
)

type Event struct {
	ID                 string
	CreatorID          string
	Title              string
	Description        *string
	StartsAt           time.Time
	EndsAt             *time.Time
	Timezone           string
	EventType          string
	LocationName       *string
	LocationAddress    *string
	OnlineURL          *string
	Visibility         string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatorUsername    string
	CreatorDisplayName string
	CreatorAvatarURL   *string
	ViewerRSVP         *string
	GoingCount         int64
	InterestedCount    int64
	CanEdit            bool
	CanDelete          bool
}

type Cursor struct {
	StartsAt time.Time
	ID       string
}

type CreateInput struct {
	CreatorID       string
	Title           string
	Description     *string
	StartsAt        time.Time
	EndsAt          *time.Time
	Timezone        string
	EventType       string
	LocationName    *string
	LocationAddress *string
	OnlineURL       *string
	Visibility      string
}

type UpdateInput struct {
	Title           *string
	Description     OptionalString
	StartsAt        *time.Time
	EndsAt          OptionalTime
	Timezone        *string
	EventType       *string
	LocationName    OptionalString
	LocationAddress OptionalString
	OnlineURL       OptionalString
	Visibility      *string
}

type OptionalString struct {
	Set   bool
	Value *string
}
type OptionalTime struct {
	Set   bool
	Value *time.Time
}

type Attendee struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl"`
	Status      string    `json:"status"`
	JoinedAt    time.Time `json:"joinedAt"`
}

type Repository interface {
	Create(context.Context, CreateInput) (*Event, error)
	Get(context.Context, string, string) (*Event, error)
	List(context.Context, string, *Cursor, int, string, string) ([]Event, error)
	Update(context.Context, string, string, UpdateInput) (*Event, error)
	Delete(context.Context, string, string) error
	SetRSVP(context.Context, string, string, string) (*Event, error)
	RemoveRSVP(context.Context, string, string) (*Event, error)
	ListAttendees(context.Context, string, string, *Cursor, string, int) ([]Attendee, error)
}
