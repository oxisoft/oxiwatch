package journal

import (
	"bufio"
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

type journalEntry struct {
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Message           string `json:"MESSAGE"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
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
	var entry journalEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		r.logger.Debug("failed to parse journal entry", "error", err)
		return nil
	}

	r.logger.Debug("journal entry", "identifier", entry.SyslogIdentifier, "message", entry.Message)

	if entry.SyslogIdentifier != "sshd" && entry.SyslogIdentifier != "sshd-session" {
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
