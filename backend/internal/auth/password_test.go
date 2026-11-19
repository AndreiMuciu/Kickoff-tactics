package auth

import (
	"strings"
	"testing"
)

func TestHashVerify(t *testing.T) {
	h, err := Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") || strings.Contains(h, "correct") {
		t.Fatalf("unexpected encoding %q", h)
	}
	if !Verify("correct horse battery", h) {
		t.Fatal("right password rejected")
	}
	if Verify("correct horse batterz", h) || Verify("", h) {
		t.Fatal("wrong password accepted")
	}
	h2, _ := Hash("correct horse battery")
	if h == h2 {
		t.Fatal("salt is not random")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=999999999,t=2,p=1$AAAA$AAAA", "$bcrypt$x$y$z$w"} {
		if Verify("x", bad) {
			t.Fatalf("malformed hash accepted: %q", bad)
		}
	}
}
