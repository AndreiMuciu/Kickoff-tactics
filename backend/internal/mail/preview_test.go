package mail

import (
	"os"
	"testing"
)

// PREVIEW=<dir> go test -run TestPreview writes the letters as HTML files to look at in a browser.
func TestPreview(t *testing.T) {
	dir := os.Getenv("PREVIEW")
	if dir == "" {
		t.Skip("set PREVIEW to a directory")
	}
	site := "https://kickoff-tactics.com"
	letters := map[string]Letter{
		"verify.html": {Subject: "Confirm your email address", Name: "muciu", Site: site,
			Text:   "Welcome to the dugout. Confirm this address to finish setting up your manager account.",
			Button: "Confirm my address", URL: site + "/#verify=3kq9Zt1vXc7Pp0aLm2NnB8rT5yUuWw4EeRrTtYy",
			Note: "The link works once and expires in 24 hours. If you did not create this account, ignore this message."},
		"changed.html": {Subject: "Your password was changed", Name: "muciu", Site: site,
			Text: "The password of your manager account was just changed from your profile.",
			Note: "If that was not you, reset it now with \"Forgot password\" on the sign-in page."},
	}
	for name, l := range letters {
		if err := os.WriteFile(dir+"/"+name, []byte(l.Message().HTML), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
