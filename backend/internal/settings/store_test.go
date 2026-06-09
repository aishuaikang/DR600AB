package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dr600ab-api/internal/model"
)

func TestStoreKeepsDetectionGPSNetworkAndUserSettings(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	detectionReq := model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}
	gpsReq := model.GPSSessionRequest{
		DataPortName:    "/dev/ttyUSB1",
		ControlPortName: "/dev/ttyUSB2",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		Parity:          "none",
	}
	compassReq := model.CompassSessionRequest{
		PortName: "/dev/ttyUSB3",
		BaudRate: 115200,
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}
	networkReq := model.NetworkSettings{
		Priorities: []model.NetworkPrioritySetting{
			{InterfaceName: "eth0", ConnectionName: "eth0", RouteMetric: 100},
			{InterfaceName: "wlan0", ConnectionName: "Office", RouteMetric: 300},
		},
	}
	userReq := model.UserSettings{
		DeviceSN:                  "10125",
		ManualDeviceLocation:      &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
		ScreenStrikeChannelLabels: []string{"2.4G", "5.2G", "5.8G"},
	}
	wantDeviceSN := StandardDeviceSN(userReq.DeviceSN)

	if err := store.Save(detectionReq); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.SaveGPS(gpsReq); err != nil {
		t.Fatalf("SaveGPS() error = %v", err)
	}
	if err := store.SaveCompass(compassReq); err != nil {
		t.Fatalf("SaveCompass() error = %v", err)
	}
	if err := store.SaveNetwork(networkReq); err != nil {
		t.Fatalf("SaveNetwork() error = %v", err)
	}
	if err := store.SaveUser(userReq); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}

	gotDetection, ok, err := store.Load()
	if err != nil || !ok {
		t.Fatalf("Load() = %+v, %v, %v", gotDetection, ok, err)
	}
	gotGPS, ok, err := store.LoadGPS()
	if err != nil || !ok {
		t.Fatalf("LoadGPS() = %+v, %v, %v", gotGPS, ok, err)
	}
	gotCompass, ok, err := store.LoadCompass()
	if err != nil || !ok {
		t.Fatalf("LoadCompass() = %+v, %v, %v", gotCompass, ok, err)
	}
	gotNetwork, ok, err := store.LoadNetwork()
	if err != nil || !ok {
		t.Fatalf("LoadNetwork() = %+v, %v, %v", gotNetwork, ok, err)
	}
	gotUser, ok, err := store.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", gotUser, ok, err)
	}
	if gotDetection.RxPortName != detectionReq.RxPortName || gotDetection.TxPortName != detectionReq.TxPortName {
		t.Fatalf("detection settings = %+v, want %+v", gotDetection, detectionReq)
	}
	if gotGPS.DataPortName != gpsReq.DataPortName || gotGPS.ControlPortName != gpsReq.ControlPortName {
		t.Fatalf("gps settings = %+v, want %+v", gotGPS, gpsReq)
	}
	if gotCompass.PortName != compassReq.PortName {
		t.Fatalf("compass settings = %+v, want %+v", gotCompass, compassReq)
	}
	if len(gotNetwork.Priorities) != 2 || gotNetwork.Priorities[0].RouteMetric != 100 {
		t.Fatalf("network settings = %+v, want %+v", gotNetwork, networkReq)
	}
	if gotUser.ManualDeviceLocation == nil ||
		gotUser.ManualDeviceLocation.Latitude != userReq.ManualDeviceLocation.Latitude ||
		gotUser.ManualDeviceLocation.Longitude != userReq.ManualDeviceLocation.Longitude {
		t.Fatalf("user settings = %+v, want %+v", gotUser, userReq)
	}
	if gotUser.DeviceSN != wantDeviceSN {
		t.Fatalf("user device SN = %q, want %q", gotUser.DeviceSN, wantDeviceSN)
	}
	if gotUser.DeviceHardwareID != userReq.DeviceSN {
		t.Fatalf("user hardware ID = %q, want %q", gotUser.DeviceHardwareID, userReq.DeviceSN)
	}
	if len(gotUser.ScreenStrikeChannelLabels) != len(userReq.ScreenStrikeChannelLabels) ||
		gotUser.ScreenStrikeChannelLabels[0] != userReq.ScreenStrikeChannelLabels[0] ||
		gotUser.ScreenStrikeChannelLabels[1] != userReq.ScreenStrikeChannelLabels[1] ||
		gotUser.ScreenStrikeChannelLabels[2] != userReq.ScreenStrikeChannelLabels[2] {
		t.Fatalf("user strike labels = %+v, want %+v", gotUser.ScreenStrikeChannelLabels, userReq.ScreenStrikeChannelLabels)
	}
}

