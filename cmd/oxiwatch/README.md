# oxiwatch command

The `oxiwatch` binary: a self-hosted SSH login monitor for Debian and Ubuntu servers.
It sends an instant **Telegram, Matrix or email** alert on every successful SSH login
(user, source IP, country) and a daily report of failed attempts with GeoIP.

- **Install:** one-line installer, see [Quick install](../../README.md#quick-install), or
  `go install github.com/oxisoft/oxiwatch/cmd/oxiwatch@latest`
- **Configure:** [Configuration](../../README.md#configuration),
  [Telegram](../../README.md#telegram-bot-setup), [Matrix](../../README.md#matrix-setup),
  [email](../../README.md#email-setup)
- **Commands:** `oxiwatch help`, or the [command reference](../../README.md#usage)
- **Website:** [oxiwatch.oxisoft.io](https://oxiwatch.oxisoft.io)

This directory only holds the CLI entry point; the monitor itself lives in `internal/`.

Developed by [OxiSoft](https://oxisoft.io). MIT licensed.
