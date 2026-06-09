package main

import (
	"fmt"
	"strings"
)

func (a *App) StopAllServices(installDir string) (string, error) {
	installDir = a.getInstallDir(installDir)
	if _, err := a.runCommand(buildStopAllServicesScript(installDir)); err != nil {
		return "", fmt.Errorf("关闭全部服务失败: %w", err)
	}
	return "已关闭后端服务和屏幕进程", nil
}

func buildStopAllServicesScript(installDir string) string {
	return fmt.Sprintf(`set -eu
INSTALL_DIR=%s
SUDO=
if [ "$(id -u)" != "0" ]; then
  SUDO=sudo
fi

is_current_shell_or_ancestor() {
  candidate="$1"
  current="$$"
  while [ -n "$current" ] && [ "$current" != "0" ]; do
    if [ "$candidate" = "$current" ]; then
      return 0
    fi
    if [ ! -r "/proc/$current/status" ]; then
      break
    fi
    current="$(awk '/^PPid:/ { print $2; exit }' "/proc/$current/status" 2>/dev/null || true)"
  done
  return 1
}

stop_pid_list() {
  pids="$1"
  [ -n "$pids" ] || return 0
  for pid in $pids; do
    case "$pid" in
      ''|*[!0-9]*) continue ;;
    esac
    if is_current_shell_or_ancestor "$pid"; then
      continue
    fi
    $SUDO kill -TERM "$pid" >/dev/null 2>&1 || true
  done
  sleep 1
  for pid in $pids; do
    case "$pid" in
      ''|*[!0-9]*) continue ;;
    esac
    if is_current_shell_or_ancestor "$pid"; then
      continue
    fi
    if kill -0 "$pid" >/dev/null 2>&1; then
      $SUDO kill -KILL "$pid" >/dev/null 2>&1 || true
    fi
  done
}

if command -v systemctl >/dev/null 2>&1; then
  $SUDO systemctl stop dr600ab-kiosk.service >/dev/null 2>&1 || true
  $SUDO systemctl stop dr600ab.service >/dev/null 2>&1 || true
fi

if command -v pgrep >/dev/null 2>&1; then
  backend_pids="$(pgrep -x dr600ab 2>/dev/null || true)"
else
  backend_pids="$(ps -eo pid=,comm= 2>/dev/null | awk '$2 == "dr600ab" { print $1 }' || true)"
fi
stop_pid_list "$backend_pids"

if command -v pgrep >/dev/null 2>&1; then
  kiosk_pids="$(pgrep -f '[d]r600ab-kiosk-start|[c]hromium.*127[.]0[.]0[.]1:18080' 2>/dev/null || true)"
else
  kiosk_pids="$(ps -eo pid=,args= 2>/dev/null | awk '/[d]r600ab-kiosk-start|[c]hromium.*127[.]0[.]0[.]1:18080/ { print $1 }' || true)"
fi
stop_pid_list "$kiosk_pids"
echo "stopped"
`, shellQuote(strings.TrimRight(installDir, "/")))
}
