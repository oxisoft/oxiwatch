package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

// newTestStorage creates a Storage backed by a temp-file DB (not :memory:, to
// avoid the connection-pool gotcha where each pooled connection gets its own
// independent in-memory database).
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return s
}

// ev builds a parser.SSHEvent with the given fields.
func ev(et parser.EventType, user, ip string, port int, method string, ts time.Time, invalid bool) *parser.SSHEvent {
	return &parser.SSHEvent{
		Timestamp:   ts,
		EventType:   et,
		Username:    user,
		IP:          ip,
		Port:        port,
		Method:      method,
		InvalidUser: invalid,
	}
}

func TestInsertEvent_Success(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	if err := s.InsertEvent(ev(parser.EventSuccess, "root", "1.2.3.4", 22, "password", now, false), "US", "NYC"); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	count, err := s.GetSuccessCount(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetSuccessCount() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("GetSuccessCount() = %d, want 1", count)
	}
}

func TestInsertEvent_Failure_ClosedDB(t *testing.T) {
	// Closing the DB then inserting should produce an error (error path).
	s, err := New(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err = s.InsertEvent(ev(parser.EventSuccess, "root", "1.2.3.4", 22, "password", time.Now(), false), "US", "NYC")
	if err == nil {
		t.Fatal("InsertEvent() on closed DB: expected error, got nil")
	}
}

func TestInsertEvent_EmptyCountryCityStoredAsNull(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// Empty country/city -> stored as NULL, read back via COALESCE as "".
	if err := s.InsertEvent(ev(parser.EventSuccess, "alice", "9.9.9.9", 2222, "publickey", now, false), "", ""); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	logins, err := s.GetSuccessfulLogins(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetSuccessfulLogins() error = %v", err)
	}
	if len(logins) != 1 {
		t.Fatalf("GetSuccessfulLogins() len = %d, want 1", len(logins))
	}
	if logins[0].Country != "" || logins[0].City != "" {
		t.Fatalf("country/city = %q/%q, want empty/empty", logins[0].Country, logins[0].City)
	}

	// Verify it is genuinely stored as SQL NULL (not the empty string literal).
	var nullCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM ssh_events WHERE country IS NULL AND city IS NULL`).Scan(&nullCount)
	if err != nil {
		t.Fatalf("null check query error = %v", err)
	}
	if nullCount != 1 {
		t.Fatalf("rows with NULL country/city = %d, want 1", nullCount)
	}
}

func TestGetSuccessfulLogins(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// Two successes, one failure (should be ignored).
	if err := s.InsertEvent(ev(parser.EventSuccess, "root", "1.1.1.1", 22, "password", now.Add(-2*time.Minute), false), "US", "NYC"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventSuccess, "bob", "2.2.2.2", 22, "publickey", now.Add(-1*time.Minute), false), "DE", "Berlin"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventFailure, "x", "3.3.3.3", 22, "password", now, true), "", ""); err != nil {
		t.Fatal(err)
	}

	logins, err := s.GetSuccessfulLogins(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetSuccessfulLogins() error = %v", err)
	}
	if len(logins) != 2 {
		t.Fatalf("GetSuccessfulLogins() len = %d, want 2", len(logins))
	}
	// Ordered by timestamp DESC: bob first.
	if logins[0].Username != "bob" || logins[1].Username != "root" {
		t.Fatalf("ordering wrong: got %q then %q", logins[0].Username, logins[1].Username)
	}
	if logins[0].Country != "DE" || logins[0].City != "Berlin" {
		t.Fatalf("country/city = %q/%q, want DE/Berlin", logins[0].Country, logins[0].City)
	}

	// "since" filter excludes the older success.
	recent, err := s.GetSuccessfulLogins(now.Add(-90 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Username != "bob" {
		t.Fatalf("since filter: got %d rows, want 1 (bob)", len(recent))
	}
}

func TestGetFailedAttempts(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	if err := s.InsertEvent(ev(parser.EventFailure, "root", "1.1.1.1", 22, "password", now.Add(-2*time.Minute), true), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventFailure, "admin", "2.2.2.2", 22, "password", now.Add(-1*time.Minute), false), "FR", "Paris"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventSuccess, "ok", "3.3.3.3", 22, "publickey", now, false), "US", "LA"); err != nil {
		t.Fatal(err)
	}

	failed, err := s.GetFailedAttempts(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetFailedAttempts() error = %v", err)
	}
	if len(failed) != 2 {
		t.Fatalf("GetFailedAttempts() len = %d, want 2", len(failed))
	}
	if failed[0].Username != "admin" {
		t.Fatalf("ordering: first = %q, want admin", failed[0].Username)
	}
	if !failed[1].InvalidUser {
		t.Fatalf("expected root attempt to have InvalidUser=true")
	}
}

func TestGetTopUsernames_OrderingAndLimit(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	insertN := func(user string, n int) {
		for i := 0; i < n; i++ {
			if err := s.InsertEvent(ev(parser.EventFailure, user, "1.1.1.1", 22, "password", now, false), "", ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	insertN("root", 5)
	insertN("admin", 3)
	insertN("test", 1)
	// A success must not be counted.
	if err := s.InsertEvent(ev(parser.EventSuccess, "root", "1.1.1.1", 22, "password", now, false), "", ""); err != nil {
		t.Fatal(err)
	}

	top, err := s.GetTopUsernames(now.Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("GetTopUsernames() error = %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("limit not respected: len = %d, want 2", len(top))
	}
	if top[0].Username != "root" || top[0].Count != 5 {
		t.Fatalf("top[0] = %+v, want root/5", top[0])
	}
	if top[1].Username != "admin" || top[1].Count != 3 {
		t.Fatalf("top[1] = %+v, want admin/3", top[1])
	}
}

func TestGetTopIPs_OrderingAndLimit(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	insertN := func(ip, country, city string, n int) {
		for i := 0; i < n; i++ {
			if err := s.InsertEvent(ev(parser.EventFailure, "x", ip, 22, "password", now, false), country, city); err != nil {
				t.Fatal(err)
			}
		}
	}
	insertN("10.0.0.1", "US", "NYC", 4)
	insertN("10.0.0.2", "", "", 2)
	insertN("10.0.0.3", "DE", "Berlin", 1)

	top, err := s.GetTopIPs(now.Add(-time.Hour), 2)
	if err != nil {
		t.Fatalf("GetTopIPs() error = %v", err)
	}
	if len(top) != 2 {
		t.Fatalf("limit not respected: len = %d, want 2", len(top))
	}
	if top[0].IP != "10.0.0.1" || top[0].Count != 4 {
		t.Fatalf("top[0] = %+v, want 10.0.0.1/4", top[0])
	}
	if top[0].Country != "US" || top[0].City != "NYC" {
		t.Fatalf("top[0] geo = %q/%q, want US/NYC", top[0].Country, top[0].City)
	}
	if top[1].IP != "10.0.0.2" || top[1].Count != 2 {
		t.Fatalf("top[1] = %+v, want 10.0.0.2/2", top[1])
	}
	// NULL geo for second IP reads back as "".
	if top[1].Country != "" || top[1].City != "" {
		t.Fatalf("top[1] geo = %q/%q, want empty/empty", top[1].Country, top[1].City)
	}
}

func TestGetFailedStats(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// 4 failures: ips {a,a,b,c} -> 3 unique; usernames {root,root,admin,admin} -> 2 unique.
	rows := []struct {
		user, ip string
	}{
		{"root", "1.1.1.1"},
		{"root", "1.1.1.1"},
		{"admin", "2.2.2.2"},
		{"admin", "3.3.3.3"},
	}
	for _, r := range rows {
		if err := s.InsertEvent(ev(parser.EventFailure, r.user, r.ip, 22, "password", now, false), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	// A success must be excluded from failed stats.
	if err := s.InsertEvent(ev(parser.EventSuccess, "ok", "9.9.9.9", 22, "publickey", now, false), "", ""); err != nil {
		t.Fatal(err)
	}

	stats, err := s.GetFailedStats(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetFailedStats() error = %v", err)
	}
	if stats.TotalAttempts != 4 {
		t.Errorf("TotalAttempts = %d, want 4", stats.TotalAttempts)
	}
	if stats.UniqueIPs != 3 {
		t.Errorf("UniqueIPs = %d, want 3", stats.UniqueIPs)
	}
	if stats.UniqueUsernames != 2 {
		t.Errorf("UniqueUsernames = %d, want 2", stats.UniqueUsernames)
	}
}

func TestGetSuccessCount(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	if err := s.InsertEvent(ev(parser.EventSuccess, "a", "1.1.1.1", 22, "password", now.Add(-2*time.Hour), false), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventSuccess, "b", "2.2.2.2", 22, "password", now, false), "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventFailure, "c", "3.3.3.3", 22, "password", now, false), "", ""); err != nil {
		t.Fatal(err)
	}

	all, err := s.GetSuccessCount(now.Add(-3 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if all != 2 {
		t.Errorf("GetSuccessCount(all) = %d, want 2", all)
	}

	recent, err := s.GetSuccessCount(now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if recent != 1 {
		t.Errorf("GetSuccessCount(recent) = %d, want 1", recent)
	}
}

func TestGetOverallStats(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// 2 successes, 3 failures.
	// IPs overall: 1.1.1.1, 2.2.2.2, 3.3.3.3 -> 3 unique.
	// Usernames overall: ok, root, admin -> 3 unique.
	inserts := []*parser.SSHEvent{
		ev(parser.EventSuccess, "ok", "1.1.1.1", 22, "password", now, false),
		ev(parser.EventSuccess, "ok", "1.1.1.1", 22, "password", now, false),
		ev(parser.EventFailure, "root", "2.2.2.2", 22, "password", now, false),
		ev(parser.EventFailure, "admin", "3.3.3.3", 22, "password", now, false),
		ev(parser.EventFailure, "admin", "3.3.3.3", 22, "password", now, false),
	}
	for _, e := range inserts {
		if err := s.InsertEvent(e, "", ""); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := s.GetOverallStats(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetOverallStats() error = %v", err)
	}
	if stats.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", stats.SuccessCount)
	}
	if stats.FailedCount != 3 {
		t.Errorf("FailedCount = %d, want 3", stats.FailedCount)
	}
	if stats.UniqueIPs != 3 {
		t.Errorf("UniqueIPs = %d, want 3", stats.UniqueIPs)
	}
	if stats.UniqueUsernames != 3 {
		t.Errorf("UniqueUsernames = %d, want 3", stats.UniqueUsernames)
	}
}

func TestGetLastLoginForUser_Found(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// Two successful logins for "root"; latest should be returned.
	if err := s.InsertEvent(ev(parser.EventSuccess, "root", "1.1.1.1", 22, "password", now.Add(-time.Hour), false), "US", "NYC"); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertEvent(ev(parser.EventSuccess, "root", "5.5.5.5", 2222, "publickey", now, false), "DE", "Berlin"); err != nil {
		t.Fatal(err)
	}
	// A failure for root must not be returned.
	if err := s.InsertEvent(ev(parser.EventFailure, "root", "9.9.9.9", 22, "password", now.Add(time.Hour), false), "", ""); err != nil {
		t.Fatal(err)
	}

	rec, err := s.GetLastLoginForUser("root")
	if err != nil {
		t.Fatalf("GetLastLoginForUser() error = %v", err)
	}
	if rec.IP != "5.5.5.5" || rec.Port != 2222 || rec.Method != "publickey" {
		t.Fatalf("latest login = %+v, want ip 5.5.5.5 / port 2222 / publickey", rec)
	}
	if rec.Country != "DE" || rec.City != "Berlin" {
		t.Fatalf("geo = %q/%q, want DE/Berlin", rec.Country, rec.City)
	}
}

func TestGetLastLoginForUser_NoRows(t *testing.T) {
	s := newTestStorage(t)

	rec, err := s.GetLastLoginForUser("nobody")
	if rec != nil {
		t.Fatalf("expected nil record, got %+v", rec)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error = %v, want sql.ErrNoRows", err)
	}
}

func TestCleanup(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// retentionDays = 30; cutoff is ~30 days ago.
	oldTS := now.AddDate(0, 0, -40) // older than retention -> deleted
	recentTS := now.AddDate(0, 0, -5)

	// Three old rows.
	for i := 0; i < 3; i++ {
		if err := s.InsertEvent(ev(parser.EventFailure, "old", "1.1.1.1", 22, "password", oldTS, false), "", ""); err != nil {
			t.Fatal(err)
		}
	}
	// Two recent rows.
	for i := 0; i < 2; i++ {
		if err := s.InsertEvent(ev(parser.EventSuccess, "new", "2.2.2.2", 22, "publickey", recentTS, false), "US", "NYC"); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := s.Cleanup(30)
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("Cleanup() deleted = %d, want 3", deleted)
	}

	// Recent rows remain.
	var remaining int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ssh_events`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("remaining rows = %d, want 2", remaining)
	}

	logins, err := s.GetSuccessfulLogins(now.AddDate(0, 0, -10))
	if err != nil {
		t.Fatal(err)
	}
	if len(logins) != 2 {
		t.Fatalf("recent successful logins = %d, want 2", len(logins))
	}
}

