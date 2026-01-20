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

# Interactive configuration (read from /dev/tty for curl|bash compatibility)
echo ""
echo "=== OxiWatch Configuration ==="
echo ""
read -p "Telegram Bot Token: " TELEGRAM_TOKEN < /dev/tty
read -p "Telegram Chat ID: " TELEGRAM_CHAT_ID < /dev/tty
read -p "Enable GeoIP lookup? [Y/n]: " GEOIP_ENABLED < /dev/tty
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
read -p "Start oxiwatch service now? [Y/n]: " START_NOW < /dev/tty
START_NOW=${START_NOW:-Y}
if [[ $START_NOW =~ ^[Yy] ]]; then
  systemctl start oxiwatch
  echo "Service started. Check status with: systemctl status oxiwatch"
fi
