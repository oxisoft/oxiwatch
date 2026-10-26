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
