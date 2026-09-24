package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"together/backend/internal/community"
	"together/backend/internal/config"
	"together/backend/internal/conversation"
	"together/backend/internal/storage"
	"together/backend/internal/translation"
	"together/backend/internal/user"
)

// fakeCommunityRepo scripts only what the message path consults: the caller's
// kind/role in a conversation. Everything else is unused here.
type fakeCommunityRepo struct {
	kind community.Kind
	role community.Role
	err  error // e.g. community.ErrNotMember for direct conversations
}

func (f *fakeCommunityRepo) Authorize(context.Context, string, string) (community.Kind, community.Role, error) {
	return f.kind, f.role, f.err
}
func (f *fakeCommunityRepo) MemberIDs(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeCommunityRepo) AuthorizeMessage(context.Context, string, string) error {
	return nil
}
func (f *fakeCommunityRepo) Create(context.Context, string, community.CreateInput) (*community.Community, error) {
	return nil, community.ErrInvalid
}
func (f *fakeCommunityRepo) List(context.Context, string, community.Kind, *time.Time, string, int) (community.Page, error) {
	return community.Page{}, nil
}
func (f *fakeCommunityRepo) Get(context.Context, string, community.Kind, string) (*community.Community, error) {
	return nil, community.ErrNotFound
}
func (f *fakeCommunityRepo) SearchChannels(context.Context, string, string, int) ([]community.Community, error) {
	return nil, nil
}
func (f *fakeCommunityRepo) JoinChannel(context.Context, string, string) (*community.Community, error) {
	return nil, community.ErrNotFound
}
func (f *fakeCommunityRepo) Leave(context.Context, string, community.Kind, string) error { return nil }
func (f *fakeCommunityRepo) ListMembers(context.Context, string, community.Kind, string, *time.Time, string, int) (community.MemberPage, error) {
	return community.MemberPage{}, nil
}
func (f *fakeCommunityRepo) AddMembers(context.Context, string, string, []string) ([]community.Member, error) {
	return nil, nil
}
func (f *fakeCommunityRepo) SetRole(context.Context, string, string, string, string) (community.Member, error) {
	return community.Member{}, nil
}
func (f *fakeCommunityRepo) RemoveMember(context.Context, string, string, string) error { return nil }

// voiceServer serves the message routes with a stored MediaRecorder upload
// whose Content-Type still carries the codecs parameter.
func voiceServer(conv *fakeConversationRepo, communities community.Repository, extra ...any) *Server {
	me := mkUser("me-id", "me_user")
	users := &fakeUserRepo{byID: map[string]*user.User{"me-id": me}}
	sr := &fakeStorageRepo{headInfo: &storage.ObjectInfo{ContentType: "audio/webm;codecs=opus", SizeBytes: 2048}}
	return newServer(
		config.Config{Env: "test", Port: "8080", AppOrigin: testOrigin},
		fakePinger{}, users, activeSession("me-id"), &fakeFollowRepo{}, &fakeBlockRepo{},
		&fakePostRepo{}, &fakeLikeRepo{}, &fakeCommentRepo{}, &fakeMediaRepo{}, sr,
		&fakeBookmarkRepo{}, &fakeNotificationRepo{}, &fakePostCreate{}, &fakeFollowNotifier{},
		&fakeLikeNotifier{}, &fakeCommentNotifier{}, conv, append([]any{communities}, extra...)...,
	)
}

func postVoice(t *testing.T, s *Server, content string) (int, messageResponse) {
	t.Helper()
	body, _ := json.Marshal(content)
	rec := postMessageTo(s, validPostID, `{"content":`+string(body)+`,"attachment":{"storageKey":"users/me-id/private/voice.webm","filename":"voice.webm"}}`)
	var resp messageResponse
	if rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
	}
	return rec.Code, resp
}

