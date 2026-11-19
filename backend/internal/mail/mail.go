// Package mail sends the two account emails: confirm your address and reset
// your password. With SMTP_HOST set it talks to a real mail server; without
// it, messages are written to the server log so the demo works offline.
package mail

import (
	"fmt"
	"log/slog"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Body    string // plain text
}

type Mailer interface {
	Send(m Message) error
	Name() string
}

// LogMailer prints mails instead of sending them. Development only: the log
// then contains working sign-in links.
type LogMailer struct{ Log *slog.Logger }

func (l LogMailer) Name() string { return "log (development: mails are printed, not sent)" }

func (l LogMailer) Send(m Message) error {
	l.Log.Info("mail not sent, no SMTP configured", "to", m.To, "subject", m.Subject, "body", m.Body)
	return nil
}

// SMTP sends through a mail server with STARTTLS (port 587). Credentials are
// only ever sent over TLS, or to localhost (net/smtp enforces this).
type SMTP struct {
	Host, Port, User, Pass, From string
}

func (s SMTP) Name() string { return "smtp " + s.Host }

func clean(v string) string {
	// Header values must never carry a line break: that is how header injection works.
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

func (s SMTP) Send(m Message) error {
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}
	to := clean(m.To)
	header, sender := sender(s.From)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		header, to, clean(m.Subject), time.Now().UTC().Format(time.RFC1123Z), strings.ReplaceAll(m.Body, "\n", "\r\n"))
	return smtp.SendMail(net.JoinHostPort(s.Host, s.Port), auth, sender, []string{to}, []byte(msg))
}

// sender splits MAIL_FROM into the From header and the bare address the mail
// server wants as envelope sender. "Kickoff Tactics <no-reply@example.com>"
// gives that header and "no-reply@example.com"; a server refuses the long form
// as a sender.
func sender(from string) (header, address string) {
	from = clean(from)
	a, err := netmail.ParseAddress(from)
	if err != nil {
		return from, from
	}
	return a.String(), a.Address
}
