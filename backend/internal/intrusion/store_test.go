package intrusion

import (
	"path/filepath"
	"testing"
	"time"

	"dr600ab-api/internal/model"
)

func TestStoreArchivesAndListsRecords(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().Add(-time.Minute).UTC()
	speed := 8.5
	height := 30.0

	if err := store.ArchiveDetection(model.ScreenDetectionTarget{
		ID:        "detect-1",
		Model:     "PAL Analog",
		Frequency: 5865,
		RSSI:      -56,
		Device:    "device-a",
		FirstSeen: base,
		LastSeen:  base.Add(2 * time.Second),
		HitCount:  2,
		LastRecord: model.ScreenDetectionLastRecord{
			ID:         "record-1",
			Kind:       "detect",
			ReceivedAt: base.Add(2 * time.Second),
			Device:     "device-a",
			Model:      "PAL Analog",
			Frequency:  5865,
			RSSI:       -56,
		},
	}); err != nil {
		t.Fatalf("ArchiveDetection() error = %v", err)
	}
	if err := store.ArchivePosition(model.ScreenPositionTarget{
		ID:        "position-1",
		Serial:    "sn-1",
		Model:     "DJI Mini",
		Source:    "rid",
		Frequency: 2437,
		RSSI:      -68,
		Device:    "device-b",
		Drone:     &model.ScreenPositionPoint{Latitude: 31.2, Longitude: 121.4},
		Pilot:     &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3},
		DroneTrajectory: []model.ScreenPositionTrackPoint{
			{Latitude: 31.2, Longitude: 121.4, Speed: &speed, Height: &height, Time: base},
		},
		Speed:     &speed,
		Height:    &height,
		FirstSeen: base.Add(time.Second),
		LastSeen:  base.Add(3 * time.Second),
		HitCount:  3,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       "rid",
			ReceivedAt: base.Add(3 * time.Second),
			Device:     "device-b",
			Serial:     "sn-1",
			Model:      "DJI Mini",
			Frequency:  2437,
			RSSI:       -68,
		},
	}); err != nil {
		t.Fatalf("ArchivePosition() error = %v", err)
	}

	items, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items count = %d, want 2", len(items))
	}
	if items[0].TargetType != model.IntrusionTargetTypePosition {
		t.Fatalf("first type = %q, want position", items[0].TargetType)
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 31.2 {
		t.Fatalf("position drone = %#v, want archived point", items[0].Drone)
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want 1", len(items[0].DroneTrajectory))
	}
	if items[0].DroneTrajectory[0].Speed == nil || *items[0].DroneTrajectory[0].Speed != speed {
		t.Fatalf("trajectory speed = %#v, want %v", items[0].DroneTrajectory[0].Speed, speed)
	}

	detections, err := store.List(QueryOptions{Limit: 10, TargetType: model.IntrusionTargetTypeDetection})
	if err != nil {
		t.Fatalf("List(detection) error = %v", err)
	}
	if len(detections) != 1 {
		t.Fatalf("detection count = %d, want 1", len(detections))
	}
	if detections[0].Model != "PAL Analog" {
		t.Fatalf("detection model = %q, want PAL Analog", detections[0].Model)
	}
}

func TestStoreIgnoresDuplicateArchive(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().UTC()
	target := model.ScreenDetectionTarget{
		ID:        "detect-1",
		Model:     "DJI_OC123_10M",
		Frequency: 5730,
		RSSI:      -60,
		Device:    "device-a",
		FirstSeen: base,
		LastSeen:  base,
		HitCount:  1,
	}

	if err := store.ArchiveDetection(target); err != nil {
		t.Fatalf("ArchiveDetection() error = %v", err)
	}
	if err := store.ArchiveDetection(target); err != nil {
		t.Fatalf("ArchiveDetection() duplicate error = %v", err)
	}

	items, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items count = %d, want 1", len(items))
	}
}

func TestParseTargetType(t *testing.T) {
	if _, err := ParseTargetType("invalid"); err == nil {
		t.Fatalf("ParseTargetType(invalid) error = nil, want error")
	}
	if targetType, err := ParseTargetType("position"); err != nil || targetType != model.IntrusionTargetTypePosition {
		t.Fatalf("ParseTargetType(position) = %q, %v; want position, nil", targetType, err)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := NewStore(filepath.Join(t.TempDir(), "intrusions.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}
