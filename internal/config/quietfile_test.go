package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oxisoft/oxiwatch/internal/quiet"
)

const sampleConfig = `{
  "telegram_bot_token": "123:abc",
  "telegram_chat_id": "-100",
  "server_name": "web1",
  "timezone": "UTC",
  "some_future_option": {"nested": [1, 2, 3]},
  "retention_days": 90
}
`

func writeTemp(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSaveQuietIPsAppendsAndKeepsRest(t *testing.T) {
	p := writeTemp(t, sampleConfig, 0o600)
	entries := []quiet.Entry{
		{IP: "203.0.113.7", User: "alex", Label: "Alex \"home\""},
		{IP: "198.51.100.0/24", Label: "Office", Expires: "2026-12-31"},
	}
	if err := SaveQuietIPs(p, entries); err != nil {
		t.Fatalf("SaveQuietIPs() error: %v", err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)

	// Everything before the closing brace is unchanged byte for byte.
	head := strings.TrimRight(sampleConfig[:strings.LastIndex(sampleConfig, "}")], " \n")
	if !strings.HasPrefix(s, head) {
		t.Fatalf("original content changed:\n%s", s)
	}
	want := `  "retention_days": 90,
  "quiet_ips": [
    {"ip": "203.0.113.7", "user": "alex", "label": "Alex \"home\""},
    {"ip": "198.51.100.0/24", "label": "Office", "expires": "2026-12-31"}
  ]
}
`
	if !strings.HasSuffix(s, want) {
		t.Fatalf("unexpected layout:\n%s", s)
	}

	back, err := LoadQuietIPs(p)
	if err != nil || len(back) != 2 || back[0] != entries[0] || back[1] != entries[1] {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	var anyMap map[string]any
	if err := json.Unmarshal(got, &anyMap); err != nil || anyMap["some_future_option"] == nil {
		t.Fatalf("unknown option lost or invalid JSON: %v", err)
	}
}

func TestSaveQuietIPsReplacesInPlace(t *testing.T) {
	cfg := `{
	"server_name": "web1",
	"quiet_ips": [ {"ip": "10.0.0.1"} ],
	"retention_days": 30
}`
	p := writeTemp(t, cfg, 0o640)
	if err := SaveQuietIPs(p, []quiet.Entry{{IP: "10.0.0.2", User: "bob"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := `{
	"server_name": "web1",
	"quiet_ips": [
	  {"ip": "10.0.0.2", "user": "bob"}
	],
	"retention_days": 30
}`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// Emptying the list keeps the key as [].
	if err := SaveQuietIPs(p, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(p)
	if !strings.Contains(string(got), `"quiet_ips": [],`) {
		t.Fatalf("got:\n%s", got)
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestSaveQuietIPsEmptyObject(t *testing.T) {
	p := writeTemp(t, "{}\n", 0o600)
	if err := SaveQuietIPs(p, []quiet.Entry{{IP: "10.0.0.1"}}); err != nil {
		t.Fatal(err)
	}
	back, err := LoadQuietIPs(p)
	if err != nil || len(back) != 1 {
		t.Fatalf("got %+v, %v", back, err)
	}
}

func TestSaveQuietIPsRejectsInvalid(t *testing.T) {
	p := writeTemp(t, sampleConfig, 0o600)
	err := SaveQuietIPs(p, []quiet.Entry{{IP: "not-an-ip"}})
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != sampleConfig {
		t.Fatal("file was modified despite the error")
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %d entries", len(entries))
	}
}

func TestSaveQuietIPsBrokenFile(t *testing.T) {
	p := writeTemp(t, `{"server_name": `, 0o600)
	if err := SaveQuietIPs(p, []quiet.Entry{{IP: "10.0.0.1"}}); err == nil {
		t.Fatal("expected an error for a broken config file")
	}
	if err := SaveQuietIPs(filepath.Join(t.TempDir(), "missing.json"), nil); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	p2 := writeTemp(t, `[1,2]`, 0o600)
	if err := SaveQuietIPs(p2, nil); err == nil {
		t.Fatal("expected an error for a non-object config")
	}
}

func TestValidateRejectsBadQuietIPs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TelegramBotToken, cfg.TelegramChatID = "t", "c"
	cfg.QuietIPs = []quiet.Entry{{IP: "10.0.0.1/99"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an invalid quiet_ips entry")
	}
	cfg.QuietIPs = []quiet.Entry{{IP: "10.0.0.0/8", User: "alex"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}