func TestQueries_ClosedDB_ErrorPaths(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	since := time.Now().Add(-time.Hour)

	if _, err := s.GetSuccessfulLogins(since); err == nil {
		t.Error("GetSuccessfulLogins() on closed DB: expected error")
	}
	if _, err := s.GetFailedAttempts(since); err == nil {
		t.Error("GetFailedAttempts() on closed DB: expected error")
	}
	if _, err := s.GetTopUsernames(since, 5); err == nil {
		t.Error("GetTopUsernames() on closed DB: expected error")
	}
	if _, err := s.GetTopIPs(since, 5); err == nil {
		t.Error("GetTopIPs() on closed DB: expected error")
	}
	if _, err := s.GetFailedStats(since); err == nil {
		t.Error("GetFailedStats() on closed DB: expected error")
	}
	if _, err := s.GetSuccessCount(since); err == nil {
		t.Error("GetSuccessCount() on closed DB: expected error")
	}
	if _, err := s.GetOverallStats(since); err == nil {
		t.Error("GetOverallStats() on closed DB: expected error")
	}
	if _, err := s.GetLastLoginForUser("root"); err == nil {
		t.Error("GetLastLoginForUser() on closed DB: expected error")
	}
	if _, err := s.Cleanup(30); err == nil {
		t.Error("Cleanup() on closed DB: expected error")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// A path inside a non-existent directory should fail (error path for New).
	_, err := New(filepath.Join(t.TempDir(), "does-not-exist", "t.db"))
	if err == nil {
		t.Fatal("New() with bad path: expected error, got nil")
	}
}
