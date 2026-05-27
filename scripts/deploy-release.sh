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
  --kiosk-user USER   Remote user running Chromium kiosk. Default: SSH user, auto-detect, or dr600ab-kiosk when SSH user is root
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

detect_kiosk_user() {
  local candidate detected
  for candidate in ask peite orangepi pi ubuntu debian; do
    if id "$candidate" >/dev/null 2>&1; then
      echo "$candidate"
      return 0
    fi
  done
  detected="$(getent passwd | awk -F: '($3 >= 1000 && $3 < 60000 && $7 !~ /(nologin|false)$/) { print $1; exit }')"
  if [[ -n "$detected" ]]; then
    echo "$detected"
    return 0
  fi
  return 1
}

if [[ "$KIOSK_USER" == "root" ]]; then
  if detected_kiosk_user="$(detect_kiosk_user)"; then
    if [[ "$KIOSK_USER_EXPLICIT" == "true" ]]; then
      echo "Chromium kiosk cannot use root; detected kiosk user: $detected_kiosk_user"
    fi
    KIOSK_USER="$detected_kiosk_user"
  else
    KIOSK_USER="dr600ab-kiosk"
    if [[ "$KIOSK_USER_EXPLICIT" == "true" ]]; then
      echo "Chromium kiosk cannot use root; creating kiosk user: $KIOSK_USER"
    fi
  fi
fi
if ! id "$KIOSK_USER" >/dev/null 2>&1 && [[ "$KIOSK_USER" == "dr600ab-kiosk" ]]; then
  "${SUDO[@]}" useradd -m -s /usr/sbin/nologin "$KIOSK_USER"
fi
if ! id "$KIOSK_USER" >/dev/null 2>&1; then
  echo "Kiosk user not found: $KIOSK_USER. Pass --kiosk-user USER." >&2
  exit 1
fi
if [[ "$KIOSK_USER" == "root" ]]; then
  echo "Chromium kiosk cannot use root. Pass --kiosk-user USER for a non-root desktop user." >&2
  exit 1
fi

KIOSK_HOME="$(getent passwd "$KIOSK_USER" | cut -d: -f6)"
if [[ -z "$KIOSK_HOME" ]]; then
  KIOSK_HOME="/home/$KIOSK_USER"
fi
KIOSK_UID="$(id -u "$KIOSK_USER")"
CHROMIUM_RUNTIME_DIR="/run/dr600ab-kiosk"
CHROMIUM_USER_DATA_DIR="$CHROMIUM_RUNTIME_DIR/profile"
CHROMIUM_CACHE_DIR="$CHROMIUM_RUNTIME_DIR/cache"
LEGACY_CHROMIUM_USER_DATA_DIR="$KIOSK_HOME/.chromium-kiosk"
KIOSK_LAUNCHER="/usr/local/bin/dr600ab-kiosk-start"
AUTOSTART_DIR="$KIOSK_HOME/.config/autostart"
AUTOSTART_FILE="$AUTOSTART_DIR/dr600ab-kiosk.desktop"
SYSTEM_AUTOSTART_DIR="/etc/xdg/autostart"
SYSTEM_AUTOSTART_FILE="$SYSTEM_AUTOSTART_DIR/dr600ab-kiosk.desktop"

prepare_kiosk_xauthority() {
  local xauth_source=""
  for candidate in \
    "/run/user/$KIOSK_UID/gdm/Xauthority" \
    "/run/user/$KIOSK_UID/lightdm/Xauthority" \
    "/run/user/$KIOSK_UID/Xauthority" \
    "/var/run/lightdm/root/${DISPLAY_VALUE}" \
    "$KIOSK_HOME/.Xauthority" \
    "/root/.Xauthority"; do
    if [[ -f "$candidate" ]]; then
      xauth_source="$candidate"
      break
    fi
  done
  if [[ -z "$xauth_source" ]]; then
    echo "Xauthority not found for display $DISPLAY_VALUE. Kiosk service may need manual X access setup." >&2
    return 0
  fi
  if [[ "$xauth_source" != "$KIOSK_HOME/.Xauthority" ]]; then
    "${SUDO[@]}" install -D -m 0600 -o "$KIOSK_USER" -g "$KIOSK_USER" "$xauth_source" "$KIOSK_HOME/.Xauthority"
  else
    "${SUDO[@]}" chown "$KIOSK_USER:" "$KIOSK_HOME/.Xauthority"
    "${SUDO[@]}" chmod 0600 "$KIOSK_HOME/.Xauthority"
  fi
}

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

