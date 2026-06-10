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

func TestMemoryStoreScreenDetectionsGeneratesSerial(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "DJI_OC123_20M", 5735, -55, base.Add(time.Second)))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	if items[0].Serial == "" {
		t.Fatalf("serial is empty")
	}
	if items[0].Serial == items[0].Device {
		t.Fatalf("serial = device = %q, want generated detection serial", items[0].Serial)
	}
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
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

func TestMemoryStoreScreenDetectionsNormalizesNumericModelPrefix(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "66-Air 2S", 5180, -58, base))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	if items[0].Model != "Air 2S" {
		t.Fatalf("model = %q, want Air 2S", items[0].Model)
	}
	if items[0].DisplayModel != "Air 2S" {
		t.Fatalf("display model = %q, want Air 2S", items[0].DisplayModel)
	}
	if items[0].LastRecord.Model != "Air 2S" {
		t.Fatalf("last record model = %q, want Air 2S", items[0].LastRecord.Model)
	}
	if items[0].LastRecord.DisplayModel != "Air 2S" {
		t.Fatalf("last record display model = %q, want Air 2S", items[0].LastRecord.DisplayModel)
	}
}

func TestMemoryStoreScreenDetectionsKeepsNonNumericHyphenModel(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI-Drone", 5180, -58, base))

	items := st.ListScreenDetections(10)
	if len(items) != 1 {
		t.Fatalf("screen detections count = %d, want 1", len(items))
	}
	if items[0].Model != "DJI-Drone" {
		t.Fatalf("model = %q, want DJI-Drone", items[0].Model)
	}
	if items[0].LastRecord.Model != "DJI-Drone" {
		t.Fatalf("last record model = %q, want DJI-Drone", items[0].LastRecord.Model)
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

func TestMemoryStoreScreenDetectionsArchivesExpiredTargets(t *testing.T) {
	st := NewMemoryStore(10, 10)
	archiver := &memoryIntrusionArchiver{}
	st.SetIntrusionArchiver(archiver)
	base := time.Now()

	st.AddDetection(screenDetectionRecord("r1", "device-a", "DJI_OC123_10M", 5730, -60, base))
	st.AddDetection(screenDetectionRecord("r2", "device-b", "Autel_type1", 5800, -55, base.Add(screenDetectionTTL+time.Second)))
	st.ListScreenDetections(10)

	if len(archiver.detections) != 1 {
		t.Fatalf("archived detections count = %d, want 1", len(archiver.detections))
	}
	if archiver.detections[0].Device != "device-a" {
		t.Fatalf("archived device = %q, want device-a", archiver.detections[0].Device)
	}
	if archiver.detections[0].Serial == "" {
		t.Fatalf("archived serial is empty")
	}
	if archiver.detections[0].Serial == archiver.detections[0].Device {
		t.Fatalf("archived serial = device = %q, want generated detection serial", archiver.detections[0].Serial)
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

func TestMemoryStoreScreenPositionsIgnoreInvalidIncomingFields(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()
	speed := 8.5
	height := 30.0
	altitude := 120.0

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	first.Pilot = &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3}
	first.Home = &model.ScreenPositionPoint{Latitude: 31.0, Longitude: 121.2}
	first.Speed = &speed
	first.Height = &height
	first.Altitude = &altitude

	second := screenPositionTarget("sn-1", "", "DJI Mini 4", base.Add(time.Second))
	second.Frequency = 0
	second.RSSI = 0
	second.Device = ""
	second.Drone = &model.ScreenPositionPoint{Latitude: 0, Longitude: 0}
	second.Pilot = nil
	second.Home = nil
	second.Speed = nil
	second.Height = nil
	second.Altitude = nil
	second.LastRecord = model.ScreenPositionLastRecord{
		Type:       "rid",
		ReceivedAt: base.Add(time.Second),
		Serial:     "sn-1",
	}

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	item := items[0]
	if item.Model != "DJI Mini 4" {
		t.Fatalf("model = %q, want latest valid model", item.Model)
	}
	if item.Frequency != 2437 {
		t.Fatalf("frequency = %v, want previous valid 2437", item.Frequency)
	}
	if item.RSSI != -68 {
		t.Fatalf("rssi = %v, want previous valid -68", item.RSSI)
	}
	if item.Device != "device-a" {
		t.Fatalf("device = %q, want previous valid device-a", item.Device)
	}
	if item.Drone == nil || item.Drone.Latitude != 31.2 || item.Drone.Longitude != 121.4 {
		t.Fatalf("drone point = %#v, want previous valid drone point", item.Drone)
	}
	if item.Pilot == nil || item.Pilot.Latitude != 31.1 || item.Pilot.Longitude != 121.3 {
		t.Fatalf("pilot point = %#v, want previous valid pilot point", item.Pilot)
	}
	if item.Home == nil || item.Home.Latitude != 31.0 || item.Home.Longitude != 121.2 {
		t.Fatalf("home point = %#v, want previous valid home point", item.Home)
	}
	if item.Speed == nil || *item.Speed != speed {
		t.Fatalf("speed = %#v, want previous valid %v", item.Speed, speed)
	}
	if item.Height == nil || *item.Height != height {
		t.Fatalf("height = %#v, want previous valid %v", item.Height, height)
	}
	if item.Altitude == nil || *item.Altitude != altitude {
		t.Fatalf("altitude = %#v, want previous valid %v", item.Altitude, altitude)
	}
	if item.LastRecord.Device != "device-a" ||
		item.LastRecord.Model != "DJI Mini" ||
		item.LastRecord.Frequency != 2437 ||
		item.LastRecord.RSSI != -68 {
		t.Fatalf("last record = %#v, want invalid incoming fields ignored", item.LastRecord)
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
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20005, Longitude: 121.40005}
	second.Pilot = &model.ScreenPositionPoint{Latitude: 31.10005, Longitude: 121.30005}
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
	if secondPilot.Latitude != 31.10005 || secondPilot.Longitude != 121.30005 {
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

func TestMemoryStoreScreenPositionsMergesTrajectoryJitter(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	first.Pilot = &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3}
	first.Speed = floatPtr(8.5)
	first.Height = floatPtr(30)
	second := screenPositionTarget("sn-1", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20002, Longitude: 121.4}
	second.Pilot = &model.ScreenPositionPoint{Latitude: 31.10002, Longitude: 121.3}
	second.Speed = floatPtr(0)
	second.Height = floatPtr(45)

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want 1: %#v", len(items[0].DroneTrajectory), items[0].DroneTrajectory)
	}
	if len(items[0].PilotTrajectory) != 1 {
		t.Fatalf("pilot trajectory count = %d, want 1: %#v", len(items[0].PilotTrajectory), items[0].PilotTrajectory)
	}

	dronePoint := items[0].DroneTrajectory[0]
	if dronePoint.Latitude != 31.20002 || dronePoint.Longitude != 121.4 {
		t.Fatalf("drone trajectory point = %#v, want latest jitter coordinate", dronePoint)
	}
	if !dronePoint.Time.Equal(base.Add(time.Second)) {
		t.Fatalf("drone trajectory time = %v, want %v", dronePoint.Time, base.Add(time.Second))
	}
	if dronePoint.Speed == nil || *dronePoint.Speed != 0 {
		t.Fatalf("drone trajectory speed = %#v, want 0", dronePoint.Speed)
	}
	if dronePoint.Height == nil || *dronePoint.Height != 45 {
		t.Fatalf("drone trajectory height = %#v, want 45", dronePoint.Height)
	}
}

func TestMemoryStoreScreenPositionsKeepsTrajectoryPointWhenLatestValuesMissing(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	first.Speed = floatPtr(8.5)
	first.Height = floatPtr(30)
	second := screenPositionTarget("sn-1", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20002, Longitude: 121.4}
	second.Speed = nil
	second.Height = nil

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want 1: %#v", len(items[0].DroneTrajectory), items[0].DroneTrajectory)
	}
	point := items[0].DroneTrajectory[0]
	if point.Latitude != 31.20002 || point.Longitude != 121.4 {
		t.Fatalf("drone trajectory point = %#v, want latest jitter coordinate", point)
	}
	if point.Speed == nil || *point.Speed != 8.5 {
		t.Fatalf("drone trajectory speed = %#v, want previous 8.5", point.Speed)
	}
	if point.Height == nil || *point.Height != 30 {
		t.Fatalf("drone trajectory height = %#v, want previous 30", point.Height)
	}
}

func TestMemoryStoreScreenPositionsKeepsTrajectoryMovement(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	second := screenPositionTarget("sn-1", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20005, Longitude: 121.4}

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 2 {
		t.Fatalf("drone trajectory count = %d, want 2: %#v", len(items[0].DroneTrajectory), items[0].DroneTrajectory)
	}
}

func TestMemoryStoreScreenPositionsRestartsTrajectoryOnGPSJump(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	first := screenPositionTarget("sn-1", "device-a", "DJI Mini", base)
	first.Pilot = &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3}
	second := screenPositionTarget("sn-1", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.21, Longitude: 121.4}
	second.Pilot = &model.ScreenPositionPoint{Latitude: 31.11, Longitude: 121.3}

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want restarted 1: %#v", len(items[0].DroneTrajectory), items[0].DroneTrajectory)
	}
	if got := items[0].DroneTrajectory[0]; got.Latitude != 31.21 || got.Longitude != 121.4 {
		t.Fatalf("drone trajectory point = %#v, want jump point as new trajectory start", got)
	}
	if len(items[0].PilotTrajectory) != 1 {
		t.Fatalf("pilot trajectory count = %d, want restarted 1: %#v", len(items[0].PilotTrajectory), items[0].PilotTrajectory)
	}
	if got := items[0].PilotTrajectory[0]; got.Latitude != 31.11 || got.Longitude != 121.3 {
		t.Fatalf("pilot trajectory point = %#v, want jump point as new trajectory start", got)
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

	st.AddScreenPosition(screenPositionTarget("sn-1", "device-a", "Vendor-Drone", base))

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "Vendor-Drone" {
		t.Fatalf("model = %q, want Vendor-Drone", items[0].Model)
	}
	if items[0].LastRecord.Model != "Vendor-Drone" {
		t.Fatalf("last record model = %q, want Vendor-Drone", items[0].LastRecord.Model)
	}
}

func TestMemoryStoreScreenPositionsKeepsUncrackedDJIDrone(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()
	target := screenPositionTarget("sn-1", "device-a", "DJI-Drone", base)
	target.Source = "did_encrypted"
	target.Cracked = false
	target.Drone = nil

	_, updated := st.AddScreenPosition(target)
	if !updated {
		t.Fatalf("uncracked DJI-Drone should update screen positions")
	}

	items := st.ListScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "DJI-Drone" || items[0].Cracked {
		t.Fatalf("fallback target = %#v", items[0])
	}
	if items[0].Drone != nil {
		t.Fatalf("fallback drone point = %#v, want nil", items[0].Drone)
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
	if !stringSlicesEqual(items[0].Sources, []string{"rid", "did_encrypted"}) {
		t.Fatalf("sources = %#v, want rid and did_encrypted", items[0].Sources)
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
	if !stringSlicesEqual(items[0].Sources, []string{"rid", "did_encrypted"}) {
		t.Fatalf("sources after later RID = %#v, want rid and did_encrypted", items[0].Sources)
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

func TestMemoryStoreScreenPositionsReplacesDIDEncryptedFallbackWithCrackedTarget(t *testing.T) {
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

	if _, updated := st.AddScreenPosition(fallback); !updated {
		t.Fatalf("uncracked DJI-Drone fallback should update screen positions")
	}
	removed, ok := st.RemoveUncrackedDIDScreenPositionByCorrelationID("did_encrypted:86ca8046")
	if !ok {
		t.Fatalf("expected DID encrypted fallback to be removed")
	}
	if removed.Serial != "86ca8046" || removed.Model != "DJI-Drone" || removed.Cracked {
		t.Fatalf("removed target = %#v", removed)
	}
	if _, updated := st.AddScreenPosition(decoded); !updated {
		t.Fatalf("decoded DID encrypted target should update screen positions")
	}

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
		t.Fatalf("expected cracked target")
	}
	if items[0].HitCount != 1 {
		t.Fatalf("hit count = %d, want decoded target after fallback removal", items[0].HitCount)
	}
	if items[0].Device != "device-b" {
		t.Fatalf("device = %q, want latest device-b", items[0].Device)
	}
}

func TestMemoryStoreHasCrackedScreenPositionByCorrelationID(t *testing.T) {
	st := NewMemoryStore(10, 10)
	base := time.Now()

	fallback := screenPositionTarget("86ca8046", "device-a", "DJI-Drone", base)
	fallback.CorrelationID = "did_encrypted:86ca8046"
	fallback.Source = "did_encrypted"
	fallback.Cracked = false
	decoded := screenPositionTarget("o3-sn", "device-b", "DJI O4", base.Add(time.Second))
	decoded.CorrelationID = "did_encrypted:86ca8046"
	decoded.Source = "did_encrypted"
	decoded.Cracked = true

	st.AddScreenPosition(fallback)
	if st.HasCrackedScreenPositionByCorrelationID("did_encrypted:86ca8046") {
		t.Fatalf("fallback should not count as cracked correlation")
	}
	st.RemoveUncrackedDIDScreenPositionByCorrelationID("did_encrypted:86ca8046")
	st.AddScreenPosition(decoded)
	if !st.HasCrackedScreenPositionByCorrelationID("did_encrypted:86ca8046") {
		t.Fatalf("decoded target should count as cracked correlation")
	}
	if st.HasCrackedScreenPositionByCorrelationID("did_encrypted:missing") {
		t.Fatalf("missing correlation should not count as cracked")
	}
}

func TestMemoryStoreScreenPositionsDoesNotMergeLaterFallbackIntoCrackedDIDEncrypted(t *testing.T) {
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

	if _, updated := st.AddScreenPosition(fallback); !updated {
		t.Fatalf("uncracked DJI-Drone fallback should update screen positions")
	}
	st.RemoveUncrackedDIDScreenPositionByCorrelationID("did_encrypted:86ca8046")
	st.AddScreenPosition(decoded)
	if _, updated := st.AddScreenPosition(secondFallback); !updated {
		t.Fatalf("later uncracked DJI-Drone fallback should update screen positions")
	}

	items := st.ListScreenPositions(10)
	if len(items) != 2 {
		t.Fatalf("screen positions count = %d, want cracked target plus fallback", len(items))
	}

	var cracked, pending *model.ScreenPositionTarget
	for index := range items {
		switch {
		case items[index].Cracked:
			cracked = &items[index]
		case items[index].Model == "DJI-Drone":
			pending = &items[index]
		}
	}
	if cracked == nil {
		t.Fatalf("cracked target not found: %#v", items)
	}
	if pending == nil {
		t.Fatalf("pending fallback target not found: %#v", items)
	}
	if cracked.Serial != "o3-sn" {
		t.Fatalf("serial = %q, want decrypted serial", cracked.Serial)
	}
	if cracked.Model != "DJI O4" {
		t.Fatalf("model = %q, want decrypted model", cracked.Model)
	}
	if cracked.Drone == nil || cracked.Drone.Latitude != 31.2 {
		t.Fatalf("expected decrypted coordinates to be preserved, got %#v", cracked.Drone)
	}
	if cracked.LastRecord.Model != "DJI O4" || !cracked.LastRecord.Cracked {
		t.Fatalf("expected decrypted last record to be preserved, got %#v", cracked.LastRecord)
	}
	if pending.Serial != "86ca8046" || pending.Cracked {
		t.Fatalf("pending fallback = %#v", pending)
	}

	if _, ok := st.RemoveUncrackedDIDScreenPositionByCorrelationID("did_encrypted:86ca8046"); !ok {
		t.Fatalf("expected later fallback to be removed by correlation id")
	}
	items = st.ListScreenPositions(10)
	if len(items) != 1 || items[0].Serial != "o3-sn" || !items[0].Cracked {
		t.Fatalf("positions after fallback removal = %#v", items)
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

func TestMemoryStoreScreenPositionsArchivesExpiredTargetsWithTrajectory(t *testing.T) {
	st := NewMemoryStore(10, 10)
	archiver := &memoryIntrusionArchiver{}
	st.SetIntrusionArchiver(archiver)
	base := time.Now()

	first := screenPositionTarget("sn-old", "device-a", "DJI Mini", base)
	first.Pilot = &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3}
	first.Speed = floatPtr(8.5)
	first.Height = floatPtr(30)
	second := screenPositionTarget("sn-old", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20005, Longitude: 121.40005}
	second.Pilot = &model.ScreenPositionPoint{Latitude: 31.10005, Longitude: 121.30005}
	second.Speed = floatPtr(10.5)
	second.Height = floatPtr(45)
	newTarget := screenPositionTarget("sn-new", "device-b", "DJI Mini 4", base.Add(screenPositionTTL+2*time.Second))

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)
	st.AddScreenPosition(newTarget)
	st.ListScreenPositions(10)

	if len(archiver.positions) != 1 {
		t.Fatalf("archived positions count = %d, want 1", len(archiver.positions))
	}
	archived := archiver.positions[0]
	if archived.Serial != "sn-old" {
		t.Fatalf("archived serial = %q, want sn-old", archived.Serial)
	}
	if len(archived.DroneTrajectory) != 2 {
		t.Fatalf("archived drone trajectory count = %d, want 2", len(archived.DroneTrajectory))
	}
	point := archived.DroneTrajectory[1]
	if point.Latitude != 31.20005 || point.Longitude != 121.40005 {
		t.Fatalf("archived trajectory point = %#v, want latest drone point", point)
	}
	if point.Speed == nil || *point.Speed != 10.5 {
		t.Fatalf("archived trajectory speed = %#v, want 10.5", point.Speed)
	}
	if point.Height == nil || *point.Height != 45 {
		t.Fatalf("archived trajectory height = %#v, want 45", point.Height)
	}
}

func TestMemoryStoreScreenPositionsArchivesDeduplicatedTrajectory(t *testing.T) {
	st := NewMemoryStore(10, 10)
	archiver := &memoryIntrusionArchiver{}
	st.SetIntrusionArchiver(archiver)
	base := time.Now()

	first := screenPositionTarget("sn-old", "device-a", "DJI Mini", base)
	first.Speed = floatPtr(8.5)
	first.Height = floatPtr(30)
	second := screenPositionTarget("sn-old", "device-a", "DJI Mini", base.Add(time.Second))
	second.Drone = &model.ScreenPositionPoint{Latitude: 31.20002, Longitude: 121.4}
	second.Speed = floatPtr(0)
	second.Height = floatPtr(45)
	newTarget := screenPositionTarget("sn-new", "device-b", "DJI Mini 4", base.Add(screenPositionTTL+2*time.Second))

	st.AddScreenPosition(first)
	st.AddScreenPosition(second)
	st.AddScreenPosition(newTarget)
	st.ListScreenPositions(10)

	if len(archiver.positions) != 1 {
		t.Fatalf("archived positions count = %d, want 1", len(archiver.positions))
	}
	archived := archiver.positions[0]
	if len(archived.DroneTrajectory) != 1 {
		t.Fatalf("archived drone trajectory count = %d, want 1: %#v", len(archived.DroneTrajectory), archived.DroneTrajectory)
	}
	point := archived.DroneTrajectory[0]
	if point.Latitude != 31.20002 || point.Longitude != 121.4 {
		t.Fatalf("archived trajectory point = %#v, want latest jitter coordinate", point)
	}
	if !point.Time.Equal(base.Add(time.Second)) {
		t.Fatalf("archived trajectory time = %v, want %v", point.Time, base.Add(time.Second))
	}
	if point.Speed == nil || *point.Speed != 0 {
		t.Fatalf("archived trajectory speed = %#v, want 0", point.Speed)
	}
	if point.Height == nil || *point.Height != 45 {
		t.Fatalf("archived trajectory height = %#v, want 45", point.Height)
	}
}

func TestMemoryStoreScreenPositionsArchivesExpiredTargetsOnce(t *testing.T) {
	st := NewMemoryStore(10, 10)
	archiver := &memoryIntrusionArchiver{}
	st.SetIntrusionArchiver(archiver)
	base := time.Now()

	st.AddScreenPosition(screenPositionTarget("sn-old", "device-a", "DJI Mini", base))
	st.AddScreenPosition(screenPositionTarget("sn-new", "device-b", "DJI Mini 4", base.Add(screenPositionTTL+time.Second)))
	st.ListScreenPositions(10)
	st.ListScreenPositions(10)

	if len(archiver.positions) != 1 {
		t.Fatalf("archived positions count = %d, want 1", len(archiver.positions))
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

func TestMemoryStoreScreenPositionsWithDeviceLocationCalculatesDistances(t *testing.T) {
	st := NewMemoryStore(10, 10)
	target := screenPositionTarget("sn-1", "device-a", "DJI Mini", time.Now())
	target.Pilot = &model.ScreenPositionPoint{Latitude: 31.201, Longitude: 121.401}
	st.AddScreenPosition(target)

	deviceLocation := &model.ScreenDeviceLocationResponse{
		Source: "manual",
		Point:  &model.GeoPoint{Latitude: 31.199, Longitude: 121.399},
		Valid:  true,
	}
	items := st.ListScreenPositionsWithDeviceLocation(10, deviceLocation)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].DroneDistanceM == nil || *items[0].DroneDistanceM <= 0 {
		t.Fatalf("droneDistanceM = %#v, want calculated positive distance", items[0].DroneDistanceM)
	}
	if items[0].PilotDistanceM == nil || *items[0].PilotDistanceM <= 0 {
		t.Fatalf("pilotDistanceM = %#v, want calculated positive distance", items[0].PilotDistanceM)
	}
}

func TestMemoryStoreScreenPositionsWithoutDeviceLocationOmitsDistances(t *testing.T) {
	st := NewMemoryStore(10, 10)
	target := screenPositionTarget("sn-1", "device-a", "DJI Mini", time.Now())
	target.Pilot = &model.ScreenPositionPoint{Latitude: 31.201, Longitude: 121.401}
	st.AddScreenPosition(target)

	items := st.ListScreenPositionsWithDeviceLocation(10, nil)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].DroneDistanceM != nil || items[0].PilotDistanceM != nil {
		t.Fatalf("distances = drone:%#v pilot:%#v, want nil without device location", items[0].DroneDistanceM, items[0].PilotDistanceM)
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

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type memoryIntrusionArchiver struct {
	detections []model.ScreenDetectionTarget
	positions  []model.ScreenPositionTarget
}

func (a *memoryIntrusionArchiver) ArchiveDetection(target model.ScreenDetectionTarget) error {
	a.detections = append(a.detections, target)
	return nil
}

func (a *memoryIntrusionArchiver) ArchivePosition(target model.ScreenPositionTarget) error {
	a.positions = append(a.positions, target)
	return nil
}
