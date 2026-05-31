package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// clearOxiwatchEnv removes any OXIWATCH_* env vars that may be set in the
// ambient environment so tests start from a clean slate.
func clearOxiwatchEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		// kv is "KEY=VALUE"; extract KEY.
		eq := -1
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				eq = i
				break
			}
		}
		if eq <= 0 {
			continue
		}
		key := kv[:eq]
		if len(key) >= len("OXIWATCH_") && key[:len("OXIWATCH_")] == "OXIWATCH_" {
			// Setenv + restore handled by t.Setenv requires a value; use
			// Setenv to "" then ensure unset for the duration of the test.
			old := os.Getenv(key)
			os.Unsetenv(key)
			t.Cleanup(func() { os.Setenv(key, old) })
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	hostname, _ := os.Hostname()
	if cfg.ServerName != hostname {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, hostname)
	}
	if !cfg.GeoIPEnabled {
		t.Error("GeoIPEnabled = false, want true")
	}
	if cfg.GeoIPDatabasePath != DefaultGeoIPPath {
		t.Errorf("GeoIPDatabasePath = %q, want %q", cfg.GeoIPDatabasePath, DefaultGeoIPPath)
	}
	if cfg.DatabasePath != DefaultDatabasePath {
		t.Errorf("DatabasePath = %q, want %q", cfg.DatabasePath, DefaultDatabasePath)
	}
	if !cfg.DailyReportEnabled {
		t.Error("DailyReportEnabled = false, want true")
	}
	if cfg.DailyReportTime != "08:00" {
		t.Errorf("DailyReportTime = %q, want %q", cfg.DailyReportTime, "08:00")
	}
	if cfg.DailyReportTimezone != "UTC" {
		t.Errorf("DailyReportTimezone = %q, want %q", cfg.DailyReportTimezone, "UTC")
	}
	if cfg.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want 90", cfg.RetentionDays)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.MetricsEnabled {
		t.Error("MetricsEnabled = true, want false")
	}
	if cfg.MetricsListen != "127.0.0.1:9184" {
		t.Errorf("MetricsListen = %q, want %q", cfg.MetricsListen, "127.0.0.1:9184")
	}
	// Channel enable pointers should be nil by default.
	if cfg.TelegramEnabled != nil || cfg.MatrixEnabled != nil || cfg.EmailEnabled != nil {
		t.Error("channel enabled pointers should be nil by default")
	}
}

func TestLoad_MissingFileReturnsDefaultsAndAppliesEnv(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.json")

	t.Setenv("OXIWATCH_SERVER_NAME", "env-server")
	t.Setenv("OXIWATCH_LOG_LEVEL", "debug")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load(missing) returned error: %v", err)
	}
	// Defaults preserved where no env override.
	if cfg.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want default 90", cfg.RetentionDays)
	}
	if cfg.MetricsListen != "127.0.0.1:9184" {
		t.Errorf("MetricsListen = %q, want default", cfg.MetricsListen)
	}
	// Env overrides applied.
	if cfg.ServerName != "env-server" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "env-server")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoad_ValidJSONFile(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"server_name": "file-server",
		"retention_days": 30,
		"log_level": "warn",
		"telegram_bot_token": "tok",
		"telegram_chat_id": "chat",
		"email_to": ["a@example.com", "b@example.com"],
		"metrics_enabled": true,
		"metrics_listen": "0.0.0.0:9999"
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(valid) returned error: %v", err)
	}
	if cfg.ServerName != "file-server" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "file-server")
	}
	if cfg.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", cfg.RetentionDays)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
	if cfg.TelegramBotToken != "tok" || cfg.TelegramChatID != "chat" {
		t.Errorf("telegram fields not loaded: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.EmailTo, []string{"a@example.com", "b@example.com"}) {
		t.Errorf("EmailTo = %v, want two addresses", cfg.EmailTo)
	}
	if !cfg.MetricsEnabled || cfg.MetricsListen != "0.0.0.0:9999" {
		t.Errorf("metrics fields not loaded: enabled=%v listen=%q", cfg.MetricsEnabled, cfg.MetricsListen)
	}
	// Field not present in JSON keeps default.
	if cfg.DatabasePath != DefaultDatabasePath {
		t.Errorf("DatabasePath = %q, want default", cfg.DatabasePath)
	}
}

