package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/interferencereport"
	"dr600ab-api/internal/model"
)

type memoryInterferenceReportStore struct {
	items []model.InterferenceReport
}

func (s *memoryInterferenceReportStore) List(options interferencereport.QueryOptions) ([]model.InterferenceReportSummary, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = len(s.items)
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]model.InterferenceReportSummary, 0, len(s.items))
	for _, item := range s.items {
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		filtered = append(filtered, item.InterferenceReportSummary)
	}
	if offset >= len(filtered) {
		return []model.InterferenceReportSummary{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]model.InterferenceReportSummary(nil), filtered[offset:end]...), nil
}

func (s *memoryInterferenceReportStore) Get(id string) (model.InterferenceReport, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.InterferenceReport{}, interferencereport.ErrNotFound
}

func (s *memoryInterferenceReportStore) DeleteFailed(id string) (int64, error) {
	for index, item := range s.items {
		if item.ID != id {
			continue
		}
		if item.Status != model.InterferenceReportStatusFailed {
			return 0, interferencereport.ErrNotFailed
		}
		s.items = append(s.items[:index], s.items[index+1:]...)
		return 1, nil
	}
	return 0, interferencereport.ErrNotFound
}

func (s *memoryInterferenceReportStore) CloseRunning(reason string, now time.Time) (int64, error) {
	var closed int64
	for index := range s.items {
		if s.items[index].Status != model.InterferenceReportStatusRunning {
			continue
		}
		s.items[index].Status = model.InterferenceReportStatusAbnormal
		s.items[index].AbnormalReason = reason
		s.items[index].EndedAt = &now
		closed++
	}
	return closed, nil
}

func (s *memoryInterferenceReportStore) Close() error {
	return nil
}

func TestHandleInterferenceReportsFiltersByStatus(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		interferenceReports: &memoryInterferenceReportStore{
			items: []model.InterferenceReport{
				{
					InterferenceReportSummary: model.InterferenceReportSummary{
						ID:              "completed-1",
						Status:          model.InterferenceReportStatusCompleted,
						StartedAt:       startedAt,
						ChannelIDs:      []string{"io1"},
						ChannelLabels:   []string{"IOC4"},
						DurationSeconds: 30,
					},
				},
				{
					InterferenceReportSummary: model.InterferenceReportSummary{
						ID:        "running-1",
						Status:    model.InterferenceReportStatusRunning,
						StartedAt: startedAt.Add(time.Minute),
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/interference-reports?status=completed&limit=1", nil)
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

	var body model.ListResponse[model.InterferenceReportSummary]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("Count = %d, want 1", body.Count)
	}
	if body.Items[0].ID != "completed-1" || body.Items[0].Status != model.InterferenceReportStatusCompleted {
		t.Fatalf("Items = %#v, want completed report", body.Items)
	}
}

func TestHandleInterferenceReportsReturnsPageInfo(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		interferenceReports: &memoryInterferenceReportStore{
			items: []model.InterferenceReport{
				{InterferenceReportSummary: model.InterferenceReportSummary{ID: "report-1", Status: model.InterferenceReportStatusCompleted, StartedAt: startedAt}},
				{InterferenceReportSummary: model.InterferenceReportSummary{ID: "report-2", Status: model.InterferenceReportStatusCompleted, StartedAt: startedAt}},
				{InterferenceReportSummary: model.InterferenceReportSummary{ID: "report-3", Status: model.InterferenceReportStatusCompleted, StartedAt: startedAt}},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/interference-reports?limit=2", nil)
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

	var body model.ListResponse[model.InterferenceReportSummary]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 2 || !body.HasMore || body.NextOffset != 2 {
		t.Fatalf("page = count %d hasMore %v nextOffset %d, want 2 true 2", body.Count, body.HasMore, body.NextOffset)
	}
	if body.Items[0].ID != "report-1" || body.Items[1].ID != "report-2" {
		t.Fatalf("Items = %#v, want first two reports", body.Items)
	}
}

func TestHandleInterferenceReportsRejectsInvalidStatus(t *testing.T) {
	server := &Server{
		translator:          mustTranslator(t),
		interferenceReports: &memoryInterferenceReportStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/interference-reports?status=deleted", nil)
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

func TestHandleInterferenceReportReturnsDetailAndNotFound(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		interferenceReports: &memoryInterferenceReportStore{
			items: []model.InterferenceReport{
				{
					InterferenceReportSummary: model.InterferenceReportSummary{
						ID:        "report-1",
						Status:    model.InterferenceReportStatusCompleted,
						StartedAt: startedAt,
					},
					Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1"}, DurationSeconds: 30},
					StartState: &model.ScreenStrikeState{
						Active:     true,
						ChannelIDs: []string{"io1"},
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/interference-reports/report-1", nil)
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
	var detail model.InterferenceReport
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if detail.ID != "report-1" || len(detail.Request.ChannelIDs) != 1 || detail.StartState == nil {
		t.Fatalf("detail = %#v, want report detail", detail)
	}

	req, err = http.NewRequest(http.MethodGet, "/api/v1/interference-reports/missing", nil)
	if err != nil {
		t.Fatalf("NewRequest() missing error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() missing error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleDeleteFailedInterferenceReport(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		interferenceReports: &memoryInterferenceReportStore{
			items: []model.InterferenceReport{
				{
					InterferenceReportSummary: model.InterferenceReportSummary{
						ID:     "failed-1",
						Status: model.InterferenceReportStatusFailed,
					},
				},
				{
					InterferenceReportSummary: model.InterferenceReportSummary{
						ID:     "completed-1",
						Status: model.InterferenceReportStatusCompleted,
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodDelete, "/api/v1/interference-reports/failed-1", nil)
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
	var body model.InterferenceReportDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Deleted != 1 {
		t.Fatalf("Deleted = %d, want 1", body.Deleted)
	}

	req, err = http.NewRequest(http.MethodDelete, "/api/v1/interference-reports/completed-1", nil)
	if err != nil {
		t.Fatalf("NewRequest() completed error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() completed error = %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("completed status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	req, err = http.NewRequest(http.MethodDelete, "/api/v1/interference-reports/missing", nil)
	if err != nil {
		t.Fatalf("NewRequest() missing error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() missing error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleInterferenceReportInternalError(t *testing.T) {
	server := &Server{
		translator:          mustTranslator(t),
		interferenceReports: failingInterferenceReportStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerInterferenceReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/interference-reports/report-1", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

type failingInterferenceReportStore struct{}

func (failingInterferenceReportStore) List(interferencereport.QueryOptions) ([]model.InterferenceReportSummary, error) {
	return nil, errors.New("boom")
}

func (failingInterferenceReportStore) Get(string) (model.InterferenceReport, error) {
	return model.InterferenceReport{}, errors.New("boom")
}

func (failingInterferenceReportStore) DeleteFailed(string) (int64, error) {
	return 0, errors.New("boom")
}

func (failingInterferenceReportStore) CloseRunning(string, time.Time) (int64, error) {
	return 0, nil
}

func (failingInterferenceReportStore) Close() error {
	return nil
}
