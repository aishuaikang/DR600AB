// Package systemtime 管理运行设备的本机时间、时区和 NTP 状态。
package systemtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	manualDateTimeLayout = "2006-01-02 15:04:05"
	commandTimeout       = 10 * time.Second
	queryTimeout         = 15 * time.Second
	changeTimeout        = 40 * time.Second
)

var knownNTPServices = []string{
	"systemd-timesyncd.service",
	"chrony.service",
	"chronyd.service",
	"ntp.service",
	"ntpd.service",
}

var (
	ErrUnsupported       = errors.New("time management is only supported on linux")
	ErrInvalidTimezone   = errors.New("invalid timezone")
	ErrInvalidManualTime = errors.New("invalid manual time")
)

// Info 描述本机当前时间设置。
type Info struct {
	Platform                string `json:"platform"`
	TimeManagementSupported bool   `json:"time_management_supported"`
	CurrentTime             string `json:"current_time,omitempty"`
	Timezone                string `json:"timezone,omitempty"`
	UTCOffset               string `json:"utc_offset,omitempty"`
	NTPEnabled              bool   `json:"ntp_enabled"`
	NTPSynced               bool   `json:"ntp_synced"`
}

type command struct {
	name  string
	args  []string
	stdin string
}

type commandRunner interface {
	run(context.Context, command) (string, error)
}

type execCommandRunner struct{}

