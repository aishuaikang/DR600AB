package main

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	timeInfoCurrentPrefix  = "DR600AB_CURRENT_TIME="
	timeInfoTimezonePrefix = "DR600AB_TIMEZONE="
	timeInfoNTPPrefix      = "DR600AB_NTP_ENABLED="
	timeInfoZonePrefix     = "DR600AB_TIMEZONE_ITEM="
	timeCommandSuccess     = "DR600AB_TIME_COMMAND_OK"
	manualTimeLayout       = "2006-01-02 15:04:05"
	ntpServiceList         = "systemd-timesyncd.service chrony.service chronyd.service ntp.service ntpd.service"
)

var timezoneNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)*$`)

// TimeInfo describes the current time settings of the connected device.
type TimeInfo struct {
	CurrentTime string   `json:"currentTime"`
	Timezone    string   `json:"timezone"`
	NTPEnabled  bool     `json:"ntpEnabled"`
	Timezones   []string `json:"timezones"`
}

// GetTimeInfo reads the current time, timezone, and supported timezones from the connected device.
func (a *App) GetTimeInfo() (TimeInfo, error) {
	output, err := a.runCommand(buildGetTimeInfoScript())
	if err != nil {
		return TimeInfo{}, fmt.Errorf("读取设备时间失败: %w", err)
	}

	info := parseTimeInfo(output)
	if info.CurrentTime == "" {
		return TimeInfo{}, errors.New("读取设备时间失败: 设备未返回当前时间")
	}
	return info, nil
}

// SetTimezone changes the timezone of the connected device.
func (a *App) SetTimezone(timezone string) error {
	timezone, err := validateTimezone(timezone)
	if err != nil {
		return err
	}

	output, err := a.runCommand(buildSetTimezoneScript(timezone))
	if err != nil {
		return fmt.Errorf("设置设备时区失败: %w", err)
	}
	if !hasTimeCommandSuccess(output) {
		return errors.New("设置设备时区失败: 设备未确认设置结果")
	}
	return nil
}

// SetNTPEnabled enables or disables automatic time synchronization on the connected device.
func (a *App) SetNTPEnabled(enabled bool) error {
	output, err := a.runCommand(buildSetNTPEnabledScript(enabled))
	if err != nil {
		return fmt.Errorf("设置 NTP 自动同步失败: %w", err)
	}
	if !hasTimeCommandSuccess(output) {
		return errors.New("设置 NTP 自动同步失败: 设备未确认设置结果")
	}
	return nil
}

// SetManualTime disables automatic NTP synchronization and changes the connected device time.
func (a *App) SetManualTime(datetime string) error {
	datetime, err := validateManualTime(datetime)
	if err != nil {
		return err
	}

	output, err := a.runCommand(buildSetManualTimeScript(datetime))
	if err != nil {
		return fmt.Errorf("设置设备时间失败: %w", err)
	}
	if !hasTimeCommandSuccess(output) {
		return errors.New("设置设备时间失败: 设备未确认设置结果")
	}
	return nil
}

func buildGetTimeInfoScript() string {
	return `set -eu
export LC_ALL=C

current_time="$(date '+%Y-%m-%d %H:%M:%S')"
timezone=""
ntp_enabled=false

if command -v timedatectl >/dev/null 2>&1; then
  timezone="$(timedatectl show --property=Timezone --value 2>/dev/null || true)"
fi
if [ -z "$timezone" ] && [ -r /etc/timezone ]; then
  timezone="$(head -n 1 /etc/timezone 2>/dev/null || true)"
fi
if [ -z "$timezone" ] && [ -L /etc/localtime ]; then
  timezone="$(readlink /etc/localtime 2>/dev/null | sed 's#^/usr/share/zoneinfo/##' || true)"
fi

if command -v timedatectl >/dev/null 2>&1; then
  ntp_status="$(timedatectl show 2>/dev/null || true)"
  if printf '%s\n' "$ntp_status" | grep -qE '^(NTP|NTPActive)=yes$'; then
    ntp_enabled=true
  fi
fi

if command -v systemctl >/dev/null 2>&1; then
	  for service in ` + ntpServiceList + `; do
	    if systemctl is-active --quiet "$service" 2>/dev/null; then
	      ntp_enabled=true
	      break
	    fi
	  done
fi

printf '%s%s\n' '` + timeInfoCurrentPrefix + `' "$current_time"
printf '%s%s\n' '` + timeInfoTimezonePrefix + `' "$timezone"
printf '%s%s\n' '` + timeInfoNTPPrefix + `' "$ntp_enabled"

timezone_list=""
if command -v timedatectl >/dev/null 2>&1; then
  timezone_list="$(timedatectl list-timezones 2>/dev/null || true)"
