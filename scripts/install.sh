#!/bin/bash
set -e

REPO="oxisoft/oxiwatch"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/oxiwatch"
DATA_DIR="/var/lib/oxiwatch"

# Check root
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root (sudo)"
  exit 1
fi

# Check if interactive input is available
check_tty() {
  if ! exec 3</dev/tty 2>/dev/null; then
    echo ""
    echo "ERROR: Cannot read from terminal."
    echo ""
    echo "This can happen when piping directly to bash. Try instead:"
    echo "  curl -sSL https://raw.githubusercontent.com/oxisoft/oxiwatch/main/scripts/install.sh -o /tmp/install.sh"
    echo "  sudo bash /tmp/install.sh"
    echo ""
    exit 1
  fi
  exec 3<&-
}

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release
echo "Fetching latest release..."
LATEST=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep tag_name | cut -d'"' -f4)
VERSION=${LATEST#v}
BINARY_URL="https://github.com/$REPO/releases/download/$LATEST/oxiwatch-linux-$ARCH"

# Download binary
echo "Downloading oxiwatch $VERSION for linux/$ARCH..."
curl -L -o /tmp/oxiwatch "$BINARY_URL"
chmod +x /tmp/oxiwatch

# Create user/group
if ! id oxiwatch &>/dev/null; then
  useradd -r -s /bin/false oxiwatch
fi

# Create directories
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chown oxiwatch:oxiwatch "$DATA_DIR"

# Install binary
mv /tmp/oxiwatch "$INSTALL_DIR/oxiwatch"

# Validate bot token format (number:alphanumeric)
validate_token() {
  if [[ ! $1 =~ ^[0-9]+:[A-Za-z0-9_-]+$ ]]; then
    return 1
  fi
  return 0
}

# Validate chat ID (numeric, can be negative for groups)
validate_chat_id() {
  if [[ ! $1 =~ ^-?[0-9]+$ ]]; then
    return 1
  fi
  return 0
}

# Escape a value for safe inclusion in a JSON string
json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# Ask a yes/no question (default No). Returns 0 for yes.
ask_yes_no() {
  local ans
  echo -n "$1 [y/N]: "
  read ans < /dev/tty
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
    echo -n "  Telegram Bot Token: "
    read token < /dev/tty
    if [ -z "$token" ]; then echo "  Error: cannot be empty"; continue; fi
    if ! validate_token "$token"; then echo "  Error: invalid format (expected 123456789:ABCdef...)"; continue; fi
    break
  done
  while true; do
    echo -n "  Telegram Chat ID: "
    read chat < /dev/tty
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
    read hs < /dev/tty
    [ -n "$hs" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  Matrix Room ID (e.g. !abcd:chat.example.ch): "
    read room < /dev/tty
    [ -n "$room" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  Matrix Access Token: "
    read token < /dev/tty
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
    read smtp < /dev/tty
    [ -n "$smtp" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  From address: "
    read from < /dev/tty
    [ -n "$from" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  Recipient(s), comma-separated: "
    read to < /dev/tty
    [ -n "$to" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  SMTP username: "
    read user < /dev/tty
    [ -n "$user" ] && break; echo "  Error: cannot be empty"
  done
  while true; do
    echo -n "  SMTP password: "
    read -s pass < /dev/tty; echo
    [ -n "$pass" ] && break; echo "  Error: cannot be empty"
  done
  # Build a JSON array from the comma-separated recipient list.
  to_json=""
  first=1
  local IFS=','
  for rcpt in $to; do
    rcpt="$(echo "$rcpt" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
    [ -z "$rcpt" ] && continue
    if [ $first -eq 1 ]; then
      to_json="\"$(json_escape "$rcpt")\""; first=0
    else
      to_json="$to_json, \"$(json_escape "$rcpt")\""
    fi
  done
  unset IFS
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
  echo "No channel configured — you must enable at least one. Let's try again."
  echo ""
done

echo ""
echo -n "Enable GeoIP lookup? [Y/n]: "
read GEOIP_ENABLED < /dev/tty
GEOIP_ENABLED=${GEOIP_ENABLED:-Y}
[[ $GEOIP_ENABLED =~ ^[Yy] ]] && GEOIP_ENABLED="true" || GEOIP_ENABLED="false"

echo ""
METRICS_ENABLED="false"
METRICS_LISTEN="127.0.0.1:9184"
if ask_yes_no "Expose Prometheus metrics?"; then
  METRICS_ENABLED="true"
  echo -n "  Metrics listen address [${METRICS_LISTEN}]: "
  read METRICS_LISTEN_INPUT < /dev/tty
  METRICS_LISTEN=${METRICS_LISTEN_INPUT:-$METRICS_LISTEN}
fi

# Generate config
cat > "$CONFIG_DIR/config.json" << EOF
{
$CHANNELS_JSON
  "server_name": "$(hostname)",
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
chown oxiwatch:oxiwatch "$CONFIG_DIR/config.json"
# Secrets live here (bot tokens, Matrix access token, SMTP password) — keep it
# readable only by the oxiwatch service user (and root), not world-readable.
chmod 600 "$CONFIG_DIR/config.json"

# Install systemd service
cat > /etc/systemd/system/oxiwatch.service << 'EOF'
[Unit]
Description=OxiWatch SSH Login Monitor
After=network.target

[Service]
Type=simple
User=oxiwatch
Group=oxiwatch
SupplementaryGroups=systemd-journal
ExecStart=/usr/local/bin/oxiwatch daemon --foreground
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

systemctl daemon-reload
systemctl enable oxiwatch

echo ""
echo "=== Installation Complete ==="
echo "Binary: $INSTALL_DIR/oxiwatch"
echo "Config: $CONFIG_DIR/config.json"
echo "Data:   $DATA_DIR/"
if [ "$METRICS_ENABLED" = "true" ]; then
  echo "Metrics: http://$METRICS_LISTEN/metrics"
fi
echo ""
echo -n "Start oxiwatch service now? [Y/n]: "
read START_NOW < /dev/tty
START_NOW=${START_NOW:-Y}
if [[ $START_NOW =~ ^[Yy] ]]; then
  systemctl start oxiwatch
  echo "Service started. Check status with: systemctl status oxiwatch"
fi
