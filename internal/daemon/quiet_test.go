package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/notifier"
	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/quiet"
)

type recordingChannel struct {
	mu   sync.Mutex
	msgs []notifier.Message
}

func (r *recordingChannel) Name() string { return "rec" }
func (r *recordingChannel) Send(m notifier.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)
	return nil
}
func (r *recordingChannel) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

func newQuietDaemon(t *testing.T, entries []quiet.Entry) (*Daemon, *recordingChannel) {
	t.Helper()
	rec := &recordingChannel{}
	mgr := notifier.NewManager("test", time.UTC, discardLogger())
	mgr.Add(rec)
	d := &Daemon{storage: newTestStorage(t), logger: discardLogger(), notifier: mgr}
	ql, err := quiet.Compile(entries, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	d.quiet.Store(ql)
	return d, rec
}

func login(user, ip string) *parser.SSHEvent {
	return &parser.SSHEvent{Timestamp: time.Now(), EventType: parser.EventSuccess, Username: user, IP: ip, Port: 22, Method: "publickey"}
}

func TestProcessEventQuietSuppressesAlertButStores(t *testing.T) {
	d, rec := newQuietDaemon(t, []quiet.Entry{{IP: "203.0.113.0/24", User: "alex", Label: "Alex home"}})

	d.processEvent(login("alex", "203.0.113.7"))
	if rec.count() != 0 {
		t.Fatalf("quiet login sent %d alerts, want 0", rec.count())
	}
	d.processEvent(login("root", "203.0.113.7"))
	if rec.count() != 1 {
		t.Fatalf("non-matching user sent %d alerts, want 1", rec.count())
	}
	d.processEvent(login("alex", "192.0.2.1"))
	if rec.count() != 2 {
		t.Fatalf("non-matching ip sent %d alerts, want 2", rec.count())
	}
	// Failed attempts from a quiet address are stored without a label.
	d.processEvent(&parser.SSHEvent{Timestamp: time.Now(), EventType: parser.EventFailure, Username: "alex", IP: "203.0.113.7", Method: "password"})

	logins, err := d.storage.GetSuccessfulLogins(time.Now().Add(-time.Hour))
	if err != nil || len(logins) != 3 {
		t.Fatalf("stored logins = %d, %v; want 3", len(logins), err)
	}
	labels := map[string]string{}
	for _, l := range logins {
		labels[l.Username+"@"+l.IP] = l.QuietLabel
	}
	if labels["alex@203.0.113.7"] != "Alex home" || labels["root@203.0.113.7"] != "" || labels["alex@192.0.2.1"] != "" {
		t.Fatalf("labels = %v", labels)
	}
	failed, _ := d.storage.GetFailedAttempts(time.Now().Add(-time.Hour))
	if len(failed) != 1 || failed[0].QuietLabel != "" {
		t.Fatalf("failed attempts = %+v", failed)
	}
	if n, _ := d.storage.GetQuietSuccessCount(time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("quiet count = %d, want 1", n)
	}
}

func TestReloadQuietFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	base := `{"telegram_bot_token": "t", "telegram_chat_id": "c", "database_path": "` + filepath.Join(dir, "db") + `"}`
	if err := os.WriteFile(p, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}

	d, rec := newQuietDaemon(t, nil)
	d.WatchConfig(p)

	if err := config.SaveQuietIPs(p, []quiet.Entry{{IP: "10.1.1.1", Label: "Lab"}}); err != nil {
		t.Fatal(err)
	}
	d.reloadQuiet("test")
	d.processEvent(login("bob", "10.1.1.1"))
	if rec.count() != 0 {
		t.Fatal("reloaded quiet entry did not suppress the alert")
	}

	// A broken file keeps the previous list.
	if err := os.WriteFile(p, []byte(`{"quiet_ips": [{"ip": "nope"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d.reloadQuiet("test")
	if d.quiet.Load().Len() != 1 {
		t.Fatal("broken file replaced the working quiet list")
	}
	d.processEvent(login("bob", "10.1.1.1"))
	if rec.count() != 0 {
		t.Fatal("quiet list lost after a broken reload")
	}
}

func TestWatchConfigPicksUpChanges(t *testing.T) {
	old := QuietPollInterval
	QuietPollInterval = 20 * time.Millisecond
	defer func() { QuietPollInterval = old }()

	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d, _ := newQuietDaemon(t, nil)
	d.WatchConfig(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.watchConfig(ctx)

	if err := os.WriteFile(p, []byte(`{"quiet_ips": [{"ip": "10.9.9.9", "label": "watched"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e, ok := d.quiet.Load().Match("10.9.9.9", "x", time.Now()); ok && strings.Contains(e.Label, "watched") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watcher did not reload the changed config file")
}
