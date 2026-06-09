package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// writeTestKeypair generates a throwaway keypair and writes the armored
// public and private keys into dir, returning their paths.
func writeTestKeypair(t *testing.T, dir string) (pubPath, privPath string) {
	t.Helper()
	entity, err := openpgp.NewEntity("Test Operator", "lt-crypt test", "test@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}
	pubPath = filepath.Join(dir, "operator.asc")
	privPath = filepath.Join(dir, "private.asc")
	writeArmored(t, pubPath, openpgp.PublicKeyType, func(w io.Writer) error { return entity.Serialize(w) })
	writeArmored(t, privPath, openpgp.PrivateKeyType, func(w io.Writer) error {
		return entity.SerializePrivateWithoutSigning(w, nil)
	})
	return pubPath, privPath
}

func writeArmored(t *testing.T, path, blockType string, write func(io.Writer) error) {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, blockType, nil)
	if err != nil {
		t.Fatalf("armor encode: %v", err)
	}
	if err := write(w); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close armor: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestRun_EncryptDecrypt_RoundTrip drives the CLI end to end through
// files: encrypt --pubkey ... --in ... --out ..., then decrypt with the
// private key supplied via the env var, asserting the payload survives.
func TestRun_EncryptDecrypt_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	pubPath, privPath := writeTestKeypair(t, dir)

	plainPath := filepath.Join(dir, "payload.json")
	payload := []byte(`{"recipients":[{"email":"a@example.com"}]}`)
	if err := os.WriteFile(plainPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	ctPath := filepath.Join(dir, "payload.gpg")

	if err := run([]string{"encrypt", "--pubkey", pubPath, "--in", plainPath, "--out", ctPath}, nil, io.Discard); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	ct, err := os.ReadFile(ctPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ct, []byte("BEGIN PGP MESSAGE")) {
		t.Fatalf("output is not an armored PGP message:\n%s", ct)
	}

	// decrypt via the GPG_PRIVATE_KEY env var (the CI shape), reading the
	// ciphertext from stdin and writing plaintext to a buffer.
	priv, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GPG_PRIVATE_KEY", string(priv))

	var out bytes.Buffer
	if err := run([]string{"decrypt", "--in", ctPath}, nil, &out); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Fatalf("round-trip mismatch: got %q want %q", out.Bytes(), payload)
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	if err := run([]string{"frobnicate"}, nil, io.Discard); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestRun_DecryptMissingKey(t *testing.T) {
	t.Setenv("GPG_PRIVATE_KEY", "")
	err := run([]string{"decrypt", "--in", filepath.Join(t.TempDir(), "x")}, bytes.NewReader(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error when no private key is available")
	}
}
