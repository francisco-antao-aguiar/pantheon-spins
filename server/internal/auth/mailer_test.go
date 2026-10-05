package auth

import (
	"bufio"
	"context"
	"mime"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// fakeSMTP accepts one message and returns what the client sent.
func fakeSMTP(t *testing.T) (addr string, got <-chan smtpCapture) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	ch := make(chan smtpCapture, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		w := func(s string) { conn.Write([]byte(s + "\r\n")) }
		var c smtpCapture
		w("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 fake")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				c.from = strings.TrimSpace(line[len("MAIL FROM:"):])
				w("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				c.rcpt = append(c.rcpt, strings.TrimSpace(line[len("RCPT TO:"):]))
				w("250 ok")
			case cmd == "DATA":
				w("354 go ahead")
				var data strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				c.data = data.String()
				w("250 queued")
			case cmd == "QUIT":
				w("221 bye")
				ch <- c
				return
			default:
				w("250 ok")
			}
		}
	}()
	return ln.Addr().String(), ch
}

type smtpCapture struct {
	from string
	rcpt []string
	data string
}

func TestSMTPMailerSends(t *testing.T) {
	addr, got := fakeSMTP(t)
	m := SMTPMailer{Addr: addr, From: "Pantheon Spins <no-reply@pantheon.local>"}
	body := "Hi odin,\n.\nReset here: http://x/reset?token=abc"
	if err := m.Send(context.Background(), "odin@asgard.test", "Reset your password ✨", body); err != nil {
		t.Fatal(err)
	}
	var c smtpCapture
	select {
	case c = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no message received")
	}
	if c.from != "<no-reply@pantheon.local>" || len(c.rcpt) != 1 || c.rcpt[0] != "<odin@asgard.test>" {
		t.Fatalf("envelope from=%q rcpt=%q", c.from, c.rcpt)
	}
	msg, err := mail.ReadMessage(strings.NewReader(c.data))
	if err != nil {
		t.Fatalf("parse message: %v\n%s", err, c.data)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if subject != "Reset your password ✨" || msg.Header.Get("To") != "<odin@asgard.test>" {
		t.Fatalf("headers: subject=%q to=%q", subject, msg.Header.Get("To"))
	}
	// A body line consisting of "." must survive SMTP dot-stuffing.
	if !strings.Contains(c.data, "\r\n..\r\n") {
		t.Fatalf("lone dot was not dot-stuffed:\n%s", c.data)
	}
}

func TestBuildMessageRejectsHeaderInjection(t *testing.T) {
	from, _ := mail.ParseAddress("a@b.test")
	to, _ := mail.ParseAddress("c@d.test")
	raw := string(buildMessage(from, to, "hi\r\nBcc: evil@x.test", "body", time.Unix(0, 0)))
	headers, _, _ := strings.Cut(raw, "\r\n\r\n")
	if strings.Contains(headers, "\r\nBcc:") {
		t.Fatalf("subject injected a header:\n%s", headers)
	}
}

func TestSMTPMailerRejectsBadAddresses(t *testing.T) {
	m := SMTPMailer{Addr: "127.0.0.1:1", From: "no-reply@pantheon.local"}
	if err := m.Send(context.Background(), "not an address", "s", "b"); err == nil {
		t.Fatal("bad recipient accepted")
	}
	m.From = "nope"
	if err := m.Send(context.Background(), "a@b.test", "s", "b"); err == nil {
		t.Fatal("bad From accepted")
	}
}
