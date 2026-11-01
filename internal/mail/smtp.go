package mail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type SMTP struct {
	Addr     string
	Username string
	Password string
	From     string
}

func (s SMTP) Send(_ context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("parse from address: %w", err)
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, host)
	}
	body, err := encode(from, m)
	if err != nil {
		return err
	}
	return smtp.SendMail(s.Addr, auth, from.Address, []string{m.To}, body)
}

func encode(from *mail.Address, m Message) ([]byte, error) {
	boundary := randomHex(16)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", randomHex(12), domain)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	for _, part := range []struct{ kind, content string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		if part.content == "" {
			continue
		}
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; charset=utf-8\r\n", part.kind)
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		w := quotedprintable.NewWriter(&b)
		if _, err := w.Write([]byte(strings.ReplaceAll(part.content, "\n", "\r\n"))); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String()), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
