package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"

	"github.com/fzerorubigd/life-tracker/internal/crypt"
)

func TestLoadActions(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file is empty, not an error", func(t *testing.T) {
		a, err := loadActions(filepath.Join(dir, "nope.json"))
		if err != nil || a != nil {
			t.Fatalf("got %v, %v", a, err)
		}
	})

	t.Run("valid list parses with payloads resolved relative to actions.json", func(t *testing.T) {
		p := filepath.Join(dir, "actions.json")
		if err := os.WriteFile(p, []byte(`[{"handler":"email","payload_file":"recipients.gpg"},{"handler":"http-delete","payload_file":"sub/del.gpg"}]`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := loadActions(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(a) != 2 || a[0].Handler != "email" {
			t.Fatalf("got %+v", a)
		}
		if got, want := a[0].PayloadFile, filepath.Join(dir, "recipients.gpg"); got != want {
			t.Errorf("payload 0: got %q want %q", got, want)
		}
		if got, want := a[1].PayloadFile, filepath.Join(dir, "sub/del.gpg"); got != want {
			t.Errorf("payload 1: got %q want %q", got, want)
		}
	})

	t.Run("absolute payload paths pass through unchanged", func(t *testing.T) {
		p := filepath.Join(dir, "abs-actions.json")
		abs := filepath.Join(dir, "elsewhere", "blob.gpg")
		body := fmt.Sprintf(`[{"handler":"h","payload_file":%q}]`, abs)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := loadActions(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(a) != 1 || a[0].PayloadFile != abs {
			t.Fatalf("absolute path mangled: got %+v", a)
		}
	})

	t.Run("empty payload_file is preserved", func(t *testing.T) {
		p := filepath.Join(dir, "empty-payload.json")
		if err := os.WriteFile(p, []byte(`[{"handler":"h","payload_file":""}]`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := loadActions(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(a) != 1 || a[0].PayloadFile != "" {
			t.Fatalf("empty payload mangled: got %+v", a)
		}
	})

	t.Run("malformed json errors", func(t *testing.T) {
		p := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadActions(p); err == nil {
			t.Fatal("expected error on malformed json")
		}
	})
}

func genKeypair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	e, err := openpgp.NewEntity("Trigger Test", "lt", "test@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}
	armorBlock := func(blockType string, write func(io.Writer) error) []byte {
		var buf bytes.Buffer
		w, err := armor.Encode(&buf, blockType, nil)
		if err != nil {
			t.Fatalf("armor: %v", err)
		}
		if err := write(w); err != nil {
			t.Fatalf("serialize: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		return buf.Bytes()
	}
	pub = armorBlock(openpgp.PublicKeyType, func(w io.Writer) error { return e.Serialize(w) })
	priv = armorBlock(openpgp.PrivateKeyType, func(w io.Writer) error { return e.SerializePrivateWithoutSigning(w, nil) })
	return pub, priv
}

// TestDecryptPayloads verifies the real decrypt path and that an
// unreadable or undecryptable payload is omitted (so its action fails in
// isolation) rather than aborting the others.
func TestDecryptPayloads(t *testing.T) {
	pub, priv := genKeypair(t)
	ct, err := crypt.Encrypt(pub, []byte("secret-payload"))
	if err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"good.gpg":    []byte(ct),
		"garbage.gpg": []byte("not a pgp message"),
	}
	readFile := func(p string) ([]byte, error) {
		b, ok := files[p]
		if !ok {
			return nil, os.ErrNotExist
		}
		return b, nil
	}

	actions := []Action{
		{Handler: "h", PayloadFile: "good.gpg"},
		{Handler: "h", PayloadFile: "garbage.gpg"}, // undecryptable -> omitted
		{Handler: "h", PayloadFile: "missing.gpg"}, // unreadable -> omitted
		{Handler: "h", PayloadFile: ""},            // skipped
	}

	got := decryptPayloads(actions, priv, nil, readFile)

	if string(got["good.gpg"]) != "secret-payload" {
		t.Errorf("good.gpg: got %q want %q", got["good.gpg"], "secret-payload")
	}
	if _, ok := got["garbage.gpg"]; ok {
		t.Error("garbage.gpg should be omitted (undecryptable)")
	}
	if _, ok := got["missing.gpg"]; ok {
		t.Error("missing.gpg should be omitted (unreadable)")
	}
	if len(got) != 1 {
		t.Errorf("expected exactly 1 decrypted payload, got %d", len(got))
	}
}
