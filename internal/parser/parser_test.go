package parser

import (
	"testing"
	"time"
)

func TestParseSuccessPassword(t *testing.T) {
	line := "Jan 20 14:32:15 host sshd[12345]: Accepted password for alice from 192.168.1.100 port 54321 ssh2"
	event := ParseLine(line, 2026)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventSuccess {
		t.Errorf("expected EventSuccess, got %s", event.EventType)
	}
	if event.Username != "alice" {
		t.Errorf("expected username alice, got %s", event.Username)
	}
	if event.IP != "192.168.1.100" {
		t.Errorf("expected IP 192.168.1.100, got %s", event.IP)
	}
	if event.Port != 54321 {
		t.Errorf("expected port 54321, got %d", event.Port)
	}
	if event.Method != "password" {
		t.Errorf("expected method password, got %s", event.Method)
	}
	if event.InvalidUser {
		t.Error("expected InvalidUser false")
	}

	expected := time.Date(2026, time.January, 20, 14, 32, 15, 0, time.Local)
	if !event.Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, event.Timestamp)
	}
}

func TestParseSuccessPublickey(t *testing.T) {
	line := "Jan 20 14:32:15 host sshd[12345]: Accepted publickey for bob from 10.0.0.50 port 22222 ssh2"
	event := ParseLine(line, 2026)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventSuccess {
		t.Errorf("expected EventSuccess, got %s", event.EventType)
	}
	if event.Username != "bob" {
		t.Errorf("expected username bob, got %s", event.Username)
	}
	if event.IP != "10.0.0.50" {
		t.Errorf("expected IP 10.0.0.50, got %s", event.IP)
	}
	if event.Method != "publickey" {
		t.Errorf("expected method publickey, got %s", event.Method)
	}
}

func TestParseFailedPassword(t *testing.T) {
	line := "Jan 20 14:33:00 host sshd[12346]: Failed password for root from 116.31.116.24 port 29160 ssh2"
	event := ParseLine(line, 2026)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventFailure {
		t.Errorf("expected EventFailure, got %s", event.EventType)
	}
	if event.Username != "root" {
		t.Errorf("expected username root, got %s", event.Username)
	}
	if event.IP != "116.31.116.24" {
		t.Errorf("expected IP 116.31.116.24, got %s", event.IP)
	}
	if event.Port != 29160 {
		t.Errorf("expected port 29160, got %d", event.Port)
	}
	if event.InvalidUser {
		t.Error("expected InvalidUser false")
	}
}

func TestParseFailedInvalidUser(t *testing.T) {
	line := "Jan 20 14:33:05 host sshd[12347]: Failed password for invalid user admin from 142.0.45.14 port 52772 ssh2"
	event := ParseLine(line, 2026)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventFailure {
		t.Errorf("expected EventFailure, got %s", event.EventType)
	}
	if event.Username != "admin" {
		t.Errorf("expected username admin, got %s", event.Username)
	}
	if event.IP != "142.0.45.14" {
		t.Errorf("expected IP 142.0.45.14, got %s", event.IP)
	}
	if !event.InvalidUser {
		t.Error("expected InvalidUser true")
	}
}

func TestParseNonSSHLine(t *testing.T) {
	lines := []string{
		"Jan 20 14:30:00 host systemd[1]: Started Session 1 of user root.",
		"Jan 20 14:30:00 host sshd[12345]: pam_unix(sshd:session): session opened",
		"random garbage",
		"",
	}

	for _, line := range lines {
		event := ParseLine(line, 2026)
		if event != nil {
			t.Errorf("expected nil for line %q, got %+v", line, event)
		}
	}
}

func TestParseSingleDigitDay(t *testing.T) {
	line := "Jan  5 09:12:00 host sshd[12345]: Accepted password for alice from 192.168.1.100 port 54321 ssh2"
	event := ParseLine(line, 2026)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	expected := time.Date(2026, time.January, 5, 9, 12, 0, 0, time.Local)
	if !event.Timestamp.Equal(expected) {
		t.Errorf("expected timestamp %v, got %v", expected, event.Timestamp)
	}
}

func TestParseMessageSuccess(t *testing.T) {
	ts := time.Date(2026, time.January, 20, 14, 32, 15, 0, time.UTC)
	message := "Accepted publickey for oxi from 10.6.0.2 port 49296 ssh2: ED25519 SHA256:xxx"
	event := ParseMessage(message, ts)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventSuccess {
		t.Errorf("expected EventSuccess, got %s", event.EventType)
	}
	if event.Username != "oxi" {
		t.Errorf("expected username oxi, got %s", event.Username)
	}
	if event.IP != "10.6.0.2" {
		t.Errorf("expected IP 10.6.0.2, got %s", event.IP)
	}
	if event.Port != 49296 {
		t.Errorf("expected port 49296, got %d", event.Port)
	}
	if event.Method != "publickey" {
		t.Errorf("expected method publickey, got %s", event.Method)
	}
	if !event.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, event.Timestamp)
	}
}

func TestParseMessagePasswordSuccess(t *testing.T) {
	ts := time.Date(2026, time.January, 20, 14, 32, 15, 0, time.UTC)
	message := "Accepted password for alice from 192.168.1.100 port 54321 ssh2"
	event := ParseMessage(message, ts)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventSuccess {
		t.Errorf("expected EventSuccess, got %s", event.EventType)
	}
	if event.Username != "alice" {
		t.Errorf("expected username alice, got %s", event.Username)
	}
	if event.Method != "password" {
		t.Errorf("expected method password, got %s", event.Method)
	}
}

func TestParseMessageFailure(t *testing.T) {
	ts := time.Date(2026, time.January, 20, 14, 33, 0, 0, time.UTC)
	message := "Failed password for root from 116.31.116.24 port 29160 ssh2"
	event := ParseMessage(message, ts)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventFailure {
		t.Errorf("expected EventFailure, got %s", event.EventType)
	}
	if event.Username != "root" {
		t.Errorf("expected username root, got %s", event.Username)
	}
	if event.IP != "116.31.116.24" {
		t.Errorf("expected IP 116.31.116.24, got %s", event.IP)
	}
	if event.InvalidUser {
		t.Error("expected InvalidUser false")
	}
}

func TestParseMessageFailureInvalidUser(t *testing.T) {
	ts := time.Date(2026, time.January, 20, 14, 33, 5, 0, time.UTC)
	message := "Failed password for invalid user admin from 142.0.45.14 port 52772 ssh2"
	event := ParseMessage(message, ts)

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.EventType != EventFailure {
		t.Errorf("expected EventFailure, got %s", event.EventType)
	}
	if event.Username != "admin" {
		t.Errorf("expected username admin, got %s", event.Username)
	}
	if !event.InvalidUser {
		t.Error("expected InvalidUser true")
	}
}

func TestParseMessageNonSSH(t *testing.T) {
	ts := time.Now()
	messages := []string{
		"pam_unix(sshd:session): session opened",
		"Connection closed by 10.0.0.1 port 22",
		"random garbage",
		"",
	}

	for _, msg := range messages {
		event := ParseMessage(msg, ts)
		if event != nil {
			t.Errorf("expected nil for message %q, got %+v", msg, event)
		}
	}
}