write_kiosk_desktop_file() {
  local desktop_file="$1"
  "${SUDO[@]}" tee "$desktop_file" >/dev/null <<EOF
[Desktop Entry]
Type=Application
Name=DR600AB Kiosk
Exec=env DR600AB_KIOSK_LOG=/tmp/dr600ab-kiosk.log $KIOSK_LAUNCHER
Terminal=false
X-GNOME-Autostart-enabled=true
EOF
  "${SUDO[@]}" chmod 0644 "$desktop_file" || true
}

install_user_autostart() {
  local target_user="$1"
  local target_home target_dir target_file
  if ! id "$target_user" >/dev/null 2>&1; then
    return 0
  fi
  target_home="$(getent passwd "$target_user" | cut -d: -f6)"
  if [[ -z "$target_home" ]]; then
    return 0
  fi
  target_dir="$target_home/.config/autostart"
  target_file="$target_dir/dr600ab-kiosk.desktop"
  "${SUDO[@]}" install -d -m 0755 "$target_dir"
  "${SUDO[@]}" chown "$target_user:" "$target_dir" || true
  write_kiosk_desktop_file "$target_file"
  "${SUDO[@]}" chown "$target_user:" "$target_file" || true
}

"${SUDO[@]}" install -d -m 0755 "$INSTALL_DIR"
"${SUDO[@]}" install -m 0755 "$REMOTE_TMP/dr600ab" "$INSTALL_DIR/dr600ab"
"${SUDO[@]}" install -d -m 0755 "$INSTALL_DIR/data"
"${SUDO[@]}" install -d -m 0755 "$INSTALL_DIR/static/map"
"${SUDO[@]}" chown -R "$SERVICE_USER:" "$INSTALL_DIR"
prepare_kiosk_xauthority

"${SUDO[@]}" tee "$INSTALL_DIR/dr600ab-kiosk-start" >/dev/null <<'KIOSK_EOF'
#!/bin/sh
set -eu

if [ -n "${DR600AB_KIOSK_LOG:-}" ] && [ "${DR600AB_KIOSK_LOGGED:-0}" != "1" ]; then
  export DR600AB_KIOSK_LOGGED=1
  exec "$0" "$@" >> "$DR600AB_KIOSK_LOG" 2>&1
fi

DISPLAY_VALUE="${DISPLAY_VALUE:-${DISPLAY:-:0}}"
KIOSK_HOME="${KIOSK_HOME:-$HOME}"
KIOSK_UID="${KIOSK_UID:-$(id -u)}"
if [ -z "${CHROMIUM_RUNTIME_DIR:-}" ]; then
  if [ -n "${XDG_RUNTIME_DIR:-}" ] && [ -d "$XDG_RUNTIME_DIR" ]; then
    CHROMIUM_RUNTIME_DIR="$XDG_RUNTIME_DIR/dr600ab-kiosk"
  elif [ -d "/run/user/$KIOSK_UID" ]; then
    CHROMIUM_RUNTIME_DIR="/run/user/$KIOSK_UID/dr600ab-kiosk"
  else
    CHROMIUM_RUNTIME_DIR="/tmp/dr600ab-kiosk-$KIOSK_UID"
  fi
fi
CHROMIUM_USER_DATA_DIR="${CHROMIUM_USER_DATA_DIR:-$CHROMIUM_RUNTIME_DIR/profile}"
CHROMIUM_CACHE_DIR="${CHROMIUM_CACHE_DIR:-$CHROMIUM_RUNTIME_DIR/cache}"
APP_URL="${APP_URL:-http://127.0.0.1:18080/#/screen}"

if [ -z "${CHROMIUM_CMD:-}" ]; then
  CHROMIUM_CMD="$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)"
fi
if [ -z "$CHROMIUM_CMD" ]; then
  echo "Chromium command not found. Install chromium/chromium-browser/google-chrome." >&2
  exit 1
fi

