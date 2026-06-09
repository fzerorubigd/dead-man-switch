// Command digest sends a weekly keep-alive digest email to the operator.
// It doubles as (1) keep-warm activity for the SMTP account, (2) an SMTP
// credential check, and (3) operator visibility into the dead-man-switch
// state — all exercised while the operator is alive to fix any rot.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"

	"github.com/fzerorubigd/life-tracker/handler"
	"github.com/fzerorubigd/life-tracker/internal/crypt"
	"github.com/fzerorubigd/life-tracker/internal/dmstate"

	// Register the email handler so handler.Get("email") resolves.
	_ "github.com/fzerorubigd/life-tracker/handler/email"
)

var signalPattern = regexp.MustCompile("-check.yaml$")

func getEnvDefault(e, def string) string {
	if v := os.Getenv(e); v != "" {
		return v
	}
	return def
}

func envIntDefault(e string, def int) int {
	if v := os.Getenv(e); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

type selfRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func main() {
	owner := getEnvDefault("REPOSITORY_OWNER", "fzerorubigd")
	repo := getEnvDefault("REPOSITORY_NAME", "life-tracker")
	windowDays := envIntDefault("DIGEST_WINDOW_DAYS", 30)
	selfBlob := getEnvDefault("OPERATOR_SELF_BLOB", "operator-self.gpg")
	privKey := []byte(os.Getenv("GPG_PRIVATE_KEY"))
	passphrase := []byte(os.Getenv("GPG_PASSPHRASE"))

	ctx := context.Background()
	gh := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)))

	now := time.Now().UTC()

	st, _, _, err := dmstate.Load(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	perSignal, err := collectPerSignal(ctx, gh, owner, repo)
	if err != nil {
		log.Fatalf("collect signal runs: %v", err)
	}
	names := make([]string, 0, len(perSignal))
	for n := range perSignal {
		names = append(names, n)
	}
	sort.Strings(names)
	summaries := make([]SignalSummary, 0, len(names))
	for _, n := range names {
		summaries = append(summaries, summarizeSignal(n, perSignal[n], now, windowDays))
	}

	last := "unknown"
	if st.LastSignOfLife != nil {
		last = st.LastSignOfLife.Format(time.RFC1123)
	}

	body, err := RenderDigest(DigestData{
		Status:         string(st.Status),
		LastSignOfLife: last,
		WindowDays:     windowDays,
		Signals:        summaries,
		GeneratedAt:    now.Format(time.RFC1123),
	})
	if err != nil {
		log.Fatalf("render digest: %v", err)
	}

	// Operator-self recipient (encrypted {email, name} blob).
	if len(privKey) == 0 {
		log.Fatalf("GPG_PRIVATE_KEY is empty; cannot decrypt the operator-self blob")
	}
	cipher, err := os.ReadFile(selfBlob)
	if err != nil {
		log.Fatalf("read %s: %v", selfBlob, err)
	}
	plain, err := crypt.Decrypt(privKey, passphrase, cipher)
	if err != nil {
		log.Fatalf("decrypt %s: %v", selfBlob, err)
	}
	var self selfRecipient
	if err := json.Unmarshal(plain, &self); err != nil {
		log.Fatalf("parse operator-self blob: %v", err)
	}
	if self.Email == "" {
		log.Fatalf("operator-self blob has no email")
	}

	// Send through the #6 email handler. The digest body is the message
	// itself (no template actions), so the handler's render is a
	// passthrough; this reuses the same SMTP + rate-limit path.
	payload, err := json.Marshal([]map[string]string{{
		"email":            self.Email,
		"name":             self.Name,
		"message_template": body,
	}})
	if err != nil {
		log.Fatalf("marshal digest payload: %v", err)
	}
	h, ok := handler.Get("email")
	if !ok {
		log.Fatal("email handler not registered")
	}
	if err := h.Run(ctx, payload); err != nil {
		log.Fatalf("send digest: %v", err)
	}
	log.Printf("keep-alive digest sent to operator (status=%s, %d signals)", st.Status, len(summaries))
}

// collectPerSignal reads each life-signal workflow's recent run history,
// grouped by the workflow's display name.
func collectPerSignal(ctx context.Context, gh *github.Client, owner, repo string) (map[string][]signalRun, error) {
	wfs, _, err := gh.Actions.ListWorkflows(ctx, owner, repo, nil)
	if err != nil {
		return nil, err
	}
	out := map[string][]signalRun{}
	for _, wf := range wfs.Workflows {
		if !signalPattern.MatchString(wf.GetPath()) {
			continue
		}
		runs, _, err := gh.Actions.ListWorkflowRunsByID(ctx, owner, repo, wf.GetID(), nil)
		if err != nil {
			return nil, err
		}
		name := wf.GetName()
		for _, r := range runs.WorkflowRuns {
			out[name] = append(out[name], signalRun{Conclusion: r.GetConclusion(), At: r.GetCreatedAt().Time})
		}
	}
	return out, nil
}
