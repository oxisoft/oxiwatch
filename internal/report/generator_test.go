package report

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want string
	}{
		{"zero", 0, "0"},
		{"thousand", 1000, "1,000"},
		{"million", 1234567, "1,234,567"},
		{"under thousand", 999, "999"},
		{"single digit", 5, "5"},
		{"exact ten thousand", 10000, "10,000"},
		{"negative", -1000, "-1,000"}, // minus sign counts as a leading char
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNumber(tt.in); got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name    string
		country string
		city    string
		want    string
	}{
		{"both present", "Germany", "Berlin", "Berlin, Germany"},
		{"country only", "Germany", "", "Germany"},
		{"city only", "", "Berlin", "Berlin"},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLocation(tt.country, tt.city); got != tt.want {
				t.Errorf("formatLocation(%q, %q) = %q, want %q", tt.country, tt.city, got, tt.want)
			}
		})
	}
}

// newTestStore builds a real storage backed by a temp sqlite file and seeds it
// with a known set of success and failure events.
func newTestStore(t *testing.T, events []seedEvent) *storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, ev := range events {
		e := &parser.SSHEvent{
			Timestamp:   ev.ts,
			EventType:   ev.eventType,
			Username:    ev.username,
			IP:          ev.ip,
			Port:        ev.port,
			Method:      ev.method,
			InvalidUser: ev.invalidUser,
		}
		if err := store.InsertEvent(e, ev.country, ev.city); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
	return store
}

type seedEvent struct {
	ts          time.Time
	eventType   parser.EventType
	username    string
	ip          string
	port        int
	method      string
	country     string
	city        string
	invalidUser bool
}

func TestGenerateDailyReport(t *testing.T) {
	day := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	mid := day.Add(12 * time.Hour)

	events := []seedEvent{
		// Failures: root x3, admin x2, test x1
		{ts: mid, eventType: parser.EventFailure, username: "root", ip: "1.1.1.1", port: 22, method: "password", country: "China", city: "Beijing"},
		{ts: mid.Add(time.Minute), eventType: parser.EventFailure, username: "root", ip: "1.1.1.1", port: 22, method: "password", country: "China", city: "Beijing"},
		{ts: mid.Add(2 * time.Minute), eventType: parser.EventFailure, username: "root", ip: "2.2.2.2", port: 22, method: "password"},
		{ts: mid.Add(3 * time.Minute), eventType: parser.EventFailure, username: "admin", ip: "1.1.1.1", port: 22, method: "password", country: "China", city: "Beijing"},
		{ts: mid.Add(4 * time.Minute), eventType: parser.EventFailure, username: "admin", ip: "3.3.3.3", port: 22, method: "password"},
		{ts: mid.Add(5 * time.Minute), eventType: parser.EventFailure, username: "test", ip: "2.2.2.2", port: 22, method: "password"},
		// Successes: 2
		{ts: mid.Add(6 * time.Minute), eventType: parser.EventSuccess, username: "alice", ip: "9.9.9.9", port: 22, method: "publickey"},
		{ts: mid.Add(7 * time.Minute), eventType: parser.EventSuccess, username: "bob", ip: "9.9.9.9", port: 22, method: "publickey"},
	}
	store := newTestStore(t, events)

	// Empty currentVersion to skip network version check.
	g := NewGenerator(store, "srv", "")

	out, err := g.GenerateDailyReport(day)
	if err != nil {
		t.Fatalf("GenerateDailyReport: %v", err)
	}

	wantContains := []string{
		"Daily SSH Report",
		"Server: srv",
		"2026-05-30",
		"Successful logins: 2",
		"Failed attempts: 6",
		"Unique IPs: 3",       // 1.1.1.1, 2.2.2.2, 3.3.3.3
		"Unique usernames: 3", // root, admin, test
		"Top 10 Usernames",
		"root - 3",
		"admin - 2",
		"Top 10 IPs",
		"1.1.1.1 (Beijing, China) - 3",
		"2.2.2.2 - 2",
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("report missing %q\n---\n%s", w, out)
		}
	}

	// Must not contain the update banner since currentVersion is empty.
	if strings.Contains(out, "Update Available") {
		t.Errorf("report should not contain update banner with empty version\n%s", out)
	}
}

