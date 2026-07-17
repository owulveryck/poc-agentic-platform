package ticket

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUseKeyFileGeneratesThenReloads(t *testing.T) {
	orig := secret
	t.Cleanup(func() { secret = orig })

	path := filepath.Join(t.TempDir(), "state", "ticket.key")
	if err := UseKeyFile(path); err != nil {
		t.Fatal(err)
	}
	first := secret

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("key file must be hex: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("generated key is %d bytes, want 32", len(decoded))
	}
	if string(decoded) != string(first) {
		t.Fatal("in-memory key must match the persisted key")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("key file mode = %v, want 0600", info.Mode().Perm())
		}
	}

	// Second call must reload the same key, not regenerate.
	secret = nil
	if err := UseKeyFile(path); err != nil {
		t.Fatal(err)
	}
	if string(secret) != string(first) {
		t.Fatal("reloading must yield the persisted key")
	}
}

func TestUseKeyFileEnvWins(t *testing.T) {
	orig := secret
	t.Cleanup(func() { secret = orig })

	t.Setenv(EnvSecret, "env-provided-secret")
	path := filepath.Join(t.TempDir(), "ticket.key")
	if err := UseKeyFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("with EnvSecret set, no key file must be created")
	}
}

func TestUseKeyFileRejectsBadKey(t *testing.T) {
	orig := secret
	t.Cleanup(func() { secret = orig })

	dir := t.TempDir()
	notHex := filepath.Join(dir, "nothex.key")
	if err := os.WriteFile(notHex, []byte("zz-not-hex"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UseKeyFile(notHex); err == nil {
		t.Error("non-hex key must be rejected")
	}

	short := filepath.Join(dir, "short.key")
	if err := os.WriteFile(short, []byte(hex.EncodeToString([]byte("short"))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UseKeyFile(short); err == nil {
		t.Error("short key must be rejected")
	}
}

func TestNoHardcodedSecretRoundtrip(t *testing.T) {
	// The per-process random key still signs and verifies within the process.
	tok, err := Issue(testPlan())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(tok); err != nil {
		t.Fatal(err)
	}
}