find_xauthority() {
  if [ -n "${XAUTHORITY:-}" ] && [ -r "$XAUTHORITY" ]; then
    echo "$XAUTHORITY"
    return 0
  fi
  for candidate in \
    "/run/user/$KIOSK_UID/gdm/Xauthority" \
    "/run/user/$KIOSK_UID/lightdm/Xauthority" \
    "/run/user/$KIOSK_UID/Xauthority" \
    "/var/run/lightdm/root/$DISPLAY_VALUE"; do
    if [ -r "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  for env_file in /proc/[0-9]*/environ; do
    [ -r "$env_file" ] || continue
    xauth="$(tr '\000' '\n' < "$env_file" 2>/dev/null | awk -F= '$1=="XAUTHORITY" {print substr($0, index($0,"=")+1); exit}')"
    if [ -n "$xauth" ] && [ -r "$xauth" ]; then
      echo "$xauth"
      return 0
    fi
  done
  for candidate in \
    "$KIOSK_HOME/.Xauthority" \
    "/root/.Xauthority"; do
    if [ -r "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}

mkdir -p "$CHROMIUM_USER_DATA_DIR" "$CHROMIUM_CACHE_DIR"
export DISPLAY="$DISPLAY_VALUE"
if [ -z "${XDG_RUNTIME_DIR:-}" ] || [ ! -d "$XDG_RUNTIME_DIR" ]; then
  export XDG_RUNTIME_DIR="$CHROMIUM_RUNTIME_DIR"
fi
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ] && [ -S "/run/user/$KIOSK_UID/bus" ]; then
  export DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$KIOSK_UID/bus"
fi
if xauth="$(find_xauthority)"; then
  export XAUTHORITY="$xauth"
else
  unset XAUTHORITY
  echo "Xauthority not found for display $DISPLAY_VALUE. Chromium may be unable to connect to the screen." >&2
fi
display_number="${DISPLAY_VALUE#:}"
display_number="${display_number%%.*}"
if [ ! -S "/tmp/.X11-unix/X$display_number" ]; then
  echo "X11 socket not found: /tmp/.X11-unix/X$display_number" >&2
fi

exec "$CHROMIUM_CMD" \
  --kiosk "$APP_URL" \
  --no-first-run \
  --noerrdialogs \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --disable-background-networking \
  --disable-component-update \
  --disable-extensions \
  --disable-gpu \
  --disable-gpu-shader-disk-cache \
  --disk-cache-size=1 \
  --media-cache-size=1 \
  --user-data-dir="$CHROMIUM_USER_DATA_DIR" \
  --disk-cache-dir="$CHROMIUM_CACHE_DIR"
KIOSK_EOF
"${SUDO[@]}" chmod 0755 "$INSTALL_DIR/dr600ab-kiosk-start"
"${SUDO[@]}" install -d -m 0755 /usr/local/bin
"${SUDO[@]}" ln -sf "$INSTALL_DIR/dr600ab-kiosk-start" "$KIOSK_LAUNCHER"

"${SUDO[@]}" install -d -m 0755 "$SYSTEM_AUTOSTART_DIR"
write_kiosk_desktop_file "$SYSTEM_AUTOSTART_FILE"
install_user_autostart "$KIOSK_USER"
getent passwd | awk -F: '($3 >= 1000 && $3 < 60000 && $7 !~ /(nologin|false)$/) { print $1 }' | while IFS= read -r desktop_user; do
  install_user_autostart "$desktop_user"
done

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
Environment=API_OFFLINE_MAP_PATH=$INSTALL_DIR/static/map
ExecStart=$INSTALL_DIR/dr600ab
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

"${SUDO[@]}" systemctl stop dr600ab-kiosk.service >/dev/null 2>&1 || true
clear_chromium_cache "$LEGACY_CHROMIUM_USER_DATA_DIR"
"${SUDO[@]}" rm -rf "$LEGACY_CHROMIUM_USER_DATA_DIR"
clear_chromium_cache "$CHROMIUM_USER_DATA_DIR"

"${SUDO[@]}" systemctl disable --now dr600ab-kiosk.service >/dev/null 2>&1 || true
"${SUDO[@]}" rm -f /etc/systemd/system/dr600ab-kiosk.service
"${SUDO[@]}" systemctl daemon-reload
"${SUDO[@]}" systemctl enable dr600ab.service
"${SUDO[@]}" systemctl restart dr600ab.service

echo "Installed backend: $INSTALL_DIR/dr600ab"
echo "Backend service user: $SERVICE_USER"
echo "Kiosk service user: $KIOSK_USER"
echo "Cleared Chromium cache: $CHROMIUM_USER_DATA_DIR"
echo "Screen URL: $APP_URL"
echo "Kiosk autostart: $AUTOSTART_FILE"
echo "Kiosk system autostart: $SYSTEM_AUTOSTART_FILE"
echo "Check status:"
echo "  sudo systemctl status dr600ab.service"
echo "  desktop autostart starts Chromium when $KIOSK_USER logs into the graphical session"
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
