package notifier

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

// Message is a channel-agnostic notification. Channels that support markup
// render HTML (Telegram, Matrix formatted_body, HTML email); the rest fall
// back to Text (Matrix body). Subject is used as the email subject line.
type Message struct {
	Subject string
	HTML    string
	Text    string
}

// Channel is a single notification transport (Telegram, Matrix, Email, ...).
// New channels only need to implement this interface and be wired into the
// Manager; all message formatting is shared and lives in the Manager.
type Channel interface {
	Name() string
	Send(msg Message) error
}

// Manager fans a notification out to every configured channel and owns the
// shared formatting so all channels carry identical information.
type Manager struct {
	channels   []Channel
	serverInfo string
	loc        *time.Location
	logger     *slog.Logger
}

// NewManager builds the manager and resolves the server info (name + public
// IPs) once, so every channel reports the same server identity. loc is the
// timezone used to display timestamps (UTC if nil).
func NewManager(serverName string, loc *time.Location, logger *slog.Logger) *Manager {
	if loc == nil {
		loc = time.UTC
	}
	return &Manager{
		serverInfo: buildServerInfo(serverName),
		loc:        loc,
		logger:     logger,
	}
}

// formatTime renders an instant in the configured timezone, with the UTC time
// in brackets for reference. When the configured zone is already UTC the
// bracket is omitted to avoid repeating the same value.
func (m *Manager) formatTime(t time.Time) string {
	loc := m.loc
	if loc == nil {
		loc = time.UTC
	}
	local := t.In(loc)
	s := local.Format("2006-01-02 15:04:05 MST")

	if _, offset := local.Zone(); offset == 0 {
		return s
	}

	utc := t.UTC()
	if local.Format("2006-01-02") == utc.Format("2006-01-02") {
		return s + " (" + utc.Format("15:04:05") + " UTC)"
	}
	return s + " (" + utc.Format("2006-01-02 15:04:05") + " UTC)"
}

// Add registers a channel. Nil channels are ignored.
func (m *Manager) Add(c Channel) {
	if c != nil {
		m.channels = append(m.channels, c)
	}
}

// Count returns the number of registered channels.
func (m *Manager) Count() int { return len(m.channels) }

// Names returns the names of the registered channels.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.channels))
	for _, c := range m.channels {
		names = append(names, c.Name())
	}
	return names
}

// dispatch sends a message to every channel. It is best-effort: a failure on
// one channel is logged and does not prevent the others. The joined error is
// returned so callers (e.g. the CLI test command) can surface failures.
func (m *Manager) dispatch(msg Message) error {
	var errs []error
	for _, c := range m.channels {
		if err := c.Send(msg); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Name(), err))
			if m.logger != nil {
				m.logger.Error("failed to send notification", "channel", c.Name(), "error", err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) SendLoginAlert(event *parser.SSHEvent, country, city, warning string) error {
	location := formatLocation(event.IP, country, city)

	body := fmt.Sprintf(`🔐 <b>SSH Login Alert</b>
🖥️ Server: %s

👤 User: %s
📅 Time: %s
🔓 Method: %s
🌐 IP: %s
📍 Location: %s`,
		escapeHTML(m.serverInfo),
		escapeHTML(event.Username),
		m.formatTime(event.Timestamp),
		event.Method,
		escapeHTML(event.IP),
		escapeHTML(location),
	)

	if warning != "" {
		body += fmt.Sprintf("\n\n⚠️ %s", escapeHTML(warning))
	}

	return m.dispatch(message("SSH Login Alert", body))
}

func (m *Manager) SendDailyReport(report string) error {
	return m.dispatch(message("Daily SSH Report", report))
}

func (m *Manager) SendTestMessage() error {
	body := fmt.Sprintf(`✅ <b>OxiWatch Test Message</b>
🖥️ Server: %s
📅 Time: %s

Connection successful!`,
		escapeHTML(m.serverInfo),
		m.formatTime(time.Now()),
	)
	return m.dispatch(message("OxiWatch Test Message", body))
}

func (m *Manager) SendStartupMessage(version string) error {
	body := fmt.Sprintf(`🟢 <b>OxiWatch Started</b>
🖥️ Server: %s
📅 Time: %s
📦 Version: %s`,
		escapeHTML(m.serverInfo),
		m.formatTime(time.Now()),
		escapeHTML(version),
	)
	return m.dispatch(message("OxiWatch Started", body))
}

func (m *Manager) SendShutdownMessage() error {
	body := fmt.Sprintf(`🔴 <b>OxiWatch Stopped</b>
🖥️ Server: %s
📅 Time: %s`,
		escapeHTML(m.serverInfo),
		m.formatTime(time.Now()),
	)
	return m.dispatch(message("OxiWatch Stopped", body))
}

// message builds a Message from an HTML body, deriving the plain-text variant.
func message(subject, htmlBody string) Message {
	return Message{Subject: subject, HTML: htmlBody, Text: htmlToText(htmlBody)}
}

func buildServerInfo(serverName string) string {
	ipv4 := getPublicIP("https://api.ipify.org")
	ipv6 := getPublicIP("https://api6.ipify.org")

	var ips []string
	if ipv4 != "" {
		ips = append(ips, ipv4)
	}
	if ipv6 != "" {
		ips = append(ips, ipv6)
	}
	if len(ips) > 0 {
		return fmt.Sprintf("%s (%s)", serverName, strings.Join(ips, ", "))
	}
	return serverName
}

func getPublicIP(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(body))
}

func formatLocation(ip, country, city string) string {
	if country == "" && city == "" {
		return ip
	}
	if city != "" && country != "" {
		return fmt.Sprintf("%s, %s", city, country)
	}
	if country != "" {
		return country
	}
	return city
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// htmlToText converts the small HTML subset used in notifications into plain
// text by dropping tags and unescaping entities.
func htmlToText(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&amp;", "&")
	return out
}
