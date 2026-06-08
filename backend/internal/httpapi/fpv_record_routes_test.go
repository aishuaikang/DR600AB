package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/fpvrecord"
	"dr600ab-api/internal/model"
)

type memoryFPVVideoRecordStore struct {
	items []model.FPVVideoRecord
}

func (s *memoryFPVVideoRecordStore) Insert(record model.FPVVideoRecord) error {
	s.items = append(s.items, record)
	return nil
}

func (s *memoryFPVVideoRecordStore) List(options fpvrecord.QueryOptions) ([]model.FPVVideoRecord, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = len(s.items)
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]model.FPVVideoRecord, 0, len(s.items))
	for _, item := range s.items {
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		filtered = append(filtered, item)
	}
	if offset >= len(filtered) {
		return []model.FPVVideoRecord{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]model.FPVVideoRecord(nil), filtered[offset:end]...), nil
}

func (s *memoryFPVVideoRecordStore) Delete(ids []string) (int64, error) {
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

func (s *memoryFPVVideoRecordStore) Close() error {
	return nil
}

func TestHandleFPVVideoRecordsFiltersByStatus(t *testing.T) {
	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		fpvRecords: &memoryFPVVideoRecordStore{
			items: []model.FPVVideoRecord{
				{
					ID:              "completed-1",
					Status:          model.FPVVideoRecordStatusCompleted,
					StartedAt:       startedAt,
					EndedAt:         startedAt.Add(time.Second),
					DurationSeconds: 1,
				},
				{
					ID:        "failed-1",
					Status:    model.FPVVideoRecordStatusFailed,
					StartedAt: startedAt.Add(time.Minute),
					EndedAt:   startedAt.Add(time.Minute + time.Second),
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/fpv-video-records?status=completed&limit=1", nil)
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

	var body model.ListResponse[model.FPVVideoRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 || body.Items[0].ID != "completed-1" {
		t.Fatalf("body = %#v, want completed record", body)
	}
}

func TestHandleFPVVideoRecordsReturnsPageInfo(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		fpvRecords: &memoryFPVVideoRecordStore{
			items: []model.FPVVideoRecord{
				{ID: "record-1", Status: model.FPVVideoRecordStatusCompleted, StartedAt: now, EndedAt: now.Add(time.Second)},
				{ID: "record-2", Status: model.FPVVideoRecordStatusCompleted, StartedAt: now, EndedAt: now.Add(time.Second)},
				{ID: "record-3", Status: model.FPVVideoRecordStatusCompleted, StartedAt: now, EndedAt: now.Add(time.Second)},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/fpv-video-records?limit=2", nil)
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

	var body model.ListResponse[model.FPVVideoRecord]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 2 || !body.HasMore || body.NextOffset != 2 {
		t.Fatalf("page = count %d hasMore %v nextOffset %d, want 2 true 2", body.Count, body.HasMore, body.NextOffset)
	}
}

func TestHandleFPVVideoRecordsRejectsInvalidStatus(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		fpvRecords: &memoryFPVVideoRecordStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/fpv-video-records?status=running", nil)
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

func TestHandleDeleteFPVVideoRecords(t *testing.T) {
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	store := &memoryFPVVideoRecordStore{
		items: []model.FPVVideoRecord{
			{ID: "record-1", StartedAt: now, EndedAt: now.Add(time.Second)},
			{ID: "record-2", StartedAt: now, EndedAt: now.Add(time.Second)},
		},
	}
	server := &Server{
		translator: mustTranslator(t),
		fpvRecords: store,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	payload := []byte(`{"ids":["record-1","record-1"]}`)
	req, err := http.NewRequest(http.MethodDelete, "/api/v1/fpv-video-records", bytes.NewReader(payload))
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

	var body model.FPVVideoRecordDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Deleted != 1 {
		t.Fatalf("Deleted = %d, want 1", body.Deleted)
	}
	if len(store.items) != 1 || store.items[0].ID != "record-2" {
		t.Fatalf("items = %#v, want only record-2", store.items)
	}
}

func TestHandleDeleteFPVVideoRecordsRejectsEmptyIDs(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		fpvRecords: &memoryFPVVideoRecordStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	req, err := http.NewRequest(http.MethodDelete, "/api/v1/fpv-video-records", bytes.NewReader([]byte(`{"ids":[]}`)))
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
