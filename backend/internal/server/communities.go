package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"together/backend/internal/community"
)

type communityResponse struct {
	ID          string                `json:"id"`
	Type        community.Kind        `json:"type"`
	Name        string                `json:"name"`
	Description *string               `json:"description"`
	AvatarURL   *string               `json:"avatarUrl"`
	Role        *community.Role       `json:"role"`
	MemberCount *int64                `json:"memberCount"`
	Muted       bool                  `json:"muted"`
	UnreadCount int64                 `json:"unreadCount"`
	LastMessage any                   `json:"lastMessage"`
	CreatedAt   string                `json:"createdAt"`
	UpdatedAt   string                `json:"updatedAt"`
	Permissions community.Permissions `json:"permissions"`
}
type communityMemberResponse struct {
	User     community.User `json:"user"`
	Role     community.Role `json:"role"`
	JoinedAt string         `json:"joinedAt"`
}

func communityJSON(c *community.Community) communityResponse {
	var lm any
	if c.LastMessage != nil {
		lm = map[string]any{"id": c.LastMessage.ID, "senderId": c.LastMessage.SenderID, "content": c.LastMessage.Content, "createdAt": c.LastMessage.CreatedAt.Format(time.RFC3339)}
	}
	return communityResponse{c.ID, c.Type, c.Name, c.Description, c.AvatarURL, c.Role, c.MemberCount, c.Muted, c.UnreadCount, lm, c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339), c.Permissions}
}
func memberJSON(m community.Member) communityMemberResponse {
	return communityMemberResponse{m.User, m.Role, m.JoinedAt.Format(time.RFC3339)}
}
func (s *Server) requireCommunities(w http.ResponseWriter) bool {
	if s.communities == nil {
		writeError(w, http.StatusNotImplemented, "communities unavailable")
		return false
	}
	return true
}

func validCommunityID(id string) bool { return uuidPattern.MatchString(id) }

