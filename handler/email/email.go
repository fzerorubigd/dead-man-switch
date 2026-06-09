// Package email is the first concrete death-action handler: it sends a
// per-recipient notification email rendered from a template. It registers
// itself under the name "email" so the trigger layer can dispatch to it
// (blank-import this package from the trigger).
//
// Decrypted payload shape (JSON): [{ "email", "name", "message_template" }].
// Each message_template is a Go text/template rendered with {{.Name}},
// {{.Email}}, {{.Date}} and {{.Operator}} (the OPERATOR_NAME env).
//
// Delivery is SMTP with an app password (e.g. Gmail), configured via
// SMTP_HOST / SMTP_PORT / SMTP_USERNAME / SMTP_PASSWORD / SMTP_FROM. A
// per-send delay (EMAIL_SEND_DELAY_SECONDS, default 7s, +0–50% jitter)
// keeps the handler under provider throttles.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/smtp"
	"os"
	"strconv"
	"text/template"
	"time"

	"github.com/fzerorubigd/life-tracker/handler"
)

// Recipient is one entry in the decrypted recipient list.
type Recipient struct {
	Email           string `json:"email"`
	Name            string `json:"name"`
	MessageTemplate string `json:"message_template"`
}

// Sender delivers one rendered message. Production uses SMTP; tests inject
// a capture.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Handler implements handler.Handler for the "email" action. The zero
// value is the production handler (SMTP from env, real clock + sleep,
// env-configured delay); tests set the unexported fields to inject.
type Handler struct {
	sender Sender              // nil → SMTP built from env at Run
	sleep  func(time.Duration) // nil → time.Sleep
	now    func() time.Time    // nil → time.Now().UTC
	delay  *time.Duration      // nil → EMAIL_SEND_DELAY_SECONDS (default 7s)
	testTo string              // "" → EMAIL_TEST_TO (test-mode To override)
}

func (h *Handler) Name() string { return "email" }

// Run parses the recipient list and sends each a rendered message,
// isolating per-recipient failures (logged, not fatal) and pacing sends
// with the configured delay. It returns an aggregate error if any
// recipient failed, so the trigger marks the action failed.
func (h *Handler) Run(ctx context.Context, payload []byte) error {
	var recipients []Recipient
	if err := json.Unmarshal(payload, &recipients); err != nil {
		return fmt.Errorf("email: parse payload: %w", err)
	}
	if len(recipients) == 0 {
		log.Printf("email: no recipients in payload")
		return nil
	}

	sender := h.sender
	if sender == nil {
		s, err := smtpSenderFromEnv()
		if err != nil {
			return err
		}
		sender = s
	}
	sleep := h.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	now := h.now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	base := h.baseDelay()
	subject := getEnvDefault("EMAIL_SUBJECT", "A message from "+operatorName())
	testTo := h.testTo
	if testTo == "" {
		testTo = os.Getenv("EMAIL_TEST_TO")
	}

	var failures []string
	for i, r := range recipients {
		if i > 0 {
			sleep(jitter(base))
		}
		body, err := render(r, now())
		if err != nil {
			log.Printf("email: render for %s: %v", r.Email, err)
			failures = append(failures, r.Email)
			continue
		}
		// Test-mode: redirect every message to the operator's address and
		// append the intended recipient so the operator can verify the
		// list + content without sending real death-trigger mail.
		to := r.Email
		if testTo != "" {
			to = testTo
			body += fmt.Sprintf("\n\n-- \nintended-recipient: %s\n", r.Email)
		}
		if err := sender.Send(ctx, to, subject, body); err != nil {
			log.Printf("email: send for %s (to %s): %v", r.Email, to, err)
			failures = append(failures, r.Email)
			continue
		}
		log.Printf("email: sent for %s (to %s)", r.Email, to)
	}
	if len(failures) > 0 {
		return fmt.Errorf("email: %d/%d recipients failed: %v", len(failures), len(recipients), failures)
	}
	return nil
}

func (h *Handler) baseDelay() time.Duration {
	if h.delay != nil {
		return *h.delay
	}
	secs := envIntDefault("EMAIL_SEND_DELAY_SECONDS", 7)
	if secs < 0 {
		secs = 0
	}
	return time.Duration(secs) * time.Second
}

// jitter returns base plus 0–50% extra, so sends do not arrive on a fixed
// cadence. A zero base stays zero.
func jitter(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	return base + time.Duration(rand.Int63n(int64(base)/2+1))
}

func render(r Recipient, now time.Time) (string, error) {
	tmpl, err := template.New("message").Parse(r.MessageTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	data := map[string]string{
		"Name":     r.Name,
		"Email":    r.Email,
		"Date":     now.Format("2006-01-02"),
		"Operator": operatorName(),
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func operatorName() string { return os.Getenv("OPERATOR_NAME") }

// smtpSender sends via net/smtp with PLAIN auth.
type smtpSender struct {
	addr string // host:port
	auth smtp.Auth
	from string
}

func (s *smtpSender) Send(_ context.Context, to, subject, body string) error {
	msg := buildMessage(s.from, to, subject, body)
	return smtp.SendMail(s.addr, s.auth, s.from, []string{to}, msg)
}

func buildMessage(from, to, subject, body string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&b, "\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return b.Bytes()
}

func smtpSenderFromEnv() (Sender, error) {
	host := os.Getenv("SMTP_HOST")
	user := os.Getenv("SMTP_USERNAME")
	pass := os.Getenv("SMTP_PASSWORD")
	if host == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("email: SMTP_HOST, SMTP_USERNAME and SMTP_PASSWORD must be set")
	}
	port := getEnvDefault("SMTP_PORT", "587")
	from := getEnvDefault("SMTP_FROM", user)
	return &smtpSender{
		addr: host + ":" + port,
		auth: smtp.PlainAuth("", user, pass, host),
		from: from,
	}, nil
}

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

func init() {
	handler.Register(&Handler{})
}
