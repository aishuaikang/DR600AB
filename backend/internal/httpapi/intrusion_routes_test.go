package httpapi

import (
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
	if limit <= 0 || limit > len(s.items) {
		limit = len(s.items)
	}
	items := make([]model.IntrusionRecord, 0, limit)
	for _, item := range s.items {
		if options.TargetType != "" && item.TargetType != options.TargetType {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
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
}
