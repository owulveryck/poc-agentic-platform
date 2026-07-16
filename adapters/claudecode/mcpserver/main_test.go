package main

import (
	"testing"

	"github.com/owulveryck/poc-agentic-platform/internal/plan"
	"github.com/owulveryck/poc-agentic-platform/internal/store"
)

func TestStampSessionID_NoActiveKeepsPlanValue(t *testing.T) {
	ss := store.NewMemory()
	p := &plan.Plan{SessionID: "plan-provided"}
	if stampSessionID(p, ss) {
		t.Fatal("stampSessionID should return false when no active session")
	}
	if p.SessionID != "plan-provided" {
		t.Errorf("SessionID = %q, want plan-provided", p.SessionID)
	}
}

func TestStampSessionID_ActiveOverridesPlanValue(t *testing.T) {
	ss := store.NewMemory()
	if err := ss.PutActive("real-session-42"); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{SessionID: "agent-guessed"}
	if !stampSessionID(p, ss) {
		t.Fatal("stampSessionID should return true when an active session exists")
	}
	if p.SessionID != "real-session-42" {
		t.Errorf("SessionID = %q, want real-session-42", p.SessionID)
	}
}