func TestVoiceMessageAcrossConversationTypes(t *testing.T) {
	cases := []struct {
		name        string
		communities *fakeCommunityRepo
		content     string
		wantCode    int
	}{
		{"group member voice only", &fakeCommunityRepo{kind: community.Group, role: community.RoleMember}, "", http.StatusCreated},
		{"group member text plus voice", &fakeCommunityRepo{kind: community.Group, role: community.RoleMember}, "11", http.StatusCreated},
		{"channel owner voice", &fakeCommunityRepo{kind: community.Channel, role: community.Owner}, "", http.StatusCreated},
		{"channel admin voice", &fakeCommunityRepo{kind: community.Channel, role: community.Admin}, "", http.StatusCreated},
		{"channel member voice rejected", &fakeCommunityRepo{kind: community.Channel, role: community.RoleMember}, "", http.StatusForbidden},
		{"direct message voice", &fakeCommunityRepo{err: community.ErrNotMember}, "", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := &fakeConversationRepo{}
			code, resp := postVoice(t, voiceServer(conv, tc.communities), tc.content)
			if code != tc.wantCode {
				t.Fatalf("expected %d, got %d", tc.wantCode, code)
			}
			if tc.wantCode != http.StatusCreated {
				if conv.message != nil {
					t.Fatal("rejected sender must not persist a message")
				}
				return
			}
			if resp.Content != tc.content || resp.Attachment == nil || resp.Attachment.Type != "voice" || resp.Attachment.MimeType != "audio/webm" {
				t.Fatalf("unexpected voice response: %+v attachment=%+v", resp, resp.Attachment)
			}
		})
	}
}

// Regression: production passes a nil *translation.LanguageResolver when source
// language detection is not configured. It must not become a non-nil
// interface, or every text message panics in Resolve (attachment-only sends
// skip detection and kept working, which hid the bug).
func TestTypedNilLanguageResolverIsIgnored(t *testing.T) {
	var disabled *translation.LanguageResolver
	s := voiceServer(&fakeConversationRepo{}, &fakeCommunityRepo{kind: community.Group, role: community.RoleMember}, disabled)
	if s.languageResolver != nil {
		t.Fatal("typed-nil resolver must be treated as not configured")
	}
}

func TestTextMessageAcrossConversationTypes(t *testing.T) {
	var disabled *translation.LanguageResolver // mirrors production without detection
	cases := []struct {
		name        string
		communities *fakeCommunityRepo
		createErr   error
		wantCode    int
	}{
		{"group member text", &fakeCommunityRepo{kind: community.Group, role: community.RoleMember}, nil, http.StatusCreated},
		{"group non-member text", &fakeCommunityRepo{err: community.ErrNotMember}, conversation.ErrNotParticipant, http.StatusNotFound},
		{"removed member text", &fakeCommunityRepo{err: community.ErrNotMember}, conversation.ErrNotParticipant, http.StatusNotFound},
		{"channel owner text", &fakeCommunityRepo{kind: community.Channel, role: community.Owner}, nil, http.StatusCreated},
		{"channel admin text", &fakeCommunityRepo{kind: community.Channel, role: community.Admin}, nil, http.StatusCreated},
		{"channel member text rejected", &fakeCommunityRepo{kind: community.Channel, role: community.RoleMember}, nil, http.StatusForbidden},
		{"direct message text", &fakeCommunityRepo{err: community.ErrNotMember}, nil, http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conv := &fakeConversationRepo{createErr: tc.createErr}
			rec := postMessageTo(voiceServer(conv, tc.communities, disabled), validPostID, `{"content":"11"}`)
			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d (%s)", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantCode == http.StatusForbidden && conv.lastContent != "" {
				t.Fatal("read-only channel member must be rejected before persistence")
			}
			if tc.wantCode == http.StatusCreated {
				var resp messageResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Content != "11" {
					t.Fatalf("unexpected text response: %+v err=%v", resp, err)
				}
			}
		})
	}
}
