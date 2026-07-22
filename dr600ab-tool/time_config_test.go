package main

import (
	"strings"
	"testing"
)

func TestParseTimeInfo(t *testing.T) {
	t.Parallel()

	info := parseTimeInfo(strings.Join([]string{
		timeInfoCurrentPrefix + "2026-07-21 15:04:05",
		timeInfoTimezonePrefix + "Asia/Shanghai",
		timeInfoNTPPrefix + "true",
		timeInfoZonePrefix + "UTC",
		timeInfoZonePrefix + "Asia/Shanghai",
		timeInfoZonePrefix + "UTC",
	}, "\n"))

	if info.CurrentTime != "2026-07-21 15:04:05" {
		t.Fatalf("CurrentTime = %q", info.CurrentTime)
	}
	if info.Timezone != "Asia/Shanghai" {
		t.Fatalf("Timezone = %q", info.Timezone)
	}
	if !info.NTPEnabled {
		t.Fatal("NTPEnabled = false, want true")
	}
	wantZones := []string{"Asia/Shanghai", "UTC"}
	if strings.Join(info.Timezones, ",") != strings.Join(wantZones, ",") {
		t.Fatalf("Timezones = %#v, want %#v", info.Timezones, wantZones)
	}
}

func TestParseTimeInfoFallback(t *testing.T) {
	t.Parallel()

	info := parseTimeInfo(timeInfoCurrentPrefix + "2026-07-21 15:04:05")
	if info.Timezone != "未知" {
		t.Fatalf("Timezone = %q, want 未知", info.Timezone)
	}
	if len(info.Timezones) != 1 || info.Timezones[0] != "UTC" {
		t.Fatalf("Timezones = %#v, want UTC fallback", info.Timezones)
	}
}

func TestValidateTimezone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "region timezone", value: " Asia/Shanghai ", want: "Asia/Shanghai"},
		{name: "UTC", value: "UTC", want: "UTC"},
		{name: "Etc offset", value: "Etc/GMT+8", want: "Etc/GMT+8"},
		{name: "empty", value: " ", wantErr: true},
		{name: "shell injection", value: "Asia/Shanghai; reboot", wantErr: true},
		{name: "absolute path", value: "/etc/passwd", wantErr: true},
		{name: "parent traversal", value: "../etc/passwd", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateTimezone(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateTimezone(%q) returned no error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateTimezone(%q): %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("validateTimezone(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestValidateManualTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "valid", value: " 2026-07-21 15:04:05 ", want: "2026-07-21 15:04:05"},
		{name: "empty", value: "", wantErr: true},
		{name: "missing seconds", value: "2026-07-21 15:04", wantErr: true},
		{name: "invalid day", value: "2026-02-30 15:04:05", wantErr: true},
		{name: "shell injection", value: "2026-07-21 15:04:05; reboot", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateManualTime(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateManualTime(%q) returned no error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateManualTime(%q): %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("validateManualTime(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestBuildSetTimezoneScriptQuotesValue(t *testing.T) {
	t.Parallel()

	script := buildSetTimezoneScript("Test/Zone'; reboot #")
	if !strings.Contains(script, "TIMEZONE='Test/Zone'\\''; reboot #'") {
		t.Fatalf("timezone was not shell quoted:\n%s", script)
	}
	if !strings.Contains(script, "sudo -n") {
		t.Fatalf("script must use non-interactive sudo:\n%s", script)
	}
}

func TestBuildSetManualTimeScript(t *testing.T) {
	t.Parallel()

	script := buildSetManualTimeScript("2026-07-21 15:04:05")
	for _, want := range []string{
		"DATETIME='2026-07-21 15:04:05'",
		"systemctl stop \"$service\"",
		"systemctl disable \"$service\"",
		"timedatectl set-ntp false",
		"timedatectl set-time \"$DATETIME\"",
		"date -s \"$DATETIME\"",
		"hwclock --systohc",
		"sudo -n",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestBuildSetNTPEnabledScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{name: "enable", enabled: true, want: "EXPECTED=true"},
		{name: "disable", enabled: false, want: "EXPECTED=false"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			script := buildSetNTPEnabledScript(test.enabled)
			for _, want := range []string{
				test.want,
				"for service in systemd-timesyncd.service chrony.service chronyd.service ntp.service ntpd.service",
				"systemctl enable \"$service\"",
				"systemctl stop \"$service\"",
				"systemctl disable \"$service\"",
				"timedatectl set-ntp \"$EXPECTED\"",
				"ntp_state_matches",
				"timeout 10",
				"sudo -n",
			} {
				if !strings.Contains(script, want) {
					t.Fatalf("script missing %q:\n%s", want, script)
				}
			}
		})
	}
}

func TestBuildGetTimeInfoScriptChecksKnownNTPServices(t *testing.T) {
	t.Parallel()

	script := buildGetTimeInfoScript()
	for _, service := range []string{
		"systemd-timesyncd.service",
		"chrony.service",
		"chronyd.service",
		"ntp.service",
		"ntpd.service",
	} {
		if !strings.Contains(script, service) {
			t.Fatalf("time info script missing %q:\n%s", service, script)
		}
	}
}