func TestLoad_ValidJSONFile_EmptyServerNameFallsBackToHostname(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{"server_name": "", "retention_days": 10}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	hostname, _ := os.Hostname()
	if cfg.ServerName != hostname {
		t.Errorf("ServerName = %q, want hostname %q", cfg.ServerName, hostname)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{ this is not json "), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err == nil {
		t.Fatalf("Load(invalid) expected error, got nil (cfg=%+v)", cfg)
	}
	if cfg != nil {
		t.Errorf("Load(invalid) cfg = %+v, want nil", cfg)
	}
}

func TestLoad_EnvOverridesWinOverFile(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"server_name": "file-server",
		"retention_days": 30,
		"metrics_enabled": false,
		"email_to": ["file@example.com"]
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	t.Setenv("OXIWATCH_SERVER_NAME", "env-server")
	t.Setenv("OXIWATCH_RETENTION_DAYS", "7")
	t.Setenv("OXIWATCH_METRICS_ENABLED", "true")
	t.Setenv("OXIWATCH_EMAIL_TO", " one@example.com , two@example.com ,, three@example.com ")
	t.Setenv("OXIWATCH_TELEGRAM_ENABLED", "false")
	t.Setenv("OXIWATCH_GEOIP_ENABLED", "1")
	t.Setenv("OXIWATCH_DAILY_REPORT_ENABLED", "0")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ServerName != "env-server" {
		t.Errorf("ServerName = %q, want env-server", cfg.ServerName)
	}
	if cfg.RetentionDays != 7 {
		t.Errorf("RetentionDays = %d, want 7", cfg.RetentionDays)
	}
	if !cfg.MetricsEnabled {
		t.Error("MetricsEnabled = false, env should set true")
	}
	want := []string{"one@example.com", "two@example.com", "three@example.com"}
	if !reflect.DeepEqual(cfg.EmailTo, want) {
		t.Errorf("EmailTo = %v, want %v (comma list trimmed, empties dropped)", cfg.EmailTo, want)
	}
	if cfg.TelegramEnabled == nil || *cfg.TelegramEnabled != false {
		t.Errorf("TelegramEnabled = %v, want explicit false", cfg.TelegramEnabled)
	}
	if !cfg.GeoIPEnabled {
		t.Error("GeoIPEnabled = false, env=1 should set true")
	}
	if cfg.DailyReportEnabled {
		t.Error("DailyReportEnabled = true, env=0 should set false")
	}
}

func TestLoad_InvalidRetentionEnvIgnored(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	t.Setenv("OXIWATCH_RETENTION_DAYS", "not-a-number")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.RetentionDays != 90 {
		t.Errorf("RetentionDays = %d, want default 90 (invalid env ignored)", cfg.RetentionDays)
	}
}

func TestLoad_AllStringEnvOverrides(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")

	env := map[string]string{
		"OXIWATCH_TELEGRAM_BOT_TOKEN":    "tok",
		"OXIWATCH_TELEGRAM_CHAT_ID":      "chat",
		"OXIWATCH_MATRIX_HOMESERVER":     "https://hs",
		"OXIWATCH_MATRIX_ROOM_ID":        "!room",
		"OXIWATCH_MATRIX_ACCESS_TOKEN":   "mtok",
		"OXIWATCH_EMAIL_SMTP_URL":        "smtp://localhost:25",
		"OXIWATCH_EMAIL_FROM":            "from@example.com",
		"OXIWATCH_EMAIL_USERNAME":        "user",
		"OXIWATCH_EMAIL_PASSWORD":        "pass",
		"OXIWATCH_GEOIP_DATABASE_PATH":   "/tmp/geo.mmdb",
		"OXIWATCH_DATABASE_PATH":         "/tmp/db.sqlite",
		"OXIWATCH_DAILY_REPORT_TIME":     "09:30",
		"OXIWATCH_DAILY_REPORT_TIMEZONE": "Europe/London",
		"OXIWATCH_METRICS_LISTEN":        "0.0.0.0:1234",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	checks := map[string]struct{ got, want string }{
		"TelegramBotToken":    {cfg.TelegramBotToken, "tok"},
		"TelegramChatID":      {cfg.TelegramChatID, "chat"},
		"MatrixHomeserver":    {cfg.MatrixHomeserver, "https://hs"},
		"MatrixRoomID":        {cfg.MatrixRoomID, "!room"},
		"MatrixAccessToken":   {cfg.MatrixAccessToken, "mtok"},
		"EmailSMTPURL":        {cfg.EmailSMTPURL, "smtp://localhost:25"},
		"EmailFrom":           {cfg.EmailFrom, "from@example.com"},
		"EmailUsername":       {cfg.EmailUsername, "user"},
		"EmailPassword":       {cfg.EmailPassword, "pass"},
		"GeoIPDatabasePath":   {cfg.GeoIPDatabasePath, "/tmp/geo.mmdb"},
		"DatabasePath":        {cfg.DatabasePath, "/tmp/db.sqlite"},
		"DailyReportTime":     {cfg.DailyReportTime, "09:30"},
		"DailyReportTimezone": {cfg.DailyReportTimezone, "Europe/London"},
		"MetricsListen":       {cfg.MetricsListen, "0.0.0.0:1234"},
	}
	for field, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
}

func TestConfig_String(t *testing.T) {
	c := withTelegram(validBase())
	s := c.String()
	if s == "" {
		t.Fatal("String() returned empty")
	}
	if !strings.Contains(s, "\"telegram_bot_token\": \"tok\"") {
		t.Errorf("String() output missing telegram_bot_token: %s", s)
	}
}

func TestLoad_AllChannelEnvEnableFlags(t *testing.T) {
	clearOxiwatchEnv(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	t.Setenv("OXIWATCH_TELEGRAM_ENABLED", "true")
	t.Setenv("OXIWATCH_MATRIX_ENABLED", "0")
	t.Setenv("OXIWATCH_EMAIL_ENABLED", "false")

	cfg, err := Load(missing)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TelegramEnabled == nil || *cfg.TelegramEnabled != true {
		t.Errorf("TelegramEnabled = %v, want true", cfg.TelegramEnabled)
	}
	if cfg.MatrixEnabled == nil || *cfg.MatrixEnabled != false {
		t.Errorf("MatrixEnabled = %v, want false", cfg.MatrixEnabled)
	}
	if cfg.EmailEnabled == nil || *cfg.EmailEnabled != false {
		t.Errorf("EmailEnabled = %v, want false", cfg.EmailEnabled)
	}
}

// boolPtr is a small helper for building *bool literals in tables.
func boolPtr(b bool) *bool { return &b }

func validBase() *Config {
	return &Config{
		DatabasePath:  DefaultDatabasePath,
		RetentionDays: 90,
	}
}

func withTelegram(c *Config) *Config {
	c.TelegramBotToken = "tok"
	c.TelegramChatID = "chat"
	return c
}

func withMatrix(c *Config) *Config {
	c.MatrixHomeserver = "https://matrix.example.com"
	c.MatrixRoomID = "!room:example.com"
	c.MatrixAccessToken = "secret"
	return c
}

func withEmail(c *Config) *Config {
	c.EmailSMTPURL = "smtp://localhost:25"
	c.EmailFrom = "from@example.com"
	c.EmailTo = []string{"to@example.com"}
	c.EmailUsername = "user"
	c.EmailPassword = "pass"
	return c
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *Config
		wantErr bool
	}{
		{
			name:    "no channel active -> error",
			build:   func() *Config { return validBase() },
			wantErr: true,
		},
		{
			name:    "telegram-only ok",
			build:   func() *Config { return withTelegram(validBase()) },
			wantErr: false,
		},
		{
			name:    "matrix-only ok",
			build:   func() *Config { return withMatrix(validBase()) },
			wantErr: false,
		},
		{
			name:    "email-only ok",
			build:   func() *Config { return withEmail(validBase()) },
			wantErr: false,
		},
		{
			name: "partial telegram -> error",
			build: func() *Config {
				c := validBase()
				c.TelegramBotToken = "tok" // missing chat id
				return c
			},
			wantErr: true,
		},
		{
			name: "partial matrix -> error",
			build: func() *Config {
				c := validBase()
				c.MatrixHomeserver = "https://hs" // missing room/token
				return c
			},
			wantErr: true,
		},
		{
			name: "partial email -> error",
			build: func() *Config {
				c := validBase()
				c.EmailSMTPURL = "smtp://localhost" // missing rest
				return c
			},
			wantErr: true,
		},
		{
			name: "partial telegram explicitly disabled -> not error (but no active channel)",
			build: func() *Config {
				// Disable the partial telegram, and supply a working matrix so
				// that the "at least one channel" rule is satisfied. This proves
				// the partial-disabled channel does not itself error.
				c := withMatrix(validBase())
				c.TelegramBotToken = "tok" // partial telegram
				c.TelegramEnabled = boolPtr(false)
				return c
			},
			wantErr: false,
		},
		{
			name: "partial matrix explicitly disabled -> not error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.MatrixHomeserver = "https://hs" // partial matrix
				c.MatrixEnabled = boolPtr(false)
				return c
			},
			wantErr: false,
		},
		{
			name: "partial email explicitly disabled -> not error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.EmailSMTPURL = "smtp://localhost" // partial email
				c.EmailEnabled = boolPtr(false)
				return c
			},
			wantErr: false,
		},
		{
			name: "configured telegram but explicitly disabled and no other channel -> error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.TelegramEnabled = boolPtr(false)
				return c
			},
			wantErr: true,
		},
		{
			name: "metrics_enabled with empty metrics_listen -> error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.MetricsEnabled = true
				c.MetricsListen = ""
				return c
			},
			wantErr: true,
		},
		{
			name: "metrics_enabled with metrics_listen -> ok",
			build: func() *Config {
				c := withTelegram(validBase())
				c.MetricsEnabled = true
				c.MetricsListen = "127.0.0.1:9184"
				return c
			},
			wantErr: false,
		},
		{
			name: "retention_days < 1 -> error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.RetentionDays = 0
				return c
			},
			wantErr: true,
		},
		{
			name: "empty database_path -> error",
			build: func() *Config {
				c := withTelegram(validBase())
				c.DatabasePath = ""
				return c
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build().Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestTelegramActive(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil flag, configured -> active", nil, true},
		{"true flag, configured -> active", boolPtr(true), true},
		{"false flag, configured -> inactive", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := withTelegram(validBase())
			c.TelegramEnabled = tt.enabled
			if got := c.TelegramActive(); got != tt.want {
				t.Errorf("TelegramActive() = %v, want %v", got, tt.want)
			}
		})
	}

	// Not configured -> never active regardless of flag.
	c := validBase()
	c.TelegramEnabled = boolPtr(true)
	if c.TelegramActive() {
		t.Error("TelegramActive() = true for unconfigured channel, want false")
	}
}