func requireCommunityID(w http.ResponseWriter, id string) bool {
	if !validCommunityID(id) {
		writeError(w, http.StatusBadRequest, "invalid community id")
		return false
	}
	return true
}
func communityKind(r *http.Request) community.Kind {
	if strings.HasPrefix(r.URL.Path, "/api/v1/channels") {
		return community.Channel
	}
	return community.Group
}
func (s *Server) listCommunity(w http.ResponseWriter, r *http.Request, k community.Kind) {
	if !s.requireCommunities(w) {
		return
	}
	me, _ := CurrentUser(r.Context())
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, 400, "invalid limit")
		return
	}
	cur, ok := parseConversationCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	var after *time.Time
	cursorID := ""
	if cur != nil {
		after = &cur.UpdatedAt
		cursorID = cur.ID
	}
	p, err := s.communities.List(r.Context(), me.ID, k, after, cursorID, limit+1)
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	next := ""
	if len(p.Items) > limit {
		last := p.Items[limit-1]
		next = encodeCursor(last.UpdatedAt, last.ID)
		p.Items = p.Items[:limit]
	}
	items := make([]communityResponse, 0, len(p.Items))
	for i := range p.Items {
		items = append(items, communityJSON(&p.Items[i]))
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}
func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	s.listCommunity(w, r, community.Group)
}
func (s *Server) handleListChannels(w http.ResponseWriter, r *http.Request) {
	s.listCommunity(w, r, community.Channel)
}
func (s *Server) createCommunity(w http.ResponseWriter, r *http.Request, k community.Kind) {
	if !s.requireCommunities(w) {
		return
	}
	me, _ := CurrentUser(r.Context())
	var req struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		MemberIDs   []string `json:"memberIds"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	c, err := s.communities.Create(r.Context(), me.ID, community.CreateInput{Type: k, Name: req.Name, Description: req.Description, MemberIDs: req.MemberIDs})
	if err != nil {
		writeError(w, 400, "invalid community")
		return
	}
	writeJSON(w, 201, communityJSON(c))
}
func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	s.createCommunity(w, r, community.Group)
}
func (s *Server) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	s.createCommunity(w, r, community.Channel)
}
func (s *Server) getCommunity(w http.ResponseWriter, r *http.Request, k community.Kind) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) {
		return
	}
	me, _ := CurrentUser(r.Context())
	c, err := s.communities.Get(r.Context(), r.PathValue("id"), k, me.ID)
	if err != nil {
		writeError(w, 404, "community not found")
		return
	}
	writeJSON(w, 200, communityJSON(c))
}
func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	s.getCommunity(w, r, community.Group)
}
func (s *Server) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	s.getCommunity(w, r, community.Channel)
}
func (s *Server) handleSearchChannels(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	me, _ := CurrentUser(r.Context())
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, 400, "invalid limit")
		return
	}
	items, err := s.communities.SearchChannels(r.Context(), me.ID, strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		writeError(w, 500, "internal error")
		return
	}
	out := make([]communityResponse, 0, len(items))
	for i := range items {
		out = append(out, communityJSON(&items[i]))
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (s *Server) handleJoinChannel(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) {
		return
	}
	me, _ := CurrentUser(r.Context())
	c, err := s.communities.JoinChannel(r.Context(), r.PathValue("id"), me.ID)
	if err != nil {
		writeError(w, 404, "channel not found")
		return
	}
	writeJSON(w, 200, communityJSON(c))
}
func (s *Server) leaveCommunity(w http.ResponseWriter, r *http.Request, k community.Kind) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) {
		return
	}
	me, _ := CurrentUser(r.Context())
	err := s.communities.Leave(r.Context(), r.PathValue("id"), k, me.ID)
	if errors.Is(err, community.ErrOwnerInvariant) {
		writeError(w, 409, "owner cannot leave as sole owner")
		return
	}
	if err != nil {
		writeError(w, 403, "not allowed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleLeaveGroup(w http.ResponseWriter, r *http.Request) {
	s.leaveCommunity(w, r, community.Group)
}
func (s *Server) handleLeaveChannel(w http.ResponseWriter, r *http.Request) {
	s.leaveCommunity(w, r, community.Channel)
}
func (s *Server) handleListCommunityMembers(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) {
		return
	}
	me, _ := CurrentUser(r.Context())
	cur, ok := parseConversationCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	var after *time.Time
	cursorID := ""
	if cur != nil {
		after = &cur.UpdatedAt
		cursorID = cur.ID
	}
	p, err := s.communities.ListMembers(r.Context(), r.PathValue("id"), communityKind(r), me.ID, after, cursorID, 20)
	if err != nil {
		writeError(w, 404, "community not found")
		return
	}
	out := make([]communityMemberResponse, 0, len(p.Items))
	for _, m := range p.Items {
		out = append(out, memberJSON(m))
	}
	writeJSON(w, 200, map[string]any{"items": out, "nextCursor": ""})
}
func (s *Server) handleAddCommunityMembers(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) {
		return
	}
	me, _ := CurrentUser(r.Context())
	var req struct {
		UserIDs []string `json:"userIds"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	items, err := s.communities.AddMembers(r.Context(), r.PathValue("id"), me.ID, req.UserIDs)
	if err != nil {
		writeError(w, 403, "not allowed")
		return
	}
	out := make([]communityMemberResponse, 0, len(items))
	for _, m := range items {
		out = append(out, memberJSON(m))
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (s *Server) handleSetCommunityRole(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) || !validCommunityID(r.PathValue("userId")) {
		if validCommunityID(r.PathValue("id")) {
			writeError(w, http.StatusBadRequest, "invalid user id")
		}
		return
	}
	me, _ := CurrentUser(r.Context())
	var req struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	m, err := s.communities.SetRole(r.Context(), r.PathValue("id"), me.ID, r.PathValue("userId"), req.Role)
	if err != nil {
		writeError(w, 403, "not allowed")
		return
	}
	writeJSON(w, 200, memberJSON(m))
}
func (s *Server) handleRemoveCommunityMember(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunities(w) {
		return
	}
	if !requireCommunityID(w, r.PathValue("id")) || !validCommunityID(r.PathValue("userId")) {
		if validCommunityID(r.PathValue("id")) {
			writeError(w, http.StatusBadRequest, "invalid user id")
		}
		return
	}
	me, _ := CurrentUser(r.Context())
	if err := s.communities.RemoveMember(r.Context(), r.PathValue("id"), me.ID, r.PathValue("userId")); err != nil {
		writeError(w, 403, "not allowed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
