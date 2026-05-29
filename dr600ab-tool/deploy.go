package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DeployRequest struct {
	InstallDir   string `json:"installDir"`
	FirmwarePath string `json:"firmwarePath"`
	FullUpdate   bool   `json:"fullUpdate"`
}

type DeployResult struct {
	InstallDir string `json:"installDir"`
	Message    string `json:"message"`
}

const (
	defaultServiceUser = "root"
	defaultAPIHost     = "0.0.0.0"
	defaultAPIPort     = "18080"
	defaultDisplay     = ":0"
)

func (a *App) DeployDR600AB(req DeployRequest) (DeployResult, error) {
	req.InstallDir = a.getInstallDir(req.InstallDir)
	req.FirmwarePath = strings.TrimSpace(req.FirmwarePath)
	if req.FirmwarePath == "" {
		return DeployResult{}, fmt.Errorf("请选择固件包")
	}
	if err := validateFirmwarePackagePath(req.FirmwarePath); err != nil {
		return DeployResult{}, err
	}
	info, err := os.Stat(req.FirmwarePath)
	if err != nil {
		return DeployResult{}, fmt.Errorf("读取固件包失败: %w", err)
	}
	if info.IsDir() {
		return DeployResult{}, fmt.Errorf("固件包不能是目录")
	}
	if _, err := a.getSSHClient(); err != nil {
		return DeployResult{}, err
	}

	a.updateConfig(func(cfg *AppConfig) {
		cfg.InstallDir = req.InstallDir
		cfg.Firmware = req.FirmwarePath
	})

	taskDir := fmt.Sprintf("/tmp/dr600ab-tool-%d", time.Now().UnixNano())
	remotePackage := remoteJoin(taskDir, filepath.Base(req.FirmwarePath))
	a.emitProgress("deploy-progress", 0, "准备部署", "正在创建远程临时目录", "running", 0, nil)
	if _, err := a.runCommand("mkdir -p " + shellQuote(taskDir)); err != nil {
		a.emitProgress("deploy-progress", 0, "准备部署", "创建远程临时目录失败", "error", 0, err)
		return DeployResult{}, err
	}
	a.emitProgress("deploy-progress", 0, "准备部署", "远程临时目录已创建", "success", 100, nil)

	a.emitProgress("deploy-progress", 1, "上传固件包", "正在上传 "+filepath.Base(req.FirmwarePath), "running", 0, nil)
	if err := a.uploadFile(req.FirmwarePath, remotePackage, func(read, total int64) {
		progress := 0
		if total > 0 {
			progress = int(float64(read) / float64(total) * 100)
		}
		a.emitProgress("deploy-progress", 1, "上传固件包", fmt.Sprintf("已上传 %s / %s", formatBytes(read), formatBytes(total)), "running", progress, nil)
	}); err != nil {
		a.emitProgress("deploy-progress", 1, "上传固件包", "上传失败", "error", 0, err)
		return DeployResult{}, err
	}
	a.emitProgress("deploy-progress", 1, "上传固件包", "上传完成", "success", 100, nil)

	a.emitProgress("deploy-progress", 2, "安装服务", "正在安装 DR600AB 与屏幕自启动", "running", 20, nil)
	script := buildDeployScript(req, remotePackage, taskDir, a.currentSSHUser())
	output, err := a.runCommand(script)
	if err != nil {
		wrapped := fmt.Errorf("%w%s", err, commandOutputSuffix(output))
		a.emitProgress("deploy-progress", 2, "安装服务", "安装失败", "error", 20, wrapped)
		return DeployResult{}, wrapped
	}
	a.emitProgress("deploy-progress", 2, "安装服务", "DR600AB 与屏幕自启动安装完成", "success", 100, nil)
	a.emitProgress("deploy-progress", 3, "启动服务", "DR600AB 服务已启动", "success", 100, nil)
	return DeployResult{InstallDir: req.InstallDir, Message: "部署完成"}, nil
}

func (a *App) currentSSHUser() string {
	a.sshMu.Lock()
	defer a.sshMu.Unlock()
	if a.conn == nil {
		return ""
	}
	return strings.TrimSpace(a.conn.config.User)
}