func TestStoreClearsUserSettingsWithoutOverwritingOtherSettings(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	detectionReq := model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}
	userReq := model.UserSettings{
		ManualDeviceLocation: &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
	}

	if err := store.Save(detectionReq); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.SaveUser(userReq); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}
	if err := store.SaveUser(model.UserSettings{}); err != nil {
		t.Fatalf("SaveUser(clear) error = %v", err)
	}

	gotUser, ok, err := store.LoadUser()
	if err != nil {
		t.Fatalf("LoadUser() error = %v", err)
	}
	if ok || gotUser.ManualDeviceLocation != nil {
		t.Fatalf("LoadUser() = %+v, %v, want cleared", gotUser, ok)
	}
	gotDetection, ok, err := store.Load()
	if err != nil || !ok {
		t.Fatalf("Load() = %+v, %v, %v", gotDetection, ok, err)
	}
	if gotDetection.RxPortName != detectionReq.RxPortName || gotDetection.TxPortName != detectionReq.TxPortName {
		t.Fatalf("detection settings = %+v, want %+v", gotDetection, detectionReq)
	}
}

func TestStoreLoadsClearedStructuredSettings(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(model.DetectionSessionRequest{}); err != nil {
		t.Fatalf("Save(clear detection) error = %v", err)
	}
	if err := store.SaveGPS(model.GPSSessionRequest{}); err != nil {
		t.Fatalf("SaveGPS(clear gps) error = %v", err)
	}
	if err := store.SaveDeception(model.DeceptionSessionRequest{}); err != nil {
		t.Fatalf("SaveDeception(clear deception) error = %v", err)
	}
	if err := store.SaveCompass(model.CompassSessionRequest{}); err != nil {
		t.Fatalf("SaveCompass(clear compass) error = %v", err)
	}

	gotDetection, ok, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok || gotDetection.PortName != "" || gotDetection.RxPortName != "" || gotDetection.TxPortName != "" {
		t.Fatalf("Load() = %+v, %v, want empty structured detection settings", gotDetection, ok)
	}
	gotGPS, ok, err := store.LoadGPS()
	if err != nil {
		t.Fatalf("LoadGPS() error = %v", err)
	}
	if !ok || gotGPS.PortName != "" || gotGPS.DataPortName != "" || gotGPS.ControlPortName != "" {
		t.Fatalf("LoadGPS() = %+v, %v, want empty structured gps settings", gotGPS, ok)
	}
	gotDeception, ok, err := store.LoadDeception()
	if err != nil {
		t.Fatalf("LoadDeception() error = %v", err)
	}
	if !ok || gotDeception.PortName != "" {
		t.Fatalf("LoadDeception() = %+v, %v, want empty structured deception settings", gotDeception, ok)
	}
	gotCompass, ok, err := store.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass() error = %v", err)
	}
	if !ok || gotCompass.PortName != "" {
		t.Fatalf("LoadCompass() = %+v, %v, want empty structured compass settings", gotCompass, ok)
	}
}

func TestStoreWritesEmptyNetworkPrioritiesAsArray(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "settings.json")
	store := NewStore(storePath)
	req := model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}

	if err := store.Save(req); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), `"priorities": null`) {
		t.Fatalf("settings json contains null priorities: %s", string(data))
	}
	if !strings.Contains(string(data), `"priorities": []`) {
		t.Fatalf("settings json = %s, want empty priorities array", string(data))
	}
}

func TestStoreLoadsUserSettingsWithOnlyDeviceSN(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))

	if err := store.SaveUser(model.UserSettings{DeviceSN: "10125"}); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}

	gotUser, ok, err := store.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", gotUser, ok, err)
	}
	if gotUser.DeviceSN != StandardDeviceSN("10125") {
		t.Fatalf("device SN = %q, want %q", gotUser.DeviceSN, StandardDeviceSN("10125"))
	}
	if gotUser.DeviceHardwareID != "10125" {
		t.Fatalf("hardware ID = %q, want 10125", gotUser.DeviceHardwareID)
	}
}

