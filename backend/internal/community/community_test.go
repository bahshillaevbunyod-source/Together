package community

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicJSONUsesFrontendFieldNames(t *testing.T) {
	avatar := "avatar-key"
	b, err := json.Marshal(struct {
		Permissions Permissions `json:"permissions"`
		User        User        `json:"user"`
	}{Permissions: Permissions{CanPost: true, CanLeave: false}, User: User{ID: "u1", Username: "alice", DisplayName: "Alice", AvatarURL: &avatar}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"canPost":true`, `"canLeave":false`, `"id":"u1"`, `"username":"alice"`, `"displayName":"Alice"`, `"avatarUrl":"avatar-key"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("JSON missing %s: %s", want, got)
		}
	}
}

func TestPermissions(t *testing.T) {
	tests := []struct {
		name                                   string
		kind                                   Kind
		role                                   Role
		post, add, remove, manage, view, leave bool
	}{
		{"group owner", Group, Owner, true, true, true, true, true, false},
		{"group admin", Group, Admin, true, true, true, false, true, true},
		{"group member", Group, RoleMember, true, false, false, false, true, true},
		{"channel owner", Channel, Owner, true, false, true, true, true, false},
		{"channel admin", Channel, Admin, true, false, true, false, true, true},
		{"channel member", Channel, RoleMember, false, false, false, false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := permissions(tt.kind, tt.role)
			if p.CanPost != tt.post || p.CanAddMembers != tt.add || p.CanRemoveMembers != tt.remove || p.CanManageAdmins != tt.manage || p.CanViewMembers != tt.view || p.CanLeave != tt.leave {
				t.Fatalf("permissions=%+v", p)
			}
		})
	}
}
