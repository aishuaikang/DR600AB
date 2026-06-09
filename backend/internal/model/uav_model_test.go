package model

import "testing"

func TestDisplayModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "known raw model", input: "PAL Analog", expected: "Analog PAL"},
		{name: "known dji model", input: "DJI_OC123_10M", expected: "DJI-O1/O2/O3 Series Mini 2,3, 4k, Air 2, 3, Mavic 2,3, Avata, P4-2.0"},
		{name: "numeric prefix stripped", input: "66-Air 2S", expected: "Air 2S"},
		{name: "non-numeric hyphen preserved", input: "DJI-Drone", expected: "DJI-Drone"},
		{name: "unknown fallback", input: "Unknown Model", expected: "Unknown Model"},
		{name: "empty fallback", input: " ", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayModelName(tc.input); got != tc.expected {
				t.Fatalf("DisplayModelName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestIsUncrackedDJIDronePosition(t *testing.T) {
	tests := []struct {
		name   string
		target ScreenPositionTarget
		want   bool
	}{
		{
			name:   "uncracked placeholder",
			target: ScreenPositionTarget{Model: "DJI-Drone", Cracked: false},
			want:   true,
		},
		{
			name:   "case and space insensitive",
			target: ScreenPositionTarget{Model: " dji-drone ", Cracked: false},
			want:   true,
		},
		{
			name:   "cracked decoded target",
			target: ScreenPositionTarget{Model: "DJI-Drone", Cracked: true},
			want:   false,
		},
		{
			name:   "real model",
			target: ScreenPositionTarget{Model: "DJI O4", Cracked: false},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUncrackedDJIDronePosition(tc.target); got != tc.want {
				t.Fatalf("IsUncrackedDJIDronePosition(%#v) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestUserSettingsWithDefaultsAddsScreenAlarmSettings(t *testing.T) {
	settings := UserSettingsWithDefaults(UserSettings{})

	if settings.ScreenAlarmSettings == nil ||
		!settings.ScreenAlarmSettings.Detection ||
		!settings.ScreenAlarmSettings.Position ||
		!settings.ScreenAlarmSettings.FPV ||
		!settings.ScreenAlarmSettings.Sound {
		t.Fatalf("ScreenAlarmSettings = %#v, want all defaults enabled", settings.ScreenAlarmSettings)
	}
}
