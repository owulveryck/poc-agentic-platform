package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotifySvcQueuesMessage(t *testing.T) {
	srv := httptest.NewServer(newHandler("notify-svc"))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json",
		strings.NewReader(`{"channel":"email","to":"a@b.c","template":"welcome"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != "queued" || !strings.HasPrefix(out.ID, "msg_") {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestPaymentsGatewayAuthorizes(t *testing.T) {
	srv := httptest.NewServer(newHandler("payments-gateway"))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/charges", "application/json",
		strings.NewReader(`{"amount":4200,"currency":"eur","provider":"adyen"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var out struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Status != "authorized" || out.Provider != "adyen" || !strings.HasPrefix(out.ID, "chg_") {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer(newHandler("notify-svc"))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}