fi

if [ -n "$timezone_list" ]; then
  printf '%s\n' "$timezone_list" | while IFS= read -r zone; do
    [ -n "$zone" ] && printf '%s%s\n' '` + timeInfoZonePrefix + `' "$zone"
  done
elif [ -d /usr/share/zoneinfo ]; then
  find /usr/share/zoneinfo -type f 2>/dev/null \
    | sed 's#^/usr/share/zoneinfo/##' \
    | grep -Ev '^(posix|right|SystemV)/|^(localtime|posixrules|zone\.tab|zone1970\.tab|iso3166\.tab|leap-seconds\.list|leapseconds|tzdata\.zi)$' \
    | sort -u \
    | while IFS= read -r zone; do
        [ -n "$zone" ] && printf '%s%s\n' '` + timeInfoZonePrefix + `' "$zone"
      done
fi
`
}

func buildSetNTPEnabledScript(enabled bool) string {
	return fmt.Sprintf(`set -eu
EXPECTED=%t
SUDO=

if [ "$(id -u)" != "0" ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "当前用户不是 root，且设备未安装 sudo" >&2
    exit 1
  fi
  SUDO="sudo -n"
  if ! $SUDO true >/dev/null 2>&1; then
    echo "当前用户没有免密 sudo 权限" >&2
    exit 1
  fi
fi

if ! command -v timedatectl >/dev/null 2>&1; then
  echo "设备不支持 timedatectl，无法设置 NTP 自动同步" >&2
  exit 1
fi

run_bounded() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 10 "$@"
    return
  fi
  "$@"
}

find_ntp_service() {
  for service in ` + ntpServiceList + `; do
    if systemctl is-active --quiet "$service" 2>/dev/null; then
      printf '%%s' "$service"
      return 0
    fi
  done
  for service in ` + ntpServiceList + `; do
    if systemctl list-unit-files "$service" --no-legend 2>/dev/null | grep -q "^$service"; then
      printf '%%s' "$service"
      return 0
    fi
  done
  return 1
}

stop_ntp_services() {
  if ! command -v systemctl >/dev/null 2>&1; then
    return 0
  fi
  for service in ` + ntpServiceList + `; do
    run_bounded $SUDO systemctl stop "$service" >/dev/null 2>&1 || true
    run_bounded $SUDO systemctl disable "$service" >/dev/null 2>&1 || true
  done
}

apply_ntp_state() {
	  if [ "$EXPECTED" = "false" ]; then
	    stop_ntp_services
	  elif command -v systemctl >/dev/null 2>&1; then
	    service="$(find_ntp_service || true)"
	    if [ -n "$service" ]; then
	      run_bounded $SUDO systemctl enable "$service" >/dev/null 2>&1 || true
	      run_bounded $SUDO systemctl restart "$service" >/dev/null 2>&1 || true
	    fi
	  fi

  if ! run_bounded $SUDO timedatectl set-ntp "$EXPECTED" >/dev/null 2>&1; then
    return 1
  fi
}

ntp_state_matches() {
	  ntp_property="$(timedatectl show --property=NTP --value 2>/dev/null || true)"
	  ntp_active=false
	  if command -v systemctl >/dev/null 2>&1; then
	    for service in ` + ntpServiceList + `; do
	      if systemctl is-active --quiet "$service" 2>/dev/null; then
	        ntp_active=true
	        break
	      fi
	    done
	  fi

	  if [ "$EXPECTED" = "true" ]; then
	    [ "$ntp_property" = "yes" ] || [ "$ntp_active" = "true" ]
	    return
	  fi
	  [ "$ntp_property" != "yes" ] && [ "$ntp_active" = "false" ]
}

if ! apply_ntp_state; then
  echo "timedatectl 设置 NTP 自动同步失败" >&2
  exit 1
fi
sleep 1

if ! ntp_state_matches; then
  if ! apply_ntp_state; then
    echo "重试设置 NTP 自动同步失败" >&2
    exit 1
  fi
  sleep 1
fi

if ! ntp_state_matches; then
  echo "NTP 自动同步状态与设置值不一致" >&2
  exit 1
fi

printf '%%s\n' '`+timeCommandSuccess+`'
`, enabled)
}

func buildSetTimezoneScript(timezone string) string {
	return fmt.Sprintf(`set -eu
TIMEZONE=%s
ZONE_FILE="/usr/share/zoneinfo/$TIMEZONE"
SUDO=

if [ ! -f "$ZONE_FILE" ]; then
  echo "设备不支持时区: $TIMEZONE" >&2
  exit 1
fi

