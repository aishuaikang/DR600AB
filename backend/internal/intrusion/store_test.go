package intrusion

import (
	"database/sql"
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
	deviceLocationUpdatedAt := base.Add(1500 * time.Millisecond)
	store.SetDeviceLocationProvider(func() *model.ScreenDeviceLocationResponse {
		return &model.ScreenDeviceLocationResponse{
			Source:    "gps",
			Point:     &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
			UpdatedAt: &deviceLocationUpdatedAt,
			Valid:     true,
		}
	})

	if err := store.ArchiveDetection(model.ScreenDetectionTarget{
		ID:        "detect-1",
		Serial:    "DET-ABC123",
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
		Sources:   []string{"rid", "did_encrypted"},
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
	if items[0].DeviceLocation == nil || items[0].DeviceLocation.Point == nil {
		t.Fatalf("position device location = %#v, want archived device location", items[0].DeviceLocation)
	}
	if items[0].DeviceLocation.Source != "gps" ||
		items[0].DeviceLocation.Point.Latitude != 23.12911 ||
		items[0].DeviceLocation.Point.Longitude != 113.264385 ||
		items[0].DeviceLocation.UpdatedAt == nil ||
		!items[0].DeviceLocation.UpdatedAt.Equal(deviceLocationUpdatedAt) {
		t.Fatalf("position device location = %#v, want gps location", items[0].DeviceLocation)
	}
	if len(items[0].DroneTrajectory) != 1 {
		t.Fatalf("drone trajectory count = %d, want 1", len(items[0].DroneTrajectory))
	}
	if !stringSlicesEqual(items[0].Sources, []string{"rid", "did_encrypted"}) {
		t.Fatalf("position sources = %#v, want rid and did_encrypted", items[0].Sources)
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
	if detections[0].Serial != "DET-ABC123" {
		t.Fatalf("detection serial = %q, want generated serial", detections[0].Serial)
	}
	if detections[0].Device != "device-a" {
		t.Fatalf("detection device = %q, want device-a", detections[0].Device)
	}
	if detections[0].DeviceLocation == nil || detections[0].DeviceLocation.Point == nil {
		t.Fatalf("detection device location = %#v, want archived device location", detections[0].DeviceLocation)
	}
}

func TestStoreMigratesDeviceLocationColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intrusions.db")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.db.Exec(`ALTER TABLE intrusion_records DROP COLUMN device_location_json`); err != nil {
		t.Fatalf("drop device_location_json error = %v", err)
	}
	if _, err := store.db.Exec(`ALTER TABLE intrusion_records DROP COLUMN sources_json`); err != nil {
		t.Fatalf("drop sources_json error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() after old schema error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	rows, err := store.db.Query(`PRAGMA table_info(intrusion_records)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info error = %v", err)
	}
	defer rows.Close()

	foundDeviceLocation := false
	foundSources := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table info error = %v", err)
		}
		if name == "device_location_json" {
			foundDeviceLocation = true
		}
		if name == "sources_json" {
			foundSources = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error = %v", err)
	}
	if !foundDeviceLocation {
		t.Fatalf("device_location_json column not migrated")
	}
	if !foundSources {
		t.Fatalf("sources_json column not migrated")
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
	if items[0].Serial == "" {
		t.Fatalf("serial is empty")
	}
	if items[0].Serial == items[0].Device {
		t.Fatalf("serial = device = %q, want generated detection serial", items[0].Serial)
	}
}

func TestStoreDeletesRecordsByID(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().UTC()
	for _, id := range []string{"detect-1", "detect-2", "detect-3"} {
		if err := store.ArchiveDetection(model.ScreenDetectionTarget{
			ID:        id,
			Serial:    "DET-" + id,
			Model:     "PAL Analog",
			Frequency: 5865,
			RSSI:      -56,
			Device:    "device-a",
			FirstSeen: base,
			LastSeen:  base,
			HitCount:  1,
		}); err != nil {
			t.Fatalf("ArchiveDetection(%s) error = %v", id, err)
		}
	}

	deleted, err := store.Delete([]string{
		intrusionRecordID(model.IntrusionTargetTypeDetection, "detect-1", base),
		intrusionRecordID(model.IntrusionTargetTypeDetection, "detect-1", base),
		" ",
		intrusionRecordID(model.IntrusionTargetTypeDetection, "detect-3", base),
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}

	items, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items count = %d, want 1", len(items))
	}
	if items[0].TargetID != "detect-2" {
		t.Fatalf("remaining target = %q, want detect-2", items[0].TargetID)
	}
}

func TestStoreDeleteRejectsEmptyIDs(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.Delete([]string{"", " "}); err == nil {
		t.Fatalf("Delete(empty) error = nil, want error")
	}
}

func TestStorePruneRetention(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	oldFirstSeen := now.AddDate(0, 0, -100)
	recentFirstSeen := now.AddDate(0, 0, -10)
	if err := store.ArchiveDetection(model.ScreenDetectionTarget{
		ID:        "old",
		Serial:    "DET-old",
		Model:     "PAL Analog",
		Frequency: 5865,
		RSSI:      -56,
		Device:    "device-a",
		FirstSeen: oldFirstSeen,
		LastSeen:  oldFirstSeen,
		HitCount:  1,
	}); err != nil {
		t.Fatalf("ArchiveDetection(old) error = %v", err)
	}
	if err := store.ArchiveDetection(model.ScreenDetectionTarget{
		ID:        "recent",
		Serial:    "DET-recent",
		Model:     "PAL Analog",
		Frequency: 5865,
		RSSI:      -56,
		Device:    "device-a",
		FirstSeen: recentFirstSeen,
		LastSeen:  recentFirstSeen,
		HitCount:  1,
	}); err != nil {
		t.Fatalf("ArchiveDetection(recent) error = %v", err)
	}
	if _, err := store.db.Exec(
		`UPDATE intrusion_records SET archived_at = ? WHERE target_id = ?`,
		formatTime(now.AddDate(0, 0, -100)),
		"old",
	); err != nil {
		t.Fatalf("update old archived_at error = %v", err)
	}
	if _, err := store.db.Exec(
		`UPDATE intrusion_records SET archived_at = ? WHERE target_id = ?`,
		formatTime(now.AddDate(0, 0, -10)),
		"recent",
	); err != nil {
		t.Fatalf("update recent archived_at error = %v", err)
	}

	deleted, err := store.PruneRetention(90, now)
	if err != nil {
		t.Fatalf("PruneRetention() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	items, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].TargetID != "recent" {
		t.Fatalf("items = %#v, want only recent", items)
	}
}

func TestStorePruneRetentionZeroKeepsRecords(t *testing.T) {
	store := newTestStore(t)
	base := time.Now().AddDate(0, 0, -100).UTC()
	if err := store.ArchiveDetection(model.ScreenDetectionTarget{
		ID:        "old",
		Serial:    "DET-old",
		Model:     "PAL Analog",
		Frequency: 5865,
		RSSI:      -56,
		Device:    "device-a",
		FirstSeen: base,
		LastSeen:  base,
		HitCount:  1,
	}); err != nil {
		t.Fatalf("ArchiveDetection() error = %v", err)
	}
	if _, err := store.db.Exec(
		`UPDATE intrusion_records SET archived_at = ? WHERE target_id = ?`,
		formatTime(base),
		"old",
	); err != nil {
		t.Fatalf("update archived_at error = %v", err)
	}

	deleted, err := store.PruneRetention(0, time.Now().UTC())
	if err != nil {
		t.Fatalf("PruneRetention(0) error = %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
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
