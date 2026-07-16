package store

import (
	"errors"
	"sync"
	"testing"
)

func TestMemory_PutGetActive(t *testing.T) {
	m := NewMemory()
	if err := m.PutActive("s"); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetActive()
	if err != nil {
		t.Fatal(err)
	}
	if got != "s" {
		t.Errorf("GetActive = %q, want s", got)
	}
}

func TestMemory_GetActiveNotFound(t *testing.T) {
	m := NewMemory()
	if _, err := m.GetActive(); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetActive on empty: err = %v, want ErrNotFound", err)
	}
}

func TestMemory_TicketRoundtrip(t *testing.T) {
	m := NewMemory()
	if err := m.Put("sid", "t"); err != nil {
		t.Fatal(err)
	}
	got, err := m.Get("sid")
	if err != nil {
		t.Fatal(err)
	}
	if got != "t" {
		t.Errorf("Get = %q, want t", got)
	}
}

func TestMemory_ResetPurgesAllTickets(t *testing.T) {
	m := NewMemory()
	_ = m.Put("a", "1")
	_ = m.Put("b", "2")
	_ = m.PutActive("sess")
	if err := m.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(a) after Reset: err = %v, want ErrNotFound", err)
	}
	if _, err := m.Get("b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(b) after Reset: err = %v, want ErrNotFound", err)
	}
	got, err := m.GetActive()
	if err != nil || got != "sess" {
		t.Errorf("GetActive after Reset = %q, %v; want sess, nil", got, err)
	}
}

func TestMemory_ConcurrentAccessIsSafe(t *testing.T) {
	m := NewMemory()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sid := "s"
			if i%2 == 0 {
				_ = m.Put(sid, "x")
			} else {
				_, _ = m.Get(sid)
			}
		}(i)
	}
	wg.Wait()
}