func TestMatrixActive(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil flag, configured -> active", nil, true},
		{"true flag, configured -> active", boolPtr(true), true},
		{"false flag, configured -> inactive", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := withMatrix(validBase())
			c.MatrixEnabled = tt.enabled
			if got := c.MatrixActive(); got != tt.want {
				t.Errorf("MatrixActive() = %v, want %v", got, tt.want)
			}
		})
	}

	c := validBase()
	c.MatrixEnabled = boolPtr(true)
	if c.MatrixActive() {
		t.Error("MatrixActive() = true for unconfigured channel, want false")
	}
}

func TestEmailActive(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil flag, configured -> active", nil, true},
		{"true flag, configured -> active", boolPtr(true), true},
		{"false flag, configured -> inactive", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := withEmail(validBase())
			c.EmailEnabled = tt.enabled
			if got := c.EmailActive(); got != tt.want {
				t.Errorf("EmailActive() = %v, want %v", got, tt.want)
			}
		})
	}

	c := validBase()
	c.EmailEnabled = boolPtr(true)
	if c.EmailActive() {
		t.Error("EmailActive() = true for unconfigured channel, want false")
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string", "", []string{}},
		{"single", "a@example.com", []string{"a@example.com"}},
		{"multiple with spaces", " a , b ,c ", []string{"a", "b", "c"}},
		{"empties dropped", "a,,b,  ,c", []string{"a", "b", "c"}},
		{"only commas/spaces", " , , ", []string{}},
		{"trailing comma", "a,b,", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrim(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitAndTrim(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseBoolPtr(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"yes", false}, // only "true"/"1" are truthy
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseBoolPtr(tt.in)
			if got == nil {
				t.Fatalf("parseBoolPtr(%q) = nil, want non-nil", tt.in)
			}
			if *got != tt.want {
				t.Errorf("parseBoolPtr(%q) = %v, want %v", tt.in, *got, tt.want)
			}
		})
	}
}
