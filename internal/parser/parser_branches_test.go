package parser

import (
	"testing"
	"time"
)

// TestParseLineTable exercises ParseLine over success/failure happy paths,
// both timestamp spacings, IPv6 sources, multi-digit ports and usernames
// containing dots and dashes.
func TestParseLineTable(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantNil     bool
		wantType    EventType
		wantMethod  string
		wantUser    string
		wantIP      string
		wantPort    int
		wantInvalid bool
		wantTime    time.Time
	}{
		{
			name:       "success password single-space day",
			line:       "Jan 2 03:04:05 host sshd[1]: Accepted password for alice from 192.168.0.1 port 22 ssh2",
			wantType:   EventSuccess,
			wantMethod: "password",
			wantUser:   "alice",
			wantIP:     "192.168.0.1",
			wantPort:   22,
			wantTime:   time.Date(2026, time.January, 2, 3, 4, 5, 0, time.Local),
		},
		{
			name:       "success publickey double-space day",
			line:       "Jan  2 03:04:05 host sshd[1]: Accepted publickey for bob from 10.0.0.5 port 65535 ssh2",
			wantType:   EventSuccess,
			wantMethod: "publickey",
			wantUser:   "bob",
			wantIP:     "10.0.0.5",
			wantPort:   65535,
			wantTime:   time.Date(2026, time.January, 2, 3, 4, 5, 0, time.Local),
		},
		{
			name:       "success IPv6 source",
			line:       "Mar 15 22:01:09 srv sshd[999]: Accepted publickey for carol from 2001:db8::1 port 49296 ssh2",
			wantType:   EventSuccess,
			wantMethod: "publickey",
			wantUser:   "carol",
			wantIP:     "2001:db8::1",
			wantPort:   49296,
			wantTime:   time.Date(2026, time.March, 15, 22, 1, 9, 0, time.Local),
		},
		{
			name:       "success username with dots and dashes",
			line:       "Feb 28 10:10:10 host sshd[42]: Accepted password for first.last-name from 172.16.0.9 port 1234 ssh2",
			wantType:   EventSuccess,
			wantMethod: "password",
			wantUser:   "first.last-name",
			wantIP:     "172.16.0.9",
			wantPort:   1234,
			wantTime:   time.Date(2026, time.February, 28, 10, 10, 10, 0, time.Local),
		},
		{
			name:        "failure valid user",
			line:        "Jan 20 14:33:00 host sshd[12346]: Failed password for root from 116.31.116.24 port 29160 ssh2",
			wantType:    EventFailure,
			wantMethod:  "password",
			wantUser:    "root",
			wantIP:      "116.31.116.24",
			wantPort:    29160,
			wantInvalid: false,
			wantTime:    time.Date(2026, time.January, 20, 14, 33, 0, 0, time.Local),
		},
		{
			name:        "failure invalid user publickey",
			line:        "Jan  3 00:00:01 host sshd[7]: Failed publickey for invalid user admin from 142.0.45.14 port 52772 ssh2",
			wantType:    EventFailure,
			wantMethod:  "publickey",
			wantUser:    "admin",
			wantIP:      "142.0.45.14",
			wantPort:    52772,
			wantInvalid: true,
			wantTime:    time.Date(2026, time.January, 3, 0, 0, 1, 0, time.Local),
		},
		{
			name:        "failure IPv6 multi-digit port",
			line:        "Apr 9 18:45:30 host sshd[55]: Failed password for invalid user test-user from fe80::abcd port 60123 ssh2",
			wantType:    EventFailure,
			wantMethod:  "password",
			wantUser:    "test-user",
			wantIP:      "fe80::abcd",
			wantPort:    60123,
			wantInvalid: true,
			wantTime:    time.Date(2026, time.April, 9, 18, 45, 30, 0, time.Local),
		},
		{
			name:    "garbage line",
			line:    "this is not an ssh log line",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "session line ignored",
			line:    "Jan 20 14:30:00 host sshd[12345]: pam_unix(sshd:session): session opened",
			wantNil: true,
		},
		{
			name:    "accepted but malformed (no port)",
			line:    "Jan 20 14:32:15 host sshd[1]: Accepted password for alice from 192.168.0.1",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ParseLine(tt.line, 2026)
			if tt.wantNil {
				if event != nil {
					t.Fatalf("expected nil, got %+v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.EventType != tt.wantType {
				t.Errorf("EventType: want %s got %s", tt.wantType, event.EventType)
			}
			if event.Method != tt.wantMethod {
				t.Errorf("Method: want %s got %s", tt.wantMethod, event.Method)
			}
			if event.Username != tt.wantUser {
				t.Errorf("Username: want %s got %s", tt.wantUser, event.Username)
			}
			if event.IP != tt.wantIP {
				t.Errorf("IP: want %s got %s", tt.wantIP, event.IP)
			}
			if event.Port != tt.wantPort {
				t.Errorf("Port: want %d got %d", tt.wantPort, event.Port)
			}
			if event.InvalidUser != tt.wantInvalid {
				t.Errorf("InvalidUser: want %v got %v", tt.wantInvalid, event.InvalidUser)
			}
			if !event.Timestamp.Equal(tt.wantTime) {
				t.Errorf("Timestamp: want %v got %v", tt.wantTime, event.Timestamp)
			}
		})
	}
}

