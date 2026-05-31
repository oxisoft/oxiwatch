package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultConfigPath   = "/etc/oxiwatch/config.json"
	DefaultDatabasePath = "/var/lib/oxiwatch/oxiwatch.db"
	DefaultGeoIPPath    = "/var/lib/oxiwatch/dbip-city-lite.mmdb"
)

type Config struct {
	TelegramEnabled  *bool  `json:"telegram_enabled,omitempty"`
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`

	MatrixEnabled     *bool  `json:"matrix_enabled,omitempty"`
	MatrixHomeserver  string `json:"matrix_homeserver"`
	MatrixRoomID      string `json:"matrix_room_id"`
	MatrixAccessToken string `json:"matrix_access_token"`

	EmailEnabled  *bool    `json:"email_enabled,omitempty"`
	EmailSMTPURL  string   `json:"email_smtp_url"`
	EmailFrom     string   `json:"email_from"`
	EmailTo       []string `json:"email_to"`
	EmailUsername string   `json:"email_username"`
	EmailPassword string   `json:"email_password"`

	ServerName          string `json:"server_name"`
	GeoIPEnabled        bool   `json:"geoip_enabled"`
	GeoIPDatabasePath   string `json:"geoip_database_path"`
	DatabasePath        string `json:"database_path"`
	DailyReportEnabled  bool   `json:"daily_report_enabled"`
	DailyReportTime     string `json:"daily_report_time"`
	DailyReportTimezone string `json:"daily_report_timezone"`
	RetentionDays       int    `json:"retention_days"`
	LogLevel            string `json:"log_level"`

	MetricsEnabled bool   `json:"metrics_enabled"`
	MetricsListen  string `json:"metrics_listen"`
}

func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		ServerName:          hostname,
		GeoIPEnabled:        true,
		GeoIPDatabasePath:   DefaultGeoIPPath,
		DatabasePath:        DefaultDatabasePath,
		DailyReportEnabled:  true,
		DailyReportTime:     "08:00",
		DailyReportTimezone: "UTC",
		RetentionDays:       90,
		LogLevel:            "info",
		MetricsEnabled:      false,
		MetricsListen:       "127.0.0.1:9184",
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		path = DefaultConfigPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	applyEnvOverrides(cfg)

	if cfg.ServerName == "" {
		hostname, _ := os.Hostname()
		cfg.ServerName = hostname
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OXIWATCH_TELEGRAM_ENABLED"); v != "" {
		cfg.TelegramEnabled = parseBoolPtr(v)
	}
	if v := os.Getenv("OXIWATCH_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.TelegramBotToken = v
	}
	if v := os.Getenv("OXIWATCH_TELEGRAM_CHAT_ID"); v != "" {
		cfg.TelegramChatID = v
	}
	if v := os.Getenv("OXIWATCH_MATRIX_ENABLED"); v != "" {
		cfg.MatrixEnabled = parseBoolPtr(v)
	}
	if v := os.Getenv("OXIWATCH_MATRIX_HOMESERVER"); v != "" {
		cfg.MatrixHomeserver = v
	}
	if v := os.Getenv("OXIWATCH_MATRIX_ROOM_ID"); v != "" {
		cfg.MatrixRoomID = v
	}
	if v := os.Getenv("OXIWATCH_MATRIX_ACCESS_TOKEN"); v != "" {
		cfg.MatrixAccessToken = v
	}
	if v := os.Getenv("OXIWATCH_EMAIL_ENABLED"); v != "" {
		cfg.EmailEnabled = parseBoolPtr(v)
	}
	if v := os.Getenv("OXIWATCH_EMAIL_SMTP_URL"); v != "" {
		cfg.EmailSMTPURL = v
	}
	if v := os.Getenv("OXIWATCH_EMAIL_FROM"); v != "" {
		cfg.EmailFrom = v
	}
	if v := os.Getenv("OXIWATCH_EMAIL_TO"); v != "" {
		cfg.EmailTo = splitAndTrim(v)
	}
	if v := os.Getenv("OXIWATCH_EMAIL_USERNAME"); v != "" {
		cfg.EmailUsername = v
	}
	if v := os.Getenv("OXIWATCH_EMAIL_PASSWORD"); v != "" {
		cfg.EmailPassword = v
	}
	if v := os.Getenv("OXIWATCH_SERVER_NAME"); v != "" {
		cfg.ServerName = v
	}
	if v := os.Getenv("OXIWATCH_GEOIP_ENABLED"); v != "" {
		cfg.GeoIPEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("OXIWATCH_GEOIP_DATABASE_PATH"); v != "" {
		cfg.GeoIPDatabasePath = v
	}
	if v := os.Getenv("OXIWATCH_DATABASE_PATH"); v != "" {
		cfg.DatabasePath = v
	}
	if v := os.Getenv("OXIWATCH_DAILY_REPORT_ENABLED"); v != "" {
		cfg.DailyReportEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("OXIWATCH_DAILY_REPORT_TIME"); v != "" {
		cfg.DailyReportTime = v
	}
	if v := os.Getenv("OXIWATCH_DAILY_REPORT_TIMEZONE"); v != "" {
		cfg.DailyReportTimezone = v
	}
	if v := os.Getenv("OXIWATCH_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil {
			cfg.RetentionDays = days
		}
	}
	if v := os.Getenv("OXIWATCH_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("OXIWATCH_METRICS_ENABLED"); v != "" {
		cfg.MetricsEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("OXIWATCH_METRICS_LISTEN"); v != "" {
		cfg.MetricsListen = v
	}
}

// TelegramConfigured reports whether the Telegram channel has all its fields set.
func (c *Config) TelegramConfigured() bool {
	return c.TelegramBotToken != "" && c.TelegramChatID != ""
}

// MatrixConfigured reports whether the Matrix channel has all its fields set.
func (c *Config) MatrixConfigured() bool {
	return c.MatrixHomeserver != "" && c.MatrixRoomID != "" && c.MatrixAccessToken != ""
}

// EmailConfigured reports whether the Email channel has all its fields set.
func (c *Config) EmailConfigured() bool {
	return c.EmailSMTPURL != "" && c.EmailFrom != "" && len(c.EmailTo) > 0 &&
		c.EmailUsername != "" && c.EmailPassword != ""
}

// enabledFlag treats a missing (nil) flag as enabled, so a fully configured
// channel works without an explicit "*_enabled": true entry.
func enabledFlag(flag *bool) bool { return flag == nil || *flag }

// disabledFlag is true only when the flag is explicitly set to false.
func disabledFlag(flag *bool) bool { return flag != nil && !*flag }

// TelegramActive reports whether Telegram should be used (configured and not disabled).
func (c *Config) TelegramActive() bool {
	return c.TelegramConfigured() && enabledFlag(c.TelegramEnabled)
}

// MatrixActive reports whether Matrix should be used (configured and not disabled).
func (c *Config) MatrixActive() bool {
	return c.MatrixConfigured() && enabledFlag(c.MatrixEnabled)
}

// EmailActive reports whether Email should be used (configured and not disabled).
func (c *Config) EmailActive() bool {
	return c.EmailConfigured() && enabledFlag(c.EmailEnabled)
}

func (c *Config) Validate() error {
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}
	if c.RetentionDays < 1 {
		return fmt.Errorf("retention_days must be at least 1")
	}

	// Reject partially configured channels, unless explicitly disabled (so a
	// channel can be toggled off mid-setup without tripping validation).
	if !disabledFlag(c.TelegramEnabled) {
		if (c.TelegramBotToken != "" || c.TelegramChatID != "") && !c.TelegramConfigured() {
			return fmt.Errorf("telegram requires both telegram_bot_token and telegram_chat_id")
		}
	}
	if !disabledFlag(c.MatrixEnabled) {
		if (c.MatrixHomeserver != "" || c.MatrixRoomID != "" || c.MatrixAccessToken != "") && !c.MatrixConfigured() {
			return fmt.Errorf("matrix requires matrix_homeserver, matrix_room_id and matrix_access_token")
		}
	}
	if !disabledFlag(c.EmailEnabled) {
		emailPartial := c.EmailSMTPURL != "" || c.EmailFrom != "" || len(c.EmailTo) > 0 ||
			c.EmailUsername != "" || c.EmailPassword != ""
		if emailPartial && !c.EmailConfigured() {
			return fmt.Errorf("email requires email_smtp_url, email_from, email_to, email_username and email_password")
		}
	}

	if !c.TelegramActive() && !c.MatrixActive() && !c.EmailActive() {
		return fmt.Errorf("at least one notification channel must be configured and enabled (telegram, matrix or email)")
	}

	if c.MetricsEnabled && c.MetricsListen == "" {
		return fmt.Errorf("metrics_listen is required when metrics_enabled is true")
	}

	return nil
}

// parseBoolPtr parses a truthy/falsey string into a *bool ("true"/"1" => true).
func parseBoolPtr(v string) *bool {
	b := strings.ToLower(v) == "true" || v == "1"
	return &b
}

// splitAndTrim splits a comma-separated list into trimmed, non-empty entries.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}
