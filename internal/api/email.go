package api

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/tethrtv/tethr/internal/core"
	"github.com/tethrtv/tethr/internal/mail"
)

//go:embed emails/*.html
var emailFiles embed.FS

var emails = template.Must(template.ParseFS(emailFiles, "emails/*.html"))

func signInEmail(appURL string, e *core.SignInEmail) (mail.Message, error) {
	link := fmt.Sprintf("%s/verify?token=%s", appURL, e.Token)
	data := struct{ Title, Preview, Lead, Button, Link, Code, AppURL, Image, ImageAlt string }{
		Title:    "Sign in to Tethr",
		Preview:  "Your sign-in link and code for Tethr.",
		Lead:     "Here’s your link to sign in. There’s no password to remember.",
		Button:   "Sign in",
		Link:     link,
		Code:     e.Code,
		AppURL:   appURL,
		Image:    "email-signin.png",
		ImageAlt: "An old telly showing “You’re on”",
	}
	if e.Purpose == "sign_up" {
		data.Title = "Finish creating your account"
		data.Preview = "One click and your Tethr account is ready."
		data.Lead = "You’re one step away. Use the button to confirm this is your email address and sign in."
		data.Button = "Finish signing up"
		data.Image = "email-welcome.png"
		data.ImageAlt = "An old telly showing “Welcome”"
	}

	var html bytes.Buffer
	if err := emails.ExecuteTemplate(&html, "sign_in.html", data); err != nil {
		return mail.Message{}, err
	}
	text := fmt.Sprintf(`%s

%s

%s

Or type this code: %s

It works once and runs out in 15 minutes. If you didn’t ask for it, you can ignore this email.
`, data.Title, data.Lead, link, e.Code)
	return mail.Message{To: e.To, Subject: data.Title, Text: text, HTML: html.String()}, nil
}
