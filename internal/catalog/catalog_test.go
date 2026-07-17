package catalog

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeService drops a minimal service record into dir.
func writeService(t *testing.T, dir, name, frontMatter, body string) {
	t.Helper()
	content := "---\n" + frontMatter + "\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func loadTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	writeService(t, dir, "notify-svc.md",
		"service_id: notify-svc\nname: Notification Service\ncapability: notification\nstatus: recommended\ntier: 1\nendpoint: http://localhost:9110\nselectors: [notification, notify, email, sms]\nsupersedes: [legacy-mailer]",
		"POST /v1/messages with a JSON body.")
	writeService(t, dir, "legacy-mailer.md",
		"service_id: legacy-mailer\nname: Legacy Mailer\ncapability: notification\nstatus: deprecated\ntier: 5\nsuperseded_by: notify-svc\nselectors: [email, mail]",
		"Deprecated. Do not use.")
	writeService(t, dir, "payments-gateway.md",
		"service_id: payments-gateway\nname: Payments Gateway\ncapability: payment\nstatus: recommended\ntier: 1\nendpoint: http://localhost:9120\nselectors: [payment, charge, checkout]",
		"POST /v1/charges.")
	st, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return st
}

func TestLoadParsesFrontMatterAndBody(t *testing.T) {
	st := loadTestStore(t)
	if len(st.All()) != 3 {
		t.Fatalf("expected 3 services, got %d", len(st.All()))
	}
	svc, ok := st.Get("notify-svc")
	if !ok {
		t.Fatal("notify-svc not found")
	}
	if svc.Capability != "notification" || svc.Status != "recommended" || svc.Tier != 1 {
		t.Errorf("unexpected fields: %+v", svc)
	}
	if svc.Endpoint != "http://localhost:9110" {
		t.Errorf("endpoint = %q", svc.Endpoint)
	}
	if len(svc.Supersedes) != 1 || svc.Supersedes[0] != "legacy-mailer" {
		t.Errorf("supersedes = %v", svc.Supersedes)
	}
	if svc.APIUsage != "POST /v1/messages with a JSON body." {
		t.Errorf("api usage = %q", svc.APIUsage)
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	writeService(t, dir, "bad.md", "name: No ID\nstatus: recommended", "body")
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for missing service_id/capability")
	}
}

func TestRetrieveByCapability(t *testing.T) {
	st := loadTestStore(t)
	got := st.Retrieve("notification", "")
	if len(got) != 2 {
		t.Fatalf("expected 2 notification services, got %d", len(got))
	}
	for _, svc := range got {
		if svc.Capability != "notification" {
			t.Errorf("retrieved wrong capability: %s", svc.ServiceID)
		}
	}
}

func TestRetrieveByIntentSelectors(t *testing.T) {
	st := loadTestStore(t)
	got := st.Retrieve("", "please add an SMS alert when a user signs up")
	// "sms" is a notify-svc selector; "notification" its capability substring is absent,
	// but the selector match should surface both notification services that list sms/email.
	var ids []string
	for _, svc := range got {
		ids = append(ids, svc.ServiceID)
	}
	if !slices.Contains(ids, "notify-svc") {
		t.Errorf("expected notify-svc via selector match, got %v", ids)
	}
}
