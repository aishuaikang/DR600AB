package network

import (
	"context"
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

func TestParseModemList(t *testing.T) {
	got := parseModemList(stringsJoinLines(
		`modem-list.length   : 1`,
		`modem-list.value[1] : /org/freedesktop/ModemManager1/Modem/0`,
	))

	if len(got) != 1 || got[0] != "0" {
		t.Fatalf("parseModemList() = %+v, want [0]", got)
	}
}

func TestParseModemDetailQuectelEC200U(t *testing.T) {
	got := parseModemDetail(stringsJoinLines(
		`modem.dbus-path                                 : /org/freedesktop/ModemManager1/Modem/0`,
		`modem.generic.manufacturer                      : Quectel`,
		`modem.generic.model                             : EC200U`,
		`modem.generic.revision                          : EC200UCNAAR03A17M08`,
		`modem.generic.equipment-identifier              : 866738087531461`,
		`modem.generic.primary-port                      : ttyUSB5`,
		`modem.generic.ports.length                      : 4`,
		`modem.generic.ports.value[1]                    : ttyUSB0 (at)`,
		`modem.generic.ports.value[2]                    : ttyUSB5 (at)`,
		`modem.generic.ports.value[3]                    : ttyUSB6 (at)`,
		`modem.generic.ports.value[4]                    : usb0 (net)`,
		`modem.generic.state                             : failed`,
		`modem.generic.state-failed-reason               : sim-missing`,
		`modem.generic.power-state                       : on`,
		`modem.generic.signal-quality.value              : 0`,
	))

	if got.ID != "0" || got.Manufacturer != "Quectel" || got.Model != "EC200U" {
		t.Fatalf("modem identity = %+v, want Quectel EC200U id 0", got)
	}
	if got.PrimaryPort != "ttyUSB5" || got.DataInterface != "usb0" {
		t.Fatalf("modem ports = primary %q data %q, want ttyUSB5 usb0", got.PrimaryPort, got.DataInterface)
	}
	if got.State != "failed" || got.FailedReason != "sim-missing" {
		t.Fatalf("modem state = %q/%q, want failed/sim-missing", got.State, got.FailedReason)
	}
}

func TestApplyCellularModemsMarksUsbNetPort(t *testing.T) {
	interfaces := parseDeviceStatus(stringsJoinLines(
		`wlan0:wifi:connected:Office`,
		`ttyUSB5:gsm:unavailable:--`,
	))
	interfaces["usb0"] = model.NetworkInterface{
		Name:       "usb0",
		Type:       "ethernet",
		Kind:       "ethernet",
		State:      "down",
		IPv4:       []model.NetworkAddress{},
		IPv6:       []model.NetworkAddress{},
		DNS4:       []string{},
		DNS6:       []string{},
		IPv4Method: "unknown",
		ReadOnly:   true,
		Source:     "kernel",
	}
	modem := model.CellularModem{
		ID:            "0",
		Manufacturer:  "Quectel",
		Model:         "EC200U",
		PrimaryPort:   "ttyUSB5",
		DataInterface: "usb0",
		State:         "failed",
		FailedReason:  "sim-missing",
		Ports:         []string{"ttyUSB5 (at)", "usb0 (net)"},
	}

	applyCellularModems(interfaces, []model.CellularModem{modem})

	usb := interfaces["usb0"]
	if usb.Kind != "cellular" || usb.Type != "gsm" || !usb.ReadOnly {
		t.Fatalf("usb0 = %+v, want readonly cellular gsm", usb)
	}
	if usb.Modem == nil || usb.Modem.Model != "EC200U" || usb.Modem.FailedReason != "sim-missing" {
		t.Fatalf("usb0 modem = %+v, want EC200U sim-missing", usb.Modem)
	}
	if !hasCapability(model.NetworkInterface{Capabilities: interfaceCapabilities(usb)}, "cellular-connect") {
		t.Fatalf("usb0 capabilities = %+v, want cellular-connect", interfaceCapabilities(usb))
	}
	tty := interfaces["ttyUSB5"]
	if tty.Kind != "cellular" || tty.Modem == nil || tty.Modem.DataInterface != "usb0" {
		t.Fatalf("ttyUSB5 = %+v, want cellular control port with usb0 modem", tty)
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

func TestValidateCellularRequest(t *testing.T) {
	metric := 50
	tests := []struct {
		name string
		req  model.CellularConnectRequest
		want error
	}{
		{name: "valid apn", req: model.CellularConnectRequest{APN: "cmnet", RouteMetric: &metric}},
		{name: "empty apn", req: model.CellularConnectRequest{APN: " "}, want: ErrInvalidCellularConfig},
		{name: "invalid metric", req: model.CellularConnectRequest{APN: "cmnet", RouteMetric: intPtr(-2)}, want: ErrInvalidCellularConfig},
		{name: "invalid interface", req: model.CellularConnectRequest{APN: "cmnet", InterfaceName: "bad name"}, want: ErrInvalidCellularConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCellularRequest(tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("validateCellularRequest() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSelectCellularModemByDataInterface(t *testing.T) {
	modems := []model.CellularModem{
		{ID: "0", PrimaryPort: "ttyUSB5", DataInterface: "usb0"},
	}

	got, ok := selectCellularModem(modems, model.CellularConnectRequest{InterfaceName: "usb0", APN: "cmnet"})

	if !ok || got.ID != "0" {
		t.Fatalf("selectCellularModem() = %+v/%v, want modem 0", got, ok)
	}
}

func TestConfigureCellularConnectionRejectsExistingNonGSMConnection(t *testing.T) {
	runner := newScriptedNetworkRunner()
	runner.responses["nmcli -g connection.type connection show Office"] = "802-11-wireless"
	svc := NewService(runner, nil)

	err := svc.configureCellularConnection(context.Background(), model.CellularModem{PrimaryPort: "ttyUSB5"}, model.CellularConnectRequest{
		APN:            "cmnet",
		ConnectionName: "Office",
	}, "Office")

	if !errors.Is(err, ErrInvalidCellularConfig) {
		t.Fatalf("configureCellularConnection() error = %v, want %v", err, ErrInvalidCellularConfig)
	}
	if runner.called("nmcli connection modify Office") {
		t.Fatalf("configureCellularConnection() modified non-gsm connection")
	}
}

func TestConfigureCellularConnectionAllowsExistingGSMConnection(t *testing.T) {
	runner := newScriptedNetworkRunner()
	runner.responses["nmcli -g connection.type connection show 4g"] = "gsm"
	runner.responses["nmcli connection modify 4g connection.interface-name ttyUSB5 gsm.apn cmnet gsm.username  gsm.password  ipv4.method auto ipv6.method auto"] = ""
	svc := NewService(runner, nil)

	err := svc.configureCellularConnection(context.Background(), model.CellularModem{PrimaryPort: "ttyUSB5"}, model.CellularConnectRequest{
		APN:            "cmnet",
		ConnectionName: "4g",
	}, "4g")

	if err != nil {
		t.Fatalf("configureCellularConnection() error = %v, want nil", err)
	}
	if !runner.called("nmcli connection modify 4g") {
		t.Fatalf("configureCellularConnection() did not modify existing gsm connection")
	}
}

func TestValidateUpdateRequestRouteMetric(t *testing.T) {
	validMetric := 100
	autoMetric := -1
	invalidMetric := -2

	validReq := model.NetworkInterfaceUpdateRequest{
		Mode: "static",
		IPv4: []model.NetworkAddress{
			{Address: "192.168.10.25", Prefix: 24},
			{Address: "192.168.10.26", Prefix: 24},
		},
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

func TestUpdateIPv4AddressesKeepsLegacySingleAddress(t *testing.T) {
	req := model.NetworkInterfaceUpdateRequest{
		Mode:        "static",
		IPv4Address: "192.168.10.25",
		Prefix:      24,
	}

	got := updateIPv4Addresses(req)
	want := []string{"192.168.10.25/24"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("updateIPv4Addresses() = %+v, want %+v", got, want)
	}
	if err := validateUpdateRequest(req); err != nil {
		t.Fatalf("validateUpdateRequest() legacy request error = %v, want nil", err)
	}
}

func TestUpdateIPv4AddressesAllowsMultipleAddresses(t *testing.T) {
	req := model.NetworkInterfaceUpdateRequest{
		Mode: "static",
		IPv4: []model.NetworkAddress{
			{Address: "192.168.10.25", Prefix: 24},
			{Address: "10.10.0.2", Prefix: 16},
		},
	}

	got := updateIPv4Addresses(req)
	want := []string{"192.168.10.25/24", "10.10.0.2/16"}
	if len(got) != len(want) {
		t.Fatalf("updateIPv4Addresses() = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("updateIPv4Addresses()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	if err := validateUpdateRequest(req); err != nil {
		t.Fatalf("validateUpdateRequest() multiple address error = %v, want nil", err)
	}
}

func TestValidateUpdateRequestRejectsDuplicateIPv4Address(t *testing.T) {
	tests := []struct {
		name string
		req  model.NetworkInterfaceUpdateRequest
	}{
		{
			name: "same prefix",
			req: model.NetworkInterfaceUpdateRequest{
				Mode: "static",
				IPv4: []model.NetworkAddress{
					{Address: "192.168.10.25", Prefix: 24},
					{Address: "192.168.10.25", Prefix: 24},
				},
			},
		},
		{
			name: "different prefix",
			req: model.NetworkInterfaceUpdateRequest{
				Mode: "static",
				IPv4: []model.NetworkAddress{
					{Address: "192.168.10.25", Prefix: 24},
					{Address: "192.168.10.25", Prefix: 32},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateUpdateRequest(tt.req); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("validateUpdateRequest() error = %v, want %v", err, ErrInvalidConfig)
			}
		})
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

func intPtr(value int) *int {
	return &value
}

type scriptedNetworkRunner struct {
	responses map[string]string
	calls     []string
}

func newScriptedNetworkRunner() *scriptedNetworkRunner {
	return &scriptedNetworkRunner{
		responses: map[string]string{},
		calls:     []string{},
	}
}

func (r *scriptedNetworkRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := stringsJoinFields(append([]string{name}, args...)...)
	r.calls = append(r.calls, call)
	if response, ok := r.responses[call]; ok {
		return []byte(response), nil
	}
	return nil, errors.New("unexpected command: " + call)
}

func (r *scriptedNetworkRunner) called(prefix string) bool {
	for _, call := range r.calls {
		if len(call) >= len(prefix) && call[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func stringsJoinFields(fields ...string) string {
	result := ""
	for index, field := range fields {
		if index > 0 {
			result += " "
		}
		result += field
	}
	return result
}
