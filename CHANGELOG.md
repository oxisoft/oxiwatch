# Changelog

## v0.3.0 - 2026-05-29

### Added
- Universal notification architecture: a channel-agnostic `Notifier` interface with a manager that formats each message once and fans it out to every configured channel (adding a new channel is now a single file)
- Matrix notification channel via the client-server API (`m.room.message`), sending both a plain-text body and an HTML formatted body
- Email (SMTP) notification channel: HTML mail over STARTTLS on the submission port, with support for multiple recipients
- Per-channel `telegram_enabled` / `matrix_enabled` / `email_enabled` toggles to disable or re-enable a channel without clearing its credentials (and matching `OXIWATCH_*_ENABLED` environment overrides)
- Full example configuration at `docs/config.json.example`, installed to `/etc/oxiwatch/config.json.example`

### Changed
- `send-test` now delivers to every configured channel and reports which ones were used
- Interactive installer now loops over Telegram, Matrix and Email, prompting only for the channels you choose to set up
- Daemon now requires at least one notification channel to be configured and enabled, and fails on start otherwise

## v0.2.8 - 2026-03-25

### Fixed
- Failed SSH login attempts not being captured on systems running OpenSSH 10.0+ (added `sshd-auth` to journal syslog identifier filter)
- Daily report showing raw MarkdownV2 escape characters in Telegram (switched report formatting from MarkdownV2 to HTML to match the notifier's parse mode)
- Failed attempts not counted when attackers disconnect without password attempt (added parsing for `Invalid user` and pre-auth disconnect messages)
