package journal

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

func newTestReader() *Reader {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestParseTimestamp(t *testing.T) {
	r := newTestReader()

	t.Run("empty returns approximately now", func(t *testing.T) {
		before := time.Now()
		got := r.parseTimestamp("")
		after := time.Now()
		if got.Before(before.Add(-2*time.Second)) || got.After(after.Add(2*time.Second)) {
			t.Errorf("expected timestamp near now, got %v (now ~%v)", got, before)
		}
	})

	t.Run("valid microsecond string returns exact time", func(t *testing.T) {
		// 1_700_000_123_456_789 microseconds.
		const usec int64 = 1700000123456789
		want := time.Unix(usec/1000000, (usec%1000000)*1000)
		got := r.parseTimestamp("1700000123456789")
		if !got.Equal(want) {
			t.Errorf("expected %v, got %v", want, got)
		}
		// Sanity-check the derived seconds/nanoseconds.
		if got.Unix() != 1700000123 {
			t.Errorf("expected unix seconds 1700000123, got %d", got.Unix())
		}
		if got.Nanosecond() != 456789*1000 {
			t.Errorf("expected nanoseconds %d, got %d", 456789*1000, got.Nanosecond())
		}
	})

	t.Run("zero microseconds maps to unix epoch", func(t *testing.T) {
		got := r.parseTimestamp("0")
		want := time.Unix(0, 0)
		if !got.Equal(want) {
			t.Errorf("expected %v, got %v", want, got)
		}
	})

	t.Run("invalid string returns approximately now", func(t *testing.T) {
		before := time.Now()
		got := r.parseTimestamp("not-a-number")
		after := time.Now()
		if got.Before(before.Add(-2*time.Second)) || got.After(after.Add(2*time.Second)) {
			t.Errorf("expected timestamp near now for invalid input, got %v", got)
		}
	})

	t.Run("partially numeric string returns approximately now", func(t *testing.T) {
		before := time.Now()
		got := r.parseTimestamp("123abc")
		after := time.Now()
		if got.Before(before.Add(-2*time.Second)) || got.After(after.Add(2*time.Second)) {
			t.Errorf("expected timestamp near now for malformed input, got %v", got)
		}
	})
}

// makeEntry serializes a JournalEntry to a JSON line, the same shape
// parseJournalLine consumes.
func makeEntry(t *testing.T, ident, msg, ts string) string {
	t.Helper()
	b, err := json.Marshal(JournalEntry{
		RealtimeTimestamp: ts,
		Message:           msg,
		SyslogIdentifier:  ident,
	})
	if err != nil {
		t.Fatalf("failed to marshal entry: %v", err)
	}
	return string(b)
}

func TestParseJournalLine(t *testing.T) {
	const ts = "1700000123456789"
	const acceptedMsg = "Accepted password for u from 1.2.3.4 port 22 ssh2"

	tests := []struct {
		name      string
		ident     string
		message   string
		ts        string
		wantNil   bool
		wantType  parser.EventType
		wantUser  string
		wantIP    string
		wantExact bool // verify timestamp equals parsed ts
	}{
		{
			name:      "sshd accepted password yields success event",
			ident:     "sshd",
			message:   acceptedMsg,
			ts:        ts,
			wantNil:   false,
			wantType:  parser.EventSuccess,
			wantUser:  "u",
			wantIP:    "1.2.3.4",
			wantExact: true,
		},
		{
			name:     "sshd-session identifier accepted",
			ident:    "sshd-session",
			message:  acceptedMsg,
			ts:       ts,
			wantNil:  false,
			wantType: parser.EventSuccess,
			wantUser: "u",
			wantIP:   "1.2.3.4",
		},
		{
			name:     "sshd-auth identifier accepted",
			ident:    "sshd-auth",
			message:  acceptedMsg,
			ts:       ts,
			wantNil:  false,
			wantType: parser.EventSuccess,
			wantUser: "u",
			wantIP:   "1.2.3.4",
		},
		{
			name:    "non-sshd identifier skipped",
			ident:   "systemd",
			message: acceptedMsg,
			ts:      ts,
			wantNil: true,
		},
		{
			name:    "empty identifier skipped",
			ident:   "",
			message: acceptedMsg,
			ts:      ts,
			wantNil: true,
		},
		{
			name:    "kernel identifier skipped",
			ident:   "kernel",
			message: acceptedMsg,
			ts:      ts,
			wantNil: true,
		},
		{
			name:    "sshd with unrecognized message yields nil",
			ident:   "sshd",
			message: "Server listening on 0.0.0.0 port 22.",
			ts:      ts,
			wantNil: true,
		},
	}

	r := newTestReader()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := makeEntry(t, tc.ident, tc.message, tc.ts)
			got := r.parseJournalLine(line)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil event, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil event, got nil")
			}
			if got.EventType != tc.wantType {
				t.Errorf("expected event type %q, got %q", tc.wantType, got.EventType)
			}
			if got.Username != tc.wantUser {
				t.Errorf("expected username %q, got %q", tc.wantUser, got.Username)
			}
			if got.IP != tc.wantIP {
				t.Errorf("expected IP %q, got %q", tc.wantIP, got.IP)
			}
			if tc.wantExact {
				const usec int64 = 1700000123456789
				want := time.Unix(usec/1000000, (usec%1000000)*1000)
				if !got.Timestamp.Equal(want) {
					t.Errorf("expected timestamp %v, got %v", want, got.Timestamp)
				}
			}
		})
	}
}

func TestParseJournalLineMalformedJSON(t *testing.T) {
	r := newTestReader()
	cases := []string{
		"",
		"{not json",
		"this is not json at all",
		`{"MESSAGE": 12345}`, // wrong type for MESSAGE field
		"[]",
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			if got := r.parseJournalLine(line); got != nil {
				t.Errorf("expected nil for malformed JSON %q, got %+v", line, got)
			}
		})
	}
}