func (execCommandRunner) run(ctx context.Context, request command) (string, error) {
	cmd := exec.CommandContext(ctx, request.name, request.args...)
	if request.stdin != "" {
		cmd.Stdin = bytes.NewBufferString(request.stdin)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Service 执行本机系统时间操作。写操作使用同一把锁，避免 NTP 和手动校时并发互相覆盖。
type Service struct {
	mu           sync.RWMutex
	platform     string
	runner       commandRunner
	now          func() time.Time
	readFile     func(string) ([]byte, error)
	evalSymlinks func(string) (string, error)
	timezoneRoot string
}

// New 创建使用当前运行平台和系统命令的时间服务。
func New() *Service {
	return newService(runtime.GOOS, execCommandRunner{})
}

func newService(platform string, runner commandRunner) *Service {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &Service{
		platform:     platform,
		runner:       runner,
		now:          time.Now,
		readFile:     os.ReadFile,
		evalSymlinks: filepath.EvalSymlinks,
		timezoneRoot: "/usr/share/zoneinfo",
	}
}

// IsSupported 返回当前平台是否支持本机时间管理。
func (s *Service) IsSupported() bool {
	return s != nil && s.platform == "linux"
}

// GetInfo 读取当前系统时间、时区、NTP 开关和同步状态。
func (s *Service) GetInfo(ctx context.Context) (Info, error) {
	info := Info{Platform: s.platform, TimeManagementSupported: s.IsSupported()}
	if !s.IsSupported() {
		return info, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	now := s.now()
	info.CurrentTime = s.currentTime(queryCtx, now)
	info.UTCOffset = s.utcOffset(queryCtx, now)
	info.Timezone = s.timezone(queryCtx, now)

	ntpEnabled, ntpErr := s.queryNTPEnabled(queryCtx)
	if ntpErr != nil && queryCtx.Err() != nil {
		return Info{}, fmt.Errorf("querying ntp state: %w", queryCtx.Err())
	}
	info.NTPEnabled = ntpEnabled

	if synced, err := s.queryTimedatectlBool(queryCtx, "NTPSynchronized"); err == nil {
		info.NTPSynced = synced
	} else if queryCtx.Err() != nil {
		return Info{}, fmt.Errorf("querying ntp sync state: %w", queryCtx.Err())
	}
	return info, nil
}

// ListTimezones 返回设备可选择的时区名称。
func (s *Service) ListTimezones(ctx context.Context) ([]string, error) {
	if !s.IsSupported() {
		return nil, ErrUnsupported
	}
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	output, err := s.run(queryCtx, "timedatectl", "list-timezones")
	if err == nil {
		if zones := sortedUniqueLines(output); len(zones) > 0 {
			return zones, nil
		}
	}

	zones := make([]string, 0, 256)
	walkErr := filepath.WalkDir(s.timezoneRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if queryCtx.Err() != nil {
			return queryCtx.Err()
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != s.timezoneRoot && (name == "posix" || name == "right" || name == "SystemV") {
				return filepath.SkipDir
			}
			return nil
		}
		relative, relErr := filepath.Rel(s.timezoneRoot, path)
		if relErr != nil || isTimezoneMetadata(relative) {
			return nil
		}
		zones = append(zones, filepath.ToSlash(relative))
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, context.Canceled) && !errors.Is(walkErr, context.DeadlineExceeded) {
		walkErr = nil
	}
	if queryCtx.Err() != nil {
		return nil, queryCtx.Err()
	}
	if sorted := sortedUnique(zones); len(sorted) > 0 {
		return sorted, nil
	}
	return []string{"UTC"}, nil
}

// SetTimezone 设置本机时区。
func (s *Service) SetTimezone(ctx context.Context, timezone string) error {
	if !s.IsSupported() {
		return ErrUnsupported
	}
	timezone, err := validateTimezone(timezone)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	changeCtx, cancel := context.WithTimeout(ctx, changeTimeout)
	defer cancel()

	if _, err := s.runPrivileged(changeCtx, "timedatectl", "set-timezone", timezone); err == nil {
		return nil
	} else {
		timedatectlErr := err
		zoneFile := filepath.Join(s.timezoneRoot, filepath.FromSlash(timezone))
		zoneInfo, statErr := os.Stat(zoneFile)
		if statErr != nil || zoneInfo.IsDir() {
			if statErr == nil {
				statErr = errors.New("timezone path is a directory")
			}
			return fmt.Errorf("%w: timezone %q is unavailable: %v", ErrInvalidTimezone, timezone, statErr)
		}
		if _, linkErr := s.runPrivileged(changeCtx, "ln", "-snf", zoneFile, "/etc/localtime"); linkErr != nil {
			return fmt.Errorf("setting timezone: %w", errors.Join(timedatectlErr, linkErr))
		}
		if _, fileErr := s.runPrivilegedWithStdin(changeCtx, timezone+"\n", "tee", "/etc/timezone"); fileErr != nil {
			return fmt.Errorf("setting timezone: %w", fileErr)
		}
	}
	return nil
}

// SetNTPEnabled 开启或关闭 NTP 自动同步，并回读确认最终状态。
func (s *Service) SetNTPEnabled(ctx context.Context, enabled bool) error {
	if !s.IsSupported() {
		return ErrUnsupported
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	changeCtx, cancel := context.WithTimeout(ctx, changeTimeout)
	defer cancel()
	var lastErr error
	for range 2 {
		applyErr := s.applyNTPState(changeCtx, enabled)
		actual, verifyErr := s.queryNTPEnabled(changeCtx)
		if verifyErr == nil && actual == enabled {
			return nil
		}
		if verifyErr == nil {
			verifyErr = errors.New("ntp state verification mismatch")
		}
		lastErr = errors.Join(applyErr, verifyErr)
	}
	return fmt.Errorf("setting ntp state: %w", lastErr)
}

// SetManualTime 关闭 NTP 后设置本机时间，输入格式固定为 YYYY-MM-DD HH:mm:ss。
func (s *Service) SetManualTime(ctx context.Context, dateTime string) error {
	if !s.IsSupported() {
		return ErrUnsupported
	}
	dateTime = strings.TrimSpace(dateTime)
	parsed, err := time.ParseInLocation(manualDateTimeLayout, dateTime, time.Local)
	if err != nil {
		return fmt.Errorf("%w: expected format %s", ErrInvalidManualTime, manualDateTimeLayout)
	}
	dateTime = parsed.Format(manualDateTimeLayout)

	s.mu.Lock()
	defer s.mu.Unlock()
	changeCtx, cancel := context.WithTimeout(ctx, changeTimeout)
	defer cancel()
	if err := s.setNTPEnabledLocked(changeCtx, false); err != nil {
		return fmt.Errorf("disabling ntp before setting time: %w", err)
	}
	if _, err := s.runPrivileged(changeCtx, "timedatectl", "set-time", dateTime); err == nil {
		return nil
	} else {
		timedatectlErr := err
		if _, dateErr := s.runPrivileged(changeCtx, "date", "-s", dateTime); dateErr == nil {
			return nil
		} else {
			return fmt.Errorf("setting system time: %w", errors.Join(timedatectlErr, dateErr))
		}
	}
}

func (s *Service) setNTPEnabledLocked(ctx context.Context, enabled bool) error {
	var lastErr error
	for range 2 {
		applyErr := s.applyNTPState(ctx, enabled)
		actual, verifyErr := s.queryNTPEnabled(ctx)
		if verifyErr == nil && actual == enabled {
			return nil
		}
		if verifyErr == nil {
			verifyErr = errors.New("ntp state verification mismatch")
		}
		lastErr = errors.Join(applyErr, verifyErr)
	}
	return fmt.Errorf("setting ntp state: %w", lastErr)
}

func (s *Service) applyNTPState(ctx context.Context, enabled bool) error {
	if enabled {
		if service := s.findNTPService(ctx); service != "" {
			_, _ = s.runPrivileged(ctx, "systemctl", "enable", service)
			_, _ = s.runPrivileged(ctx, "systemctl", "restart", service)
		}
	} else {
		for _, service := range knownNTPServices {
			_, _ = s.runPrivileged(ctx, "systemctl", "stop", service)
			_, _ = s.runPrivileged(ctx, "systemctl", "disable", service)
		}
	}
	_, err := s.runPrivileged(ctx, "timedatectl", "set-ntp", boolString(enabled))
	return err
}

func (s *Service) findNTPService(ctx context.Context) string {
	for _, service := range knownNTPServices {
		output, err := s.run(ctx, "systemctl", "is-active", service)
		if err == nil {
			if active, ok := parseServiceActive(output); ok && active {
				return service
			}
		}
	}
	for _, service := range knownNTPServices {
		output, err := s.run(ctx, "systemctl", "list-unit-files", service, "--no-legend")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == service {
				return service
			}
		}
	}
	return ""
}

func (s *Service) queryNTPEnabled(ctx context.Context) (bool, error) {
	property, propertyErr := s.run(ctx, "timedatectl", "show", "--property=NTP", "--value")
	if value, ok := parseSystemBool(property); ok {
		return value, nil
	}
	activeProperty, activeErr := s.run(ctx, "timedatectl", "show", "--property=NTPActive", "--value")
	if value, ok := parseSystemBool(activeProperty); ok {
		return value, nil
	}
	for _, name := range knownNTPServices {
		output, _ := s.run(ctx, "systemctl", "is-active", name)
		if active, ok := parseServiceActive(output); ok && active {
			return true, nil
		}
	}
	return false, errors.Join(errors.New("unable to determine ntp state"), propertyErr, activeErr)
}

func (s *Service) queryTimedatectlBool(ctx context.Context, property string) (bool, error) {
	output, err := s.run(ctx, "timedatectl", "show", "--property="+property, "--value")
	if err != nil {
		return false, err
	}
	value, ok := parseSystemBool(output)
	if !ok {
		return false, errors.New("unexpected timedatectl boolean value")
	}
	return value, nil
}

func (s *Service) timezone(ctx context.Context, now time.Time) string {
	if content, err := s.readFile("/etc/timezone"); err == nil {
		if value := strings.TrimSpace(string(content)); value != "" {
			return value
		}
	}
	if path, err := s.evalSymlinks("/etc/localtime"); err == nil {
		if _, value, ok := strings.Cut(path, "/zoneinfo/"); ok && value != "" {
			return value
		}
	}
	if output, err := s.run(ctx, "timedatectl", "show", "--property=Timezone", "--value"); err == nil {
		if value := strings.TrimSpace(output); value != "" {
			return value
		}
	}
	if location := now.Location().String(); location != "" {
		return location
	}
	return "UTC"
}

func (s *Service) currentTime(ctx context.Context, now time.Time) string {
	if output, err := s.run(ctx, "date", "+%Y-%m-%d %H:%M:%S"); err == nil {
		if value := strings.TrimSpace(output); value != "" {
			return value
		}
	}
	return now.Format(manualDateTimeLayout)
}

func (s *Service) utcOffset(ctx context.Context, now time.Time) string {
	if output, err := s.run(ctx, "date", "+%:z"); err == nil {
		if value := strings.TrimSpace(output); value != "" {
			return value
		}
	}
	_, offset := now.Zone()
	return formatUTCOffset(offset)
}

func (s *Service) run(ctx context.Context, name string, args ...string) (string, error) {
	return s.runCommand(ctx, command{name: name, args: args})
}

func (s *Service) runCommand(ctx context.Context, request command) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	output, err := s.runner.run(commandCtx, request)
	if err != nil {
		if commandCtx.Err() != nil {
			return output, fmt.Errorf("running %s: %w", request.name, commandCtx.Err())
		}
		return output, fmt.Errorf("running %s: %w", request.name, err)
	}
	return output, nil
}

func (s *Service) runPrivileged(ctx context.Context, name string, args ...string) (string, error) {
	return s.runPrivilegedCommand(ctx, command{name: name, args: args})
}

func (s *Service) runPrivilegedWithStdin(ctx context.Context, stdin string, name string, args ...string) (string, error) {
	return s.runPrivilegedCommand(ctx, command{name: name, args: args, stdin: stdin})
}

func (s *Service) runPrivilegedCommand(ctx context.Context, request command) (string, error) {
	output, directErr := s.runCommand(ctx, request)
	if directErr == nil {
		return output, nil
	}
	sudoArgs := append([]string{"-n", request.name}, request.args...)
	sudoRequest := command{name: "sudo", args: sudoArgs, stdin: request.stdin}
	sudoOutput, sudoErr := s.runCommand(ctx, sudoRequest)
	if sudoErr == nil {
		return sudoOutput, nil
	}
	return sudoOutput, errors.Join(directErr, sudoErr)
}

func validateTimezone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return "", ErrInvalidTimezone
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrInvalidTimezone
		}
		for _, char := range part {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
				(char < '0' || char > '9') && !strings.ContainsRune("._+-", char) {
				return "", ErrInvalidTimezone
			}
		}
	}
	return value, nil
}

func sortedUniqueLines(value string) []string {
	items := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return sortedUnique(items)
}

func sortedUnique(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok || item == "" {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func isTimezoneMetadata(value string) bool {
	value = filepath.ToSlash(value)
	if strings.HasPrefix(value, "posix/") || strings.HasPrefix(value, "right/") || strings.HasPrefix(value, "SystemV/") {
		return true
	}
	switch value {
	case "localtime", "posixrules", "zone.tab", "zone1970.tab", "iso3166.tab", "leap-seconds.list", "leapseconds", "tzdata.zi":
		return true
	default:
		return false
	}
}

func parseSystemBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "active", "enabled":
		return true, true
	case "0", "false", "no", "inactive", "disabled":
		return false, true
	default:
		return false, false
	}
}

func parseServiceActive(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "activating", "reloading":
		return true, true
	case "inactive", "dead", "deactivating", "failed":
		return false, true
	default:
		return false, false
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func formatUTCOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}
