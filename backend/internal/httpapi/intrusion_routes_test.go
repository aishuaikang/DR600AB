package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/model"
	memstore "dr600ab-api/internal/store"
)

type memoryIntrusionStore struct {
	items []model.IntrusionRecord
}

func (s *memoryIntrusionStore) List(options intrusion.QueryOptions) ([]model.IntrusionRecord, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = len(s.items)
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]model.IntrusionRecord, 0, len(s.items))
	for _, item := range s.items {
		if options.TargetType != "" && item.TargetType != options.TargetType {
			continue
		}
		filtered = append(filtered, item)
	}
	if offset >= len(filtered) {
		return []model.IntrusionRecord{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]model.IntrusionRecord(nil), filtered[offset:end]...), nil
}

func (s *memoryIntrusionStore) Delete(ids []string) (int64, error) {
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	next := s.items[:0]
	var deleted int64
	for _, item := range s.items {
		if _, ok := idSet[item.ID]; ok {
			deleted++
			continue
		}
		next = append(next, item)
	}
	s.items = next
	return deleted, nil
}

func (s *memoryIntrusionStore) PruneRetention(days int, now time.Time) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days)
	next := s.items[:0]
	var deleted int64
	for _, item := range s.items {
		if item.ArchivedAt.Before(cutoff) {
			deleted++
			continue
		}
		next = append(next, item)
	}
	s.items = next
	return deleted, nil
}

func (s *memoryIntrusionStore) Close() error {
	return nil
}

