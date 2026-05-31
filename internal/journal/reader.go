package journal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strconv"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
)

type Reader struct {
	logger *slog.Logger
	events chan *parser.SSHEvent
	cmd    *exec.Cmd
}

type JournalEntry struct {
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Message           string `json:"MESSAGE"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
}

type TestResult struct {
	Identifier string
	Message    string
	Event      *parser.SSHEvent
	Status     string // "success", "failure", "skipped", "unrecognized"
}

func New(logger *slog.Logger) *Reader {
	return &Reader{
		logger: logger,
		events: make(chan *parser.SSHEvent, 100),
	}
}

func (r *Reader) Events() <-chan *parser.SSHEvent {
	return r.events
}

func (r *Reader) Start(ctx context.Context) error {
	r.cmd = exec.CommandContext(ctx, "journalctl", "-u", "ssh", "-f", "-o", "json", "--since", "now")
	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := r.cmd.Start(); err != nil {
		return err
	}

	go func() {
		defer close(r.events)

		scanner := bufio.NewScanner(stdout)
		// Allow long journal lines (default cap is 64KB); an oversized line
		// would otherwise abort the scan and stop monitoring.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if event := r.parseJournalLine(line); event != nil {
				select {
				case r.events <- event:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			r.logger.Error("journal reader error", "error", err)
		}
	}()

	return nil
}

func (r *Reader) parseJournalLine(line string) *parser.SSHEvent {
	var entry JournalEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		r.logger.Debug("failed to parse journal entry", "error", err)
		return nil
	}

	r.logger.Debug("journal entry", "identifier", entry.SyslogIdentifier, "message", entry.Message)

	if entry.SyslogIdentifier != "sshd" && entry.SyslogIdentifier != "sshd-session" && entry.SyslogIdentifier != "sshd-auth" {
		r.logger.Debug("skipping non-sshd entry", "identifier", entry.SyslogIdentifier)
		return nil
	}

	timestamp := r.parseTimestamp(entry.RealtimeTimestamp)
	event := parser.ParseMessage(entry.Message, timestamp)
	if event == nil {
		r.logger.Debug("message not parsed", "message", entry.Message)
	} else {
		r.logger.Debug("parsed event", "type", event.EventType, "user", event.Username, "ip", event.IP)
	}
	return event
}

func (r *Reader) parseTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Now()
	}

	usec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Now()
	}

	return time.Unix(usec/1000000, (usec%1000000)*1000)
}

func (r *Reader) Stop() error {
	if r.cmd != nil && r.cmd.Process != nil {
		return r.cmd.Process.Kill()
	}
	return nil
}

func ReadRecent(n int) ([]TestResult, error) {
	cmd := exec.Command("journalctl", "-u", "ssh", "-o", "json", "-n", strconv.Itoa(n), "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var results []TestResult
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			results = append(results, TestResult{
				Message: line[:min(len(line), 80)],
				Status:  "json_error",
			})
			continue
		}

		result := TestResult{
			Identifier: entry.SyslogIdentifier,
			Message:    entry.Message,
		}

		if entry.SyslogIdentifier != "sshd" && entry.SyslogIdentifier != "sshd-session" && entry.SyslogIdentifier != "sshd-auth" {
			result.Status = "skipped"
			results = append(results, result)
			continue
		}

		usec, _ := strconv.ParseInt(entry.RealtimeTimestamp, 10, 64)
		ts := time.Unix(usec/1000000, (usec%1000000)*1000)

		event := parser.ParseMessage(entry.Message, ts)
		if event != nil {
			result.Event = event
			result.Status = string(event.EventType)
		} else {
			result.Status = "unrecognized"
		}
		results = append(results, result)
	}

	return results, scanner.Err()
}
