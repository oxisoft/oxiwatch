# Changelog

## v0.4.2 - 2026-05-31

### Fixed
- GeoIP auto-update could leave a **corrupt/truncated database** (locations stopped resolving): the gzip was extracted directly over the live database file, so a partial or short download destroyed the working copy. Updates now extract to a temporary file, verify it opens as a valid mmdb, and only then atomically replace the live database, so a bad download can no longer corrupt it.
- Detect truncated downloads by comparing the received size against `Content-Length`.
- GeoIP downloads no longer use a single whole-request timeout (which aborted the ~60 MB download on slow links); connection/response-header timeouts are used instead so slow-but-progressing downloads complete.

### Changed
- Matrix messages now start with a horizontal rule so consecutive bot messages are clearly separated in clients that group messages from the same sender.

### Added
- `oxiwatch geoip lookup <ip>`: verbose, post-install troubleshooting for location detection. Shows the database path/size/build date, whether it opens, the resolved country/city, and the full decoded record.

## v0.4.0 - 2026-05-31

### Added
- Prometheus metrics exporter: opt-in `/metrics` HTTP endpoint configured via `metrics_enabled` and `metrics_listen` (default `127.0.0.1:9184`), with matching `OXIWATCH_METRICS_ENABLED` / `OXIWATCH_METRICS_LISTEN` environment overrides
- Every `oxiwatch_*` metric carries a constant `server` label (from `server_name`) so a single Prometheus can scrape many hosts and distinguish them
- Metrics exposed: `oxiwatch_ssh_login_attempts_total{result,method}`, `oxiwatch_ssh_invalid_user_attempts_total`, `oxiwatch_ssh_attempts_by_country_total{country}`, `oxiwatch_build_info{version}`, and `oxiwatch_start_time_seconds`
- Example Grafana dashboard at `docs/grafana-dashboard.json`
- Unit test suite across all packages (parser, config, storage, notifier, metrics, report, scheduler, journal, geoip, daemon, version); core logic packages now 80-100% covered
- GitHub Actions CI (`go vet` + `go test -race` with coverage) that auto-updates the README coverage badge; new `make test-race` / `make cover` / `make cover-html` targets

### Security
- Installer now writes `/etc/oxiwatch/config.json` as `0600` (was `0644`) so bot tokens, the Matrix access token, and the SMTP password are no longer world-readable
- Matrix access token is now sent in an `Authorization: Bearer` header instead of the URL query string, preventing it from leaking into error logs, homeserver access logs, or proxies
- Hardened the systemd unit (`NoNewPrivileges`, `ProtectSystem=strict`, empty capability set, `SystemCallFilter=@system-service`, namespace/address-family restrictions, etc.)
- Metrics HTTP server now sets read/write/idle timeouts to resist slow-client resource exhaustion
- GeoIP downloads use a bounded HTTP timeout so a hung server can't block daemon startup
- Journal reader tolerates long lines (1 MB) instead of aborting and stopping monitoring

### Fixed
- Matrix messages rendered on a single line because HTML ignores newlines. The formatted body now converts newlines to `<br>` (the plain-text body is unchanged)

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
