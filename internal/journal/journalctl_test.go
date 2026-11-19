package journal

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

// Journal fixture lines in `journalctl -o json` shape: real sshd messages
// (password/publickey, invalid user, IPv6), plus lines the reader must skip.
var fixtureLines = []string{
	`{"__REALTIME_TIMESTAMP":"1700000000000000","SYSLOG_IDENTIFIER":"sshd","MESSAGE":"Accepted publickey for alex from 203.0.113.7 port 49296 ssh2: ED25519 SHA256:xxx"}`,
	`{"__REALTIME_TIMESTAMP":"1700000001000000","SYSLOG_IDENTIFIER":"sshd-session","MESSAGE":"Failed password for invalid user admin from 142.0.45.14 port 52772 ssh2"}`,
	`{"__REALTIME_TIMESTAMP":"1700000002000000","SYSLOG_IDENTIFIER":"systemd","MESSAGE":"Started ssh.service"}`,
	`not json at all, a broken line that must not stop the reader`,
	`{"__REALTIME_TIMESTAMP":"1700000003000000","SYSLOG_IDENTIFIER":"sshd","MESSAGE":"Server listening on 0.0.0.0 port 22."}`,
	``,
	`{"__REALTIME_TIMESTAMP":"1700000004000000","SYSLOG_IDENTIFIER":"sshd-auth","MESSAGE":"Accepted password for carol from 2001:db8::dead:beef port 22 ssh2"}`,
}

// fakeJournalctl puts a `journalctl` script first in PATH that prints the
// given output (and records its arguments) or exits with the given code.
func fakeJournalctl(t *testing.T, output string, exitCode int) (argsFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake needs a POSIX shell")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(data, []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
	argsFile = filepath.Join(dir, "args.txt")
	script := "#!/bin/sh\n" +
		"echo \"$@\" > '" + argsFile + "'\n" +
		"cat '" + data + "'\n" +
		"exit " + string(rune('0'+exitCode)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fake journalctl was not run: %v", err)
	}
	return strings.TrimSpace(string(b))
}

func TestStartStreamsParsedEvents(t *testing.T) {
	argsFile := fakeJournalctl(t, strings.Join(fixtureLines, "\n")+"\n", 0)
	r := newTestReader()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	var got []*parser.SSHEvent
	for ev := range r.Events() {
		got = append(got, ev)
	}
	if ctx.Err() != nil {
		t.Fatal("events channel was not closed after journalctl exited")
	}
	_ = r.Stop()

	if len(got) != 3 {
		t.Fatalf("want 3 events (non-sshd, malformed and unparsed lines skipped), got %d", len(got))
	}
	if got[0].EventType != parser.EventSuccess || got[0].Username != "alex" || got[0].IP != "203.0.113.7" {
		t.Errorf("first event: %+v", got[0])
	}
	if !got[0].Timestamp.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("first event timestamp from journal: got %v", got[0].Timestamp)
	}
	if got[1].EventType != parser.EventFailure || !got[1].InvalidUser || got[1].Username != "admin" {
		t.Errorf("failed invalid-user event from sshd-session: %+v", got[1])
	}
	if got[2].EventType != parser.EventSuccess || got[2].Username != "carol" || got[2].IP != "2001:db8::dead:beef" {
		t.Errorf("IPv6 event from sshd-auth: %+v", got[2])
	}

	args := readArgs(t, argsFile)
	for _, want := range []string{"-u ssh", "-f", "-o json", "--since now"} {
		if !strings.Contains(args, want) {
			t.Errorf("journalctl args %q miss %q", args, want)
		}
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	// Many lines so the producer blocks on the channel; cancelling must free it.
	line := fixtureLines[0] + "\n"
	fakeJournalctl(t, strings.Repeat(line, 500), 0)
	r := newTestReader()

	ctx, cancel := context.WithCancel(context.Background())
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	<-r.Events() // reader is running
	cancel()

	done := make(chan struct{})
	go func() {
		for range r.Events() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("events channel not closed after context cancel")
	}
	_ = r.Stop()
}

func TestStartFailsWithoutJournalctl(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := newTestReader()
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("Start() without journalctl in PATH: want error")
	}
}

func TestStopBeforeStart(t *testing.T) {
	if err := newTestReader().Stop(); err != nil {
		t.Fatalf("Stop() before Start(): %v", err)
	}
}

func TestReadRecent(t *testing.T) {
	argsFile := fakeJournalctl(t, strings.Join(fixtureLines, "\n")+"\n", 0)

	results, err := ReadRecent(25)
	if err != nil {
		t.Fatalf("ReadRecent() error: %v", err)
	}
	if args := readArgs(t, argsFile); !strings.Contains(args, "-n 25") || !strings.Contains(args, "--no-pager") {
		t.Errorf("journalctl args %q", args)
	}

	wantStatus := []string{"success", "failure", "skipped", "json_error", "unrecognized", "success"}
	if len(results) != len(wantStatus) {
		t.Fatalf("want %d results (empty line dropped), got %d: %+v", len(wantStatus), len(results), results)
	}
	for i, want := range wantStatus {
		if results[i].Status != want {
			t.Errorf("result %d (%q): status %q, want %q", i, results[i].Message, results[i].Status, want)
		}
	}
	if ev := results[1].Event; ev == nil || !ev.InvalidUser || ev.Username != "admin" {
		t.Errorf("invalid user failure: %+v", ev)
	}
	if results[2].Identifier != "systemd" {
		t.Errorf("skipped identifier: %q", results[2].Identifier)
	}
	if len(results[3].Message) > 80 {
		t.Errorf("json_error message not truncated to 80 chars: %d", len(results[3].Message))
	}
	if results[4].Event != nil {
		t.Errorf("unrecognized line carries an event: %+v", results[4].Event)
	}
}

func TestReadRecentJournalctlFails(t *testing.T) {
	fakeJournalctl(t, "", 1)
	if _, err := ReadRecent(5); err == nil {
		t.Fatal("ReadRecent() with failing journalctl: want error")
	}
}
