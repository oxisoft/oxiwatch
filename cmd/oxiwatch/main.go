package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/oxisoft/oxiwatch/internal/config"
	"github.com/oxisoft/oxiwatch/internal/daemon"
	"github.com/oxisoft/oxiwatch/internal/geoip"
	"github.com/oxisoft/oxiwatch/internal/notifier"
	"github.com/oxisoft/oxiwatch/internal/report"
	"github.com/oxisoft/oxiwatch/internal/storage"
)

var Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	configPath := os.Getenv("OXIWATCH_CONFIG")
	if configPath == "" {
		configPath = config.DefaultConfigPath
	}

	switch os.Args[1] {
	case "daemon":
		runDaemon(configPath)
	case "stats":
		runStats(configPath)
	case "geoip":
		runGeoIP(configPath)
	case "cleanup":
		runCleanup(configPath)
	case "config":
		runConfig(configPath)
	case "send-test":
		runSendTest(configPath)
	case "version":
		fmt.Printf("oxiwatch version %s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: oxiwatch <command> [options]

Commands:
  daemon [-f|--foreground]     Run monitoring daemon
  stats today                  Show today's statistics
  stats report [-d N]          Generate report (last N days, default 1)
  stats logins [-d N]          Show successful logins (last N days, default 7)
  geoip update                 Download/update GeoIP database
  geoip status                 Show GeoIP database info
  cleanup                      Manually run retention cleanup
  config validate              Validate configuration
  config show                  Show active configuration
  send-test                    Send test Telegram message
  version                      Show version
  help                         Show this help

Environment:
  OXIWATCH_CONFIG              Path to config file (default: /etc/oxiwatch/config.json)`)
}

func runDaemon(configPath string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	foreground := fs.Bool("f", false, "Run in foreground")
	fs.BoolVar(foreground, "foreground", false, "Run in foreground")
	fs.Parse(os.Args[2:])

	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		fatal("invalid config: %v", err)
	}

	logger := setupLogger(cfg.LogLevel)

	d, err := daemon.New(cfg, logger)
	if err != nil {
		fatal("failed to initialize daemon: %v", err)
	}

	if err := d.Run(); err != nil {
		fatal("daemon error: %v", err)
	}
}

func runStats(configPath string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: oxiwatch stats <today|report|logins> [options]")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		fatal("failed to open database: %v", err)
	}
	defer store.Close()

	gen := report.NewGenerator(store, cfg.ServerName)

	switch os.Args[2] {
	case "today":
		output, err := gen.GenerateStats(1)
		if err != nil {
			fatal("failed to generate stats: %v", err)
		}
		fmt.Print(output)

	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		days := fs.Int("d", 1, "Number of days")
		fs.Parse(os.Args[3:])

		output, err := gen.GenerateStats(*days)
		if err != nil {
			fatal("failed to generate report: %v", err)
		}
		fmt.Print(output)

	case "logins":
		fs := flag.NewFlagSet("logins", flag.ExitOnError)
		days := fs.Int("d", 7, "Number of days")
		fs.Parse(os.Args[3:])

		output, err := gen.GenerateLoginsReport(*days)
		if err != nil {
			fatal("failed to generate logins report: %v", err)
		}
		fmt.Print(output)

	default:
		fmt.Fprintf(os.Stderr, "Unknown stats command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func runGeoIP(configPath string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: oxiwatch geoip <update|status>")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	logger := setupLogger(cfg.LogLevel)
	updater := geoip.NewUpdater(cfg.GeoIPDatabasePath, logger)

	switch os.Args[2] {
	case "update":
		if err := updater.Update(); err != nil {
			fatal("failed to update GeoIP database: %v", err)
		}
		fmt.Println("GeoIP database updated successfully")

	case "status":
		if !updater.DatabaseExists() {
			fmt.Println("GeoIP database: not found")
			fmt.Printf("Path: %s\n", cfg.GeoIPDatabasePath)
			fmt.Println()
			fmt.Println("Run 'oxiwatch geoip update' to download the database")
			return
		}

		modTime, size, err := updater.GetDatabaseInfo()
		if err != nil {
			fatal("failed to get database info: %v", err)
		}

		localYear, localMonth, _ := updater.GetLocalVersion()

		fmt.Println("GeoIP database: installed")
		fmt.Printf("Path: %s\n", cfg.GeoIPDatabasePath)
		fmt.Printf("Size: %.2f MB\n", float64(size)/1024/1024)
		fmt.Printf("Local version: %d-%02d\n", localYear, localMonth)
		fmt.Printf("Last modified: %s\n", modTime.Format("2006-01-02 15:04:05"))
		fmt.Println()

		fmt.Println("Remote check:")
		remoteYear, remoteMonth, err := updater.GetLatestRemoteVersion()
		if err != nil {
			fmt.Printf("  Failed to check remote: %v\n", err)
		} else {
			fmt.Printf("  Latest available: %d-%02d\n", remoteYear, remoteMonth)
			if remoteYear > localYear || (remoteYear == localYear && remoteMonth > localMonth) {
				fmt.Println("  Status: Update available")
				fmt.Println("  Run 'oxiwatch geoip update' to download the latest version")
			} else {
				fmt.Println("  Status: Up to date")
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown geoip command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func runCleanup(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		fatal("failed to open database: %v", err)
	}
	defer store.Close()

	deleted, err := store.Cleanup(cfg.RetentionDays)
	if err != nil {
		fatal("cleanup failed: %v", err)
	}

	fmt.Printf("Cleanup completed. Deleted %d records older than %d days.\n", deleted, cfg.RetentionDays)
}

func runConfig(configPath string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: oxiwatch config <validate|show>")
		os.Exit(1)
	}

	switch os.Args[2] {
	case "validate":
		cfg, err := config.Load(configPath)
		if err != nil {
			fatal("failed to load config: %v", err)
		}
		if err := cfg.Validate(); err != nil {
			fatal("validation failed: %v", err)
		}
		fmt.Println("Configuration is valid")

	case "show":
		cfg, err := config.Load(configPath)
		if err != nil {
			fatal("failed to load config: %v", err)
		}

		masked := *cfg
		if masked.TelegramBotToken != "" {
			masked.TelegramBotToken = "***"
		}

		output, _ := json.MarshalIndent(masked, "", "  ")
		fmt.Println(string(output))

	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func runSendTest(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		fatal("invalid config: %v", err)
	}

	telegram := notifier.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID, cfg.ServerName)
	if err := telegram.SendTestMessage(); err != nil {
		fatal("failed to send test message: %v", err)
	}

	fmt.Println("Test message sent successfully")
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
