package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
	"time"
)

func TestSender(t *testing.T) {
	cases := []struct{ from, header, address string }{
		{"Kickoff Tactics <no-reply@example.com>", `"Kickoff Tactics" <no-reply@example.com>`, "no-reply@example.com"},
		{"no-reply@example.com", "<no-reply@example.com>", "no-reply@example.com"},
		// Line breaks never survive, and an unreadable value is passed on as it is.
		{"not an\r\naddress", "not anaddress", "not anaddress"},
	}
	for _, c := range cases {
		header, address := sender(c.from)
		if header != c.header || address != c.address {
			t.Errorf("sender(%q) = %q, %q; want %q, %q", c.from, header, address, c.header, c.address)
		}
	}
}

func TestLetterBothForms(t *testing.T) {
	m := Letter{To: "a@example.com", Subject: "Confirm your email address", Name: "muciu<b>", Text: "Welcome & confirm.",
		Button: "Confirm my address", URL: "https://example.com/#verify=abc_DEF-123", Note: "Expires in 24 hours.", Site: "https://example.com/"}.Message()
	for _, want := range []string{"Hi muciu<b>,", "Welcome & confirm.", "https://example.com/#verify=abc_DEF-123", "Expires in 24 hours."} {
		if !strings.Contains(m.Body, want) {
			t.Errorf("text form lacks %q", want)
		}
	}
	for _, want := range []string{"muciu&lt;b&gt;", "Welcome &amp; confirm.", `href="https://example.com/#verify=abc_DEF-123"`, "Confirm my address", ">example.com<"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("HTML form lacks %q", want)
		}
	}
	if strings.Contains(m.HTML, "muciu<b>") {
		t.Error("the name reached the HTML form unescaped")
	}
}

// The message on the wire must parse as two alternatives that decode back to what was written.
func TestRawIsMultipart(t *testing.T) {
	m := Message{To: "a@example.com", Subject: "Hello", Body: "Plain line\nhttps://example.com/#verify=" + strings.Repeat("x", 120) + "\n", HTML: "<p>Rich é</p>"}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw(`"Kickoff Tactics" <no-reply@example.com>`, "no-reply@example.com", m, time.Unix(0, 0))))
	if err != nil {
		t.Fatal(err)
	}
	if id := msg.Header.Get("Message-ID"); !strings.HasSuffix(id, "@example.com>") {
		t.Errorf("Message-ID %q", id)
	}
	kind, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("content type %q, %v", kind, err)
	}
	r := multipart.NewReader(msg.Body, params["boundary"])
	var got []string
	for {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p) // multipart decodes quoted-printable itself
		got = append(got, strings.ReplaceAll(string(b), "\r\n", "\n"))
	}
	if len(got) != 2 || got[0] != m.Body || got[1] != m.HTML {
		t.Fatalf("parts = %q", got)
	}
	for _, line := range strings.Split(string(raw("a@b.c", "a@b.c", m, time.Unix(0, 0))), "\r\n") {
		// The boundary header is long by nature; the bodies must wrap.
		if len(line) > 78 && !strings.HasPrefix(line, "Content-Type: multipart/") {
			t.Errorf("line of %d characters on the wire", len(line))
		}
	}
}
