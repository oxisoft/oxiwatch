package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/quiet"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

// fakeMatrix is a Matrix homeserver that records every message body sent to it.
type fakeMatrix struct {
	mu     sync.Mutex
	bodies []string
	srv    *httptest.Server
}

func newFakeMatrix(t *testing.T) *fakeMatrix {
	t.Helper()
	fm := &fakeMatrix{}
	fm.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]string
		_ = json.NewDecoder(r.Body).Decode(&p)
		fm.mu.Lock()
		fm.bodies = append(fm.bodies, p["body"])
		fm.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_id":"$x"}`))
	}))
	t.Cleanup(fm.srv.Close)
	return fm
}

func (fm *fakeMatrix) messages() []string {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return append([]string(nil), fm.bodies...)
}

// testConfig is a config with only a Matrix channel (pointing at fm), a temp
// database, no GeoIP and no network access.
func testConfig(t *testing.T, fm *fakeMatrix) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.ServerName = "test-host"
	cfg.DatabasePath = filepath.Join(t.TempDir(), "oxiwatch.db")
	cfg.GeoIPEnabled = false
	cfg.DailyReportEnabled = false
	cfg.MatrixHomeserver = fm.srv.URL
	cfg.MatrixRoomID = "!room:example.org"
	cfg.MatrixAccessToken = "secret-token"
	return cfg
}

// fakeJournalctl puts a `journalctl` script first in PATH that prints lines.
func fakeJournalctl(t *testing.T, lines ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake needs a POSIX shell")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(data, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + data + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func journalLine(ident, msg string) string {
	b, _ := json.Marshal(map[string]string{
		"__REALTIME_TIMESTAMP": "1700000000000000",
		"SYSLOG_IDENTIFIER":    ident,
		"MESSAGE":              msg,
	})
	return string(b)
}

