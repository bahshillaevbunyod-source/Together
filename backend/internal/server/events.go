package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"together/backend/internal/event"
)

const maxEventBody = 32 << 10

type eventResponse struct {
	ID              string             `json:"id"`
	CreatorUserID   string             `json:"creatorUserId"`
	Title           string             `json:"title"`
	Description     *string            `json:"description"`
	StartsAt        time.Time          `json:"startsAt"`
	EndsAt          *time.Time         `json:"endsAt"`
	Timezone        string             `json:"timezone"`
	EventType       string             `json:"eventType"`
	LocationName    *string            `json:"locationName"`
	LocationAddress *string            `json:"locationAddress"`
	OnlineURL       *string            `json:"onlineUrl"`
	Visibility      string             `json:"visibility"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	Creator         publicUserResponse `json:"creator"`
	ViewerRSVP      *string            `json:"viewerRsvp"`
	GoingCount      int64              `json:"goingCount"`
	InterestedCount int64              `json:"interestedCount"`
	Permissions     eventPermissions   `json:"permissions"`
}
type eventPermissions struct {
	CanEdit   bool `json:"canEdit"`
	CanDelete bool `json:"canDelete"`
}
type eventRequest struct {
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	StartsAt        string  `json:"startsAt"`
	EndsAt          *string `json:"endsAt"`
	Timezone        string  `json:"timezone"`
	EventType       string  `json:"eventType"`
	LocationName    *string `json:"locationName"`
	LocationAddress *string `json:"locationAddress"`
	OnlineURL       *string `json:"onlineUrl"`
	Visibility      string  `json:"visibility"`
}
type eventPatch struct {
	Title           *string         `json:"title"`
	Description     json.RawMessage `json:"description"`
	StartsAt        *string         `json:"startsAt"`
	EndsAt          json.RawMessage `json:"endsAt"`
	Timezone        *string         `json:"timezone"`
	EventType       *string         `json:"eventType"`
	LocationName    json.RawMessage `json:"locationName"`
	LocationAddress json.RawMessage `json:"locationAddress"`
	OnlineURL       json.RawMessage `json:"onlineUrl"`
	Visibility      *string         `json:"visibility"`
}

func eventToResponse(e *event.Event) eventResponse {
	return eventResponse{ID: e.ID, CreatorUserID: e.CreatorID, Title: e.Title, Description: e.Description, StartsAt: e.StartsAt, EndsAt: e.EndsAt, Timezone: e.Timezone, EventType: e.EventType, LocationName: e.LocationName, LocationAddress: e.LocationAddress, OnlineURL: e.OnlineURL, Visibility: e.Visibility, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Creator: publicUserResponse{ID: e.CreatorID, Username: e.CreatorUsername, DisplayName: e.CreatorDisplayName, AvatarURL: e.CreatorAvatarURL}, ViewerRSVP: e.ViewerRSVP, GoingCount: e.GoingCount, InterestedCount: e.InterestedCount, Permissions: eventPermissions{CanEdit: e.CanEdit, CanDelete: e.CanDelete}}
}

func parseEventTime(raw string) (time.Time, error) { return time.Parse(time.RFC3339, raw) }
func validEventURL(raw *string) bool {
	if raw == nil || *raw == "" {
		return true
	}
	u, err := url.Parse(*raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(*raw) <= 2048
}
func validateEventFields(title, description, timezone, eventType, visibility string, starts time.Time, ends *time.Time, locationName, locationAddress, onlineURL *string) error {
	if len(strings.TrimSpace(title)) < 1 || len(title) > 200 || len(description) > 10000 || len(timezone) < 1 || len(timezone) > 100 || len(locationNameValue(locationName)) > 200 || len(locationNameValue(locationAddress)) > 500 || !validEventURL(onlineURL) {
		return errors.New("invalid event fields")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return errors.New("invalid timezone")
	}
	if eventType != event.TypeInPerson && eventType != event.TypeOnline {
		return errors.New("invalid event type")
	}
	if visibility != event.VisibilityPublic && visibility != event.VisibilityFollowers && visibility != event.VisibilityPrivate {
		return errors.New("invalid visibility")
	}
	if ends != nil && ends.Before(starts) {
		return errors.New("invalid event time")
	}
	if eventType == event.TypeInPerson && onlineURL != nil {
		return errors.New("invalid event location")
	}
	if eventType == event.TypeOnline && (locationName != nil || locationAddress != nil) {
		return errors.New("invalid event location")
	}
	return nil
}
func locationNameValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		writeError(w, 501, "events unavailable")
		return
	}
	me, _ := CurrentUser(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBody)
	var in eventRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeError(w, 400, "invalid event request")
		return
	}
	starts, err := parseEventTime(in.StartsAt)
	var ends *time.Time
	if in.EndsAt != nil {
		v, e := parseEventTime(*in.EndsAt)
		err = errors.Join(err, e)
		ends = &v
	}
	if err != nil || validateEventFields(in.Title, ptrStringValue(in.Description), in.Timezone, in.EventType, in.Visibility, starts, ends, in.LocationName, in.LocationAddress, in.OnlineURL) != nil {
		writeError(w, 400, "invalid event")
		return
	}
	e, err := s.events.Create(r.Context(), event.CreateInput{CreatorID: me.ID, Title: strings.TrimSpace(in.Title), Description: in.Description, StartsAt: starts, EndsAt: ends, Timezone: in.Timezone, EventType: in.EventType, LocationName: in.LocationName, LocationAddress: in.LocationAddress, OnlineURL: in.OnlineURL, Visibility: in.Visibility})
	if err != nil {
		s.internalError(w, "create event", err)
		return
	}
	writeJSON(w, 201, eventToResponse(e))
}
func ptrStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	e, err := s.events.Get(r.Context(), r.PathValue("id"), me.ID)
	if errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	}
	if err != nil {
		s.internalError(w, "get event", err)
		return
	}
	writeJSON(w, 200, eventToResponse(e))
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, 400, "invalid limit")
		return
	}
	var cur *event.Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		ts, id, ok := decodeCursor(raw)
		if !ok {
			writeError(w, 400, "invalid cursor")
			return
		}
		cur = &event.Cursor{StartsAt: ts, ID: id}
	}
	creator := r.URL.Query().Get("creator")
	rsvp := r.URL.Query().Get("rsvp")
	if r.URL.Query().Get("mine") == "created" {
		creator = me.ID
	}
	if rsvp != "" && rsvp != event.RSVPGoing && rsvp != event.RSVPInterested {
		writeError(w, 400, "invalid rsvp")
		return
	}
	items, err := s.events.List(r.Context(), me.ID, cur, limit+1, creator, rsvp)
	if err != nil {
		s.internalError(w, "list events", err)
		return
	}
	next := ""
	if len(items) > limit {
		last := items[limit-1]
		next = encodeCursor(last.StartsAt, last.ID)
		items = items[:limit]
	}
	out := make([]eventResponse, 0, len(items))
	for i := range items {
		out = append(out, eventToResponse(&items[i]))
	}
	writeJSON(w, 200, map[string]any{"items": out, "nextCursor": next})
}

func (s *Server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	id := r.PathValue("id")
	current, err := s.events.Get(r.Context(), id, me.ID)
	if errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	}
	if err != nil {
		s.internalError(w, "get event for update", err)
		return
	}
	if !current.CanEdit {
		writeError(w, 403, "forbidden")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEventBody)
	var p eventPatch
	if json.NewDecoder(r.Body).Decode(&p) != nil {
		writeError(w, 400, "invalid event patch")
		return
	}
	in := event.UpdateInput{Title: p.Title, Timezone: p.Timezone, EventType: p.EventType, Visibility: p.Visibility}
	if p.Description != nil {
		in.Description.Set = true
		if string(p.Description) != "null" {
			var v string
			if json.Unmarshal(p.Description, &v) != nil {
				writeError(w, 400, "invalid description")
				return
			}
			in.Description.Value = &v
		}
	}
	if p.StartsAt != nil {
		v, e := parseEventTime(*p.StartsAt)
		if e != nil {
			writeError(w, 400, "invalid startsAt")
			return
		}
		in.StartsAt = &v
	}
	if p.EndsAt != nil {
		in.EndsAt.Set = true
		if string(p.EndsAt) != "null" {
			var raw string
			if json.Unmarshal(p.EndsAt, &raw) != nil {
				writeError(w, 400, "invalid endsAt")
				return
			}
			v, e := parseEventTime(raw)
			if e != nil {
				writeError(w, 400, "invalid endsAt")
				return
			}
			in.EndsAt.Value = &v
		}
	}
	parseOptional := func(raw json.RawMessage, dst *event.OptionalString, name string) bool {
		if raw == nil {
			return true
		}
		dst.Set = true
		if string(raw) == "null" {
			return true
		}
		var v string
		if json.Unmarshal(raw, &v) != nil {
			writeError(w, 400, "invalid "+name)
			return false
		}
		dst.Value = &v
		return true
	}
	if !parseOptional(p.LocationName, &in.LocationName, "locationName") || !parseOptional(p.LocationAddress, &in.LocationAddress, "locationAddress") || !parseOptional(p.OnlineURL, &in.OnlineURL, "onlineUrl") {
		return
	}
	title := current.Title
	if in.Title != nil {
		title = *in.Title
	}
	description := current.Description
	if in.Description.Set {
		description = in.Description.Value
	}
	starts := current.StartsAt
	if in.StartsAt != nil {
		starts = *in.StartsAt
	}
	ends := current.EndsAt
	if in.EndsAt.Set {
		ends = in.EndsAt.Value
	}
	timezone := current.Timezone
	if in.Timezone != nil {
		timezone = *in.Timezone
	}
	eventType := current.EventType
	if in.EventType != nil {
		eventType = *in.EventType
	}
	visibility := current.Visibility
	if in.Visibility != nil {
		visibility = *in.Visibility
	}
	locationName := current.LocationName
	if in.LocationName.Set {
		locationName = in.LocationName.Value
	}
	locationAddress := current.LocationAddress
	if in.LocationAddress.Set {
		locationAddress = in.LocationAddress.Value
	}
	onlineURL := current.OnlineURL
	if in.OnlineURL.Set {
		onlineURL = in.OnlineURL.Value
	}
	if validateEventFields(title, ptrStringValue(description), timezone, eventType, visibility, starts, ends, locationName, locationAddress, onlineURL) != nil {
		writeError(w, 400, "invalid event")
		return
	}
	e, err := s.events.Update(r.Context(), id, me.ID, in)
	if err != nil {
		if errors.Is(err, event.ErrForbidden) {
			writeError(w, 403, "forbidden")
			return
		}
		s.internalError(w, "update event", err)
		return
	}
	writeJSON(w, 200, eventToResponse(e))
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	err := s.events.Delete(r.Context(), r.PathValue("id"), me.ID)
	if errors.Is(err, event.ErrForbidden) {
		writeError(w, 403, "forbidden")
		return
	}
	if err != nil {
		s.internalError(w, "delete event", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleSetEventRSVP(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	var in struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil || (in.Status != event.RSVPGoing && in.Status != event.RSVPInterested) {
		writeError(w, 400, "invalid rsvp")
		return
	}
	if _, err := s.events.Get(r.Context(), r.PathValue("id"), me.ID); errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	} else if err != nil {
		s.internalError(w, "get event for rsvp", err)
		return
	}
	e, err := s.events.SetRSVP(r.Context(), r.PathValue("id"), me.ID, in.Status)
	if errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	}
	if err != nil {
		s.internalError(w, "set event rsvp", err)
		return
	}
	writeJSON(w, 200, eventToResponse(e))
}
func (s *Server) handleRemoveEventRSVP(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	if _, err := s.events.Get(r.Context(), r.PathValue("id"), me.ID); errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	} else if err != nil {
		s.internalError(w, "get event for rsvp removal", err)
		return
	}
	e, err := s.events.RemoveRSVP(r.Context(), r.PathValue("id"), me.ID)
	if errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	}
	if err != nil {
		s.internalError(w, "remove event rsvp", err)
		return
	}
	writeJSON(w, 200, eventToResponse(e))
}
func (s *Server) handleListEventAttendees(w http.ResponseWriter, r *http.Request) {
	me, _ := CurrentUser(r.Context())
	if _, err := s.events.Get(r.Context(), r.PathValue("id"), me.ID); errors.Is(err, event.ErrNotFound) {
		writeError(w, 404, "event not found")
		return
	} else if err != nil {
		s.internalError(w, "get event for attendees", err)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = event.RSVPGoing
	}
	if status != event.RSVPGoing && status != event.RSVPInterested {
		writeError(w, 400, "invalid status")
		return
	}
	limit, ok := parseListLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeError(w, 400, "invalid limit")
		return
	}
	var cur *event.Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		ts, id, valid := decodeCursor(raw)
		if !valid {
			writeError(w, 400, "invalid cursor")
			return
		}
		cur = &event.Cursor{StartsAt: ts, ID: id}
	}
	items, err := s.events.ListAttendees(r.Context(), r.PathValue("id"), me.ID, cur, status, limit+1)
	if err != nil {
		s.internalError(w, "list event attendees", err)
		return
	}
	next := ""
	if len(items) > limit {
		next = encodeCursor(items[limit-1].JoinedAt, items[limit-1].ID)
		items = items[:limit]
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}
