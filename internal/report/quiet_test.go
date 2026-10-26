package report

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

func TestReportsShowQuietLogins(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	at := day.Add(time.Minute)
	if now.Before(at) {
		at = now
	}
	add := func(user, ip, label string) {
		ev := &parser.SSHEvent{Timestamp: at, EventType: parser.EventSuccess, Username: user, IP: ip, Port: 22, Method: "publickey"}
		if err := store.InsertEventQuiet(ev, "Poland", "Warsaw", label); err != nil {
			t.Fatal(err)
		}
	}
	add("alex", "203.0.113.7", "Alex home")
	add("alex", "203.0.113.7", "Alex home")
	add("root", "192.0.2.1", "")

	g := NewGenerator(store, "srv", "")

	daily, err := g.GenerateDailyReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(daily, "Successful logins: 3 (2 quiet)") {
		t.Errorf("daily report missing quiet count:\n%s", daily)
	}

	stats, err := g.GenerateStats(1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stats, "Successful logins: 3 (2 quiet)") {
		t.Errorf("stats missing quiet count:\n%s", stats)
	}

	logins, err := g.GenerateLoginsReport(1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(logins, "[quiet: Alex home]") != 2 {
		t.Errorf("logins report missing quiet labels:\n%s", logins)
	}
	for _, line := range strings.Split(logins, "\n") {
		if strings.Contains(line, "root") && strings.Contains(line, "quiet") {
			t.Errorf("normal login marked quiet: %q", line)
		}
	}
}

func TestQuietSuffix(t *testing.T) {
	if quietSuffix(0) != "" || quietSuffix(1234) != " (1,234 quiet)" {
		t.Errorf("quietSuffix wrong: %q %q", quietSuffix(0), quietSuffix(1234))
	}
}

// The daily report covers exactly one day: events from the next morning
// (before the report is sent) must not leak into "yesterday".
func TestDailyReportIsBoundedToOneDay(t *testing.T) {
	day := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	store := newTestStore(t, []seedEvent{
		{ts: day.Add(-time.Minute), eventType: parser.EventFailure, username: "before", ip: "4.4.4.4", port: 22, method: "password"},
		{ts: day.Add(10 * time.Hour), eventType: parser.EventFailure, username: "root", ip: "1.1.1.1", port: 22, method: "password"},
		{ts: day.Add(11 * time.Hour), eventType: parser.EventSuccess, username: "alice", ip: "9.9.9.9", port: 22, method: "publickey"},
		{ts: day.Add(24 * time.Hour), eventType: parser.EventFailure, username: "nextday", ip: "5.5.5.5", port: 22, method: "password"},
		{ts: day.Add(31 * time.Hour), eventType: parser.EventSuccess, username: "bob", ip: "8.8.8.8", port: 22, method: "publickey"},
	})
	out, err := NewGenerator(store, "srv", "").GenerateDailyReport(day)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Successful logins: 1\n", "Failed attempts: 1\n", "root"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	for _, leak := range []string{"nextday", "5.5.5.5", "before", "4.4.4.4"} {
		if strings.Contains(out, leak) {
			t.Errorf("report includes %q from outside the day:\n%s", leak, out)
		}
	}
}
