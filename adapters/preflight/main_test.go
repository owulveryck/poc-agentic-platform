package main

import "testing"

func TestGatewayURLDefault(t *testing.T) {
	t.Setenv("PPG_URL", "")
	if got := gatewayURL(); got != "http://localhost:8000" {
		t.Fatalf("default gateway URL = %q, want http://localhost:8000", got)
	}
}

func TestGatewayURLFromEnv(t *testing.T) {
	t.Setenv("PPG_URL", "http://localhost:8765")
	if got := gatewayURL(); got != "http://localhost:8765" {
		t.Fatalf("gateway URL = %q, want the PPG_URL value", got)
	}
}
