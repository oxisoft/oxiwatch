#!/bin/bash
# OxiWatch installer. Run as root:
#   curl -fsSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh | sudo bash
# Optional: OXIWATCH_VERSION=v0.4.5 installs that release instead of the latest.
set -euo pipefail
umask 077

REPO="oxisoft/oxiwatch"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/oxiwatch"
DATA_DIR="/var/lib/oxiwatch"
CURL=(curl -fsSL --proto '=https' --tlsv1.2 --retry 3)

die() {
  echo "ERROR: $*" >&2
  exit 1
}

# Check root
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  die "Please run as root (sudo)"
fi

for cmd in curl sha256sum install useradd grep; do
  command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
done

# Private work directory, removed on exit (no fixed /tmp paths).
WORK_DIR="$(mktemp -d)"
trap 'rm -rf -- "$WORK_DIR"' EXIT

# Check if interactive input is available
check_tty() {
  if ! { exec 3</dev/tty; } 2>/dev/null; then
    echo ""
    echo "ERROR: Cannot read from terminal."
    echo ""
    echo "This can happen when piping directly to bash. Try instead:"
    echo "  curl -fsSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh -o install.sh"
    echo "  sudo bash install.sh"
    echo ""
    exit 1
  fi
  exec 3<&-
}

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac

# Latest release, or the one pinned with OXIWATCH_VERSION
if [ -n "${OXIWATCH_VERSION:-}" ]; then
  LATEST="$OXIWATCH_VERSION"
else
  echo "Fetching latest release..."
  RELEASE_JSON="$("${CURL[@]}" "https://api.github.com/repos/$REPO/releases/latest")" \
    || die "could not query the latest release"
  LATEST="$(printf '%s\n' "$RELEASE_JSON" | grep -m1 '"tag_name"' | cut -d'"' -f4 || true)"
fi
if [[ ! "$LATEST" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "unexpected release tag: '$LATEST'"
fi
VERSION="${LATEST#v}"
ASSET="oxiwatch-linux-$ARCH"
BASE_URL="https://github.com/$REPO/releases/download/$LATEST"

# Download binary and checksums, verify before installing anything
echo "Downloading oxiwatch $VERSION for linux/$ARCH..."
"${CURL[@]}" -o "$WORK_DIR/$ASSET" "$BASE_URL/$ASSET" \
  || die "download failed: $BASE_URL/$ASSET"
"${CURL[@]}" -o "$WORK_DIR/checksums.txt" "$BASE_URL/checksums.txt" \
  || die "download failed: $BASE_URL/checksums.txt"

echo "Verifying SHA-256 checksum..."
grep -E "^[0-9a-f]{64}  ${ASSET}\$" "$WORK_DIR/checksums.txt" > "$WORK_DIR/checksum.sha256" \
  || die "no checksum for $ASSET in checksums.txt"
(cd "$WORK_DIR" && sha256sum -c --quiet checksum.sha256) \
  || die "checksum mismatch for $ASSET, refusing to install"
echo "Checksum OK."

# Create user/group (system account, no home, no login shell)
if ! id oxiwatch >/dev/null 2>&1; then
  NOLOGIN="/usr/sbin/nologin"
  [ -x "$NOLOGIN" ] || NOLOGIN="/bin/false"
  useradd --system --no-create-home --home-dir /nonexistent --shell "$NOLOGIN" oxiwatch
fi

# Directories: config readable only by root + oxiwatch, data owned by oxiwatch
install -d -m 0750 -o root -g oxiwatch "$CONFIG_DIR"
install -d -m 0750 -o oxiwatch -g oxiwatch "$DATA_DIR"

# Install binary
install -m 0755 -o root -g root "$WORK_DIR/$ASSET" "$INSTALL_DIR/oxiwatch"

# Validate bot token format (number:alphanumeric)
validate_token() {
  [[ $1 =~ ^[0-9]+:[A-Za-z0-9_-]+$ ]]
}

# Validate chat ID (numeric, can be negative for groups)
validate_chat_id() {
  [[ $1 =~ ^-?[0-9]+$ ]]
}

# Escape a value for safe inclusion in a JSON string: backslash, quote and
# every control character (newline, tab, ESC, ...).
json_escape() {
  local s="$1" out="" c i code
  for ((i = 0; i < ${#s}; i++)); do
    c="${s:i:1}"
    case "$c" in
      "\\") out+="\\\\" ;;
      '"') out+='\"' ;;
      $'\n') out+='\n' ;;
      $'\r') out+='\r' ;;
      $'\t') out+='\t' ;;
      *)
        LC_CTYPE=C printf -v code '%d' "'$c"
        if { [ "$code" -ge 0 ] && [ "$code" -lt 32 ]; } || [ "$code" -eq 127 ]; then
          printf -v c '\\u%04x' "$code"
        fi
        out+="$c"
        ;;
    esac
  done
  printf '%s' "$out"
}

