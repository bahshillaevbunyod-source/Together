package call

import (
	"errors"
	"testing"
	"time"
)

func TestRegistryLifecycleAndBusyReservation(t *testing.T) {
	r := New(time.Minute, nil)
	e, err := r.Create("conversation", "caller", "callee", Voice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create("other", "caller", "third", Voice); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected caller busy, got %v", err)
	}
	if _, err := r.BeginRinging(e.ID, "caller"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Accept(e.ID, "callee"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.End(e.ID, "callee"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get(e.ID); ok {
		t.Fatal("ended call remained in registry")
	}
	if _, err := r.Create("other", "caller", "third", Voice); err != nil {
		t.Fatalf("reservation not released: %v", err)
	}
}

func TestRegistryRejectsUnauthorizedAndInvalidTransitions(t *testing.T) {
	r := New(time.Minute, nil)
	e, _ := r.Create("conversation", "caller", "callee", Video)
	if _, err := r.BeginRinging(e.ID, "intruder"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if _, err := r.Accept(e.ID, "callee"); !errors.Is(err, ErrTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestRegistryTimeoutCleansCreatedCall(t *testing.T) {
	r := New(15*time.Millisecond, nil)
	e, err := r.Create("conversation", "caller", "callee", Voice)
	if err != nil {
		t.Fatal(err)
	}
	r.ArmTimeout(e.ID)
	time.Sleep(40 * time.Millisecond)
	if _, ok := r.Get(e.ID); ok {
		t.Fatal("expired created call remained in registry")
	}
	if _, err := r.Create("conversation", "caller", "callee", Voice); err != nil {
		t.Fatalf("reservations were not released after timeout: %v", err)
	}
}