if [ "$(id -u)" != "0" ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "当前用户不是 root，且设备未安装 sudo" >&2
    exit 1
  fi
  SUDO="sudo -n"
  if ! $SUDO true >/dev/null 2>&1; then
    echo "当前用户没有免密 sudo 权限" >&2
    exit 1
  fi
fi

if command -v timedatectl >/dev/null 2>&1 && $SUDO timedatectl set-timezone "$TIMEZONE" >/dev/null 2>&1; then
  :
else
  $SUDO ln -snf "$ZONE_FILE" /etc/localtime
  if [ -e /etc/timezone ]; then
    printf '%%s\n' "$TIMEZONE" | $SUDO tee /etc/timezone >/dev/null
  fi
fi

printf '%%s\n' '`+timeCommandSuccess+`'
`, shellQuote(timezone))
}

func buildSetManualTimeScript(datetime string) string {
	return fmt.Sprintf(`set -eu
DATETIME=%s
SUDO=

if [ "$(id -u)" != "0" ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "当前用户不是 root，且设备未安装 sudo" >&2
    exit 1
  fi
  SUDO="sudo -n"
  if ! $SUDO true >/dev/null 2>&1; then
    echo "当前用户没有免密 sudo 权限" >&2
    exit 1
  fi
fi

run_bounded() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 10 "$@"
    return
  fi
  "$@"
}

if command -v systemctl >/dev/null 2>&1; then
  for service in ` + ntpServiceList + `; do
    run_bounded $SUDO systemctl stop "$service" >/dev/null 2>&1 || true
    run_bounded $SUDO systemctl disable "$service" >/dev/null 2>&1 || true
  done
fi

if command -v timedatectl >/dev/null 2>&1; then
  if ! run_bounded $SUDO timedatectl set-ntp false >/dev/null 2>&1; then
    echo "关闭 NTP 自动同步失败" >&2
    exit 1
  fi
fi

time_set=false
if command -v timedatectl >/dev/null 2>&1 && $SUDO timedatectl set-time "$DATETIME" >/dev/null 2>&1; then
  time_set=true
elif command -v date >/dev/null 2>&1 && $SUDO date -s "$DATETIME" >/dev/null 2>&1; then
  time_set=true
fi

if [ "$time_set" != "true" ]; then
  echo "设备不支持 timedatectl 或 date 设置时间" >&2
  exit 1
fi

if command -v hwclock >/dev/null 2>&1; then
  $SUDO hwclock --systohc >/dev/null 2>&1 || true
fi

printf '%%s\n' '`+timeCommandSuccess+`'
`, shellQuote(datetime))
}

func parseTimeInfo(output string) TimeInfo {
	info := TimeInfo{Timezones: []string{}}
	seen := map[string]struct{}{}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, timeInfoCurrentPrefix):
			info.CurrentTime = strings.TrimSpace(strings.TrimPrefix(line, timeInfoCurrentPrefix))
		case strings.HasPrefix(line, timeInfoTimezonePrefix):
			info.Timezone = strings.TrimSpace(strings.TrimPrefix(line, timeInfoTimezonePrefix))
		case strings.HasPrefix(line, timeInfoNTPPrefix):
			info.NTPEnabled = strings.TrimSpace(strings.TrimPrefix(line, timeInfoNTPPrefix)) == "true"
		case strings.HasPrefix(line, timeInfoZonePrefix):
			zone := strings.TrimSpace(strings.TrimPrefix(line, timeInfoZonePrefix))
			if zone == "" {
				continue
			}
			if _, exists := seen[zone]; exists {
				continue
			}
			seen[zone] = struct{}{}
			info.Timezones = append(info.Timezones, zone)
		}
	}

	if info.Timezone == "" {
		info.Timezone = "未知"
	}
	if info.Timezone != "未知" {
		if _, exists := seen[info.Timezone]; !exists {
			info.Timezones = append(info.Timezones, info.Timezone)
		}
	}
	if len(info.Timezones) == 0 {
		info.Timezones = append(info.Timezones, "UTC")
	}
	sort.Strings(info.Timezones)
	return info
}

func validateTimezone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("时区不能为空")
	}
	if len(value) > 128 || !timezoneNamePattern.MatchString(value) {
		return "", fmt.Errorf("时区格式无效: %q", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." {
			return "", fmt.Errorf("时区格式无效: %q", value)
		}
	}
	return value, nil
}

func validateManualTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("时间不能为空")
	}
	if _, err := time.Parse(manualTimeLayout, value); err != nil {
		return "", fmt.Errorf("时间格式无效，应为 YYYY-MM-DD HH:mm:ss: %w", err)
	}
	return value, nil
}

func hasTimeCommandSuccess(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == timeCommandSuccess {
			return true
		}
	}
	return false
}
