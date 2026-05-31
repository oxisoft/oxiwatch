package daemon

import (
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/parser"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

// discardLogger returns a logger that drops all output, so tests stay quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// matrixCfg returns config fragments that fully configure the Matrix channel.
func matrixCfg(cfg *config.Config) {
	cfg.MatrixHomeserver = "https://matrix.example.org"
	cfg.MatrixRoomID = "!room:example.org"
	cfg.MatrixAccessToken = "secret-token"
}

// emailCfg returns config fragments that fully configure the Email channel.
func emailCfg(cfg *config.Config) {
	cfg.EmailSMTPURL = "smtps://smtp.example.org:465"
	cfg.EmailFrom = "alerts@example.org"
	cfg.EmailTo = []string{"admin@example.org"}
	cfg.EmailUsername = "alerts@example.org"
	cfg.EmailPassword = "hunter2"
}

func TestBuildNotifier(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*config.Config)
		wantErr   bool
		wantCount int
		wantNames []string
	}{
		{
			name:    "empty config yields error",
			setup:   func(*config.Config) {},
			wantErr: true,
		},
		{
			name:      "matrix only",
			setup:     matrixCfg,
			wantCount: 1,
			wantNames: []string{"matrix"},
		},
		{
			name:      "email only",
			setup:     emailCfg,
			wantCount: 1,
			wantNames: []string{"email"},
		},
		{
			name: "matrix and email",
			setup: func(cfg *config.Config) {
				matrixCfg(cfg)
				emailCfg(cfg)
			},
			wantCount: 2,
			wantNames: []string{"matrix", "email"},
		},
		{
			name: "matrix configured but explicitly disabled yields error",
			setup: func(cfg *config.Config) {
				matrixCfg(cfg)
				disabled := false
				cfg.MatrixEnabled = &disabled
			},
			wantErr: true,
		},
		{
			name: "email enabled flag set true",
			setup: func(cfg *config.Config) {
				emailCfg(cfg)
				enabled := true
				cfg.EmailEnabled = &enabled
			},
			wantCount: 1,
			wantNames: []string{"email"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{ServerName: "test-server"}
			tt.setup(cfg)

			mgr, err := BuildNotifier(cfg, discardLogger())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("BuildNotifier() expected error, got nil (count=%d)", mgr.Count())
				}
				if mgr != nil {
					t.Fatalf("BuildNotifier() expected nil manager on error, got %#v", mgr)
				}
				return
			}

			if err != nil {
				t.Fatalf("BuildNotifier() unexpected error: %v", err)
			}
			if mgr == nil {
				t.Fatalf("BuildNotifier() returned nil manager without error")
			}
			if got := mgr.Count(); got != tt.wantCount {
				t.Errorf("Count() = %d, want %d", got, tt.wantCount)
			}

			got := append([]string(nil), mgr.Names()...)
			want := append([]string(nil), tt.wantNames...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Names() = %v, want %v", mgr.Names(), tt.wantNames)
			}
		})
	}
}

// newTestStorage opens a fresh sqlite store under a temp dir.
func newTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New() error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// insertSuccess records a successful login for the given user/ip/country.
func insertSuccess(t *testing.T, store *storage.Storage, user, ip, country, city string) {
	t.Helper()
	event := &parser.SSHEvent{
		Timestamp: time.Now(),
		EventType: parser.EventSuccess,
		Username:  user,
		IP:        ip,
		Port:      22,
		Method:    "publickey",
	}
	if err := store.InsertEvent(event, country, city); err != nil {
		t.Fatalf("InsertEvent() error: %v", err)
	}
}

func TestCheckLocationChange(t *testing.T) {
	store := newTestStorage(t)
	insertSuccess(t, store, "bob", "1.1.1.1", "Germany", "Berlin")

	d := &Daemon{
		storage: store,
		logger:  discardLogger(),
	}

	tests := []struct {
		name        string
		event       *parser.SSHEvent
		country     string
		city        string
		wantWarning bool
	}{
		{
			name: "different ip and country yields warning",
			event: &parser.SSHEvent{
				EventType: parser.EventSuccess,
				Username:  "bob",
				IP:        "2.2.2.2",
			},
			country:     "France",
			city:        "Paris",
			wantWarning: true,
		},
		{
			name: "same ip yields no warning",
			event: &parser.SSHEvent{
				EventType: parser.EventSuccess,
				Username:  "bob",
				IP:        "1.1.1.1",
			},
			country:     "Germany",
			city:        "Berlin",
			wantWarning: false,
		},
		{
			name: "unknown user yields no warning",
			event: &parser.SSHEvent{
				EventType: parser.EventSuccess,
				Username:  "nobody",
				IP:        "9.9.9.9",
			},
			country:     "Spain",
			city:        "Madrid",
			wantWarning: false,
		},
		{
			name: "different ip but same resolved location yields no warning",
			event: &parser.SSHEvent{
				EventType: parser.EventSuccess,
				Username:  "bob",
				IP:        "3.3.3.3",
			},
			country:     "Germany",
			city:        "Berlin",
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.checkLocationChange(tt.event, tt.country, tt.city)
			if tt.wantWarning && got == "" {
				t.Errorf("checkLocationChange() = empty, want non-empty warning")
			}
			if !tt.wantWarning && got != "" {
				t.Errorf("checkLocationChange() = %q, want empty", got)
			}
		})
	}
}

// TestCheckLocationChangeIPFallback verifies the warning falls back to the
// previous IP when the prior login had no resolved location.
func TestCheckLocationChangeIPFallback(t *testing.T) {
	store := newTestStorage(t)
	insertSuccess(t, store, "carol", "5.5.5.5", "", "")

	d := &Daemon{storage: store, logger: discardLogger()}

	event := &parser.SSHEvent{
		EventType: parser.EventSuccess,
		Username:  "carol",
		IP:        "6.6.6.6",
	}
	got := d.checkLocationChange(event, "Italy", "Rome")
	if got == "" {
		t.Fatalf("checkLocationChange() = empty, want non-empty warning")
	}
	if want := "5.5.5.5"; !strings.Contains(got, want) {
		t.Errorf("checkLocationChange() = %q, want it to reference previous IP %q", got, want)
	}
}

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name    string
		country string
		city    string
		want    string
	}{
		{name: "city and country", country: "Germany", city: "Berlin", want: "Berlin, Germany"},
		{name: "country only", country: "Germany", city: "", want: "Germany"},
		{name: "city only", country: "", city: "Berlin", want: "Berlin"},
		{name: "neither", country: "", city: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLocation(tt.country, tt.city); got != tt.want {
				t.Errorf("formatLocation(%q, %q) = %q, want %q", tt.country, tt.city, got, tt.want)
			}
		})
	}
}
