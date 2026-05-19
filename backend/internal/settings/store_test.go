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
	networkReq := model.NetworkSettings{
		Priorities: []model.NetworkPrioritySetting{
			{InterfaceName: "eth0", ConnectionName: "eth0", RouteMetric: 100},
			{InterfaceName: "wlan0", ConnectionName: "Office", RouteMetric: 300},
		},
	}
	userReq := model.UserSettings{
		ManualDeviceLocation: &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
	}

	if err := store.Save(detectionReq); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.SaveGPS(gpsReq); err != nil {
		t.Fatalf("SaveGPS() error = %v", err)
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
	if len(gotNetwork.Priorities) != 2 || gotNetwork.Priorities[0].RouteMetric != 100 {
		t.Fatalf("network settings = %+v, want %+v", gotNetwork, networkReq)
	}
	if gotUser.ManualDeviceLocation == nil ||
		gotUser.ManualDeviceLocation.Latitude != userReq.ManualDeviceLocation.Latitude ||
		gotUser.ManualDeviceLocation.Longitude != userReq.ManualDeviceLocation.Longitude {
		t.Fatalf("user settings = %+v, want %+v", gotUser, userReq)
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
