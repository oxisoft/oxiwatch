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
	// QuietLabel is the quiet list entry that matched this login (no alert
	// was sent); empty for normal events.
	QuietLabel string
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

	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	return s.addColumnIfMissing("ssh_events", "quiet_label", "TEXT")
}

func (s *Storage) addColumnIfMissing(table, column, decl string) error {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl))
	return err
}

func (s *Storage) InsertEvent(event *parser.SSHEvent, country, city string) error {
	return s.InsertEventQuiet(event, country, city, "")
}

// InsertEventQuiet stores an event together with the label of the quiet list
// entry that suppressed its alert (empty when none did).
func (s *Storage) InsertEventQuiet(event *parser.SSHEvent, country, city, quietLabel string) error {
	query := `
		INSERT INTO ssh_events (timestamp, event_type, username, ip, port, method, country, city, invalid_user, quiet_label)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		nullString(quietLabel),
	)
	return err
}

func (s *Storage) GetSuccessfulLogins(since time.Time) ([]SSHEventRecord, error) {
	return s.getEvents("success", since)
}

func (s *Storage) GetLastLoginForUser(username string) (*SSHEventRecord, error) {
	query := `
		SELECT id, timestamp, event_type, username, ip, port, method,
		       COALESCE(country, ''), COALESCE(city, ''), invalid_user, created_at,
		       COALESCE(quiet_label, '')
		FROM ssh_events
		WHERE event_type = 'success' AND username = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`

	var e SSHEventRecord
	err := s.db.QueryRow(query, username).Scan(
		&e.ID, &e.Timestamp, &e.EventType, &e.Username, &e.IP,
		&e.Port, &e.Method, &e.Country, &e.City, &e.InvalidUser, &e.CreatedAt, &e.QuietLabel,
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
		       COALESCE(country, ''), COALESCE(city, ''), invalid_user, created_at,
		       COALESCE(quiet_label, '')
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
			&e.Port, &e.Method, &e.Country, &e.City, &e.InvalidUser, &e.CreatedAt, &e.QuietLabel); err != nil {
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

// GetQuietSuccessCount counts successful logins since since whose alert was
// suppressed by the quiet list.
func (s *Storage) GetQuietSuccessCount(since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM ssh_events
		WHERE event_type = 'success' AND quiet_label IS NOT NULL AND timestamp >= ?
	`, since).Scan(&count)
	return count, err
}

// GetQuietLastMatches returns, per quiet list label, the time of the most
// recent login it matched (within the retention window).
func (s *Storage) GetQuietLastMatches() (map[string]time.Time, error) {
	rows, err := s.db.Query(`
		SELECT quiet_label, timestamp FROM ssh_events
		WHERE event_type = 'success' AND quiet_label IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]time.Time)
	for rows.Next() {
		var label string
		var ts time.Time
		if err := rows.Scan(&label, &ts); err != nil {
			return nil, err
		}
		if ts.After(out[label]) {
			out[label] = ts
		}
	}
	return out, rows.Err()
}

type OverallStats struct {
	QuietCount      int
	SuccessCount    int
	FailedCount     int
	UniqueIPs       int
	UniqueUsernames int
}

func (s *Storage) GetOverallStats(since time.Time) (*OverallStats, error) {
	query := `
		SELECT
			COUNT(CASE WHEN event_type = 'success' THEN 1 END) as success,
			COUNT(CASE WHEN event_type = 'success' AND quiet_label IS NOT NULL THEN 1 END) as quiet,
			COUNT(CASE WHEN event_type = 'failure' THEN 1 END) as failed,
			COUNT(DISTINCT ip) as unique_ips,
			COUNT(DISTINCT username) as unique_usernames
		FROM ssh_events
		WHERE timestamp >= ?
	`

	var stats OverallStats
	err := s.db.QueryRow(query, since).Scan(&stats.SuccessCount, &stats.QuietCount, &stats.FailedCount, &stats.UniqueIPs, &stats.UniqueUsernames)
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
