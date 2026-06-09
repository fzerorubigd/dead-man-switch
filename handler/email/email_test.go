package email

import (
	"context"
	"errors"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/fzerorubigd/dead-man-switch/handler"
)

func TestRegisteredAsEmail(t *testing.T) {
	h, ok := handler.Get("email")
	if !ok {
		t.Fatal("email handler not registered")
	}
	if h.Name() != "email" {
		t.Fatalf("name = %q, want email", h.Name())
	}
}

type sent struct{ to, subject, body string }

type captureSender struct {
	got     []sent
	failFor map[string]bool
}

func (c *captureSender) Send(_ context.Context, to, subject, body string) error {
	if c.failFor[to] {
		return errors.New("delivery boom")
	}
	c.got = append(c.got, sent{to, subject, body})
	return nil
}

func zero() *time.Duration { d := time.Duration(0); return &d }

// TestRun_PerRecipientIsolation: a failing recipient is logged + reported
// in the aggregate error, but the others are still sent.
func TestRun_PerRecipientIsolation(t *testing.T) {
	cs := &captureSender{failFor: map[string]bool{"b@x.test": true}}
	h := &Handler{
		sender: cs,
		sleep:  func(time.Duration) {},
		now:    func() time.Time { return time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC) },
		delay:  zero(),
	}
	payload := []byte(`[
	  {"email":"a@x.test","name":"Aa","message_template":"hi {{.Name}} on {{.Date}}"},
	  {"email":"b@x.test","name":"Bb","message_template":"hi {{.Name}}"},
	  {"email":"c@x.test","name":"Cc","message_template":"hi {{.Name}}"}
	]`)

	err := h.Run(context.Background(), payload)
	if err == nil || !strings.Contains(err.Error(), "b@x.test") {
		t.Fatalf("expected aggregate error naming b@x.test, got %v", err)
	}
	if len(cs.got) != 2 {
		t.Fatalf("expected 2 delivered, got %d", len(cs.got))
	}
	if cs.got[0].to != "a@x.test" || !strings.Contains(cs.got[0].body, "hi Aa on 2026-06-09") {
		t.Errorf("a delivery wrong: %+v", cs.got[0])
	}
	if cs.got[1].to != "c@x.test" {
		t.Errorf("c not delivered after b failed (isolation broken): got %q", cs.got[1].to)
	}
}

func TestRun_BadTemplateIsIsolated(t *testing.T) {
	cs := &captureSender{}
	h := &Handler{sender: cs, sleep: func(time.Duration) {}, delay: zero()}
	payload := []byte(`[{"email":"a@x.test","name":"A","message_template":"{{.Nope"}]`)
	if err := h.Run(context.Background(), payload); err == nil {
		t.Fatal("expected error for malformed template")
	}
	if len(cs.got) != 0 {
		t.Errorf("nothing should be delivered on render failure")
	}
}

func TestRun_BadPayload(t *testing.T) {
	h := &Handler{sender: &captureSender{}}
	if err := h.Run(context.Background(), []byte("not json")); err == nil {
		t.Fatal("expected parse error")
	}
}

// TestRun_RateLimitBetweenSends: there is one inter-send delay between
// each pair of recipients (N-1 sleeps for N recipients).
func TestRun_RateLimitBetweenSends(t *testing.T) {
	cs := &captureSender{}
	sleeps := 0
	d := 3 * time.Second
	h := &Handler{sender: cs, sleep: func(time.Duration) { sleeps++ }, delay: &d}
	payload := []byte(`[{"email":"a@x.test","message_template":"x"},{"email":"b@x.test","message_template":"x"},{"email":"c@x.test","message_template":"x"}]`)
	if err := h.Run(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if sleeps != 2 {
		t.Fatalf("expected 2 inter-send delays for 3 recipients, got %d", sleeps)
	}
}

// TestRun_TestModeOverridesTo: with a testTo set (#12), every message is
// delivered to that address with the intended recipient appended to the
// body, so the operator can verify the list without real sends.
func TestRun_TestModeOverridesTo(t *testing.T) {
	cs := &captureSender{}
	h := &Handler{sender: cs, sleep: func(time.Duration) {}, delay: zero(), testTo: "operator@self.test"}
	payload := []byte(`[
	  {"email":"real-a@x.test","name":"A","message_template":"hello {{.Name}}"},
	  {"email":"real-b@x.test","name":"B","message_template":"hi {{.Name}}"}
	]`)
	if err := h.Run(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(cs.got) != 2 {
		t.Fatalf("expected 2 delivered, got %d", len(cs.got))
	}
	for i, intended := range []string{"real-a@x.test", "real-b@x.test"} {
		if cs.got[i].to != "operator@self.test" {
			t.Errorf("send %d to=%q, want operator@self.test", i, cs.got[i].to)
		}
		if !strings.Contains(cs.got[i].body, "intended-recipient: "+intended) {
			t.Errorf("send %d body missing footer for %s:\n%s", i, intended, cs.got[i].body)
		}
	}
}

// TestSMTPSender_RealSocket exercises the actual net/smtp path against a
// minimal in-process SMTP capture server (the acceptance's smtp4dev-style
// check, without an external dependency).
func TestSMTPSender_RealSocket(t *testing.T) {
	addr, dataCh := startFakeSMTP(t)
	s := &smtpSender{
		addr: addr,
		auth: smtp.PlainAuth("", "user", "pass", "127.0.0.1"),
		from: "from@x.test",
	}
	if err := s.Send(context.Background(), "to@x.test", "Subj", "Hello body"); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := string(<-dataCh)
	for _, want := range []string{"Subject: Subj", "To: to@x.test", "From: from@x.test", "Hello body"} {
		if !strings.Contains(got, want) {
			t.Errorf("captured message missing %q; got:\n%s", want, got)
		}
	}
}

// startFakeSMTP starts a minimal SMTP server that accepts one message and
// hands its DATA body to the returned channel.
func startFakeSMTP(t *testing.T) (addr string, dataCh <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ch := make(chan []byte, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tp := textproto.NewConn(conn)
		_ = tp.PrintfLine("220 fake ESMTP")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			up := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
				_ = tp.PrintfLine("250-fake greets you")
				_ = tp.PrintfLine("250 AUTH PLAIN")
			case strings.HasPrefix(up, "AUTH"):
				_ = tp.PrintfLine("235 2.7.0 accepted")
			case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
				_ = tp.PrintfLine("250 ok")
			case strings.HasPrefix(up, "DATA"):
				_ = tp.PrintfLine("354 end with .")
				b, err := tp.ReadDotBytes()
				if err != nil {
					return
				}
				ch <- b
				_ = tp.PrintfLine("250 ok queued")
			case strings.HasPrefix(up, "QUIT"):
				_ = tp.PrintfLine("221 bye")
				return
			default:
				_ = tp.PrintfLine("250 ok")
			}
		}
	}()
	return ln.Addr().String(), ch
}
