# OxiWatch

SSH login monitor for Debian-like Linux systems. Sends Telegram notifications for successful logins and generates daily reports of failed attempts.

## Features

- Real-time monitoring via systemd journal (`journalctl -u ssh`)
- Instant Telegram alerts for successful SSH logins
- Daily reports of failed login attempts with top attackers
- GeoIP lookup for IP geolocation (optional)
- SQLite storage with configurable retention
- Systemd integration

## Requirements

- Go 1.21+
- Linux with systemd (Debian, Ubuntu, etc.)
- Telegram bot token and chat ID

## Quick Install

Run the following command to install oxiwatch:

```bash
curl -sSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh | sudo bash
```

The installer will:
- Download the latest release for your architecture
- Ask for your Telegram bot token and chat ID
- Ask if you want GeoIP geolocation enabled
- Create the configuration file
- Install and enable the systemd service

After installation, check the service status:

```bash
sudo systemctl status oxiwatch
```

## Installation (from source)

```bash
# Build
make build

# Install binary and create directories
sudo make install

# Create config file
sudo cp /etc/oxiwatch/config.json.example /etc/oxiwatch/config.json
sudo nano /etc/oxiwatch/config.json

# Install and enable systemd service
sudo make install-service
sudo systemctl enable oxiwatch
sudo systemctl start oxiwatch
```

## Configuration

Create `/etc/oxiwatch/config.json`:

```json
{
  "telegram_bot_token": "123456:ABC...",
  "telegram_chat_id": "-100123...",
  "server_name": "",
  "geoip_enabled": true,
  "geoip_database_path": "/var/lib/oxiwatch/dbip-city-lite.mmdb",
  "database_path": "/var/lib/oxiwatch/oxiwatch.db",
  "daily_report_enabled": true,
  "daily_report_time": "08:00",
  "daily_report_timezone": "UTC",
  "retention_days": 90,
  "log_level": "info"
}
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `telegram_bot_token` | Telegram bot token (required) | - |
| `telegram_chat_id` | Telegram chat ID (required) | - |
| `server_name` | Server name for notifications | hostname |
| `geoip_enabled` | Enable GeoIP lookup | true |
| `geoip_database_path` | Path to DB-IP database | /var/lib/oxiwatch/dbip-city-lite.mmdb |
| `database_path` | Path to SQLite database | /var/lib/oxiwatch/oxiwatch.db |
| `daily_report_enabled` | Enable daily reports | true |
| `daily_report_time` | Time to send daily report | 08:00 |
| `daily_report_timezone` | Timezone for daily report | UTC |
| `retention_days` | Days to keep records | 90 |
| `log_level` | Log level (debug, info, warn, error) | info |

All options can be overridden via environment variables with `OXIWATCH_` prefix (e.g., `OXIWATCH_TELEGRAM_BOT_TOKEN`).

## Usage

```bash
# Run daemon in foreground
oxiwatch daemon -f

# Show today's statistics
oxiwatch stats today

# Generate report for last 7 days
oxiwatch stats report -d 7

# Show successful logins
oxiwatch stats logins -d 30

# Update GeoIP database
oxiwatch geoip update

# Show GeoIP database status
oxiwatch geoip status

# Run retention cleanup manually
oxiwatch cleanup

# Validate configuration
oxiwatch config validate

# Show active configuration (secrets masked)
oxiwatch config show

# Send test Telegram message
oxiwatch send-test

# Show version
oxiwatch version
```

## GeoIP Setup

OxiWatch uses DB-IP Lite database for IP geolocation. No registration or license key required.

The database will be downloaded automatically on first run and updated monthly (on the last day of each month).

To manually update the database:

```bash
oxiwatch geoip update
```

To check the database status:

```bash
oxiwatch geoip status
```

## Telegram Bot Setup

1. Create a bot with [@BotFather](https://t.me/BotFather)
2. Get your chat ID (send a message to [@userinfobot](https://t.me/userinfobot))
3. Add the bot token and chat ID to your config
4. Test with `oxiwatch send-test`

## License

MIT
