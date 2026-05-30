package notifier

import (
	"fmt"
	"net/smtp"
	"net/url"
	"strings"
)

// Email delivers notifications as HTML mail via SMTP. It mirrors the proven
// recipe:
//
//	curl --url 'smtp://host:587' --ssl-reqd --mail-from ... --mail-rcpt ... \
//	     --user 'user:pass' --upload-file - (text/html body)
//
// i.e. plaintext connect on the submission port, upgrade with STARTTLS, then
// authenticate. net/smtp.SendMail performs STARTTLS automatically when the
// server advertises it (as it does on port 587).
type Email struct {
	addr     string // host:port
	host     string
	from     string
	to       []string
	username string
	password string
}

func NewEmail(smtpURL, from string, to []string, username, password string) (*Email, error) {
	if smtpURL == "" || from == "" || len(to) == 0 {
		return nil, fmt.Errorf("email requires smtp url, from address and at least one recipient")
	}

	host, addr, err := parseSMTPURL(smtpURL)
	if err != nil {
		return nil, err
	}

	return &Email{
		addr:     addr,
		host:     host,
		from:     from,
		to:       to,
		username: username,
		password: password,
	}, nil
}

// parseSMTPURL accepts "smtp://host:port", "smtps://host:port" or a bare
// "host:port". The port defaults to 587 (submission) when omitted.
func parseSMTPURL(raw string) (host, addr string, err error) {
	s := raw
	if !strings.Contains(s, "://") {
		s = "smtp://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", "", fmt.Errorf("invalid smtp url %q: %w", raw, err)
	}

	host = u.Hostname()
	if host == "" {
		return "", "", fmt.Errorf("invalid smtp url %q: missing host", raw)
	}

	port := u.Port()
	if port == "" {
		port = "587"
	}
	return host, host + ":" + port, nil
}

func (e *Email) Name() string { return "email" }

func (e *Email) Send(msg Message) error {
	subject := msg.Subject
	if subject == "" {
		subject = "OxiWatch Notification"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", e.from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(e.to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString("<html><body>\r\n")
	// The shared body uses newlines for layout; turn them into line breaks so
	// the HTML mail renders the same structure as the other channels.
	b.WriteString(strings.ReplaceAll(msg.HTML, "\n", "<br>\n"))
	b.WriteString("\r\n</body></html>\r\n")

	var auth smtp.Auth
	if e.username != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}

	return smtp.SendMail(e.addr, auth, e.from, e.to, []byte(b.String()))
}
