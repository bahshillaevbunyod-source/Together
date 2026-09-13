package realtime

import (
	"testing"
	"time"
)

func TestRegisterMultipleConnections(t *testing.T) {
	h := NewHub()
	a1 := NewClient("u1", 4)
	a2 := NewClient("u1", 4)
	b1 := NewClient("u2", 4)
	h.Register(a1)
	h.Register(a2)
	h.Register(b1)

	if got := h.ConnectionCount("u1"); got != 2 {
		t.Fatalf("expected 2 connections for u1, got %d", got)
	}
	if got := h.ConnectionCount("u2"); got != 1 {
		t.Fatalf("expected 1 connection for u2, got %d", got)
	}
	if got := h.TotalConnections(); got != 3 {
		t.Fatalf("expected 3 total, got %d", got)
	}
}

func TestUnregisterCleanup(t *testing.T) {
	h := NewHub()
	c1 := NewClient("u1", 4)
	c2 := NewClient("u1", 4)
	h.Register(c1)
	h.Register(c2)

	h.Unregister(c1)
	if got := h.ConnectionCount("u1"); got != 1 {
		t.Fatalf("expected 1 after one unregister, got %d", got)
	}
	select {
	case <-c1.Closed():
	default:
		t.Fatal("unregistered client should be closed")
	}

	h.Unregister(c2)
	if got := h.ConnectionCount("u1"); got != 0 {
		t.Fatalf("expected 0 after both unregister, got %d", got)
	}
	if got := h.TotalConnections(); got != 0 {
		t.Fatalf("expected hub empty, got %d", got)
	}

	// Idempotent: a second unregister must not panic or change state.
	h.Unregister(c1)
	if got := h.TotalConnections(); got != 0 {
		t.Fatalf("expected still empty, got %d", got)
	}
}

func TestSendToUserDelivers(t *testing.T) {
	h := NewHub()
	c := NewClient("u1", 4)
	h.Register(c)

	h.SendToUser("u1", []byte("hello"))
	select {
	case msg := <-c.Send():
		if string(msg) != "hello" {
			t.Fatalf("unexpected message: %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a delivered message")
	}
}

func TestSendToUserSkipsOtherUsers(t *testing.T) {
	h := NewHub()
	c := NewClient("u1", 4)
	other := NewClient("u2", 4)
	h.Register(c)
	h.Register(other)

	h.SendToUser("u1", []byte("hi"))
	select {
	case <-other.Send():
		t.Fatal("message leaked to another user")
	default:
	}
}

func TestSlowClientDoesNotBlockOthers(t *testing.T) {
	h := NewHub()
	slow := NewClient("u1", 1)
	fast := NewClient("u1", 4)
	h.Register(slow)
	h.Register(fast)

	// Fill the slow client's buffer so the next send cannot enqueue.
	slow.send <- []byte("stale")

	done := make(chan struct{})
	go func() {
		h.SendToUser("u1", []byte("live"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SendToUser blocked on a slow client")
	}

	// The fast client still received the live message.
	select {
	case msg := <-fast.Send():
		if string(msg) != "live" {
			t.Fatalf("unexpected fast message: %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("fast client did not receive the message")
	}

	// The slow client was kicked and retired.
	if got := h.ConnectionCount("u1"); got != 1 {
		t.Fatalf("expected slow client removed, count=%d", got)
	}
	select {
	case <-slow.Closed():
	default:
		t.Fatal("slow client should be closed")
	}
}
