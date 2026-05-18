#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-release.sh <ssh-target> [options]

Examples:
  scripts/deploy-release.sh ask@192.168.10.133
  scripts/deploy-release.sh root@192.168.100.201 --password '<ssh-password>'
  scripts/deploy-release.sh ask@192.168.10.133 --host 0.0.0.0 --port 18080 --display :0

Options:
  --binary PATH       Local binary path. Default: dist/packages/dr600ab-linux-arm64/dr600ab
  --install-dir PATH  Remote install directory. Default: /opt/dr600ab
  --service-user USER Remote user running backend service. Default: root
  --kiosk-user USER   Remote user running Chromium kiosk. Default: SSH user, or auto-detect when SSH user is root
  --host HOST         API bind host. Default: 0.0.0.0
  --port PORT         API bind port. Default: 18080
  --display DISPLAY   X display used by Chromium. Default: :0
  --chromium PATH     Chromium command. Default: auto-detect chromium/chromium-browser/google-chrome
  -password PASSWORD  SSH password. Requires sshpass.
  --password PASSWORD Same as -password.

Authentication:
  The script uploads the binary and installs services through one SSH command.
  If password authentication is used, enter the SSH password once or pass
  -password PASSWORD. Passing passwords on the command line may leave them in
  shell history and process listings.
EOF
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_BINARY_PATH="$ROOT_DIR/dist/packages/dr600ab-linux-arm64/dr600ab"
SSH_TARGET="${1:-}"
if [[ -z "$SSH_TARGET" || "$SSH_TARGET" == "-h" || "$SSH_TARGET" == "--help" ]]; then
  usage
  exit 0
fi
shift || true

BINARY_PATH="$DEFAULT_BINARY_PATH"
INSTALL_DIR="/opt/dr600ab"
SERVICE_USER="root"
SSH_USER="${SSH_TARGET%@*}"
KIOSK_USER="$SSH_USER"
KIOSK_USER_EXPLICIT=false
API_HOST="0.0.0.0"
API_PORT="18080"
DISPLAY_VALUE=":0"
CHROMIUM_CMD=""
SSH_PASSWORD=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)
      BINARY_PATH="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --service-user)
      SERVICE_USER="$2"
      shift 2
      ;;
    --kiosk-user)
      KIOSK_USER="$2"
      KIOSK_USER_EXPLICIT=true
      shift 2
      ;;
    --host)
      API_HOST="$2"
      shift 2
      ;;
    --port)
      API_PORT="$2"
      shift 2
      ;;
    --display)
      DISPLAY_VALUE="$2"
      shift 2
      ;;
    --chromium)
      CHROMIUM_CMD="$2"
      shift 2
      ;;
    -password|--password)
      SSH_PASSWORD="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ ! -f "$BINARY_PATH" ]]; then
  echo "Binary not found: $BINARY_PATH" >&2
  echo "Run ./scripts/build-release.sh --all first, or pass --binary PATH." >&2
  exit 1
fi

if [[ -n "$SSH_PASSWORD" ]] && ! command -v sshpass >/dev/null 2>&1; then
  echo "sshpass is required when using -password/--password." >&2
  echo "Install it first, for example: brew install hudochenkov/sshpass/sshpass" >&2
  exit 1
fi

APP_URL="http://127.0.0.1:${API_PORT}/#/screen"

