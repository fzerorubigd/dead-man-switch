package crypt

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// genKeypair returns a throwaway OpenPGP keypair as ASCII-armored public
// and private keys. No key material is committed; every run generates a
// fresh pair. When passphrase is non-empty the private key is locked
// with it.
func genKeypair(t *testing.T, passphrase []byte) (pub, priv []byte) {
	t.Helper()
	entity, err := openpgp.NewEntity("Test Operator", "dead-man-switch test", "test@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}

	pub = armorBlock(t, openpgp.PublicKeyType, func(w io.Writer) error {
		return entity.Serialize(w)
	})

	if len(passphrase) > 0 {
		if err := entity.PrivateKey.Encrypt(passphrase); err != nil {
			t.Fatalf("lock private key: %v", err)
		}
		for _, sub := range entity.Subkeys {
			if err := sub.PrivateKey.Encrypt(passphrase); err != nil {
				t.Fatalf("lock subkey: %v", err)
			}
		}
	}

	priv = armorBlock(t, openpgp.PrivateKeyType, func(w io.Writer) error {
		return entity.SerializePrivateWithoutSigning(w, nil)
	})
	return pub, priv
}

func armorBlock(t *testing.T, blockType string, write func(io.Writer) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, blockType, nil)
	if err != nil {
		t.Fatalf("armor encode %s: %v", blockType, err)
	}
	if err := write(w); err != nil {
		t.Fatalf("serialize %s: %v", blockType, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close armor %s: %v", blockType, err)
	}
	return buf.Bytes()
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	pub, priv := genKeypair(t, nil)
	plaintext := []byte(`{"recipients":[{"email":"a@example.com","message":"hi"}]}`)

	ct, err := Encrypt(pub, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !bytes.Contains([]byte(ct), []byte("BEGIN PGP MESSAGE")) {
		t.Fatalf("ciphertext is not an ASCII-armored PGP message:\n%s", ct)
	}
	if bytes.Contains([]byte(ct), plaintext) {
		t.Fatal("plaintext leaked into ciphertext")
	}

	got, err := Decrypt(priv, nil, []byte(ct))
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plaintext)
	}
}

// TestDecrypt_FromEnvVar exercises the CI shape: the private key arrives
// as an env var (the GPG_PRIVATE_KEY secret) and decrypts the blob.
func TestDecrypt_FromEnvVar(t *testing.T) {
	pub, priv := genKeypair(t, nil)
	plaintext := []byte("alive")

	ct, err := Encrypt(pub, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	t.Setenv("GPG_PRIVATE_KEY", string(priv))
	got, err := Decrypt([]byte(os.Getenv("GPG_PRIVATE_KEY")), nil, []byte(ct))
	if err != nil {
		t.Fatalf("Decrypt from env: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("mismatch: got %q want %q", got, plaintext)
	}
}

func TestEncryptDecrypt_Passphrase(t *testing.T) {
	const pass = "correct horse battery staple"
	pub, priv := genKeypair(t, []byte(pass))
	plaintext := []byte("protected payload")

	ct, err := Encrypt(pub, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(priv, []byte(pass), []byte(ct))
	if err != nil {
		t.Fatalf("Decrypt with passphrase: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("mismatch: got %q want %q", got, plaintext)
	}

	if _, err := Decrypt(priv, []byte("wrong"), []byte(ct)); err == nil {
		t.Fatal("expected error with wrong passphrase, got nil")
	}
	if _, err := Decrypt(priv, nil, []byte(ct)); err == nil {
		t.Fatal("expected error with no passphrase on a protected key, got nil")
	}
}

func TestDecrypt_GarbageCiphertext(t *testing.T) {
	_, priv := genKeypair(t, nil)
	if _, err := Decrypt(priv, nil, []byte("not a pgp message")); err == nil {
		t.Fatal("expected error decrypting garbage, got nil")
	}
}

func TestEncrypt_NoKeys(t *testing.T) {
	if _, err := Encrypt([]byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n\n-----END PGP PUBLIC KEY BLOCK-----\n"), []byte("x")); err == nil {
		t.Fatal("expected error with empty keyring, got nil")
	}
}
