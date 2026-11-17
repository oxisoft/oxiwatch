// Command oxiwatch is a self-hosted SSH login monitor for Debian and Ubuntu
// servers.
//
// It follows the systemd journal in real time and sends an instant alert via
// Telegram, Matrix or email on every successful SSH login, with the user,
// source IP and country. Every morning it sends a report of the previous day's
// failed login attempts with GeoIP. A quiet list skips alerts for known
// addresses (office, home) while still counting those logins. History is kept
// in SQLite; optional Prometheus metrics come with a ready Grafana dashboard.
//
// # Install
//
// One-line installer (binary, systemd unit, config wizard):
//
//	curl -sSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh | sudo bash
//
// Or build from source:
//
//	go install github.com/oxisoft/oxiwatch/cmd/oxiwatch@latest
//
// # Usage
//
//	oxiwatch daemon               run the monitor (normally via systemd)
//	oxiwatch send-test            send a test message to every channel
//	oxiwatch stats report -d 7    failed-attempt report for the last 7 days
//	oxiwatch quiet add <ip|cidr>  add an address or range to the quiet list
//	oxiwatch help                 all commands
//
// Configuration lives in /etc/oxiwatch/config.json (OXIWATCH_CONFIG overrides
// the path; every option can be set with an OXIWATCH_ environment variable).
//
// Website: https://oxiwatch.oxisoft.io
//
// Developed by OxiSoft (https://oxisoft.io). MIT licensed.
package main
