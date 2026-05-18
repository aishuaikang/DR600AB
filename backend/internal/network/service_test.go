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

func TestApplyDeviceDetailsIndexedFields(t *testing.T) {
	interfaces := parseDeviceStatus(`eth0:ethernet:connected:Wired connection 1`)
	applyDeviceDetails(interfaces, stringsJoinLines(
		`GENERAL.DEVICE:eth0`,
		`GENERAL.HWADDR:AA\:BB\:CC\:DD\:EE\:FF`,
		`GENERAL.MTU:1500`,
		`IP4.ADDRESS[1]:192.168.10.25/24`,
		`IP4.GATEWAY:192.168.10.1`,
		`IP4.DNS[1]:114.114.114.114`,
		`IP4.DNS[2]:8.8.8.8`,
	))

	item := interfaces["eth0"]
	if item.HardwareAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("HardwareAddress = %q, want %q", item.HardwareAddress, "AA:BB:CC:DD:EE:FF")
	}
	if item.MTU != 1500 {
		t.Fatalf("MTU = %d, want 1500", item.MTU)
	}
	if len(item.IPv4) != 1 || item.IPv4[0].Address != "192.168.10.25" || item.IPv4[0].Prefix != 24 {
		t.Fatalf("IPv4 = %+v, want 192.168.10.25/24", item.IPv4)
	}
	if item.Gateway4 != "192.168.10.1" {
		t.Fatalf("Gateway4 = %q, want %q", item.Gateway4, "192.168.10.1")
	}
	if len(item.DNS4) != 2 || item.DNS4[0] != "114.114.114.114" || item.DNS4[1] != "8.8.8.8" {
		t.Fatalf("DNS4 = %+v, want [114.114.114.114 8.8.8.8]", item.DNS4)
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

func TestValidateUpdateRequestRouteMetric(t *testing.T) {
	validMetric := 100
	autoMetric := -1
	invalidMetric := -2

	validReq := model.NetworkInterfaceUpdateRequest{
		Mode:        "static",
		IPv4Address: "192.168.10.25",
		Prefix:      24,
		RouteMetric: &validMetric,
	}
	if err := validateUpdateRequest(validReq); err != nil {
		t.Fatalf("validateUpdateRequest() error = %v, want nil", err)
	}
	autoReq := model.NetworkInterfaceUpdateRequest{
		Mode:        "dhcp",
		RouteMetric: &autoMetric,
	}
	if err := validateUpdateRequest(autoReq); err != nil {
		t.Fatalf("validateUpdateRequest() error = %v, want nil", err)
	}

	invalidReq := model.NetworkInterfaceUpdateRequest{
		Mode:        "dhcp",
		RouteMetric: &invalidMetric,
	}
	if err := validateUpdateRequest(invalidReq); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("validateUpdateRequest() error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestAppendRouteMetricArgs(t *testing.T) {
	metric := 600

	got := appendRouteMetricArgs([]string{"connection", "modify", "Office"}, &metric)
	want := []string{
		"connection", "modify", "Office",
		"ipv4.route-metric", "600",
		"connection.autoconnect", "yes",
		"connection.autoconnect-priority", "399",
	}
	if len(got) != len(want) {
		t.Fatalf("appendRouteMetricArgs() = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("appendRouteMetricArgs()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAutoconnectPriorityForRouteMetric(t *testing.T) {
	tests := []struct {
		name        string
		routeMetric int
		want        int
	}{
		{name: "automatic metric uses default priority", routeMetric: -1, want: 0},
		{name: "highest metric priority is clamped", routeMetric: 0, want: 999},
		{name: "frontend first priority", routeMetric: 100, want: 899},
		{name: "low priority is clamped", routeMetric: 9999, want: -999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := autoconnectPriorityForRouteMetric(tt.routeMetric)
			if got != tt.want {
				t.Fatalf("autoconnectPriorityForRouteMetric(%d) = %d, want %d", tt.routeMetric, got, tt.want)
			}
		})
	}
}

func TestShouldReactivateConnection(t *testing.T) {
	tests := []struct {
		name   string
		target model.NetworkInterface
		want   bool
	}{
		{
			name:   "connected interface",
			target: model.NetworkInterface{Name: "eth0", State: "connected", ConnectionName: "eth0"},
			want:   true,
		},
		{
			name:   "connecting wifi is not forced up",
			target: model.NetworkInterface{Name: "wlan0", State: "connecting (configuring)", ConnectionName: "Office"},
			want:   false,
		},
		{
			name:   "disconnected wifi is not forced up",
			target: model.NetworkInterface{Name: "wlan0", State: "disconnected", ConnectionName: "Office"},
			want:   false,
		},
		{
			name:   "missing connection name",
			target: model.NetworkInterface{Name: "eth0", State: "connected"},
			want:   false,
		},
		{
			name:   "unmanaged connection marker",
			target: model.NetworkInterface{Name: "eth0", State: "connected", ConnectionName: "--"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldReactivateConnection(tt.target)
			if got != tt.want {
				t.Fatalf("shouldReactivateConnection(%+v) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestCommandErrorAvoidsDuplicateSignalKilled(t *testing.T) {
	err := commandError("nmcli", []string{"connection", "up", "Office"}, []byte("signal: killed"), errors.New("signal: killed"))
	if got := err.Error(); got != "nmcli connection up Office: signal: killed" {
		t.Fatalf("commandError() = %q, want command context without duplicate error", got)
	}
}

func TestUpsertNetworkPriority(t *testing.T) {
	settings := model.NetworkSettings{
		Priorities: []model.NetworkPrioritySetting{
			{InterfaceName: "eth0", ConnectionName: "eth0", RouteMetric: 100},
			{InterfaceName: "wlan0", ConnectionName: "Office", RouteMetric: 300},
		},
	}

	got := upsertNetworkPriority(settings, model.NetworkPrioritySetting{
		InterfaceName:  "eth0",
		ConnectionName: "eth0",
		RouteMetric:    500,
	})

	if len(got.Priorities) != 2 {
		t.Fatalf("priorities len = %d, want 2", len(got.Priorities))
	}
	if got.Priorities[0].InterfaceName != "eth0" || got.Priorities[0].RouteMetric != 500 {
		t.Fatalf("first priority = %+v, want eth0 metric 500", got.Priorities[0])
	}
	if got.Priorities[1].InterfaceName != "wlan0" || got.Priorities[1].RouteMetric != 300 {
		t.Fatalf("second priority = %+v, want wlan0 metric 300", got.Priorities[1])
	}
}

func TestResolvePriorityTargets(t *testing.T) {
	items := []model.NetworkInterface{
		{Name: "eth0", State: "connected", ConnectionName: "eth0", Managed: true},
		{Name: "wlan0", State: "connected", ConnectionName: "Office", Managed: true},
	}

	got, err := resolvePriorityTargets(items, []model.NetworkPriorityBatchItem{
		{InterfaceName: "wlan0", RouteMetric: 100},
		{InterfaceName: "eth0", RouteMetric: 300},
	})
	if err != nil {
		t.Fatalf("resolvePriorityTargets() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("targets len = %d, want 2", len(got))
	}
	if got[0].item.Name != "wlan0" || got[0].routeMetric != 100 {
		t.Fatalf("first target = %+v, want wlan0 metric 100", got[0])
	}
	if got[1].item.Name != "eth0" || got[1].routeMetric != 300 {
		t.Fatalf("second target = %+v, want eth0 metric 300", got[1])
	}
}

func TestResolvePriorityTargetsRejectsDuplicateInterface(t *testing.T) {
	items := []model.NetworkInterface{
		{Name: "eth0", State: "connected", ConnectionName: "eth0", Managed: true},
	}

	_, err := resolvePriorityTargets(items, []model.NetworkPriorityBatchItem{
		{InterfaceName: "eth0", RouteMetric: 100},
		{InterfaceName: "eth0", RouteMetric: 300},
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("resolvePriorityTargets() error = %v, want %v", err, ErrInvalidConfig)
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
