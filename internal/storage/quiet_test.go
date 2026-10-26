package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
	_ "modernc.org/sqlite"
)

// A database created by an older version (no quiet_label column) is migrated
// in place and keeps its rows.
func TestMigrateAddsQuietLabel(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE ssh_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp DATETIME NOT NULL,
		event_type TEXT NOT NULL, username TEXT NOT NULL, ip TEXT NOT NULL,
		port INTEGER, method TEXT NOT NULL, country TEXT, city TEXT,
		invalid_user BOOLEAN DEFAULT FALSE, created_at DATETIME DEFAULT CURRENT_TIMESTAMP);
		INSERT INTO ssh_events (timestamp, event_type, username, ip, port, method)
		VALUES (?, 'success', 'old', '1.1.1.1', 22, 'publickey');`, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	for i := 0; i < 2; i++ { // second open must be a no-op
		s, err := New(p)
		if err != nil {
			t.Fatalf("New() on old schema: %v", err)
		}
		s.Close()
	}
	s, _ := New(p)
	defer s.Close()

	ev := &parser.SSHEvent{Timestamp: time.Now(), EventType: parser.EventSuccess, Username: "alex", IP: "2.2.2.2", Port: 22, Method: "publickey"}
	if err := s.InsertEventQuiet(ev, "", "", "Home"); err != nil {
		t.Fatal(err)
	}
	logins, err := s.GetSuccessfulLogins(time.Now().Add(-time.Hour))
	if err != nil || len(logins) != 2 {
		t.Fatalf("logins = %d, %v", len(logins), err)
	}
	stats, err := s.GetOverallStats(time.Now().Add(-time.Hour))
	if err != nil || stats.SuccessCount != 2 || stats.QuietCount != 1 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
	last, err := s.GetQuietLastMatches()
	if err != nil || len(last) != 1 || last["Home"].IsZero() {
		t.Fatalf("last matches = %v, %v", last, err)
	}
	l, err := s.GetLastLoginForUser("alex")
	if err != nil || l.QuietLabel != "Home" {
		t.Fatalf("last login = %+v, %v", l, err)
	}
}