# Ask a yes/no question (default No). Returns 0 for yes.
ask_yes_no() {
  local ans=""
  echo -n "$1 [y/N]: "
  read -r ans < /dev/tty || true
  [[ $ans =~ ^[Yy] ]]
}

# Accumulated channel config (JSON lines). Each block ends with a comma.
CHANNELS_JSON=""
append_channel() {
  if [ -n "$CHANNELS_JSON" ]; then
    CHANNELS_JSON="$CHANNELS_JSON
$1"
  else
    CHANNELS_JSON="$1"
  fi
}

configure_telegram() {
  local token chat
  while true; do
    echo -n "  Telegram Bot Token (input hidden): "
    read -rs token < /dev/tty; echo
    if [ -z "$token" ]; then echo "  Error: cannot be empty"; continue; fi
    if ! validate_token "$token"; then echo "  Error: invalid format (expected 123456789:ABCdef...)"; continue; fi
    break
  done
  while true; do
    echo -n "  Telegram Chat ID: "
    read -r chat < /dev/tty
    if [ -z "$chat" ]; then echo "  Error: cannot be empty"; continue; fi
    if ! validate_chat_id "$chat"; then echo "  Error: must be numeric (e.g. 123456789 or -100123456789)"; continue; fi
    break
  done
  append_channel "  \"telegram_bot_token\": \"$(json_escape "$token")\",
  \"telegram_chat_id\": \"$(json_escape "$chat")\","
}

configure_matrix() {
  local hs room token
  while true; do
    echo -n "  Matrix Homeserver URL (e.g. https://chat.example.ch): "
    read -r hs < /dev/tty
    if [ -z "$hs" ]; then echo "  Error: cannot be empty"; continue; fi
    if [[ ! $hs =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?/?$ ]]; then
      echo "  Error: must be an https:// URL without a path (e.g. https://chat.example.ch)"; continue
    fi
    break
  done
  while true; do
    echo -n "  Matrix Room ID (e.g. !abcd:chat.example.ch): "
    read -r room < /dev/tty
    if [ -z "$room" ]; then echo "  Error: cannot be empty"; continue; fi
    if [[ ! $room =~ ^![^[:space:]:]+:[^[:space:]]+$ ]]; then echo "  Error: expected !id:server"; continue; fi
    break
  done
  while true; do
    echo -n "  Matrix Access Token (input hidden): "
    read -rs token < /dev/tty; echo
    [ -n "$token" ] && break; echo "  Error: cannot be empty"
  done
  append_channel "  \"matrix_homeserver\": \"$(json_escape "$hs")\",
  \"matrix_room_id\": \"$(json_escape "$room")\",
  \"matrix_access_token\": \"$(json_escape "$token")\","
}

