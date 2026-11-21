package mail

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// A minimal mail server: answers the SMTP dialogue and keeps the message.
func fakeSMTP(t *testing.T, stall bool) (port string, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		if stall { // accept the connection and never say a word
			time.Sleep(3 * time.Second)
			return
		}
		r := bufio.NewReader(c)
		say := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		say("220 fake ready")
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250 fake")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				say("354 go on")
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
				say("250 queued")
				got <- data.String()
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("502 no")
			}
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, got
}

func TestSMTPSend(t *testing.T) {
	port, got := fakeSMTP(t, false)
	s := SMTP{Host: "127.0.0.1", Port: port, From: "Kickoff Tactics <no-reply@game.test>"}
	if err := s.Send(Message{To: "owner@game.test", Subject: "Hello", Body: "A test letter."}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if !strings.Contains(msg, "Subject: Hello") || !strings.Contains(msg, "A test letter.") {
			t.Fatalf("message: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message arrived")
	}
}

func TestSMTPStallDoesNotHang(t *testing.T) {
	port, _ := fakeSMTP(t, true)
	s := SMTP{Host: "127.0.0.1", Port: port, From: "no-reply@game.test"}
	errc := make(chan error, 1)
	go func() {
		// The real deadline is 30 s; the stalled server hangs up after 3 s,
		// which ends the dialogue with an error instead of waiting forever.
		errc <- s.Send(Message{To: "owner@game.test", Subject: "x", Body: "y"})
	}()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("a silent server accepted the mail")
		}
	case <-time.After(sendTimeout + 5*time.Second):
		t.Fatal("Send hung")
	}
}