func TestHandleIntrusionRecordsFiltersByType(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &memoryIntrusionStore{
			items: []model.IntrusionRecord{
				{
					ID:         "position-1",
					TargetID:   "target-position",
					TargetType: model.IntrusionTargetTypePosition,
					Model:      "DJI Mini",
					FirstSeen:  time.Now(),
					LastSeen:   time.Now(),
					ArchivedAt: time.Now(),
				},
				{
					ID:         "detection-1",
					TargetID:   "target-detection",
					TargetType: model.IntrusionTargetTypeDetection,
					Model:      "PAL Analog",
					FirstSeen:  time.Now(),
					LastSeen:   time.Now(),
					ArchivedAt: time.Now(),
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions?type=position", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.ListResponse[model.IntrusionRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("count = %d, want 1", body.Count)
	}
	if body.Items[0].TargetType != model.IntrusionTargetTypePosition {
		t.Fatalf("target type = %q, want position", body.Items[0].TargetType)
	}
}

func TestHandleIntrusionRecordsReturnsPageInfo(t *testing.T) {
	now := time.Now()
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &memoryIntrusionStore{
			items: []model.IntrusionRecord{
				{ID: "record-1", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: now, LastSeen: now, ArchivedAt: now},
				{ID: "record-2", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: now, LastSeen: now, ArchivedAt: now},
				{ID: "record-3", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: now, LastSeen: now, ArchivedAt: now},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions?limit=2", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.ListResponse[model.IntrusionRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 2 || !body.HasMore || body.NextOffset != 2 {
		t.Fatalf("page = count %d hasMore %v nextOffset %d, want 2 true 2", body.Count, body.HasMore, body.NextOffset)
	}
	if body.Items[0].ID != "record-1" || body.Items[1].ID != "record-2" {
		t.Fatalf("items = %#v, want first two records", body.Items)
	}
}

func TestHandleIntrusionRecordsRejectsInvalidType(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &memoryIntrusionStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions?type=invalid", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleIntrusionRecordsPrunesByCurrentUserSettings(t *testing.T) {
	now := time.Now()
	oldArchivedAt := now.AddDate(0, 0, -100)
	recentArchivedAt := now.AddDate(0, 0, -10)
	retentionDays := 90
	store := &memoryIntrusionStore{
		items: []model.IntrusionRecord{
			{
				ID:         "old",
				TargetID:   "old",
				TargetType: model.IntrusionTargetTypeDetection,
				FirstSeen:  oldArchivedAt,
				LastSeen:   oldArchivedAt,
				ArchivedAt: oldArchivedAt,
			},
			{
				ID:         "recent",
				TargetID:   "recent",
				TargetType: model.IntrusionTargetTypeDetection,
				FirstSeen:  recentArchivedAt,
				LastSeen:   recentArchivedAt,
				ArchivedAt: recentArchivedAt,
			},
		},
	}
	server := &Server{
		translator: mustTranslator(t),
		userSettings: &memoryUserSettingsStore{
			settings: model.UserSettings{IntrusionRetentionDays: &retentionDays},
			ok:       true,
		},
		intrusions: store,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.ListResponse[model.IntrusionRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("count = %d, want 1", body.Count)
	}
	if body.Items[0].ID != "recent" {
		t.Fatalf("record id = %q, want recent", body.Items[0].ID)
	}
	if len(store.items) != 1 || store.items[0].ID != "recent" {
		t.Fatalf("store items = %#v, want only recent", store.items)
	}
}

func TestHandleDeleteIntrusionRecords(t *testing.T) {
	store := &memoryIntrusionStore{
		items: []model.IntrusionRecord{
			{
				ID:         "record-1",
				TargetID:   "target-1",
				TargetType: model.IntrusionTargetTypeDetection,
				FirstSeen:  time.Now(),
				LastSeen:   time.Now(),
				ArchivedAt: time.Now(),
			},
			{
				ID:         "record-2",
				TargetID:   "target-2",
				TargetType: model.IntrusionTargetTypePosition,
				FirstSeen:  time.Now(),
				LastSeen:   time.Now(),
				ArchivedAt: time.Now(),
			},
		},
	}
	server := &Server{
		translator: mustTranslator(t),
		intrusions: store,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	body, err := json.Marshal(model.IntrusionDeleteRequest{
		IDs: []string{"record-1", "record-1", " "},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodDelete, "/api/v1/intrusions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var response model.IntrusionDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if response.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", response.Deleted)
	}
	if len(store.items) != 1 || store.items[0].ID != "record-2" {
		t.Fatalf("remaining items = %#v, want record-2", store.items)
	}
}

func TestHandleDeleteIntrusionRecordsRejectsEmptyIDs(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &memoryIntrusionStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	body, err := json.Marshal(model.IntrusionDeleteRequest{IDs: []string{"", " "}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodDelete, "/api/v1/intrusions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleIntrusionRecordsArchivesExpiredTargetsBeforeQuery(t *testing.T) {
	translator := mustTranslator(t)
	state := memstore.NewMemoryStore(10, 10)
	intrusionStore, err := intrusion.NewStore(filepath.Join(t.TempDir(), "intrusions.db"))
	if err != nil {
		t.Fatalf("intrusion.NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := intrusionStore.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	state.SetIntrusionArchiver(intrusionStore)
	state.AddDetection(model.DetectionRecord{
		ID:         "record-1",
		SessionID:  "session-1",
		PortName:   "COM1",
		Kind:       "detect",
		ReceivedAt: time.Now().Add(-2 * time.Minute),
		Device:     "device-a",
		Model:      "PAL Analog",
		Frequency:  5865,
		RSSI:       -56,
		Summary:    "PAL Analog",
	})

	server := &Server{
		translator: translator,
		detection:  detection.NewService(state, translator, nil, detection.Options{}),
		intrusions: intrusionStore,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions?type=detection", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.ListResponse[model.IntrusionRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("count = %d, want 1", body.Count)
	}
	if body.Items[0].TargetType != model.IntrusionTargetTypeDetection {
		t.Fatalf("target type = %q, want detection", body.Items[0].TargetType)
	}
	if body.Items[0].Model != "PAL Analog" {
		t.Fatalf("model = %q, want PAL Analog", body.Items[0].Model)
	}
	if body.Items[0].Serial == "" {
		t.Fatalf("serial is empty")
	}
	if body.Items[0].Serial == body.Items[0].Device {
		t.Fatalf("serial = device = %q, want generated detection serial", body.Items[0].Serial)
	}
	if body.Items[0].Device != "device-a" {
		t.Fatalf("device = %q, want device-a", body.Items[0].Device)
	}
}

func TestHandleIntrusionRecordsArchivesExpiredPositionTrajectoryBeforeQuery(t *testing.T) {
	translator := mustTranslator(t)
	state := memstore.NewMemoryStore(10, 10)
	intrusionStore, err := intrusion.NewStore(filepath.Join(t.TempDir(), "intrusions.db"))
	if err != nil {
		t.Fatalf("intrusion.NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := intrusionStore.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	state.SetIntrusionArchiver(intrusionStore)

	base := time.Now().Add(-2 * time.Minute)
	firstSpeed := 8.5
	firstHeight := 30.0
	secondSpeed := 10.5
	secondHeight := 45.0
	state.AddScreenPosition(model.ScreenPositionTarget{
		Serial:           "sn-trajectory",
		Model:            "DJI Mini",
		Source:           "rid",
		Frequency:        2437,
		RSSI:             -68,
		Device:           "device-a",
		Drone:            &model.ScreenPositionPoint{Latitude: 31.2, Longitude: 121.4},
		Pilot:            &model.ScreenPositionPoint{Latitude: 31.1, Longitude: 121.3},
		Speed:            &firstSpeed,
		Height:           &firstHeight,
		FirstSeen:        base,
		LastSeen:         base,
		TrajectorySpeed:  &firstSpeed,
		TrajectoryHeight: &firstHeight,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       "rid",
			ReceivedAt: base,
			Device:     "device-a",
			Serial:     "sn-trajectory",
			Model:      "DJI Mini",
			Frequency:  2437,
			RSSI:       -68,
		},
	})
	state.AddScreenPosition(model.ScreenPositionTarget{
		Serial:           "sn-trajectory",
		Model:            "DJI Mini",
		Source:           "rid",
		Frequency:        2437,
		RSSI:             -66,
		Device:           "device-a",
		Drone:            &model.ScreenPositionPoint{Latitude: 31.20005, Longitude: 121.40005},
		Pilot:            &model.ScreenPositionPoint{Latitude: 31.10005, Longitude: 121.30005},
		Speed:            &secondSpeed,
		Height:           &secondHeight,
		FirstSeen:        base,
		LastSeen:         base.Add(time.Second),
		TrajectorySpeed:  &secondSpeed,
		TrajectoryHeight: &secondHeight,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       "rid",
			ReceivedAt: base.Add(time.Second),
			Device:     "device-a",
			Serial:     "sn-trajectory",
			Model:      "DJI Mini",
			Frequency:  2437,
			RSSI:       -66,
		},
	})

	server := &Server{
		translator: translator,
		detection:  detection.NewService(state, translator, nil, detection.Options{}),
		intrusions: intrusionStore,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerIntrusionRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/intrusions?type=position", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.ListResponse[model.IntrusionRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("count = %d, want 1", body.Count)
	}
	record := body.Items[0]
	if record.TargetType != model.IntrusionTargetTypePosition {
		t.Fatalf("target type = %q, want position", record.TargetType)
	}
	if len(record.DroneTrajectory) != 2 {
		t.Fatalf("drone trajectory count = %d, want 2", len(record.DroneTrajectory))
	}
	if len(record.PilotTrajectory) != 2 {
		t.Fatalf("pilot trajectory count = %d, want 2", len(record.PilotTrajectory))
	}
	dronePoint := record.DroneTrajectory[1]
	if dronePoint.Latitude != 31.20005 || dronePoint.Longitude != 121.40005 {
		t.Fatalf("drone trajectory point = %#v, want latest drone point", dronePoint)
	}
	if dronePoint.Speed == nil || *dronePoint.Speed != secondSpeed {
		t.Fatalf("drone trajectory speed = %#v, want %v", dronePoint.Speed, secondSpeed)
	}
	if dronePoint.Height == nil || *dronePoint.Height != secondHeight {
		t.Fatalf("drone trajectory height = %#v, want %v", dronePoint.Height, secondHeight)
	}
	pilotPoint := record.PilotTrajectory[1]
	if pilotPoint.Latitude != 31.10005 || pilotPoint.Longitude != 121.30005 {
		t.Fatalf("pilot trajectory point = %#v, want latest pilot point", pilotPoint)
	}
}
