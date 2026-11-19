package mail

import "testing"

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
