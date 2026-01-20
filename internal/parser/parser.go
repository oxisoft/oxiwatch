package parser

import (
	"regexp"
	"strconv"
	"time"
)

type EventType string

const (
	EventSuccess EventType = "success"
	EventFailure EventType = "failure"
)

type SSHEvent struct {
	Timestamp   time.Time
	EventType   EventType
	Username    string
	IP          string
	Port        int
	Method      string
	InvalidUser bool
}

var (
	successPattern = regexp.MustCompile(
		`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Accepted\s+(password|publickey)\s+for\s+(\S+)\s+from\s+(\S+)\s+port\s+(\d+)`,
	)

	failedPattern = regexp.MustCompile(
		`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+sshd\[\d+\]:\s+Failed\s+(password|publickey)\s+for\s+(invalid user\s+)?(\S+)\s+from\s+(\S+)\s+port\s+(\d+)`,
	)

	messageSuccessPattern = regexp.MustCompile(
		`^Accepted\s+(password|publickey)\s+for\s+(\S+)\s+from\s+(\S+)\s+port\s+(\d+)`,
	)

	messageFailedPattern = regexp.MustCompile(
		`^Failed\s+(password|publickey)\s+for\s+(invalid user\s+)?(\S+)\s+from\s+(\S+)\s+port\s+(\d+)`,
	)
)

func ParseLine(line string, year int) *SSHEvent {
	if event := parseSuccess(line, year); event != nil {
		return event
	}
	return parseFailure(line, year)
}

func parseSuccess(line string, year int) *SSHEvent {
	matches := successPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	timestamp, err := parseTimestamp(matches[1], year)
	if err != nil {
		return nil
	}

	port, _ := strconv.Atoi(matches[5])

	return &SSHEvent{
		Timestamp: timestamp,
		EventType: EventSuccess,
		Method:    matches[2],
		Username:  matches[3],
		IP:        matches[4],
		Port:      port,
	}
}

func parseFailure(line string, year int) *SSHEvent {
	matches := failedPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	timestamp, err := parseTimestamp(matches[1], year)
	if err != nil {
		return nil
	}

	port, _ := strconv.Atoi(matches[6])

	return &SSHEvent{
		Timestamp:   timestamp,
		EventType:   EventFailure,
		Method:      matches[2],
		InvalidUser: matches[3] != "",
		Username:    matches[4],
		IP:          matches[5],
		Port:        port,
	}
}

func parseTimestamp(ts string, year int) (time.Time, error) {
	layout := "Jan 2 15:04:05"
	t, err := time.Parse(layout, ts)
	if err != nil {
		layout = "Jan  2 15:04:05"
		t, err = time.Parse(layout, ts)
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.Date(year, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local), nil
}

func ParseMessage(message string, timestamp time.Time) *SSHEvent {
	if event := parseMessageSuccess(message, timestamp); event != nil {
		return event
	}
	return parseMessageFailure(message, timestamp)
}

func parseMessageSuccess(message string, timestamp time.Time) *SSHEvent {
	matches := messageSuccessPattern.FindStringSubmatch(message)
	if matches == nil {
		return nil
	}

	port, _ := strconv.Atoi(matches[4])

	return &SSHEvent{
		Timestamp: timestamp,
		EventType: EventSuccess,
		Method:    matches[1],
		Username:  matches[2],
		IP:        matches[3],
		Port:      port,
	}
}

func parseMessageFailure(message string, timestamp time.Time) *SSHEvent {
	matches := messageFailedPattern.FindStringSubmatch(message)
	if matches == nil {
		return nil
	}

	port, _ := strconv.Atoi(matches[5])

	return &SSHEvent{
		Timestamp:   timestamp,
		EventType:   EventFailure,
		Method:      matches[1],
		InvalidUser: matches[2] != "",
		Username:    matches[3],
		IP:          matches[4],
		Port:        port,
	}
}
