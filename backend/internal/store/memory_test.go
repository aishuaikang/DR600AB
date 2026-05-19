package store

import (
	"testing"
	"time"

	"dr600ab-api/internal/model"
)

func TestMemoryStoreScreenDetectionsMergeSameModelWithinThreshold(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "DJI_OC123_10M", 5744, -55, base.Add(time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	if items[0].Frequency != 5744 {
		t.Fatalf("frequency = %v, want latest 5744", items[0].Frequency)
	}
	if items[0].RSSI != -55 {
		t.Fatalf("rssi = %v, want latest -55", items[0].RSSI)
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want 2", items[0].HitCount)
	}
	wantDevices := []string{"device-a", "device-b"}
	assertStrings(t, items[0].Devices, wantDevices)
}

func TestMemoryStoreScreenDetectionsCreatesStableUniqueIDs(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Autel_type1", 5730, -55, base))

	items := st.ListScreenDetections(10)
	if len(items) != 2 {
		t.Fatalf("screen detections count = %d, want 2", len(items))
	}
	if items[0].ID == items[1].ID {
		t.Fatalf("screen detection ids should be unique, got %q", items[0].ID)
	}
}

func TestMemoryStoreScreenDetectionsMergeDJIFamily(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "DJI_OC123_20M", 5740, -55, base.Add(time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
}

func TestMemoryStoreScreenDetectionsAutelThreshold(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "Autel_type1", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Autel_type4", 5769, -55, base.Add(time.Second)))
	st.AddDetection(screenDetectionRecord("r3", "device-c", "DJI_OC123_10M", 5770, -50, base.Add(2*time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 2 {
		t.Fatalf("screen detections count = %d, want 2", len(items))
	}
	autel := findScreenDetectionByModel(items, "Autel_type4")
	if autel == nil {
		t.Fatalf("autel target not found: %#v", items)
	}
	assertStrings(t, autel.Devices, []string{"device-a", "device-b"})
}

func TestMemoryStoreScreenDetectionsKeepStableOrderAfterUpdate(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Autel_type1", 5730, -55, base.Add(time.Second)))
	st.AddDetection(screenDetectionRecord("r3", "device-c", "DJI_OC123_20M", 5735, -50, base.Add(2*time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 2 {
		t.Fatalf("screen detections count = %d, want 2", len(items))
	}
	if items[0].Model != "Autel_type1" || items[1].Model != "DJI_OC123_20M" {
		t.Fatalf("unexpected stable order/models: %#v", items)
	}
}

func TestMemoryStoreScreenDetectionsRetainsNewestTargetsAtLimit(t *testing.T) {
	st := NewMemoryStore(2, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Autel_type1", 5730, -55, base.Add(time.Second)))
	st.AddDetection(screenDetectionRecord("r3", "device-c", "DJI_OC123_20M", 5735, -50, base.Add(2*time.Second)))
	st.AddDetection(screenDetectionRecord("r4", "device-d", "O3+_ofdm_datalink", 5770, -45, base.Add(3*time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 2 {
		t.Fatalf("screen detections count = %d, want 2", len(items))
	}
	if items[0].Model != "O3+_ofdm_datalink" || items[1].Model != "Autel_type1" {
		t.Fatalf("unexpected retained targets: %#v", items)
	}
}

func TestMemoryStoreScreenDetectionsCollapsesExistingDuplicates(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Different", 5800, -59, base.Add(time.Second)))

	st.mu.Lock()
	st.screen = append(st.screen, model.ScreenDetectionTarget{
		ID:        "legacy-duplicate",
		Model:     "DJI_OC123_10M",
		Frequency: 5730,
		RSSI:      -58,
		Devices:   []string{"device-legacy"},
		FirstSeen: base.Add(-time.Minute),
		LastSeen:  base.Add(-time.Second),
		HitCount:  1,
	})
	st.mu.Unlock()

	st.AddDetection(screenDetectionRecord("r3", "device-c", "DJI_OC123_20M", 5735, -50, base.Add(2*time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 2 {
		t.Fatalf("screen detections count = %d, want 2", len(items))
	}
	merged := findScreenDetectionByModel(items, "DJI_OC123_20M")
	if merged == nil {
		t.Fatalf("merged target not found: %#v", items)
	}
	assertStrings(t, merged.Devices, []string{"device-a", "device-legacy", "device-c"})
}

func TestMemoryStoreScreenDetectionsLastRecordOmitsParsedPayload(t *testing.T) {
	st := NewMemoryStore(10, 10)
	record := screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, time.Now())
	record.Parsed = model.ParsedMessage{
		Type: "detect",
		Raw:  "device=device-a, model=DJI_OC123_10M, freq=5730, rssi=-60",
	}

	st.AddDetection(record)

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	if items[0].LastRecord.ID != "r1" {
		t.Fatalf("last record id = %q, want r1", items[0].LastRecord.ID)
	}
}

func TestMemoryStoreScreenDetectionsPrunesExpiredTargets(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "DJI_OC123_20M", 5735, -55, base.Add(screenDetectionTTL+time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	assertStrings(t, items[0].Devices, []string{"device-b"})
}

func TestMemoryStoreScreenDetectionsIgnoreNonDetect(t *testing.T) {
	st := NewMemoryStore(10, 10)
	record := screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, time.Now())
	record.Kind = "heartbeat"

	st.AddDetection(record)

	items := st.ListScreenDetections(10)
	if len(items) != 0 {
		t.Fatalf("screen detections count = %d, want 0", len(items))
	}
}

func TestMemoryStoreScreenPositionsMergeBySerial(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddScreenPosition(screenPositionTarget("sn-1", "device-a", "DJI Mini", base))
	st.AddScreenPosition(screenPositionTarget("sn-1", "device-b", "DJI Mini 4", base.Add(time.Second)))

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "DJI Mini 4" {
		t.Fatalf("model = %q, want latest DJI Mini 4", items[0].Model)
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want 2", items[0].HitCount)
	}
	assertStrings(t, items[0].Devices, []string{"device-a", "device-b"})
	if items[0].Drone == nil || items[0].Drone.Latitude != 31.2 {
		t.Fatalf("unexpected drone point: %#v", items[0].Drone)
	}
}

func TestMemoryStoreScreenPositionsPrunesExpiredTargets(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddScreenPosition(screenPositionTarget("sn-old", "device-a", "DJI Mini", base))
	st.AddScreenPosition(screenPositionTarget("sn-new", "device-b", "DJI Mini 4", base.Add(screenPositionTTL+time.Second)))

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "sn-new" {
		t.Fatalf("serial = %q, want sn-new", items[0].Serial)
	}
}

func TestMemoryStoreScreenPositionsIgnoreIncompleteTarget(t *testing.T) {
	st := NewMemoryStore(10, 10)
	target := screenPositionTarget("", "device-a", "DJI Mini", time.Now())

	st.AddScreenPosition(target)

	items := st.ListScreenPositions(10)
	if len(items) != 0 {
		t.Fatalf("screen positions count = %d, want 0", len(items))
	}
}

func TestMemoryStoreScreenPositionsKeepTargetWithoutCoordinates(t *testing.T) {
	st := NewMemoryStore(10, 10)
	target := screenPositionTarget("sn-1", "device-a", "DJI Mini", time.Now())
	target.Drone = nil
	target.Pilot = nil
	target.Home = nil

	st.AddScreenPosition(target)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "sn-1" {
		t.Fatalf("serial = %q, want sn-1", items[0].Serial)
	}
	if items[0].Drone != nil || items[0].Pilot != nil || items[0].Home != nil {
		t.Fatalf("expected no coordinates, got drone=%#v pilot=%#v home=%#v", items[0].Drone, items[0].Pilot, items[0].Home)
	}
}

func screenDetectionRecord(
	id string,
	device string,
	modelName string,
	frequency float64,
	rssi float64,
	receivedAt time.Time,
) model.DetectionRecord {
	return model.DetectionRecord{
		ID:         id,
		SessionID:  "session",
		PortName:   "COM1",
		Kind:       "detect",
		ReceivedAt: receivedAt,
		Device:     device,
		Model:      modelName,
		Frequency:  frequency,
		RSSI:       rssi,
		Summary:    modelName,
	}
}

func screenPositionTarget(serial string, device string, modelName string, seenAt time.Time) model.ScreenPositionTarget {
	return model.ScreenPositionTarget{
		Serial:    serial,
		Model:     modelName,
		Source:    "rid",
		Frequency: 2437,
		RSSI:      -68,
		Devices:   []string{device},
		Drone:     &model.ScreenPositionPoint{Latitude: 31.2, Longitude: 121.4},
		FirstSeen: seenAt,
		LastSeen:  seenAt,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       "rid",
			ReceivedAt: seenAt,
			Device:     device,
			Serial:     serial,
			Model:      modelName,
			Frequency:  2437,
			RSSI:       -68,
		},
	}
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("strings len = %d, want %d: got %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("strings[%d] = %q, want %q: got %#v", index, got[index], want[index], got)
		}
	}
}

func findScreenDetectionByModel(items []model.ScreenDetectionTarget, modelName string) *model.ScreenDetectionTarget {
	for index := range items {
		if items[index].Model == modelName {
			return &items[index]
		}
	}
	return nil
}