// TestParseLineInvalidTimestamp ensures a line that matches the SSH structure
// but carries an unparseable month aborts to nil via the parseTimestamp error path.
func TestParseLineInvalidTimestamp(t *testing.T) {
	// "Foo" is not a valid month abbreviation; \w{3} still matches, forcing
	// parseTimestamp to fail on both layouts.
	line := "Foo 20 14:32:15 host sshd[1]: Accepted password for alice from 192.168.0.1 port 22 ssh2"
	if event := ParseLine(line, 2026); event != nil {
		t.Fatalf("expected nil for invalid month, got %+v", event)
	}

	failLine := "Foo 20 14:33:00 host sshd[1]: Failed password for root from 1.2.3.4 port 22 ssh2"
	if event := ParseLine(failLine, 2026); event != nil {
		t.Fatalf("expected nil for invalid month (failure), got %+v", event)
	}
}

// TestParseMessageTable exercises ParseMessage across every branch:
// success (password/publickey), failed (valid + invalid user),
// Invalid user pattern, and all three auth-disconnect verbs.
func TestParseMessageTable(t *testing.T) {
	ts := time.Date(2026, time.January, 20, 14, 32, 15, 0, time.UTC)

	tests := []struct {
		name        string
		message     string
		wantNil     bool
		wantType    EventType
		wantMethod  string
		wantUser    string
		wantIP      string
		wantPort    int
		wantInvalid bool
	}{
		{
			name:       "success publickey with key suffix",
			message:    "Accepted publickey for oxi from 10.6.0.2 port 49296 ssh2: ED25519 SHA256:xxx",
			wantType:   EventSuccess,
			wantMethod: "publickey",
			wantUser:   "oxi",
			wantIP:     "10.6.0.2",
			wantPort:   49296,
		},
		{
			name:       "success password",
			message:    "Accepted password for alice from 192.168.1.100 port 54321 ssh2",
			wantType:   EventSuccess,
			wantMethod: "password",
			wantUser:   "alice",
			wantIP:     "192.168.1.100",
			wantPort:   54321,
		},
		{
			name:       "success IPv6",
			message:    "Accepted publickey for carol from 2001:db8::dead:beef port 22 ssh2",
			wantType:   EventSuccess,
			wantMethod: "publickey",
			wantUser:   "carol",
			wantIP:     "2001:db8::dead:beef",
			wantPort:   22,
		},
		{
			name:        "failed valid user",
			message:     "Failed password for root from 116.31.116.24 port 29160 ssh2",
			wantType:    EventFailure,
			wantMethod:  "password",
			wantUser:    "root",
			wantIP:      "116.31.116.24",
			wantPort:    29160,
			wantInvalid: false,
		},
		{
			name:        "failed invalid user",
			message:     "Failed password for invalid user admin from 142.0.45.14 port 52772 ssh2",
			wantType:    EventFailure,
			wantMethod:  "password",
			wantUser:    "admin",
			wantIP:      "142.0.45.14",
			wantPort:    52772,
			wantInvalid: true,
		},
		{
			name:        "failed invalid user with dots/dashes",
			message:     "Failed publickey for invalid user a.b-c from fe80::1 port 12345 ssh2",
			wantType:    EventFailure,
			wantMethod:  "publickey",
			wantUser:    "a.b-c",
			wantIP:      "fe80::1",
			wantPort:    12345,
			wantInvalid: true,
		},
		{
			name:        "invalid user pattern",
			message:     "Invalid user mallory from 203.0.113.7 port 40000",
			wantType:    EventFailure,
			wantUser:    "mallory",
			wantIP:      "203.0.113.7",
			wantPort:    40000,
			wantInvalid: true,
		},
		{
			name:        "invalid user pattern IPv6",
			message:     "Invalid user oracle from 2001:db8::99 port 7",
			wantType:    EventFailure,
			wantUser:    "oracle",
			wantIP:      "2001:db8::99",
			wantPort:    7,
			wantInvalid: true,
		},
		{
			name:        "disconnected from authenticating user",
			message:     "Disconnected from authenticating user root 218.92.0.1 port 11122 [preauth]",
			wantType:    EventFailure,
			wantUser:    "root",
			wantIP:      "218.92.0.1",
			wantPort:    11122,
			wantInvalid: false,
		},
		{
			name:        "connection closed by authenticating user",
			message:     "Connection closed by authenticating user admin 45.55.1.2 port 33344 [preauth]",
			wantType:    EventFailure,
			wantUser:    "admin",
			wantIP:      "45.55.1.2",
			wantPort:    33344,
			wantInvalid: false,
		},
		{
			name:        "connection reset by authenticating user",
			message:     "Connection reset by authenticating user bob 2001:db8::abc port 50505 [preauth]",
			wantType:    EventFailure,
			wantUser:    "bob",
			wantIP:      "2001:db8::abc",
			wantPort:    50505,
			wantInvalid: false,
		},
		{
			name:    "non-auth disconnect (no authenticating user) -> nil",
			message: "Connection closed by 10.0.0.1 port 22",
			wantNil: true,
		},
		{
			name:    "session open ignored",
			message: "pam_unix(sshd:session): session opened",
			wantNil: true,
		},
		{
			name:    "garbage",
			message: "random garbage",
			wantNil: true,
		},
		{
			name:    "empty",
			message: "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ParseMessage(tt.message, ts)
			if tt.wantNil {
				if event != nil {
					t.Fatalf("expected nil, got %+v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.EventType != tt.wantType {
				t.Errorf("EventType: want %s got %s", tt.wantType, event.EventType)
			}
			if event.Method != tt.wantMethod {
				t.Errorf("Method: want %q got %q", tt.wantMethod, event.Method)
			}
			if event.Username != tt.wantUser {
				t.Errorf("Username: want %s got %s", tt.wantUser, event.Username)
			}
			if event.IP != tt.wantIP {
				t.Errorf("IP: want %s got %s", tt.wantIP, event.IP)
			}
			if event.Port != tt.wantPort {
				t.Errorf("Port: want %d got %d", tt.wantPort, event.Port)
			}
			if event.InvalidUser != tt.wantInvalid {
				t.Errorf("InvalidUser: want %v got %v", tt.wantInvalid, event.InvalidUser)
			}
			if !event.Timestamp.Equal(ts) {
				t.Errorf("Timestamp: want %v got %v", ts, event.Timestamp)
			}
		})
	}
}

// TestParseMessageTimestampPassthrough verifies ParseMessage uses the supplied
// timestamp verbatim regardless of branch taken.
func TestParseMessageTimestampPassthrough(t *testing.T) {
	ts := time.Date(2025, time.December, 31, 23, 59, 59, 0, time.Local)
	event := ParseMessage("Invalid user x from 1.1.1.1 port 2", ts)
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if !event.Timestamp.Equal(ts) {
		t.Errorf("Timestamp: want %v got %v", ts, event.Timestamp)
	}
}
