package quiet

import (
	"strings"
	"testing"
	"time"
)

func mustCompile(t *testing.T, entries []Entry, loc *time.Location) *List {
	t.Helper()
	l, err := Compile(entries, loc)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	return l
}

func TestMatch(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	l := mustCompile(t, []Entry{
		{IP: "203.0.113.7", User: "alex", Label: "Alex home"},
		{IP: "198.51.100.0/24", Label: "Office"},
		{IP: "2001:db8::/32", User: "deploy", Label: "v6 net"},
		{IP: "192.0.2.1", Label: "Old hotel", Expires: "2026-10-25"},
		{IP: "192.0.2.2", Label: "Today", Expires: "2026-10-26"},
	}, time.UTC)

	tests := []struct {
		name, ip, user, want string
	}{
		{"exact ip and user", "203.0.113.7", "alex", "Alex home"},
		{"exact ip, other user alerts", "203.0.113.7", "root", ""},
		{"range any user", "198.51.100.200", "root", "Office"},
		{"outside range", "198.51.101.1", "root", ""},
		{"ipv4-mapped ipv6", "::ffff:198.51.100.9", "bob", "Office"},
		{"ipv6 range with user", "2001:db8:1::5", "deploy", "v6 net"},
		{"ipv6 range wrong user", "2001:db8:1::5", "root", ""},
		{"expired yesterday", "192.0.2.1", "x", ""},
		{"expires today still active", "192.0.2.2", "x", "Today"},
		{"garbage ip", "not-an-ip", "alex", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := l.Match(tt.ip, tt.user, now)
			if tt.want == "" {
				if ok {
					t.Fatalf("Match() = %q, want no match", e.DisplayLabel())
				}
				return
			}
			if !ok || e.DisplayLabel() != tt.want {
				t.Fatalf("Match() = %q, %v; want %q", e.DisplayLabel(), ok, tt.want)
			}
		})
	}
}

func TestExpiryUsesTimezone(t *testing.T) {
	warsaw, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Skip("tzdata not available")
	}
	l := mustCompile(t, []Entry{{IP: "192.0.2.9", Expires: "2026-10-26"}}, warsaw)
	// 23:30 UTC on the 26th is already the 27th in Warsaw (UTC+1).
	late := time.Date(2026, 10, 26, 23, 30, 0, 0, time.UTC)
	if _, ok := l.Match("192.0.2.9", "x", late); ok {
		t.Fatal("entry should have expired at Warsaw midnight")
	}
	if _, ok := l.Match("192.0.2.9", "x", late.Add(-time.Hour)); !ok {
		t.Fatal("entry should still be active at 23:30 Warsaw time")
	}
	if l.Active(late) != 0 || l.Len() != 1 {
		t.Fatalf("Active/Len = %d/%d, want 0/1", l.Active(late), l.Len())
	}
}

func TestNilListMatchesNothing(t *testing.T) {
	var l *List
	if _, ok := l.Match("1.2.3.4", "root", time.Now()); ok {
		t.Fatal("nil list matched")
	}
	if l.Len() != 0 || l.Active(time.Now()) != 0 {
		t.Fatal("nil list has entries")
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		want    string
	}{
		{"missing ip", []Entry{{Label: "x"}}, "ip is required"},
		{"bad ip", []Entry{{IP: "300.1.1.1"}}, "invalid IP address"},
		{"bad range", []Entry{{IP: "10.0.0.0/40"}}, "invalid IP range"},
		{"zone", []Entry{{IP: "fe80::1%eth0"}}, "invalid IP address"},
		{"bad date", []Entry{{IP: "10.0.0.1", Expires: "tomorrow"}}, "expires must be a date"},
		{"duplicate label", []Entry{{IP: "10.0.0.1", Label: "a"}, {IP: "10.0.0.2", Label: "a"}}, "already used"},
		{"duplicate default label", []Entry{{IP: "10.0.0.1"}, {IP: "10.0.0.1"}}, "already used"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(tt.entries, time.UTC)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Compile() error = %v, want containing %q", err, tt.want)
			}
		})
	}
	if _, err := Compile([]Entry{{IP: "10.0.0.1"}, {IP: "10.0.0.1", User: "alex"}}, nil); err != nil {
		t.Fatalf("same ip with different users should be allowed: %v", err)
	}
}

func TestCanonical(t *testing.T) {
	tests := map[string]string{
		"203.0.113.7":         "203.0.113.7",
		" 203.0.113.7 ":       "203.0.113.7",
		"203.0.113.7/32":      "203.0.113.7",
		"198.51.100.77/24":    "198.51.100.0/24",
		"::ffff:10.1.2.3":     "10.1.2.3",
		"::ffff:10.1.2.3/120": "10.1.2.0/24",
		"2001:DB8::1":         "2001:db8::1",
		"2001:db8::1/48":      "2001:db8::/48",
	}
	for in, want := range tests {
		got, err := Canonical(in)
		if err != nil || got != want {
			t.Errorf("Canonical(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestDisplayLabel(t *testing.T) {
	if got := (Entry{IP: "1.2.3.4", User: "alex"}).DisplayLabel(); got != "1.2.3.4 alex" {
		t.Errorf("got %q", got)
	}
	if got := (Entry{IP: "1.2.3.4"}).DisplayLabel(); got != "1.2.3.4" {
		t.Errorf("got %q", got)
	}
	if got := (Entry{IP: "1.2.3.4", Label: "Home"}).DisplayLabel(); got != "Home" {
		t.Errorf("got %q", got)
	}
}

func TestParseExpiry(t *testing.T) {
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	tests := map[string]string{
		"":           "",
		"30d":        "2026-11-25",
		"2w":         "2026-11-09",
		"2026-12-31": "2026-12-31",
		"1D":         "2026-10-27",
	}
	for in, want := range tests {
		got, err := ParseExpiry(in, now, time.UTC)
		if err != nil || got != want {
			t.Errorf("ParseExpiry(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"0d", "-3d", "3m", "d", "2026-13-01", "1.5d"} {
		if _, err := ParseExpiry(bad, now, time.UTC); err == nil {
			t.Errorf("ParseExpiry(%q) should fail", bad)
		}
	}
	if !Expired(Entry{Expires: "2026-10-25"}, now, time.UTC) || Expired(Entry{Expires: "2026-10-26"}, now, time.UTC) || Expired(Entry{}, now, nil) {
		t.Error("Expired() wrong")
	}
}
