// Command trigger is the death-action firing layer. On a schedule it
// reads the dead-man-switch state; only when the aggregate has marked the
// switch trigger_ready does it decrypt the configured action payloads
// (with the GPG_PRIVATE_KEY secret, via internal/crypt) and dispatch each
// to its registered handler. Handlers run in isolation — one failure
// never blocks the others — and a per-handler audit trail is logged.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"

	"github.com/fzerorubigd/dead-man-switch/handler"
	"github.com/fzerorubigd/dead-man-switch/internal/crypt"
	"github.com/fzerorubigd/dead-man-switch/internal/dmstate"

	// Concrete death-action handlers register themselves via blank
	// imports here as they land.
	_ "github.com/fzerorubigd/dead-man-switch/handler/email"
)

func getEnvDefault(e, def string) string {
	if v := os.Getenv(e); v != "" {
		return v
	}
	return def
}

func main() {
	owner := getEnvDefault("REPOSITORY_OWNER", "fzerorubigd")
	repo := getEnvDefault("REPOSITORY_NAME", "dead-man-switch")
	actionsFile := getEnvDefault("ACTIONS_FILE", defaultActionsFile)
	privKey := []byte(os.Getenv("GPG_PRIVATE_KEY"))
	passphrase := []byte(os.Getenv("GPG_PASSPHRASE"))
	force := os.Getenv("FORCE") == "true"
	// Test-mode (#12): bypass the state gate AND dispatch only email
	// actions, so the operator can exercise the email path on demand
	// without firing destructive handlers. The email handler applies its
	// EMAIL_TEST_TO override separately.
	testMode := os.Getenv("TRIGGER_TEST_MODE") == "true"

	ctx := context.Background()
	gh := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)))

	// Gate: only fire when the aggregate marked the switch trigger_ready.
	st, _, _, err := dmstate.Load(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}
	if st.Status != dmstate.StatusTriggerReady && !force && !testMode {
		log.Printf("status=%s — not trigger_ready; nothing to fire (FORCE=true overrides)", st.Status)
		return
	}

	actions, err := loadActions(actionsFile)
	if err != nil {
		log.Fatalf("load actions: %v", err)
	}
	if testMode {
		actions = emailActions(actions)
		log.Printf("TEST MODE: state gate bypassed; dispatching %d email action(s) only (status=%s)", len(actions), st.Status)
	} else {
		log.Printf("status=%s force=%v — firing death actions", st.Status, force)
	}
	if len(actions) == 0 {
		log.Printf("no death actions configured in %s; nothing to dispatch", actionsFile)
		return
	}
	if len(privKey) == 0 {
		log.Fatalf("GPG_PRIVATE_KEY is empty; cannot decrypt action payloads")
	}

	payloads := decryptPayloads(actions, privKey, passphrase, os.ReadFile)
	results := Dispatch(ctx, actions, payloads, handler.Get)
	if summarize(results) > 0 {
		os.Exit(1)
	}
}

// decryptPayloads reads + decrypts each action's payload file. A payload
// that can't be read or decrypted is omitted, so its action reports the
// failure in isolation during dispatch rather than aborting the run.
func decryptPayloads(actions []Action, privKey, passphrase []byte, readFile func(string) ([]byte, error)) map[string][]byte {
	out := map[string][]byte{}
	for _, a := range actions {
		if a.PayloadFile == "" {
			continue
		}
		if _, done := out[a.PayloadFile]; done {
			continue
		}
		cipher, err := readFile(a.PayloadFile)
		if err != nil {
			log.Printf("payload %s: read failed: %v", a.PayloadFile, err)
			continue
		}
		plain, err := crypt.Decrypt(privKey, passphrase, cipher)
		if err != nil {
			log.Printf("payload %s: decrypt failed: %v", a.PayloadFile, err)
			continue
		}
		out[a.PayloadFile] = plain
	}
	return out
}

// summarize logs a per-handler audit trail to stdout (the CI run log is
// the trigger's record) and returns the number of failed actions.
func summarize(results []Result) int {
	failed := 0
	for _, r := range results {
		if r.OK {
			log.Printf("action ok: handler=%s payload=%s", r.Handler, r.PayloadFile)
			continue
		}
		failed++
		log.Printf("action FAILED: handler=%s payload=%s err=%s", r.Handler, r.PayloadFile, r.Error)
	}
	out, _ := json.MarshalIndent(struct {
		Total   int      `json:"total"`
		Failed  int      `json:"failed"`
		Results []Result `json:"results"`
	}{Total: len(results), Failed: failed, Results: results}, "", "  ")
	log.Printf("trigger summary:\n%s", out)
	return failed
}
