package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oxisoft/oxiwatch/internal/config"
)

// runCLI runs main() with the given arguments against configPath and returns
// what it printed to stdout. Only success paths are run in-process: error
// paths call os.Exit.
func runCLI(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	t.Setenv("OXIWATCH_CONFIG", configPath)
	oldArgs, oldOut := os.Args, os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"oxiwatch"}, args...)
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Args, os.Stdout = oldArgs, oldOut
	}()
	main()
	w.Close()
	return <-done
}

func writeConfig(t *testing.T, extra map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	cfg := map[string]any{
		"server_name":    "test-box",
		"timezone":       "Europe/Warsaw",
		"database_path":  filepath.Join(dir, "oxiwatch.db"),
		"geoip_enabled":  false,
		"retention_days": 30,
	}
	for k, v := range extra {
		cfg[k] = v
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, b, 0o660); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustContain(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q:\n%s", w, out)
		}
	}
}

func TestQuietCLI_Lifecycle(t *testing.T) {
	cfgPath := writeConfig(t, nil)

	mustContain(t, runCLI(t, cfgPath, "quiet", "list"), "quiet list is empty")

	// Flags after the positional argument, IP normalised from a host CIDR.
	out := runCLI(t, cfgPath, "quiet", "add", "45.155.149.185/32", "--user", "oxi", "--label", "Office")
	mustContain(t, out, "Normalised", "Added: 45.155.149.185, user oxi, label \"Office\", expires never")
	// Flags before the positional argument; no user = warning about root.
	out = runCLI(t, cfgPath, "quiet", "add", "--label", "Home net", "--expires", "30d", "10.0.0.0/8")
	mustContain(t, out, "Added: 10.0.0.0/8, user (any)", "ANY user (root included)")
	// Already expired entry, kept for prune.
	runCLI(t, cfgPath, "quiet", "add", "192.0.2.7", "--user", "bob", "--expires", "2020-01-01")

	entries, err := config.LoadQuietIPs(cfgPath)
	if err != nil || len(entries) != 3 {
		t.Fatalf("stored entries = %v, %v; want 3", entries, err)
	}
	// The rest of the config file must survive quiet edits.
	if cfg, _ := config.Load(cfgPath); cfg.ServerName != "test-box" || cfg.Timezone != "Europe/Warsaw" {
		t.Errorf("config lost keys after quiet add: %+v", cfg)
	}

	out = runCLI(t, cfgPath, "quiet", "list")
	mustContain(t, out, "Office", "Home net", "(any)", "never", "(expired)")

	// The decision the daemon makes for an alert.
	mustContain(t, runCLI(t, cfgPath, "quiet", "check", "45.155.149.185", "--user", "oxi"), "Quiet: matches \"Office\"")
	mustContain(t, runCLI(t, cfgPath, "quiet", "check", "--user", "root", "45.155.149.185"), "Not quiet")
	mustContain(t, runCLI(t, cfgPath, "quiet", "check", "10.20.30.40", "--user", "root"), "Quiet: matches \"Home net\"")
	mustContain(t, runCLI(t, cfgPath, "quiet", "check", "192.0.2.7", "--user", "bob"), "Not quiet")

	mustContain(t, runCLI(t, cfgPath, "quiet", "prune"), "Removed expired: 192.0.2.7, user bob")
	mustContain(t, runCLI(t, cfgPath, "quiet", "prune"), "No expired entries.")

	mustContain(t, runCLI(t, cfgPath, "quiet", "remove", "Home net"), "Removed: 10.0.0.0/8")
	mustContain(t, runCLI(t, cfgPath, "quiet", "rm", "1"), "Removed: 45.155.149.185, user oxi")
	if entries, _ := config.LoadQuietIPs(cfgPath); len(entries) != 0 {
		t.Errorf("entries left after removes: %v", entries)
	}

	runCLI(t, cfgPath, "quiet", "add", "2001:db8::1", "--user", "deploy")
	mustContain(t, runCLI(t, cfgPath, "quiet", "remove", "2001:db8::1"), "Removed: 2001:db8::1, user deploy")

	mustContain(t, runCLI(t, cfgPath, "quiet", "help"), "Usage: oxiwatch quiet")
}

func TestConfigCLI(t *testing.T) {
	cfgPath := writeConfig(t, map[string]any{
		"telegram_bot_token":  "123:SECRET",
		"telegram_chat_id":    "42",
		"matrix_homeserver":   "https://m.example",
		"matrix_room_id":      "!r:m.example",
		"matrix_access_token": "syt_SECRET",
		"email_password":      "hunter2",
	})
	out := runCLI(t, cfgPath, "config", "show")
	for _, secret := range []string{"123:SECRET", "syt_SECRET", "hunter2"} {
		if strings.Contains(out, secret) {
			t.Errorf("config show leaks %q", secret)
		}
	}
	mustContain(t, out, `"***"`, "test-box")

	valid := writeConfig(t, map[string]any{"telegram_bot_token": "123:abc", "telegram_chat_id": "42"})
	mustContain(t, runCLI(t, valid, "config", "validate"), "Configuration is valid")
}

func TestStatsAndCleanupCLI(t *testing.T) {
	cfgPath := writeConfig(t, nil)
	mustContain(t, runCLI(t, cfgPath, "stats", "today"), "test-box")
	mustContain(t, runCLI(t, cfgPath, "stats", "report", "-d", "7"), "test-box")
	runCLI(t, cfgPath, "stats", "logins", "-d", "3")
	mustContain(t, runCLI(t, cfgPath, "cleanup"), "Deleted 0 records older than 30 days")
}

func TestHelpersCLI(t *testing.T) {
	mustContain(t, runCLI(t, "", "help"), "Usage: oxiwatch <command>")
	if truncate("abc", 5) != "abc" || truncate("abcdef", 3) != "abc..." {
		t.Error("truncate")
	}
	if emptyDash("") != "(none)" || emptyDash("x") != "x" {
		t.Error("emptyDash")
	}
	if orAny("") != "(any)" || orNever("") != "never" || orNever("2026-12-31") != "2026-12-31" {
		t.Error("orAny/orNever")
	}
	for _, lvl := range []string{"debug", "warn", "error", "info", ""} {
		if setupLogger(lvl) == nil {
			t.Errorf("setupLogger(%q) = nil", lvl)
		}
	}
}
