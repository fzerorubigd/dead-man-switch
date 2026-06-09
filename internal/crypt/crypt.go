// Package crypt provides OpenPGP (GPG) encryption helpers for
// life-tracker's death-key blobs: the operator encrypts payloads to
// their public key locally, and the trigger layer decrypts them at
// runtime with the private key supplied via a GitHub secret.
//
// Blobs are ASCII-armored "PGP MESSAGE" ciphertext wrapping an opaque
// payload (typically JSON). Crypto is delegated to the maintained
// github.com/ProtonMail/go-crypto OpenPGP implementation; this package
// only wires the armor + keyring plumbing.
package crypt

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

const messageBlockType = "PGP MESSAGE"

// Encrypt encrypts plaintext to every public key in the ASCII-armored
// pubKey keyring and returns ASCII-armored ciphertext.
func Encrypt(pubKey, plaintext []byte) (string, error) {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(pubKey))
	if err != nil {
		return "", fmt.Errorf("crypt: read public key: %w", err)
	}
	if len(keyring) == 0 {
		return "", errors.New("crypt: no public keys in keyring")
	}

	var buf bytes.Buffer
	armorWriter, err := armor.Encode(&buf, messageBlockType, nil)
	if err != nil {
		return "", fmt.Errorf("crypt: armor encode: %w", err)
	}
	cipherWriter, err := openpgp.Encrypt(armorWriter, keyring, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("crypt: encrypt: %w", err)
	}
	if _, err := cipherWriter.Write(plaintext); err != nil {
		return "", fmt.Errorf("crypt: write plaintext: %w", err)
	}
	if err := cipherWriter.Close(); err != nil {
		return "", fmt.Errorf("crypt: finalize ciphertext: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return "", fmt.Errorf("crypt: finalize armor: %w", err)
	}
	return buf.String(), nil
}

// Decrypt decrypts ASCII-armored ciphertext using the ASCII-armored
// privKey. passphrase unlocks a passphrase-protected key; pass nil for
// an unprotected key (the common GitHub-secret shape).
func Decrypt(privKey, passphrase, ciphertext []byte) ([]byte, error) {
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(privKey))
	if err != nil {
		return nil, fmt.Errorf("crypt: read private key: %w", err)
	}
	if len(keyring) == 0 {
		return nil, errors.New("crypt: no private keys in keyring")
	}

	// Unlock encrypted private-key material up front so the keyring is
	// ready to unwrap the session key. Doing it here (rather than via the
	// ReadMessage prompt callback) keeps a wrong passphrase a single
	// deterministic error instead of a prompt retry loop.
	if len(passphrase) > 0 {
		for _, entity := range keyring {
			if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
				if err := entity.PrivateKey.Decrypt(passphrase); err != nil {
					return nil, fmt.Errorf("crypt: unlock private key: %w", err)
				}
			}
			for _, sub := range entity.Subkeys {
				if sub.PrivateKey != nil && sub.PrivateKey.Encrypted {
					if err := sub.PrivateKey.Decrypt(passphrase); err != nil {
						return nil, fmt.Errorf("crypt: unlock subkey: %w", err)
					}
				}
			}
		}
	}

	block, err := armor.Decode(bytes.NewReader(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("crypt: armor decode: %w", err)
	}

	// Keys are unlocked above; if ReadMessage still asks for a passphrase
	// the key was protected but none was supplied.
	prompt := func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
		return nil, errors.New("crypt: private key is passphrase-protected; supply a passphrase")
	}

	md, err := openpgp.ReadMessage(block.Body, keyring, prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("crypt: read message: %w", err)
	}
	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, fmt.Errorf("crypt: read plaintext: %w", err)
	}
	return plaintext, nil
}
