package mail

import (
	"bytes"
	"html/template"
	"strings"
)

// Letter is the content of one account email. It is written once and sent in
// two forms: plain text, and HTML in the colours of the site.
type Letter struct {
	To      string
	Subject string
	Name    string // manager name, greeted as "Hi <name>,"
	Text    string // what happened or what to do, one short paragraph
	Button  string // label of the button; empty for a letter without one
	URL     string // where the button leads
	Note    string // small print under the button
	Site    string // public address of the game, shown in the footer
}

func (l Letter) Message() Message {
	l.Site = strings.TrimRight(l.Site, "/")

	var text strings.Builder
	text.WriteString("Hi " + l.Name + ",\n\n" + l.Text + "\n")
	if l.URL != "" {
		text.WriteString("\n" + l.URL + "\n")
	}
	if l.Note != "" {
		text.WriteString("\n" + l.Note + "\n")
	}
	text.WriteString("\n-- \nKickoff Tactics\n")
	if l.Site != "" {
		text.WriteString(l.Site + "\n")
	}

	var html bytes.Buffer
	// The template only fails on a programming error; the text form still goes out.
	if err := letterHTML.Execute(&html, map[string]any{
		"L":        l,
		"SiteName": strings.TrimPrefix(strings.TrimPrefix(l.Site, "https://"), "http://"),
		// template.URL: the links are built by the server, never taken from a request.
		"URL":  template.URL(l.URL),
		"Home": template.URL(l.Site),
	}); err != nil {
		return Message{To: l.To, Subject: l.Subject, Body: text.String()}
	}
	return Message{To: l.To, Subject: l.Subject, Body: text.String(), HTML: html.String()}
}

// Mail programs ignore style sheets and most modern CSS, so the layout is
// tables with inline styles. Colours are the site's: --bg, --panel, --line,
// --text, --muted, the green-to-blue title and the lime primary button.
var letterHTML = template.Must(template.New("letter").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark">
<meta name="supported-color-schemes" content="dark">
<title>{{.L.Subject}}</title>
</head>
<body style="margin:0;padding:0;background-color:#0b1220;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:#0b1220;">{{.L.Text}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" bgcolor="#0b1220" style="background-color:#0b1220;">
<tr><td align="center" style="padding:32px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;">
<tr><td align="center" style="padding:0 0 22px 0;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:30px;line-height:36px;font-weight:800;letter-spacing:.5px;">
<span style="color:#4ade80;">Kickoff</span> <span style="color:#38bdf8;">Tactics</span>
</td></tr>
<tr><td bgcolor="#131c2e" style="background-color:#131c2e;border:1px solid #2a3a5c;border-radius:14px;padding:28px 28px 26px 28px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<p style="margin:0 0 6px 0;font-size:20px;line-height:26px;font-weight:700;color:#e6edf7;">{{.L.Subject}}</p>
<p style="margin:0 0 14px 0;font-size:15px;line-height:22px;color:#8ea0bd;">Hi <b style="color:#e6edf7;">{{.L.Name}}</b>,</p>
<p style="margin:0;font-size:15px;line-height:23px;color:#e6edf7;">{{.L.Text}}</p>
{{if .L.Button}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:24px 0 0 0;">
<tr><td align="center" bgcolor="#b6f23c" style="background-color:#b6f23c;border:1px solid #d3ff73;border-radius:10px;">
<a href="{{.URL}}" target="_blank" style="display:inline-block;padding:13px 28px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:15px;line-height:20px;font-weight:800;letter-spacing:.2px;color:#0c1a08;text-decoration:none;">{{.L.Button}}</a>
</td></tr>
</table>{{end}}
{{if .L.Note}}<p style="margin:22px 0 0 0;font-size:13px;line-height:19px;color:#8ea0bd;">{{.L.Note}}</p>{{end}}
{{if .L.Button}}<p style="margin:18px 0 0 0;padding:14px 0 0 0;border-top:1px solid #2a3a5c;font-size:12px;line-height:18px;color:#8ea0bd;">Button not working? Copy this link into your browser:<br><a href="{{.URL}}" target="_blank" style="color:#38bdf8;text-decoration:none;word-break:break-all;">{{.L.URL}}</a></p>{{end}}
</td></tr>
<tr><td align="center" style="padding:20px 8px 0 8px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:18px;color:#8ea0bd;">
Sign players, set them up on the pitch and watch them play.{{if .SiteName}}<br><a href="{{.Home}}" target="_blank" style="color:#8ea0bd;text-decoration:underline;">{{.SiteName}}</a>{{end}}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>
`))
