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
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
	}
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
	if autel.Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", autel.Device)
	}
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
		Device:    "device-legacy",
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
	if merged.Device != "device-c" {
		t.Fatalf("device = %q, want latest device-c", merged.Device)
	}
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
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want device-b", items[0].Device)
	}
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
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 31.2 {
		t.Fatalf("unexpected drone point: %#v", items[0].Drone)
	}
}

func TestMemoryStoreScreenPositionsTracksDroneAndPilotTrajectory(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	first.Pilot = &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3}
	first.Speed = floatPtr(8.5)
	first.Height = floatPtr(30)
	second := screenPositionTarget("sn-1", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.25, Longitude: 121.45}
	second.Pilot = &model.ScreenPositionPoint{Latitude: 31.15, Longitude: 121.35}
	second.Speed = floatPtr(10.5)
	second.Height = floatPtr(45)

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 2 {
		t.Fatalf("drone trajectory count = %d, want 2: %#v", len(items[0].DroneTrajectory), items[0].DroneTrajectory)
	}
	if len(items[0].PilotTrajectory) != 2 {
		t.Fatalf("pilot trajectory count = %d, want 2: %#v", len(items[0].PilotTrajectory), items[0].PilotTrajectory)
	}

	firstDrone := items[0].DroneTrajectory[0]
	if firstDrone.Latitude != 31.2 || firstDrone.Longitude != 121.4 {
		t.Fatalf("first drone trajectory point = %#v, want initial drone coordinate", firstDrone)
	}
	if firstDrone.Speed == nil || *firstDrone.Speed != 8.5 {
		t.Fatalf("first drone speed = %#v, want 8.5", firstDrone.Speed)
	}
	if firstDrone.Height == nil || *firstDrone.Height != 30 {
		t.Fatalf("first drone height = %#v, want 30", firstDrone.Height)
	}

	secondPilot := items[0].PilotTrajectory[1]
	if secondPilot.Latitude != 31.15 || secondPilot.Longitude != 121.35 {
		t.Fatalf("second pilot trajectory point = %#v, want latest pilot coordinate", secondPilot)
	}
	if secondPilot.Speed == nil || *secondPilot.Speed != 10.5 {
		t.Fatalf("second pilot speed = %#v, want 10.5", secondPilot.Speed)
	}
	if secondPilot.Height == nil || *secondPilot.Height != 45 {
		t.Fatalf("second pilot height = %#v, want 45", secondPilot.Height)
	}
}

func TestMemoryStoreScreenPositionsTracksZeroSpeedAndHeight(t *testing.T) {
	st := NewMemoryStore(10, 10)
	target := screenPositionTarget("sn-1", "device-a", "DJI Mini", time.Now())
	target.Speed = nil
	target.Height = nil
	target.TrajectorySpeed = floatPtr(0)
	target.TrajectoryHeight = floatPtr(0)

	st.AddScreenPosition(target)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want 1", len(items[0].DroneTrajectory))
	}
	point := items[0].DroneTrajectory[0]
	if point.Speed == nil || *point.Speed != 0 {
		t.Fatalf("trajectory speed = %#v, want 0", point.Speed)
	}
	if point.Height == nil || *point.Height != 0 {
		t.Fatalf("trajectory height = %#v, want 0", point.Height)
	}
	if items[0].Speed != nil || items[0].Height != nil {
		t.Fatalf("list speed/height should keep existing zero-as-empty behavior, got speed=%#v height=%#v", items[0].Speed, items[0].Height)
	}
}

func TestMemoryStoreScreenPositionsNormalizesNumericModelPrefix(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddScreenPosition(screenPositionTarget("sn-1", "device-a", "66-Air 2S", base))

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "Air 2S" {
		t.Fatalf("model = %q, want Air 2S", items[0].Model)
	}
	if items[0].LastRecord.Model != "Air 2S" {
		t.Fatalf("last record model = %q, want Air 2S", items[0].LastRecord.Model)
	}
}

func TestMemoryStoreScreenPositionsKeepsNonNumericHyphenModel(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddScreenPosition(screenPositionTarget("sn-1", "device-a", "DJI-Drone", base))

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "DJI-Drone" {
		t.Fatalf("model = %q, want DJI-Drone", items[0].Model)
	}
	if items[0].LastRecord.Model != "DJI-Drone" {
		t.Fatalf("last record model = %q, want DJI-Drone", items[0].LastRecord.Model)
	}
}