func TestStandardDeviceSN(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "single hardware id",
			raw:  "10125",
			want: "SL67CB3FC848FA0E795P",
		},
		{
			name: "multiple hardware ids are sorted",
			raw:  "20250/10125",
			want: "SL68326A20CF8DBBE36P",
		},
		{
			name: "already standard",
			raw:  "sl67cb3fc848fa0e795p",
			want: "SL67CB3FC848FA0E795P",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StandardDeviceSN(tt.raw); got != tt.want {
				t.Fatalf("StandardDeviceSN(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestStoreSavesEditableUserSettingsWithoutOverwritingDeviceSN(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.SaveUserDeviceSN("10125"); err != nil {
		t.Fatalf("SaveUserDeviceSN() error = %v", err)
	}
	wantDeviceSN := StandardDeviceSN("10125")
	wantHardwareID := "10125"

	saved, err := store.SaveEditableUser(model.UserSettings{
		DeviceSN:             "client-sn",
		DeviceHardwareID:     "client-hardware-id",
		ManualDeviceLocation: &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
	})
	if err != nil {
		t.Fatalf("SaveEditableUser() error = %v", err)
	}
	if saved.DeviceSN != wantDeviceSN {
		t.Fatalf("returned device SN = %q, want preserved %q", saved.DeviceSN, wantDeviceSN)
	}
	if saved.DeviceHardwareID != wantHardwareID {
		t.Fatalf("returned hardware ID = %q, want preserved %q", saved.DeviceHardwareID, wantHardwareID)
	}

	gotUser, ok, err := store.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", gotUser, ok, err)
	}
	if gotUser.DeviceSN != wantDeviceSN {
		t.Fatalf("stored device SN = %q, want preserved %q", gotUser.DeviceSN, wantDeviceSN)
	}
	if gotUser.DeviceHardwareID != wantHardwareID {
		t.Fatalf("stored hardware ID = %q, want preserved %q", gotUser.DeviceHardwareID, wantHardwareID)
	}
	if gotUser.ManualDeviceLocation == nil ||
		gotUser.ManualDeviceLocation.Latitude != 23.12911 ||
		gotUser.ManualDeviceLocation.Longitude != 113.264385 {
		t.Fatalf("manual location = %+v, want saved value", gotUser.ManualDeviceLocation)
	}
}

func TestStoreSavesArchiveRetentionDays(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	intrusionRetentionDays := 0
	fpvVideoRetentionDays := 180

	if err := store.SaveUser(model.UserSettings{
		IntrusionRetentionDays: &intrusionRetentionDays,
		FPVVideoRetentionDays:  &fpvVideoRetentionDays,
	}); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}

	gotUser, ok, err := store.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", gotUser, ok, err)
	}
	if gotUser.IntrusionRetentionDays == nil || *gotUser.IntrusionRetentionDays != 0 {
		t.Fatalf("retention days = %#v, want 0", gotUser.IntrusionRetentionDays)
	}
	if gotUser.FPVVideoRetentionDays == nil || *gotUser.FPVVideoRetentionDays != 180 {
		t.Fatalf("fpv video retention days = %#v, want 180", gotUser.FPVVideoRetentionDays)
	}
}

func TestStoreSavesWhitelistAndScreenAlarmSettings(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	req := model.UserSettings{
		Whitelist: []model.WhitelistItem{
			{Serial: "DJI-001", Model: "Mavic 3", Source: "manual"},
		},
		ScreenAlarmSettings: &model.ScreenAlarmSettings{
			Detection: true,
			Position:  false,
			FPV:       true,
			Sound:     false,
		},
	}

	if err := store.SaveUser(req); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}

	gotUser, ok, err := store.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", gotUser, ok, err)
	}
	if len(gotUser.Whitelist) != 1 || gotUser.Whitelist[0].Serial != "DJI-001" {
		t.Fatalf("whitelist = %#v, want saved item", gotUser.Whitelist)
	}
	if gotUser.ScreenAlarmSettings == nil ||
		!gotUser.ScreenAlarmSettings.Detection ||
		gotUser.ScreenAlarmSettings.Position ||
		!gotUser.ScreenAlarmSettings.FPV ||
		gotUser.ScreenAlarmSettings.Sound {
		t.Fatalf("screen alarm settings = %#v, want saved explicit values", gotUser.ScreenAlarmSettings)
	}
}
