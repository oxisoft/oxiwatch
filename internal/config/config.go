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
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramChatID      string `json:"telegram_chat_id"`
	ServerName          string `json:"server_name"`
	GeoIPEnabled        bool   `json:"geoip_enabled"`
	GeoIPDatabasePath   string `json:"geoip_database_path"`
	DatabasePath        string `json:"database_path"`
	DailyReportEnabled  bool   `json:"daily_report_enabled"`
	DailyReportTime     string `json:"daily_report_time"`
	DailyReportTimezone string `json:"daily_report_timezone"`
	RetentionDays       int    `json:"retention_days"`
	LogLevel            string `json:"log_level"`
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
	if v := os.Getenv("OXIWATCH_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.TelegramBotToken = v
	}
	if v := os.Getenv("OXIWATCH_TELEGRAM_CHAT_ID"); v != "" {
		cfg.TelegramChatID = v
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
}

func (c *Config) Validate() error {
	if c.TelegramBotToken == "" {
		return fmt.Errorf("telegram_bot_token is required")
	}
	if c.TelegramChatID == "" {
		return fmt.Errorf("telegram_chat_id is required")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}
	if c.RetentionDays < 1 {
		return fmt.Errorf("retention_days must be at least 1")
	}
	return nil
}

func (c *Config) String() string {
	data, _ := json.MarshalIndent(c, "", "  ")
	return string(data)
}
