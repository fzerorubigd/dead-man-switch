// Command lt-crypt is the operator-side helper for dead-man-switch's
// encrypted death-key blobs. It encrypts a payload (e.g. the recipient
// list or a death-action config) to the operator's GPG public key, and
// decrypts a blob with the private key — the same operation the trigger
// layer performs at runtime with the GPG_PRIVATE_KEY secret.
//
//	lt-crypt encrypt --pubkey pubkey/operator.asc --in recipients.json --out recipients.gpg
//	GPG_PRIVATE_KEY="$(cat key.asc)" lt-crypt decrypt --in recipients.gpg
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fzerorubigd/dead-man-switch/internal/crypt"
)

const defaultPubKeyPath = "pubkey/operator.asc"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "lt-crypt:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: lt-crypt <encrypt|decrypt> [flags] (use -h on a subcommand for details)")
	}
	switch args[0] {
	case "encrypt":
		return runEncrypt(args[1:], stdin, stdout)
	case "decrypt":
		return runDecrypt(args[1:], stdin, stdout)
	default:
		return fmt.Errorf("unknown subcommand %q (want encrypt or decrypt)", args[0])
	}
}

func runEncrypt(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("encrypt", flag.ContinueOnError)
	pubPath := fs.String("pubkey", defaultPubKeyPath, "path to the operator's ASCII-armored GPG public key")
	in := fs.String("in", "", "plaintext input file (default: stdin)")
	out := fs.String("out", "", "ciphertext output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return ignoreHelp(err)
	}

	pub, err := os.ReadFile(*pubPath)
	if err != nil {
		return fmt.Errorf("read public key: %w", err)
	}
	plaintext, err := readInput(*in, stdin)
	if err != nil {
		return err
	}
	ciphertext, err := crypt.Encrypt(pub, plaintext)
	if err != nil {
		return err
	}
	return writeOutput(*out, []byte(ciphertext), stdout)
}

func runDecrypt(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	keyEnv := fs.String("key-env", "GPG_PRIVATE_KEY", "env var holding the ASCII-armored GPG private key")
	keyFile := fs.String("keyfile", "", "path to the ASCII-armored GPG private key (overrides --key-env)")
	passEnv := fs.String("passphrase-env", "GPG_PASSPHRASE", "env var holding the private-key passphrase (optional)")
	in := fs.String("in", "", "ciphertext input file (default: stdin)")
	out := fs.String("out", "", "plaintext output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return ignoreHelp(err)
	}

	var priv []byte
	if *keyFile != "" {
		b, err := os.ReadFile(*keyFile)
		if err != nil {
			return fmt.Errorf("read private key: %w", err)
		}
		priv = b
	} else {
		v := os.Getenv(*keyEnv)
		if v == "" {
			return fmt.Errorf("env %s is empty; set it or pass --keyfile", *keyEnv)
		}
		priv = []byte(v)
	}
	passphrase := []byte(os.Getenv(*passEnv))

	ciphertext, err := readInput(*in, stdin)
	if err != nil {
		return err
	}
	plaintext, err := crypt.Decrypt(priv, passphrase, ciphertext)
	if err != nil {
		return err
	}
	return writeOutput(*out, plaintext, stdout)
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func writeOutput(path string, data []byte, stdout io.Writer) error {
	if path == "" {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ignoreHelp swallows flag.ErrHelp (the flag package already printed
// usage) so -h exits cleanly rather than as an error.
func ignoreHelp(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}