configure_email() {
  local smtp from to user pass to_json rcpt first
  while true; do
    echo -n "  SMTP URL (e.g. smtp://mail.example.ch:587): "
    read -r smtp < /dev/tty
    [ -n "$smtp" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  From address: "
    read -r from < /dev/tty
    [ -n "$from" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  Recipient(s), comma-separated: "
    read -r to < /dev/tty
    [ -n "$to" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  SMTP username: "
    read -r user < /dev/tty
    [ -n "$user" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  SMTP password (input hidden): "
    read -rs pass < /dev/tty; echo
    [ -n "$pass" ] && break; echo "  Error: cannot be empty"
  done
  # Build a JSON array from the comma-separated recipient list.
  to_json=""
  first=1
  local -a rcpts
  IFS=',' read -r -a rcpts <<< "$to"
  for rcpt in "${rcpts[@]}"; do
    rcpt="$(printf '%s' "$rcpt" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    [ -z "$rcpt" ] && continue
    if [ "$first" -eq 1 ]; then
      to_json="\"$(json_escape "$rcpt")\""; first=0
    else
      to_json="$to_json, \"$(json_escape "$rcpt")\""
    fi
  done
  append_channel "  \"email_smtp_url\": \"$(json_escape "$smtp")\",
  \"email_from\": \"$(json_escape "$from")\",
  \"email_to\": [$to_json],
  \"email_username\": \"$(json_escape "$user")\",
  \"email_password\": \"$(json_escape "$pass")\","
}

# Interactive configuration
check_tty
echo ""
echo "=== OxiWatch Configuration ==="
echo ""
echo "Set up your notification channels. At least one must be enabled."
echo ""

while true; do
  if ask_yes_no "Configure Telegram?"; then configure_telegram; fi
  if ask_yes_no "Configure Matrix?";   then configure_matrix;   fi
  if ask_yes_no "Configure Email?";    then configure_email;    fi

  if [ -n "$CHANNELS_JSON" ]; then
    break
  fi

  echo ""
  echo "No channel configured. You must enable at least one. Let's try again."
  echo ""
done

echo ""
echo -n "Enable GeoIP lookup? [Y/n]: "
GEOIP_ENABLED=""
read -r GEOIP_ENABLED < /dev/tty || true
GEOIP_ENABLED=${GEOIP_ENABLED:-Y}
if [[ $GEOIP_ENABLED =~ ^[Yy] ]]; then GEOIP_ENABLED="true"; else GEOIP_ENABLED="false"; fi

echo ""
METRICS_ENABLED="false"
METRICS_LISTEN="127.0.0.1:9184"
if ask_yes_no "Expose Prometheus metrics?"; then
  METRICS_ENABLED="true"
  while true; do
    METRICS_LISTEN_INPUT=""
    echo -n "  Metrics listen address [${METRICS_LISTEN}]: "
    read -r METRICS_LISTEN_INPUT < /dev/tty || true
    METRICS_LISTEN_INPUT=${METRICS_LISTEN_INPUT:-$METRICS_LISTEN}
    if [[ $METRICS_LISTEN_INPUT =~ ^[][A-Za-z0-9.:-]*:[0-9]{1,5}$ ]]; then
      METRICS_LISTEN="$METRICS_LISTEN_INPUT"; break
    fi
    echo "  Error: expected host:port (e.g. 127.0.0.1:9184)"
  done
fi

# Generate config. It is written into the private work dir (umask 077) and
# installed 0600 oxiwatch:oxiwatch, so the secrets (bot token, Matrix access
# token, SMTP password) are never readable by other users, not even briefly.
SERVER_NAME="$(hostname 2>/dev/null || echo oxiwatch)"
cat > "$WORK_DIR/config.json" << EOF
{
$CHANNELS_JSON
  "server_name": "$(json_escape "$SERVER_NAME")",
  "geoip_enabled": $GEOIP_ENABLED,
  "geoip_database_path": "/var/lib/oxiwatch/dbip-city-lite.mmdb",
  "database_path": "/var/lib/oxiwatch/oxiwatch.db",
  "daily_report_enabled": true,
  "daily_report_time": "08:00",
  "daily_report_timezone": "UTC",
  "retention_days": 90,
  "log_level": "info",
  "metrics_enabled": $METRICS_ENABLED,
  "metrics_listen": "$(json_escape "$METRICS_LISTEN")"
}
EOF
install -m 0600 -o oxiwatch -g oxiwatch "$WORK_DIR/config.json" "$CONFIG_DIR/config.json"

# Install systemd service
cat > "$WORK_DIR/oxiwatch.service" << 'EOF'
[Unit]
Description=OxiWatch SSH Login Monitor
After=network.target

[Service]
Type=simple
User=oxiwatch
Group=oxiwatch
SupplementaryGroups=systemd-journal
ExecStart=/usr/local/bin/oxiwatch daemon --foreground
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5

# --- Hardening ---
NoNewPrivileges=true
CapabilityBoundingSet=
AmbientCapabilities=
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/oxiwatch
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
ProtectClock=true
ProtectHostname=true
ProtectProc=invisible
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
install -m 0644 -o root -g root "$WORK_DIR/oxiwatch.service" /etc/systemd/system/oxiwatch.service

HAVE_SYSTEMD=0
if [ -d /run/systemd/system ]; then
  HAVE_SYSTEMD=1
  systemctl daemon-reload
  systemctl enable oxiwatch
else
  echo "systemd is not running here: unit installed, enable it later with: systemctl enable --now oxiwatch"
fi

echo ""
echo "=== Installation Complete ==="
echo "Binary: $INSTALL_DIR/oxiwatch"
echo "Config: $CONFIG_DIR/config.json"
echo "Data:   $DATA_DIR/"
if [ "$METRICS_ENABLED" = "true" ]; then
  echo "Metrics: http://$METRICS_LISTEN/metrics"
fi
if [ "$HAVE_SYSTEMD" -eq 1 ]; then
  echo ""
  echo -n "Start oxiwatch service now? [Y/n]: "
  START_NOW=""
  read -r START_NOW < /dev/tty || true
  START_NOW=${START_NOW:-Y}
  if [[ $START_NOW =~ ^[Yy] ]]; then
    systemctl start oxiwatch
    echo "Service started. Check status with: systemctl status oxiwatch"
  fi
fi
