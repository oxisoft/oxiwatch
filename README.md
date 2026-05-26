<div align="center">

<img src="docs/assets/banner.png" alt="OxiWatch — SSH login monitor with Telegram alerts for Linux" width="100%">

# OxiWatch

### SSH login monitor with instant Telegram alerts for Linux servers

Know the moment someone logs into your server over SSH — and get a daily report of every brute-force attempt that didn't.

[![Latest release](https://img.shields.io/github/v/release/oxisoft/oxiwatch?sort=semver&color=2ea44f)](https://github.com/oxisoft/oxiwatch/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Platform](https://img.shields.io/badge/platform-Linux%20%2F%20systemd-333)](#requirements)

**[Website](https://oxiwatch.oxisoft.io) · [Install](#quick-install) · [Configuration](#configuration) · [FAQ](#faq)**

</div>

---

**OxiWatch** is a lightweight, self-hosted SSH login monitor for Debian and Ubuntu servers. It watches the systemd journal in real time and sends an **instant Telegram notification** every time someone successfully logs in over SSH. Each morning it delivers a **daily report of failed SSH login attempts** — who tried, how often, and where they came from (GeoIP) — so you can spot brute-force attacks at a glance.

It's a single Go binary, runs as a systemd service, stores history in SQLite, and needs nothing more than a Telegram bot to start alerting.

<div align="center">
<img src="docs/assets/telegram-alert.png" alt="OxiWatch SSH login alert and daily failed-attempt report in Telegram" width="420">
</div>

## Table of contents

- [Why OxiWatch](#why-oxiwatch)
- [Features](#features)
- [OxiWatch and fail2ban](#oxiwatch-and-fail2ban)
- [Requirements](#requirements)
- [Quick install](#quick-install)
- [Upgrading](#upgrading)
- [Install from source](#install-from-source)
- [Configuration](#configuration)
- [Usage](#usage)
- [GeoIP setup](#geoip-setup)
- [Telegram bot setup](#telegram-bot-setup)
- [FAQ](#faq)
- [About](#about)

## Why OxiWatch

If you run a Linux server exposed to the internet, SSH is constantly probed. Most of the time you only find out something happened when you go digging through `journalctl` or `/var/log/auth.log` after the fact. OxiWatch flips that around: it tells you *as it happens*.

Use OxiWatch to:

- **Get a Telegram alert the instant someone logs in over SSH** — including the username, source IP, and country.
- **Monitor SSH logins across one or many Linux servers** from a single Telegram chat.
- **Catch SSH brute-force attacks** with a daily summary of failed login attempts and your top attackers.
- **See where login attempts come from** with built-in GeoIP geolocation (no API key required).
- **Keep an audit trail** of successful logins in a local SQLite database with configurable retention.
- **Replace fragile log-tailing scripts** with one maintained binary that survives reboots and updates itself.

If you've ever searched for "*how to get a Telegram notification on SSH login*", "*monitor SSH logins on a Linux server*", or "*alert me when someone SSHes into my server*" — that's exactly what OxiWatch does.

## Features

- **Real-time SSH login monitoring** via the systemd journal (`journalctl -u ssh`)
- **Instant Telegram alerts** for every successful SSH login
- **Daily reports of failed login attempts** with top attacker IPs and counts
- **GeoIP geolocation** of source IPs (optional, DB-IP Lite — no registration or license key)
- **SQLite storage** with configurable retention
- **systemd integration** — runs as a service, starts on boot
- **Self-upgrade** from GitHub releases (`oxiwatch upgrade`)
- **Single static binary** — no runtime dependencies
- Works on **Debian, Ubuntu, and other Debian-based** distributions

## OxiWatch and fail2ban

**OxiWatch is not a replacement for [fail2ban](https://github.com/fail2ban/fail2ban) — it complements it, and the two run happily on the same machine.**

| | fail2ban | OxiWatch |
|---|---|---|
| Role | **Prevention** — bans IPs after repeated failures | **Visibility** — alerts you to logins and summarizes attacks |
| Acts on | Failed attempts (firewall bans) | Successful logins (instant alert) + failed attempts (daily report) |
| Notifies you | Not out of the box | Telegram, in real time |
| Tells you *who got in* | No | Yes |

fail2ban blocks the brute-forcers; OxiWatch tells you the moment an authorized (or unauthorized) user actually gets a shell, and gives you the morning rundown of what was thrown at the box. Run both: fail2ban keeps attackers out, OxiWatch keeps you informed.

## Requirements

- Linux with **systemd** (tested on Debian 12/13; works on Ubuntu and other Debian-based systems)
- A **Telegram bot token and chat ID** ([setup below](#telegram-bot-setup))
- Go 1.21+ *(only if building from source)*

## Quick install

Run the following command to install OxiWatch:

```bash
curl -sSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh | sudo bash
```

**If you get a "Cannot read from terminal" error**, download and run separately:

```bash
curl -sSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh -o /tmp/install.sh
sudo bash /tmp/install.sh
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

## Upgrading

After the initial installation, OxiWatch can upgrade itself to the latest release:

```bash
sudo oxiwatch upgrade
sudo systemctl restart oxiwatch
```

Daily reports will notify you when a new version is available.

## Install from source

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

### Configuration options

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

All options can be overridden via environment variables with the `OXIWATCH_` prefix (e.g., `OXIWATCH_TELEGRAM_BOT_TOKEN`).

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

# Self-upgrade to latest release
sudo oxiwatch upgrade

# Show version
oxiwatch version
```

## GeoIP setup

OxiWatch uses the **DB-IP Lite** database for IP geolocation. No registration or license key is required.

The database is downloaded automatically on first run and updated monthly (on the last day of each month).

To manually update the database:

```bash
oxiwatch geoip update
```

To check the database status:

```bash
oxiwatch geoip status
```

## Telegram bot setup

1. Create a bot with [@BotFather](https://t.me/BotFather)
2. Get your chat ID (send a message to [@userinfobot](https://t.me/userinfobot))
3. Add the bot token and chat ID to your config
4. Test with `oxiwatch send-test`

## FAQ

### How do I get a Telegram notification when someone logs into my server over SSH?

Install OxiWatch, point it at a Telegram bot, and it sends an instant message on every successful SSH login — including the username, source IP, and country. See [Quick install](#quick-install).

### Does OxiWatch replace fail2ban?

No. fail2ban *blocks* repeated failed attempts; OxiWatch *tells you* who actually logged in and gives you a daily summary of attacks. They solve different problems and run together on the same machine. See [OxiWatch and fail2ban](#oxiwatch-and-fail2ban).

### Which Linux distributions are supported?

Any Debian-based distribution with systemd. It's tested on Debian 12 and 13 and works on Ubuntu and derivatives. SSH events are read from the systemd journal, so a `journald`-based system is required.

### Does it work with OpenSSH 10.0+?

Yes. OxiWatch tracks the `sshd`, `sshd-session`, and `sshd-auth` syslog identifiers, so successful logins and failed attempts are captured on newer OpenSSH releases as well.

### How does OxiWatch detect SSH logins?

It reads the systemd journal in real time (`journalctl -u ssh`) and parses sshd authentication messages — no log files to tail, no PAM hooks to install.

### Is a GeoIP API key required?

No. OxiWatch uses the free DB-IP Lite database and downloads/updates it automatically. No account, key, or license is needed.

### Does it monitor failed SSH login attempts too?

Yes. Failed attempts (including invalid users and pre-auth disconnects) are recorded and summarized in a daily Telegram report with the top attacker IPs.

### Can I monitor multiple servers in one Telegram chat?

Yes. Install OxiWatch on each server and point them at the same chat. Set `server_name` so each alert is clearly labeled.

### Where is login history stored?

In a local SQLite database (`/var/lib/oxiwatch/oxiwatch.db` by default) with configurable retention via `retention_days`.

### Is OxiWatch free and open source?

Yes — it's MIT licensed. See the [LICENSE](LICENSE).

## About

Developed by [OxiSoft](https://oxisoft.io) — we build robust backend systems with Go and Rust, and cross-platform apps with Flutter.

## License

[MIT](LICENSE)
