# Changelog

## v0.2.7 - 2026-03-25

### Fixed
- Failed SSH login attempts not being captured on systems running OpenSSH 10.0+ (added `sshd-auth` to journal syslog identifier filter)
- Daily report showing raw MarkdownV2 escape characters in Telegram (switched report formatting from MarkdownV2 to HTML to match the notifier's parse mode)
