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

# Interactive configuration
check_tty
echo ""
echo "=== OxiWatch Configuration ==="
echo ""

while true; do
  echo -n "Telegram Bot Token: "
  read TELEGRAM_TOKEN < /dev/tty
  if [ -z "$TELEGRAM_TOKEN" ]; then
    echo "Error: Bot token cannot be empty"
    continue
  fi
  if ! validate_token "$TELEGRAM_TOKEN"; then
    echo "Error: Invalid bot token format (expected: 123456789:ABCdefGHI...)"
    continue
  fi
  break
done

while true; do
  echo -n "Telegram Chat ID: "
  read TELEGRAM_CHAT_ID < /dev/tty
  if [ -z "$TELEGRAM_CHAT_ID" ]; then
    echo "Error: Chat ID cannot be empty"
    continue
  fi
  if ! validate_chat_id "$TELEGRAM_CHAT_ID"; then
    echo "Error: Invalid chat ID (must be numeric, e.g., 123456789 or -100123456789)"
    continue
  fi
  break
done

echo -n "Enable GeoIP lookup? [Y/n]: "
read GEOIP_ENABLED < /dev/tty
GEOIP_ENABLED=${GEOIP_ENABLED:-Y}
[[ $GEOIP_ENABLED =~ ^[Yy] ]] && GEOIP_ENABLED="true" || GEOIP_ENABLED="false"

# Generate config
cat > "$CONFIG_DIR/config.json" << EOF
{
  "telegram_bot_token": "$TELEGRAM_TOKEN",
  "telegram_chat_id": "$TELEGRAM_CHAT_ID",
  "server_name": "$(hostname)",
  "geoip_enabled": $GEOIP_ENABLED,
  "geoip_database_path": "/var/lib/oxiwatch/dbip-city-lite.mmdb",
  "database_path": "/var/lib/oxiwatch/oxiwatch.db",
  "daily_report_enabled": true,
  "daily_report_time": "08:00",
  "daily_report_timezone": "UTC",
  "retention_days": 90,
  "log_level": "info"
}
EOF
chown oxiwatch:oxiwatch "$CONFIG_DIR/config.json"
chmod 644 "$CONFIG_DIR/config.json"

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
echo ""
echo -n "Start oxiwatch service now? [Y/n]: "
read START_NOW < /dev/tty
START_NOW=${START_NOW:-Y}
if [[ $START_NOW =~ ^[Yy] ]]; then
  systemctl start oxiwatch
  echo "Service started. Check status with: systemctl status oxiwatch"
fi