// TestRunEndToEnd drives the whole alert path: journal lines -> parse ->
// store -> quiet list -> notifier, then a clean shutdown when the journal ends.
func TestRunEndToEnd(t *testing.T) {
	fm := newFakeMatrix(t)
	cfg := testConfig(t, fm)
	cfg.MetricsEnabled = true
	cfg.MetricsListen = "127.0.0.1:0"
	cfg.DailyReportEnabled = true
	cfg.QuietIPs = []quiet.Entry{{IP: "2001:db8::/32", User: "carol", Label: "Carol office"}}

	fakeJournalctl(t,
		journalLine("sshd", "Accepted publickey for alex from 203.0.113.7 port 49296 ssh2: ED25519 SHA256:xxx"),
		journalLine("sshd-session", "Failed password for invalid user admin from 142.0.45.14 port 52772 ssh2"),
		journalLine("sshd", "Accepted password for carol from 2001:db8::dead:beef port 22 ssh2"),
		journalLine("sshd", "Accepted publickey for root from 2001:db8::1 port 22 ssh2"),
		journalLine("systemd", "Started ssh.service"),
		"garbage line",
	)

	d, err := New(cfg, discardLogger(), "1.2.3-test")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if d.metrics == nil {
		t.Fatal("metrics enabled in config but not created")
	}
	d.WatchConfig(filepath.Join(t.TempDir(), "absent.json"))

	done := make(chan error, 1)
	go func() { done <- d.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run() did not return after the journal ended")
	}

	msgs := fm.messages()
	if len(msgs) != 4 {
		t.Fatalf("want 4 Matrix messages (startup, alex, root, shutdown), got %d: %q", len(msgs), msgs)
	}
	if !strings.Contains(msgs[0], "1.2.3-test") {
		t.Errorf("startup message lacks version: %q", msgs[0])
	}
	if !strings.Contains(msgs[1], "alex") || !strings.Contains(msgs[1], "203.0.113.7") {
		t.Errorf("login alert for alex: %q", msgs[1])
	}
	if !strings.Contains(msgs[2], "root") {
		t.Errorf("root from a quiet network but another user must alert: %q", msgs[2])
	}
	for _, m := range msgs {
		if strings.Contains(m, "carol") || strings.Contains(m, "admin") {
			t.Errorf("quiet login or failed attempt produced an alert: %q", m)
		}
	}

	// Run closed the store; reopen the file and check what was recorded.
	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logins, err := store.GetSuccessfulLogins(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(logins) != 3 {
		t.Errorf("want 3 stored logins (quiet one too), got %d", len(logins))
	}
	failed, err := store.GetFailedAttempts(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].Username != "admin" {
		t.Errorf("want 1 stored failed attempt for admin, got %+v", failed)
	}
	quietCount, err := store.GetQuietSuccessCount(time.Unix(0, 0), farFuture)
	if err != nil {
		t.Fatal(err)
	}
	if quietCount != 1 {
		t.Errorf("want 1 quiet login stored, got %d", quietCount)
	}
}

func TestRunFailsWithoutJournalctl(t *testing.T) {
	fm := newFakeMatrix(t)
	d, err := New(testConfig(t, fm), discardLogger(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.storage.Close() })
	t.Setenv("PATH", t.TempDir())
	if err := d.Run(); err == nil {
		t.Fatal("Run() without journalctl: want error")
	}
	if n := len(fm.messages()); n != 0 {
		t.Errorf("no message expected before monitoring starts, got %d", n)
	}
}

func TestRunRejectsBadReportTime(t *testing.T) {
	fakeJournalctl(t)
	fm := newFakeMatrix(t)
	cfg := testConfig(t, fm)
	cfg.DailyReportEnabled = true
	cfg.DailyReportTime = "25:99"
	d, err := New(cfg, discardLogger(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.storage.Close() })
	if err := d.Run(); err == nil {
		t.Fatal("Run() with invalid daily report time: want error")
	}
}

func TestNewErrors(t *testing.T) {
	fm := newFakeMatrix(t)

	t.Run("no channel", func(t *testing.T) {
		cfg := testConfig(t, fm)
		cfg.MatrixHomeserver = ""
		if _, err := New(cfg, discardLogger(), "test"); err == nil {
			t.Fatal("want error without any notification channel")
		}
	})
	t.Run("unusable database path", func(t *testing.T) {
		cfg := testConfig(t, fm)
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		cfg.DatabasePath = filepath.Join(file, "sub", "db.sqlite")
		if _, err := New(cfg, discardLogger(), "test"); err == nil {
			t.Fatal("want error for a database under a regular file")
		}
	})
	t.Run("invalid quiet entry", func(t *testing.T) {
		cfg := testConfig(t, fm)
		cfg.QuietIPs = []quiet.Entry{{IP: "not-an-address"}}
		if _, err := New(cfg, discardLogger(), "test"); err == nil {
			t.Fatal("want error for an invalid quiet list entry")
		}
	})
}

func TestNewBrokenGeoIPDatabaseContinues(t *testing.T) {
	fm := newFakeMatrix(t)
	cfg := testConfig(t, fm)
	cfg.GeoIPEnabled = true
	cfg.GeoIPDatabasePath = filepath.Join(t.TempDir(), "broken.mmdb")
	if err := os.WriteFile(cfg.GeoIPDatabasePath, []byte("not an mmdb"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := New(cfg, discardLogger(), "test")
	if err != nil {
		t.Fatalf("a broken GeoIP file must not stop the daemon: %v", err)
	}
	t.Cleanup(func() { _ = d.storage.Close() })
	if d.geoip != nil {
		t.Error("resolver set from a broken database")
	}
}

func TestSendDailyReport(t *testing.T) {
	fm := newFakeMatrix(t)
	d, err := New(testConfig(t, fm), discardLogger(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.storage.Close() })
	if err := d.sendDailyReport(context.Background()); err != nil {
		t.Fatalf("sendDailyReport() error: %v", err)
	}
	msgs := fm.messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "test-host") {
		t.Fatalf("want one daily report naming the server, got %q", msgs)
	}
}

func TestRunCleanupDeletesOldEvents(t *testing.T) {
	fm := newFakeMatrix(t)
	cfg := testConfig(t, fm)
	cfg.RetentionDays = 30
	d, err := New(cfg, discardLogger(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.storage.Close() })

	old := login("alex", "203.0.113.7")
	old.Timestamp = time.Now().AddDate(0, 0, -60)
	if err := d.storage.InsertEvent(old, "", ""); err != nil {
		t.Fatal(err)
	}
	insertSuccess(t, d.storage, "alex", "203.0.113.7", "", "")

	if err := d.runCleanup(context.Background()); err != nil {
		t.Fatalf("runCleanup() error: %v", err)
	}
	logins, err := d.storage.GetSuccessfulLogins(time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(logins) != 1 {
		t.Fatalf("want only the recent login kept, got %d", len(logins))
	}
	// Nothing left to delete: still no error.
	if err := d.runCleanup(context.Background()); err != nil {
		t.Fatalf("second runCleanup() error: %v", err)
	}
}

func TestProcessEventFailureAndStoreError(t *testing.T) {
	d, rec := newQuietDaemon(t, nil)

	failed := &parser.SSHEvent{Timestamp: time.Now(), EventType: parser.EventFailure, Username: "root", IP: "198.51.100.9", Port: 22, Method: "password"}
	d.processEvent(failed)
	if rec.count() != 0 {
		t.Fatalf("failed attempt must not alert, got %d messages", rec.count())
	}
	got, err := d.storage.GetFailedAttempts(time.Unix(0, 0))
	if err != nil || len(got) != 1 {
		t.Fatalf("failed attempt not stored: %v %d", err, len(got))
	}

	// A store that cannot write must not send an alert for an unrecorded event.
	_ = d.storage.Close()
	d.processEvent(login("alex", "203.0.113.7"))
	if rec.count() != 0 {
		t.Fatalf("alert sent although storing failed: %d messages", rec.count())
	}
}

func TestReloadQuietWithoutConfigPath(t *testing.T) {
	d, _ := newQuietDaemon(t, []quiet.Entry{{IP: "203.0.113.7"}})
	d.reloadQuiet("test")
	if d.quiet.Load().Len() != 1 {
		t.Fatal("reload without a config path changed the quiet list")
	}
}
