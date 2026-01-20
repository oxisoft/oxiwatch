package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/oxisoft/oxiwatch/internal/parser"
	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

type SSHEventRecord struct {
	ID          int64
	Timestamp   time.Time
	EventType   string
	Username    string
	IP          string
	Port        int
	Method      string
	Country     string
	City        string
	InvalidUser bool
	CreatedAt   time.Time
}

type Stats struct {
	TotalAttempts   int
	UniqueIPs       int
	UniqueUsernames int
}

type UsernameCount struct {
	Username string
	Count    int
}

type IPCount struct {
	IP      string
	Country string
	City    string
	Count   int
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return s, nil
}

func (s *Storage) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS ssh_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		event_type TEXT NOT NULL,
		username TEXT NOT NULL,
		ip TEXT NOT NULL,
		port INTEGER,
		method TEXT NOT NULL,
		country TEXT,
		city TEXT,
		invalid_user BOOLEAN DEFAULT FALSE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_timestamp ON ssh_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_event_type ON ssh_events(event_type);
	CREATE INDEX IF NOT EXISTS idx_ip ON ssh_events(ip);
	CREATE INDEX IF NOT EXISTS idx_username ON ssh_events(username);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *Storage) InsertEvent(event *parser.SSHEvent, country, city string) error {
	query := `
		INSERT INTO ssh_events (timestamp, event_type, username, ip, port, method, country, city, invalid_user)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		event.Timestamp,
		string(event.EventType),
		event.Username,
		event.IP,
		event.Port,
		event.Method,
		nullString(country),
		nullString(city),
		event.InvalidUser,
	)
	return err
}

func (s *Storage) GetSuccessfulLogins(since time.Time) ([]SSHEventRecord, error) {
	return s.getEvents("success", since)
}

func (s *Storage) GetLastLoginForUser(username string) (*SSHEventRecord, error) {
	query := `
		SELECT id, timestamp, event_type, username, ip, port, method,
		       COALESCE(country, ''), COALESCE(city, ''), invalid_user, created_at
		FROM ssh_events
		WHERE event_type = 'success' AND username = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`

	var e SSHEventRecord
	err := s.db.QueryRow(query, username).Scan(
		&e.ID, &e.Timestamp, &e.EventType, &e.Username, &e.IP,
		&e.Port, &e.Method, &e.Country, &e.City, &e.InvalidUser, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Storage) GetFailedAttempts(since time.Time) ([]SSHEventRecord, error) {
	return s.getEvents("failure", since)
}

func (s *Storage) getEvents(eventType string, since time.Time) ([]SSHEventRecord, error) {
	query := `
		SELECT id, timestamp, event_type, username, ip, port, method,
		       COALESCE(country, ''), COALESCE(city, ''), invalid_user, created_at
		FROM ssh_events
		WHERE event_type = ? AND timestamp >= ?
		ORDER BY timestamp DESC
	`

	rows, err := s.db.Query(query, eventType, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SSHEventRecord
	for rows.Next() {
		var e SSHEventRecord
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.EventType, &e.Username, &e.IP,
			&e.Port, &e.Method, &e.Country, &e.City, &e.InvalidUser, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Storage) GetFailedStats(since time.Time) (*Stats, error) {
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(DISTINCT ip) as unique_ips,
			COUNT(DISTINCT username) as unique_usernames
		FROM ssh_events
		WHERE event_type = 'failure' AND timestamp >= ?
	`

	var stats Stats
	err := s.db.QueryRow(query, since).Scan(&stats.TotalAttempts, &stats.UniqueIPs, &stats.UniqueUsernames)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (s *Storage) GetTopUsernames(since time.Time, limit int) ([]UsernameCount, error) {
	query := `
		SELECT username, COUNT(*) as count
		FROM ssh_events
		WHERE event_type = 'failure' AND timestamp >= ?
		GROUP BY username
		ORDER BY count DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UsernameCount
	for rows.Next() {
		var uc UsernameCount
		if err := rows.Scan(&uc.Username, &uc.Count); err != nil {
			return nil, err
		}
		results = append(results, uc)
	}
	return results, rows.Err()
}

func (s *Storage) GetTopIPs(since time.Time, limit int) ([]IPCount, error) {
	query := `
		SELECT ip, COALESCE(country, ''), COALESCE(city, ''), COUNT(*) as count
		FROM ssh_events
		WHERE event_type = 'failure' AND timestamp >= ?
		GROUP BY ip
		ORDER BY count DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []IPCount
	for rows.Next() {
		var ic IPCount
		if err := rows.Scan(&ic.IP, &ic.Country, &ic.City, &ic.Count); err != nil {
			return nil, err
		}
		results = append(results, ic)
	}
	return results, rows.Err()
}

func (s *Storage) GetSuccessCount(since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM ssh_events
		WHERE event_type = 'success' AND timestamp >= ?
	`, since).Scan(&count)
	return count, err
}

type OverallStats struct {
	SuccessCount    int
	FailedCount     int
	UniqueIPs       int
	UniqueUsernames int
}

func (s *Storage) GetOverallStats(since time.Time) (*OverallStats, error) {
	query := `
		SELECT
			COUNT(CASE WHEN event_type = 'success' THEN 1 END) as success,
			COUNT(CASE WHEN event_type = 'failure' THEN 1 END) as failed,
			COUNT(DISTINCT ip) as unique_ips,
			COUNT(DISTINCT username) as unique_usernames
		FROM ssh_events
		WHERE timestamp >= ?
	`

	var stats OverallStats
	err := s.db.QueryRow(query, since).Scan(&stats.SuccessCount, &stats.FailedCount, &stats.UniqueIPs, &stats.UniqueUsernames)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (s *Storage) Cleanup(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.db.Exec(`DELETE FROM ssh_events WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