func TestGenerateDailyReportEmpty(t *testing.T) {
	day := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	store := newTestStore(t, nil)
	g := NewGenerator(store, "emptysrv", "")

	out, err := g.GenerateDailyReport(day)
	if err != nil {
		t.Fatalf("GenerateDailyReport: %v", err)
	}

	for _, w := range []string{
		"Daily SSH Report",
		"Server: emptysrv",
		"Successful logins: 0",
		"Failed attempts: 0",
		"Unique IPs: 0",
		"Unique usernames: 0",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("report missing %q\n---\n%s", w, out)
		}
	}
	// No top sections when there is no data.
	if strings.Contains(out, "Top 10 Usernames") {
		t.Errorf("did not expect Top Usernames section with no data\n%s", out)
	}
	if strings.Contains(out, "Top 10 IPs") {
		t.Errorf("did not expect Top IPs section with no data\n%s", out)
	}
}

func TestGenerateStats(t *testing.T) {
	now := time.Now()
	events := []seedEvent{
		{ts: now.Add(-1 * time.Hour), eventType: parser.EventSuccess, username: "alice", ip: "10.0.0.1", port: 22, method: "publickey"},
		{ts: now.Add(-2 * time.Hour), eventType: parser.EventSuccess, username: "bob", ip: "10.0.0.2", port: 22, method: "publickey"},
		{ts: now.Add(-3 * time.Hour), eventType: parser.EventFailure, username: "root", ip: "10.0.0.3", port: 22, method: "password"},
		{ts: now.Add(-4 * time.Hour), eventType: parser.EventFailure, username: "root", ip: "10.0.0.3", port: 22, method: "password"},
		{ts: now.Add(-5 * time.Hour), eventType: parser.EventFailure, username: "admin", ip: "10.0.0.4", port: 22, method: "password"},
	}
	store := newTestStore(t, events)
	g := NewGenerator(store, "statsrv", "")

	out, err := g.GenerateStats(7)
	if err != nil {
		t.Fatalf("GenerateStats: %v", err)
	}

	for _, w := range []string{
		"SSH Statistics (last 7 days)",
		"Server: statsrv",
		"Successful logins: 2",
		"Failed attempts: 3",
		"Unique IPs: 4",
		"Unique usernames: 4", // distinct usernames across all events: alice, bob, root, admin
	} {
		if !strings.Contains(out, w) {
			t.Errorf("stats missing %q\n---\n%s", w, out)
		}
	}
}

func TestGenerateLoginsReport(t *testing.T) {
	now := time.Now()
	events := []seedEvent{
		{ts: now.Add(-1 * time.Hour), eventType: parser.EventSuccess, username: "alice", ip: "10.0.0.1", port: 22, method: "publickey", country: "Germany", city: "Berlin"},
		{ts: now.Add(-2 * time.Hour), eventType: parser.EventSuccess, username: "bob", ip: "10.0.0.2", port: 22, method: "password"},
		// A failure that must NOT appear in the logins report.
		{ts: now.Add(-3 * time.Hour), eventType: parser.EventFailure, username: "root", ip: "10.0.0.9", port: 22, method: "password"},
	}
	store := newTestStore(t, events)
	g := NewGenerator(store, "loginsrv", "")

	out, err := g.GenerateLoginsReport(7)
	if err != nil {
		t.Fatalf("GenerateLoginsReport: %v", err)
	}

	for _, w := range []string{
		"Successful SSH Logins (last 7 days)",
		"Server: loginsrv",
		"alice",
		"bob",
		"10.0.0.1",
		"Berlin, Germany",
		"publickey",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("logins report missing %q\n---\n%s", w, out)
		}
	}
	// The failing root login must not be listed.
	if strings.Contains(out, "10.0.0.9") {
		t.Errorf("logins report should not include failure event IP\n%s", out)
	}
	if strings.Contains(out, "No successful logins") {
		t.Errorf("did not expect empty message when logins exist\n%s", out)
	}
}

func TestGenerateLoginsReportNoData(t *testing.T) {
	// Seed an old success outside the 1-day range, plus a recent failure.
	now := time.Now()
	events := []seedEvent{
		{ts: now.AddDate(0, 0, -30), eventType: parser.EventSuccess, username: "alice", ip: "10.0.0.1", port: 22, method: "publickey"},
		{ts: now.Add(-1 * time.Hour), eventType: parser.EventFailure, username: "root", ip: "10.0.0.9", port: 22, method: "password"},
	}
	store := newTestStore(t, events)
	g := NewGenerator(store, "loginsrv", "")

	out, err := g.GenerateLoginsReport(1)
	if err != nil {
		t.Fatalf("GenerateLoginsReport: %v", err)
	}

	if !strings.Contains(out, "No successful logins") {
		t.Errorf("expected 'No successful logins' message\n---\n%s", out)
	}
	if strings.Contains(out, "alice") {
		t.Errorf("old login outside range should not appear\n%s", out)
	}
}