shell_quote() {
  local value=${1//\'/\'\\\'\'}
  printf "'%s'" "$value"
}

PACKAGE_TMP="$(mktemp -d "${TMPDIR:-/tmp}/dr600ab-deploy.XXXXXX")"
cleanup_local() {
  rm -rf "$PACKAGE_TMP"
}
trap cleanup_local EXIT

cp "$BINARY_PATH" "$PACKAGE_TMP/dr600ab"

cat > "$PACKAGE_TMP/install.sh" <<'REMOTE'
set -euo pipefail

SUDO=(sudo)
if [[ "$(id -u)" -eq 0 ]]; then
  SUDO=()
fi

if [[ -z "$CHROMIUM_CMD" ]]; then
  CHROMIUM_CMD="$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)"
fi
if [[ -z "$CHROMIUM_CMD" ]]; then
  echo "Chromium command not found. Install chromium or pass --chromium PATH." >&2
  exit 1
fi
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "Service user not found: $SERVICE_USER" >&2
  exit 1
fi
if [[ "$KIOSK_USER" == "root" && "$KIOSK_USER_EXPLICIT" != "true" ]]; then
  for candidate in ask peite orangepi pi ubuntu debian; do
    if id "$candidate" >/dev/null 2>&1; then
      KIOSK_USER="$candidate"
      break
    fi
  done
fi
if ! id "$KIOSK_USER" >/dev/null 2>&1; then
  echo "Kiosk user not found: $KIOSK_USER. Pass --kiosk-user USER." >&2
  exit 1
fi
if [[ "$KIOSK_USER" == "root" ]]; then
  echo "Chromium kiosk should not run as root. Pass --kiosk-user USER." >&2
  exit 1
fi

KIOSK_HOME="$(getent passwd "$KIOSK_USER" | cut -d: -f6)"
if [[ -z "$KIOSK_HOME" ]]; then
  KIOSK_HOME="/home/$KIOSK_USER"
fi
CHROMIUM_USER_DATA_DIR="$KIOSK_HOME/.chromium-kiosk"

clear_chromium_cache() {
  local profile="$1"
  local cache_paths=(
    "$profile/Cache"
    "$profile/Code Cache"
    "$profile/GPUCache"
    "$profile/DawnCache"
    "$profile/GrShaderCache"
    "$profile/ShaderCache"
    "$profile/Default/Cache"
    "$profile/Default/Code Cache"
    "$profile/Default/GPUCache"
    "$profile/Default/DawnCache"
    "$profile/Default/GrShaderCache"
    "$profile/Default/ShaderCache"
    "$profile/Default/Service Worker/CacheStorage"
    "$profile/Default/Service Worker/ScriptCache"
    "$profile/Default/blob_storage"
  )
  "${SUDO[@]}" rm -rf "${cache_paths[@]}"
}

"${SUDO[@]}" install -d -m 0755 "$INSTALL_DIR"
"${SUDO[@]}" install -m 0755 "$REMOTE_TMP/dr600ab" "$INSTALL_DIR/dr600ab"
"${SUDO[@]}" install -d -m 0755 "$INSTALL_DIR/data"
"${SUDO[@]}" chown -R "$SERVICE_USER:" "$INSTALL_DIR"

"${SUDO[@]}" tee /etc/systemd/system/dr600ab.service >/dev/null <<EOF
[Unit]
Description=DR600AB Backend
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
Environment=API_ADDR=$API_HOST:$API_PORT
Environment=API_SETTINGS_PATH=$INSTALL_DIR/data/detection-settings.json
ExecStart=$INSTALL_DIR/dr600ab
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

"${SUDO[@]}" tee /etc/systemd/system/dr600ab-kiosk.service >/dev/null <<EOF
[Unit]
Description=DR600AB Chromium Kiosk
After=graphical.target dr600ab.service
Wants=dr600ab.service

[Service]
Type=simple
User=$KIOSK_USER
Environment=DISPLAY=$DISPLAY_VALUE
Environment=XAUTHORITY=$KIOSK_HOME/.Xauthority
ExecStart=$CHROMIUM_CMD --kiosk "$APP_URL" --no-first-run --noerrdialogs --disable-infobars --disable-session-crashed-bubble --disable-background-networking --disable-component-update --disable-extensions --user-data-dir=$CHROMIUM_USER_DATA_DIR
Restart=always
RestartSec=3

[Install]
WantedBy=graphical.target
EOF

"${SUDO[@]}" systemctl stop dr600ab-kiosk.service >/dev/null 2>&1 || true
clear_chromium_cache "$CHROMIUM_USER_DATA_DIR"

"${SUDO[@]}" systemctl daemon-reload
"${SUDO[@]}" systemctl enable dr600ab.service dr600ab-kiosk.service
"${SUDO[@]}" systemctl restart dr600ab.service
"${SUDO[@]}" systemctl restart dr600ab-kiosk.service

echo "Installed backend: $INSTALL_DIR/dr600ab"
echo "Backend service user: $SERVICE_USER"
echo "Kiosk service user: $KIOSK_USER"
echo "Cleared Chromium cache: $CHROMIUM_USER_DATA_DIR"
echo "Screen URL: $APP_URL"
echo "Check status:"
echo "  sudo systemctl status dr600ab.service"
echo "  sudo systemctl status dr600ab-kiosk.service"
REMOTE
chmod +x "$PACKAGE_TMP/install.sh"

REMOTE_COMMAND="set -eu; remote_tmp=\$(mktemp -d /tmp/dr600ab.XXXXXX); cleanup(){ rm -rf \"\$remote_tmp\"; }; trap cleanup EXIT; tar -xzf - -C \"\$remote_tmp\"; REMOTE_TMP=\"\$remote_tmp\" INSTALL_DIR=$(shell_quote "$INSTALL_DIR") SERVICE_USER=$(shell_quote "$SERVICE_USER") KIOSK_USER=$(shell_quote "$KIOSK_USER") KIOSK_USER_EXPLICIT=$(shell_quote "$KIOSK_USER_EXPLICIT") API_HOST=$(shell_quote "$API_HOST") API_PORT=$(shell_quote "$API_PORT") DISPLAY_VALUE=$(shell_quote "$DISPLAY_VALUE") APP_URL=$(shell_quote "$APP_URL") CHROMIUM_CMD=$(shell_quote "$CHROMIUM_CMD") bash \"\$remote_tmp/install.sh\""

SSH_CMD=(ssh)
if [[ -n "$SSH_PASSWORD" ]]; then
  SSH_CMD=(sshpass -e ssh -o PreferredAuthentications=password -o PubkeyAuthentication=no)
fi

echo "Uploading $BINARY_PATH and installing services on $SSH_TARGET..."
COPYFILE_DISABLE=1 tar --no-xattrs -czf - -C "$PACKAGE_TMP" . | SSHPASS="$SSH_PASSWORD" "${SSH_CMD[@]}" "$SSH_TARGET" "$REMOTE_COMMAND"

echo "Deployment completed."
