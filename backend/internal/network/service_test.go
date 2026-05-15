package network

import (
	"errors"
	"testing"

	"dr600ab-api/internal/model"
)

func TestParseWiFiListEscapedFields(t *testing.T) {
	input := stringsJoinLines(
		`*:AA\:BB\:CC\:DD\:EE\:FF:Office\:AP:Infra:6:540 Mbit/s:82:WPA2:wlan0`,
		`:11\:22\:33\:44\:55\:66:Guest Wi-Fi:Infra:11:130 Mbit/s:41:--:wlan1`,
	)

	got := parseWiFiList(input)

	if len(got) != 2 {
		t.Fatalf("len(parseWiFiList()) = %d, want 2", len(got))
	}
	assertWiFiNetwork(t, got[0], model.WiFiNetwork{
		Active:   true,
		BSSID:    "AA:BB:CC:DD:EE:FF",
		SSID:     "Office:AP",
		Mode:     "Infra",
		Channel:  "6",
		Rate:     "540 Mbit/s",
		Signal:   82,
		Security: "WPA2",
		Device:   "wlan0",
	})
	assertWiFiNetwork(t, got[1], model.WiFiNetwork{
		BSSID:    "11:22:33:44:55:66",
		SSID:     "Guest Wi-Fi",
		Mode:     "Infra",
		Channel:  "11",
		Rate:     "130 Mbit/s",
		Signal:   41,
		Security: "--",
		Device:   "wlan1",
	})
}

func TestParseWiFiListDedupeKeepsActiveOrStrongest(t *testing.T) {
	input := stringsJoinLines(
		`:00\:00\:00\:00\:00\:01:Office:Infra:1:130 Mbit/s:70:WPA2:wlan0`,
		`:00\:00\:00\:00\:00\:02:Office:Infra:6:270 Mbit/s:82:WPA2:wlan0`,
		`*:00\:00\:00\:00\:00\:03:Guest:Infra:11:130 Mbit/s:35:WPA2:wlan1`,
		`:00\:00\:00\:00\:00\:04:Guest:Infra:3:130 Mbit/s:93:WPA2:wlan1`,
	)

	got := parseWiFiList(input)

	if len(got) != 2 {
		t.Fatalf("len(parseWiFiList()) = %d, want 2", len(got))
	}
	if got[0].SSID != "Office" || got[0].BSSID != "00:00:00:00:00:02" || got[0].Signal != 82 {
		t.Fatalf("Office network = %+v, want strongest AP", got[0])
	}
	if got[1].SSID != "Guest" || got[1].BSSID != "00:00:00:00:00:03" || !got[1].Active {
		t.Fatalf("Guest network = %+v, want active AP", got[1])
	}
}

func TestParseDeviceStatusEscapedConnection(t *testing.T) {
	got := parseDeviceStatus(`wlan0:wifi:connected:Office\:AP`)

	item, ok := got["wlan0"]
	if !ok {
		t.Fatal("parseDeviceStatus() missing wlan0")
	}
	if item.ConnectionName != "Office:AP" {
		t.Fatalf("ConnectionName = %q, want %q", item.ConnectionName, "Office:AP")
	}
}

func TestValidateWiFiRequest(t *testing.T) {
	tests := []struct {
		name string
		req  model.WiFiConnectRequest
		want error
	}{
		{name: "valid open network", req: model.WiFiConnectRequest{SSID: "Guest"}},
		{name: "valid password", req: model.WiFiConnectRequest{SSID: "Office", Password: "12345678"}},
		{name: "empty ssid", req: model.WiFiConnectRequest{SSID: " "}, want: ErrInvalidWiFiConfig},
		{name: "too long ssid", req: model.WiFiConnectRequest{SSID: "123456789012345678901234567890123"}, want: ErrInvalidWiFiConfig},
		{name: "too long password", req: model.WiFiConnectRequest{SSID: "Office", Password: "12345678901234567890123456789012345678901234567890123456789012345"}, want: ErrInvalidWiFiConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWiFiRequest(tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateWiFiRequest() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func assertWiFiNetwork(t *testing.T, got, want model.WiFiNetwork) {
	t.Helper()
	if got != want {
		t.Fatalf("WiFiNetwork = %+v, want %+v", got, want)
	}
}

func stringsJoinLines(lines ...string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
