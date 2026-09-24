package call

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrBusy       = errors.New("call: participant busy")
	ErrNotFound   = errors.New("call: not found")
	ErrForbidden  = errors.New("call: forbidden")
	ErrTransition = errors.New("call: invalid transition")
)

type Type string

const (
	Voice Type = "voice"
	Video Type = "video"
)

type State string

const (
	Created  State = "created"
	Ringing  State = "ringing"
	Accepted State = "accepted"
	Ended    State = "ended"
)

type Entry struct {
	ID, ConversationID, CallerID, CalleeID string
	Type                                   Type
	State                                  State
	CreatedAt, AcceptedAt, EndedAt         time.Time
	timer                                  *time.Timer
}

type Registry struct {
	mu        sync.Mutex
	calls     map[string]*Entry
	active    map[string]string
	timeout   time.Duration
	onTimeout func(Entry)
}

func New(timeout time.Duration, onTimeout func(Entry)) *Registry {
	return &Registry{calls: make(map[string]*Entry), active: make(map[string]string), timeout: timeout, onTimeout: onTimeout}
}

func (r *Registry) Create(conversationID, callerID, calleeID string, typ Type) (Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.active[callerID]; ok {
		return Entry{}, ErrBusy
	}
	if _, ok := r.active[calleeID]; ok {
		return Entry{}, ErrBusy
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return Entry{}, err
	}
	e := &Entry{ID: hex.EncodeToString(b), ConversationID: conversationID, CallerID: callerID, CalleeID: calleeID, Type: typ, State: Created, CreatedAt: time.Now().UTC()}
	r.calls[e.ID] = e
	r.active[callerID] = e.ID
	r.active[calleeID] = e.ID
	return *e, nil
}

func (r *Registry) Get(id string) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.calls[id]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

func (r *Registry) EndByUser(userID string) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	ended := make([]Entry, 0)
	for id, e := range r.calls {
		if e.CallerID != userID && e.CalleeID != userID {
			continue
		}
		e.State = Ended
		e.EndedAt = time.Now().UTC()
		ended = append(ended, *e)
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(r.calls, id)
		delete(r.active, e.CallerID)
		delete(r.active, e.CalleeID)
	}
	return ended
}

func (r *Registry) BeginRinging(id, sender string) (Entry, error) {
	return r.transition(id, sender, func(e *Entry) error {
		if e.State != Created {
			return ErrTransition
		}
		e.State = Ringing
		return nil
	})
}
func (r *Registry) Accept(id, sender string) (Entry, error) {
	return r.transition(id, sender, func(e *Entry) error {
		if e.State != Ringing {
			return ErrTransition
		}
		e.State = Accepted
		e.AcceptedAt = time.Now().UTC()
		if e.timer != nil {
			e.timer.Stop()
		}
		return nil
	})
}
func (r *Registry) End(id, sender string) (Entry, error) {
	return r.transition(id, sender, func(e *Entry) error {
		if e.State == Ended {
			return ErrTransition
		}
		e.State = Ended
		e.EndedAt = time.Now().UTC()
		return nil
	})
}

func (r *Registry) transition(id, sender string, fn func(*Entry) error) (Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.calls[id]
	if !ok {
		return Entry{}, ErrNotFound
	}
	if sender != e.CallerID && sender != e.CalleeID {
		return Entry{}, ErrForbidden
	}
	if err := fn(e); err != nil {
		return Entry{}, err
	}
	out := *e
	if e.State == Ended {
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(r.calls, id)
		delete(r.active, e.CallerID)
		delete(r.active, e.CalleeID)
	}
	return out, nil
}

func (r *Registry) ArmTimeout(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.calls[id]
	if e == nil || r.timeout <= 0 {
		return
	}
	e.timer = time.AfterFunc(r.timeout, func() { r.timeoutCall(id) })
}

func (r *Registry) timeoutCall(id string) {
	r.mu.Lock()
	e := r.calls[id]
	if e == nil || (e.State != Created && e.State != Ringing) {
		r.mu.Unlock()
		return
	}
	e.State = Ended
	e.EndedAt = time.Now().UTC()
	out := *e
	delete(r.calls, id)
	delete(r.active, e.CallerID)
	delete(r.active, e.CalleeID)
	r.mu.Unlock()
	if r.onTimeout != nil {
		r.onTimeout(out)
	}
}
