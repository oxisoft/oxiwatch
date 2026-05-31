package notifier

import (
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

// newTestManager builds a Manager without invoking buildServerInfo (which would
// reach the network), by populating the struct fields directly.
func newTestManager(serverInfo string, ch ...Channel) *Manager {
	m := &Manager{serverInfo: serverInfo}
	for _, c := range ch {
		m.Add(c)
	}
	return m
}

func TestSendLoginAlert(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv (1.2.3.4)", fc)

	ev := &parser.SSHEvent{
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Username:  "root",
		IP:        "9.9.9.9",
		Method:    "publickey",
	}
	if err := m.SendLoginAlert(ev, "Germany", "Berlin", "suspicious"); err != nil {
		t.Fatalf("SendLoginAlert() error: %v", err)
	}
	if len(fc.received) != 1 {
		t.Fatalf("received %d messages, want 1", len(fc.received))
	}
	got := fc.received[0]
	if got.Subject != "SSH Login Alert" {
		t.Errorf("Subject = %q, want SSH Login Alert", got.Subject)
	}
	for _, want := range []string{"root", "9.9.9.9", "Berlin, Germany", "publickey", "suspicious"} {
		if !strings.Contains(got.HTML, want) {
			t.Errorf("HTML missing %q: %q", want, got.HTML)
		}
	}
	// Text variant must equal htmlToText(HTML).
	if got.Text != htmlToText(got.HTML) {
		t.Errorf("Text != htmlToText(HTML)")
	}
}

func TestSendLoginAlertNoWarning(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv", fc)

	ev := &parser.SSHEvent{
		Timestamp: time.Now(),
		Username:  "bob",
		IP:        "5.5.5.5",
		Method:    "password",
	}
	if err := m.SendLoginAlert(ev, "", "", ""); err != nil {
		t.Fatalf("SendLoginAlert() error: %v", err)
	}
	got := fc.received[0]
	if strings.Contains(got.HTML, "⚠️") {
		t.Errorf("warning emoji present despite empty warning: %q", got.HTML)
	}
	// With no geo, location falls back to the IP.
	if !strings.Contains(got.HTML, "5.5.5.5") {
		t.Errorf("HTML missing IP-fallback location: %q", got.HTML)
	}
}

func TestSendDailyReport(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv", fc)

	if err := m.SendDailyReport("the report body"); err != nil {
		t.Fatalf("SendDailyReport() error: %v", err)
	}
	got := fc.received[0]
	if got.Subject != "Daily SSH Report" {
		t.Errorf("Subject = %q, want Daily SSH Report", got.Subject)
	}
	if !strings.Contains(got.HTML, "the report body") {
		t.Errorf("HTML missing report body: %q", got.HTML)
	}
}

func TestSendTestMessage(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv (1.1.1.1)", fc)

	if err := m.SendTestMessage(); err != nil {
		t.Fatalf("SendTestMessage() error: %v", err)
	}
	got := fc.received[0]
	if got.Subject != "OxiWatch Test Message" {
		t.Errorf("Subject = %q", got.Subject)
	}
	if !strings.Contains(got.HTML, "Connection successful!") {
		t.Errorf("HTML missing success text: %q", got.HTML)
	}
}

func TestSendStartupMessage(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv", fc)

	if err := m.SendStartupMessage("v1.2.3"); err != nil {
		t.Fatalf("SendStartupMessage() error: %v", err)
	}
	got := fc.received[0]
	if got.Subject != "OxiWatch Started" {
		t.Errorf("Subject = %q", got.Subject)
	}
	if !strings.Contains(got.HTML, "v1.2.3") {
		t.Errorf("HTML missing version: %q", got.HTML)
	}
}

func TestSendShutdownMessage(t *testing.T) {
	fc := &fakeChannel{name: "fc"}
	m := newTestManager("srv", fc)

	if err := m.SendShutdownMessage(); err != nil {
		t.Fatalf("SendShutdownMessage() error: %v", err)
	}
	got := fc.received[0]
	if got.Subject != "OxiWatch Stopped" {
		t.Errorf("Subject = %q", got.Subject)
	}
}