func TestMemoryStoreScreenPositionsMergeRIDSerialPrefixDifference(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	rid := screenPositionTarget("1581F67QC238Q014Z681", "RID-1581F67QC238Q014Z681", "DJI Mavic 3", base)
	rid.Source = "rid"
	rid.Frequency = 2437
	rid.RSSI = -80
	rid.Drone = &model.ScreenPositionPoint{Latitude: 0, Longitude: 0}
	rid.LastRecord.Type = "rid"
	rid.LastRecord.Device = "RID-1581F67QC238Q014Z681"
	rid.LastRecord.Model = "DJI Mavic 3"
	rid.LastRecord.Frequency = 2437
	rid.LastRecord.RSSI = -80

	decoded := screenPositionTarget("F67QC238Q014Z681", "10134", "Mavic 3 Pro", base.Add(time.Second))
	decoded.Source = "did_encrypted"
	decoded.Cracked = true
	decoded.Frequency = 5816.5
	decoded.RSSI = -59
	decoded.Pilot = &model.ScreenPositionPoint{Latitude: 28.170931, Longitude: 116.994057}
	decoded.LastRecord.Type = "did_encrypted"
	decoded.LastRecord.Device = "10134"
	decoded.LastRecord.Model = "Mavic 3 Pro"
	decoded.LastRecord.Frequency = 5816.5
	decoded.LastRecord.RSSI = -59
	decoded.LastRecord.Cracked = true

	st.AddScreenPosition(rid)
	st.AddScreenPosition(decoded)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "F67QC238Q014Z681" {
		t.Fatalf("serial = %q, want decrypted serial", items[0].Serial)
	}
	if items[0].Model != "Mavic 3 Pro" {
		t.Fatalf("model = %q, want decrypted model", items[0].Model)
	}
	if items[0].Source != "did_encrypted" {
		t.Fatalf("source = %q, want did_encrypted", items[0].Source)
	}
	if items[0].Device != "10134" {
		t.Fatalf("device = %q, want latest device", items[0].Device)
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want 2", items[0].HitCount)
	}

	laterRID := screenPositionTarget("1581F67QC238Q014Z681", "RID-1581F67QC238Q014Z681", "DJI Mavic 3", base.Add(2*time.Second))
	laterRID.Source = "rid"
	laterRID.Frequency = 2437
	laterRID.RSSI = -82
	laterRID.LastRecord.Type = "rid"
	laterRID.LastRecord.Device = "RID-1581F67QC238Q014Z681"
	laterRID.LastRecord.Model = "DJI Mavic 3"
	laterRID.LastRecord.Frequency = 2437
	laterRID.LastRecord.RSSI = -82

	st.AddScreenPosition(laterRID)

	items = st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count after later RID = %d, want 1", len(items))
	}
	if items[0].Model != "Mavic 3 Pro" || items[0].Source != "did_encrypted" || !items[0].Cracked {
		t.Fatalf("expected decoded target to be preserved after later RID, got model=%q source=%q cracked=%v", items[0].Model, items[0].Source, items[0].Cracked)
	}
	if items[0].HitCount != 3 {
		t.Fatalf("hit count after later RID = %d, want 3", items[0].HitCount)
	}
}

func TestMemoryStoreScreenPositionsMergeCorruptedSerialPrefix(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("#'iAL320040274", "device-a", "DJI Mavic 3", base)
	second := screenPositionTarget("3YTBL320040274", "device-b", "DJI Mavic 3", base.Add(time.Second))

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "3YTBL320040274" {
		t.Fatalf("serial = %q, want latest valid serial", items[0].Serial)
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want 2", items[0].HitCount)
	}
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
	}
}

func TestMemoryStoreScreenPositionsDoesNotMergeCorruptedSerialMiddle(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("ABC-1234567890", "device-a", "DJI Mavic 3", base)
	second := screenPositionTarget("XYZ1234567890", "device-b", "DJI Mavic 3", base.Add(time.Second))

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 2 {
		t.Fatalf("screen positions count = %d, want 2", len(items))
	}
}

