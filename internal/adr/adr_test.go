package adr

import (
	"path/filepath"
	"testing"
)

func TestRetrievePaymentIntentReturnsProxyInvariant(t *testing.T) {
	store, err := Load(filepath.Join("..", "..", "examples", "adr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Invariants) == 0 {
		t.Fatal("expected the ADR store to contain invariants")
	}
	matched := store.Retrieve("Add the Seka payment method to checkout")
	found := false
	for _, inv := range matched {
		if inv.ADRID == "ADR-042" {
			found = true
			if inv.Nature != "amplifier" {
				t.Errorf("ADR-042 should be an amplifier, got %s", inv.Nature)
			}
		}
	}
	if !found {
		t.Fatalf("expected ADR-042 for a payment intent, got %v", matched)
	}
}