func buildDeployScript(req DeployRequest, remotePackage, taskDir, sshUser string) string {
	full := "0"
	if req.FullUpdate {
		full = "1"
	}
	if strings.TrimSpace(sshUser) == "" {
		sshUser = defaultServiceUser
	}
	return fmt.Sprintf(`set -eu
REMOTE_PACKAGE=%s
INSTALL_DIR=%s
TASK_DIR=%s
FULL_UPDATE=%s
SERVICE_USER=%s
KIOSK_USER=%s
API_HOST=%s
API_PORT=%s
DISPLAY_VALUE=%s
APP_URL="http://127.0.0.1:$API_PORT/#/screen"
CHROMIUM_CMD=
SUDO=
if [ "$(id -u)" != "0" ]; then
  SUDO=sudo
fi
if ! command -v systemctl >/dev/null 2>&1; then
  echo "设备未安装 systemctl" >&2
  exit 1
fi
if ! command -v tar >/dev/null 2>&1; then
  echo "设备未安装 tar" >&2
  exit 1
fi
if [ -z "$CHROMIUM_CMD" ]; then
  CHROMIUM_CMD="$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)"
fi
if [ -z "$CHROMIUM_CMD" ]; then
  echo "未找到 Chromium，请安装 chromium/chromium-browser/google-chrome" >&2
  exit 1
fi
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  echo "服务用户不存在: $SERVICE_USER" >&2
  exit 1
fi

detect_kiosk_user() {
  for candidate in ask peite orangepi pi ubuntu debian; do
    if id "$candidate" >/dev/null 2>&1; then
      echo "$candidate"
      return 0
    fi
  done
  if command -v getent >/dev/null 2>&1; then
    detected="$(getent passwd | awk -F: '($3 >= 1000 && $3 < 60000 && $7 !~ /(nologin|false)$/) { print $1; exit }')"
    if [ -n "$detected" ]; then
      echo "$detected"
      return 0
    fi
  fi
  return 1
}

if [ "$KIOSK_USER" = "root" ]; then
  if detected_kiosk_user="$(detect_kiosk_user)"; then
    KIOSK_USER="$detected_kiosk_user"
  else
    KIOSK_USER="dr600ab-kiosk"
  fi
fi
if ! id "$KIOSK_USER" >/dev/null 2>&1 && [ "$KIOSK_USER" = "dr600ab-kiosk" ]; then
  $SUDO useradd -m -s /usr/sbin/nologin "$KIOSK_USER" || $SUDO useradd -m "$KIOSK_USER"
fi
if ! id "$KIOSK_USER" >/dev/null 2>&1; then
  echo "屏幕服务用户不存在: $KIOSK_USER" >&2
  exit 1
fi
if [ "$KIOSK_USER" = "root" ]; then
  echo "Chromium 屏幕服务不能使用 root，请使用非 root 用户" >&2
  exit 1
fi
if command -v getent >/dev/null 2>&1; then
  KIOSK_HOME="$(getent passwd "$KIOSK_USER" | cut -d: -f6)"
else
  KIOSK_HOME=""
fi
if [ -z "$KIOSK_HOME" ]; then
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
  xauth_source=""
  for candidate in \
    "/run/user/$KIOSK_UID/gdm/Xauthority" \
    "/run/user/$KIOSK_UID/lightdm/Xauthority" \
    "/run/user/$KIOSK_UID/Xauthority" \
    "/var/run/lightdm/root/$DISPLAY_VALUE" \
    "$KIOSK_HOME/.Xauthority" \
    "/root/.Xauthority"; do
    if [ -f "$candidate" ]; then
      xauth_source="$candidate"
      break
    fi
  done
  if [ -z "$xauth_source" ]; then
    echo "未找到 display $DISPLAY_VALUE 的 Xauthority，屏幕服务可能需要人工配置 X 访问权限" >&2
    return 0
  fi
  if [ "$xauth_source" != "$KIOSK_HOME/.Xauthority" ]; then
    $SUDO mkdir -p "$KIOSK_HOME"
    $SUDO cp "$xauth_source" "$KIOSK_HOME/.Xauthority"
  fi
  $SUDO chown "$KIOSK_USER:" "$KIOSK_HOME/.Xauthority" || true
  $SUDO chmod 0600 "$KIOSK_HOME/.Xauthority" || true
}

clear_chromium_cache() {
  profile="$1"
  $SUDO rm -rf \
    "$profile/Cache" \
    "$profile/Code Cache" \
    "$profile/GPUCache" \
    "$profile/DawnCache" \
    "$profile/GrShaderCache" \
    "$profile/ShaderCache" \
    "$profile/Default/Cache" \
    "$profile/Default/Code Cache" \
    "$profile/Default/GPUCache" \
    "$profile/Default/DawnCache" \
    "$profile/Default/GrShaderCache" \
    "$profile/Default/ShaderCache" \
    "$profile/Default/Service Worker/CacheStorage" \
    "$profile/Default/Service Worker/ScriptCache" \
	    "$profile/Default/blob_storage"
}

write_kiosk_desktop_file() {
  desktop_file="$1"
  cat <<DESKTOP_EOF | $SUDO tee "$desktop_file" >/dev/null
[Desktop Entry]
Type=Application
Name=DR600AB Kiosk
Exec=env DR600AB_KIOSK_LOG=/tmp/dr600ab-kiosk.log $KIOSK_LAUNCHER
Terminal=false
X-GNOME-Autostart-enabled=true
DESKTOP_EOF
  $SUDO chmod 0644 "$desktop_file" || true
}

install_user_autostart() {
  target_user="$1"
  if ! id "$target_user" >/dev/null 2>&1; then
    return 0
  fi
  if command -v getent >/dev/null 2>&1; then
    target_home="$(getent passwd "$target_user" | cut -d: -f6)"
  else
    target_home="/home/$target_user"
  fi
  if [ -z "$target_home" ]; then
    return 0
  fi
  target_dir="$target_home/.config/autostart"
  target_file="$target_dir/dr600ab-kiosk.desktop"
  $SUDO install -d -m 0755 "$target_dir"
  $SUDO chown "$target_user:" "$target_dir" || true
  write_kiosk_desktop_file "$target_file"
  $SUDO chown "$target_user:" "$target_file" || true
}

migrate_legacy_database() {
  name="$1"
  legacy="$INSTALL_DIR/backend/data/$name"
  target="$INSTALL_DIR/data/$name"
  if [ ! -f "$legacy" ] || [ -f "$target" ]; then
    return 0
  fi
  echo "迁移旧数据库: $legacy -> $target"
  $SUDO mv "$legacy" "$target"
  for suffix in -wal -shm; do
    if [ -f "$legacy$suffix" ] && [ ! -f "$target$suffix" ]; then
      $SUDO mv "$legacy$suffix" "$target$suffix"
    fi
  done
}

EXTRACT_DIR="$TASK_DIR/extract"
rm -rf "$EXTRACT_DIR"
mkdir -p "$EXTRACT_DIR"
tar -xzf "$REMOTE_PACKAGE" -C "$EXTRACT_DIR"
BINARY="$(find "$EXTRACT_DIR" -type f -name dr600ab | head -n 1)"
if [ -z "$BINARY" ]; then
  echo "固件包中未找到 dr600ab 可执行文件" >&2
  exit 1
fi

$SUDO mkdir -p "$INSTALL_DIR"
$SUDO systemctl stop dr600ab-kiosk.service >/dev/null 2>&1 || true
$SUDO systemctl stop dr600ab.service >/dev/null 2>&1 || true
if [ -d "$INSTALL_DIR" ] && [ "$(find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]; then
  BACKUP_DIR="${INSTALL_DIR}.backup.$(date +%%Y%%m%%d%%H%%M%%S)"
  $SUDO cp -a "$INSTALL_DIR" "$BACKUP_DIR"
  echo "备份目录: $BACKUP_DIR"
fi
if [ "$FULL_UPDATE" = "1" ]; then
  $SUDO find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
else
  $SUDO mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/backend/data" "$INSTALL_DIR/static/map"
fi
$SUDO mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/backend/data" "$INSTALL_DIR/static/map"
migrate_legacy_database intrusions.db
migrate_legacy_database deception-reports.db
migrate_legacy_database interference-reports.db
$SUDO install -m 0755 "$BINARY" "$INSTALL_DIR/dr600ab"
$SUDO chown -R "$SERVICE_USER:" "$INSTALL_DIR" || true
prepare_kiosk_xauthority

cat <<'KIOSK_EOF' | $SUDO tee "$INSTALL_DIR/dr600ab-kiosk-start" >/dev/null
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
  echo "未找到 Chromium，请安装 chromium/chromium-browser/google-chrome" >&2
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
  echo "未找到 display $DISPLAY_VALUE 的 Xauthority，Chromium 可能无法连接屏幕" >&2
fi
display_number="${DISPLAY_VALUE#:}"
display_number="${display_number%%.*}"
if [ ! -S "/tmp/.X11-unix/X$display_number" ]; then
  echo "未检测到 X11 socket /tmp/.X11-unix/X$display_number" >&2
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
$SUDO chmod 0755 "$INSTALL_DIR/dr600ab-kiosk-start"
$SUDO install -d -m 0755 /usr/local/bin
$SUDO ln -sf "$INSTALL_DIR/dr600ab-kiosk-start" "$KIOSK_LAUNCHER"

$SUDO install -d -m 0755 "$SYSTEM_AUTOSTART_DIR"
write_kiosk_desktop_file "$SYSTEM_AUTOSTART_FILE"
install_user_autostart "$KIOSK_USER"
if command -v getent >/dev/null 2>&1; then
  getent passwd | awk -F: '($3 >= 1000 && $3 < 60000 && $7 !~ /(nologin|false)$/) { print $1 }' | while IFS= read -r desktop_user; do
    install_user_autostart "$desktop_user"
  done
fi

cat <<SERVICE_EOF | $SUDO tee /etc/systemd/system/dr600ab.service >/dev/null
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
Environment=API_INTRUSION_DB_PATH=$INSTALL_DIR/data/intrusions.db
Environment=API_DECEPTION_REPORT_DB_PATH=$INSTALL_DIR/data/deception-reports.db
Environment=API_INTERFERENCE_REPORT_DB_PATH=$INSTALL_DIR/data/interference-reports.db
Environment=API_OFFLINE_MAP_PATH=$INSTALL_DIR/static/map
ExecStart=$INSTALL_DIR/dr600ab
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
SERVICE_EOF

clear_chromium_cache "$LEGACY_CHROMIUM_USER_DATA_DIR"
$SUDO rm -rf "$LEGACY_CHROMIUM_USER_DATA_DIR"
clear_chromium_cache "$CHROMIUM_USER_DATA_DIR"
$SUDO systemctl disable --now dr600ab-kiosk.service >/dev/null 2>&1 || true
$SUDO rm -f /etc/systemd/system/dr600ab-kiosk.service
$SUDO systemctl daemon-reload
$SUDO systemctl enable dr600ab.service >/dev/null
$SUDO systemctl restart dr600ab.service
rm -rf "$TASK_DIR"
echo "Installed backend: $INSTALL_DIR/dr600ab"
echo "Backend service user: $SERVICE_USER"
echo "Kiosk service user: $KIOSK_USER"
echo "Screen URL: $APP_URL"
echo "Kiosk autostart: $AUTOSTART_FILE"
echo "Kiosk system autostart: $SYSTEM_AUTOSTART_FILE"
echo "Backend status: $(systemctl is-active dr600ab.service)"
echo "Kiosk startup: desktop autostart"
`, shellQuote(remotePackage),
		shellQuote(req.InstallDir),
		shellQuote(taskDir),
		shellQuote(full),
		shellQuote(defaultServiceUser),
		shellQuote(sshUser),
		shellQuote(defaultAPIHost),
		shellQuote(defaultAPIPort),
		shellQuote(defaultDisplay),
	)
}

func validateFirmwarePackagePath(path string) error {
	path = strings.TrimSpace(path)
	name := strings.ToLower(filepath.Base(path))
	if name == "" || name == "." {
		return fmt.Errorf("请选择固件包")
	}
	if strings.Contains(name, "darwin") || strings.Contains(name, "windows") {
		return fmt.Errorf("请选择 Linux 固件包，不能部署 %s", filepath.Base(path))
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		return fmt.Errorf("固件包格式无效，请选择 .tar.gz 固件包")
	}
	return nil
}

func commandOutputSuffix(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	return ": " + output
}

func formatBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	for _, unit := range units {
		size /= 1024
		if size < 1024 {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", size/1024)
}