func TestMemoryStoreScreenPositionsMergeByCorrelationID(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	fallback := screenPositionTarget("86ca8046", "device-a", "DJI-Drone", base)
	fallback.CorrelationID = "did_encrypted:86ca8046"
	fallback.Source = "did_encrypted"
	decoded := screenPositionTarget("o3-sn", "device-b", "DJI O4", base.Add(time.Second))
	decoded.CorrelationID = "did_encrypted:86ca8046"
	decoded.Source = "did_encrypted"
	decoded.Cracked = true
	decoded.LastRecord = model.ScreenPositionLastRecord{
		Type:       "did_encrypted",
		ReceivedAt: decoded.LastSeen,
		Device:     "device-b",
		Serial:     "o3-sn",
		Model:      "DJI O4",
		Frequency:  decoded.Frequency,
		RSSI:       decoded.RSSI,
		Cracked:    true,
	}

	st.AddScreenPosition(fallback)
	st.AddScreenPosition(decoded)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "o3-sn" {
		t.Fatalf("serial = %q, want decrypted serial", items[0].Serial)
	}
	if items[0].Model != "DJI O4" {
		t.Fatalf("model = %q, want decrypted model", items[0].Model)
	}
	if items[0].CorrelationID != "did_encrypted:86ca8046" {
		t.Fatalf("correlation id = %q, want did_encrypted:86ca8046", items[0].CorrelationID)
	}
	if !items[0].Cracked {
		t.Fatalf("expected cracked target after merge")
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want 2", items[0].HitCount)
	}
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
	}
}

func TestMemoryStoreScreenPositionsDoesNotDowngradeCrackedTarget(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	fallback := screenPositionTarget("86ca8046", "device-a", "DJI-Drone", base)
	fallback.CorrelationID = "did_encrypted:86ca8046"
	fallback.Source = "did_encrypted"
	fallback.Drone = nil
	decoded := screenPositionTarget("o3-sn", "device-b", "DJI O4", base.Add(time.Second))
	decoded.CorrelationID = "did_encrypted:86ca8046"
	decoded.Source = "did_encrypted"
	decoded.Cracked = true
	decoded.LastRecord = model.ScreenPositionLastRecord{
		Type:       "did_encrypted",
		ReceivedAt: decoded.LastSeen,
		Device:     "device-b",
		Serial:     "o3-sn",
		Model:      "DJI O4",
		Frequency:  decoded.Frequency,
		RSSI:       decoded.RSSI,
		Cracked:    true,
	}
	secondFallback := screenPositionTarget("86ca8046", "device-c", "DJI-Drone", base.Add(2*time.Second))
	secondFallback.CorrelationID = "did_encrypted:86ca8046"
	secondFallback.Source = "did_encrypted"
	secondFallback.Frequency = 5776.5
	secondFallback.RSSI = -76
	secondFallback.Drone = nil
	secondFallback.LastRecord = model.ScreenPositionLastRecord{
		Type:       "did_encrypted",
		ReceivedAt: secondFallback.LastSeen,
		Device:     "device-c",
		Serial:     "86ca8046",
		Model:      "DJI-Drone",
		Frequency:  5776.5,
		RSSI:       -76,
		Cracked:    false,
	}

	st.AddScreenPosition(fallback)
	st.AddScreenPosition(decoded)
	st.AddScreenPosition(secondFallback)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "o3-sn" {
		t.Fatalf("serial = %q, want decrypted serial", items[0].Serial)
	}
	if items[0].Model != "DJI O4" {
		t.Fatalf("model = %q, want decrypted model", items[0].Model)
	}
	if !items[0].Cracked {
		t.Fatalf("expected target to stay cracked")
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 31.2 {
		t.Fatalf("expected decrypted coordinates to be preserved, got %#v", items[0].Drone)
	}
	if !items[0].LastSeen.Equal(secondFallback.LastSeen) {
		t.Fatalf("last seen = %s, want %s", items[0].LastSeen, secondFallback.LastSeen)
	}
	if items[0].Frequency != 5776.5 || items[0].RSSI != -76 {
		t.Fatalf("expected latest signal fields, got freq=%v rssi=%v", items[0].Frequency, items[0].RSSI)
	}
	if items[0].LastRecord.Model != "DJI O4" || !items[0].LastRecord.Cracked {
		t.Fatalf("expected decrypted last record to be preserved, got %#v", items[0].LastRecord)
	}
	if items[0].Device != "device-c" {
		t.Fatalf("device = %q, want latest device-c", items[0].Device)
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
		Device:    device,
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

func findScreenDetectionByModel(items []model.ScreenDetectionTarget, modelName string) *model.ScreenDetectionTarget {
	for index := range items {
		if items[index].Model == modelName {
			return &items[index]
		}
	}
	return nil
}

func floatPtr(value float64) *float64 {
	return &value
}
