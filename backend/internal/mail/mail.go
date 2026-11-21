// Package mail sends the account emails (confirm your address, reset or
// changed password, changed address), as plain text plus HTML in the colours
// of the site. With SMTP_HOST set it talks to a real mail server; without
// it, messages are written to the server log so the demo works offline.
package mail

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Body    string // plain text, always present
	HTML    string // optional: the same letter for mail programs that show HTML
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
	header, sender := sender(s.From)
	return s.send(auth, sender, clean(m.To), raw(header, sender, m, time.Now()))
}

// sendTimeout bounds a whole delivery: a mail server that stops answering
// must not hold a goroutine and a connection forever.
const sendTimeout = 30 * time.Second

// send is smtp.SendMail with a deadline on the connection.
func (s SMTP) send(auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(s.Host, s.Port), 10*time.Second)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: the server does not offer authentication")
		}
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// raw writes the message as it travels: headers, then the plain text, and the
// HTML form next to it when there is one. A mail program shows the richest
// form it understands. Bodies are quoted-printable, which keeps every line
// short and survives any mail server.
func raw(from, sender string, m Message, now time.Time) []byte {
	var b bytes.Buffer
	domain := "localhost"
	if at := strings.LastIndex(sender, "@"); at >= 0 && at < len(sender)-1 {
		domain = sender[at+1:]
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\n",
		from, clean(m.To), clean(m.Subject), now.UTC().Format(time.RFC1123Z), hex.EncodeToString(id[:]), domain)

	part := func(w io.Writer, body string) {
		q := quotedprintable.NewWriter(w)
		_, _ = q.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")))
		_ = q.Close()
	}
	if m.HTML == "" {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		part(&b, m.Body)
		return b.Bytes()
	}
	mw := multipart.NewWriter(&b)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mw.Boundary())
	for _, p := range []struct{ kind, body string }{{"text/plain", m.Body}, {"text/html", m.HTML}} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.kind + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			continue
		}
		part(w, p.body)
	}
	_ = mw.Close()
	return b.Bytes()
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
