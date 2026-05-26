package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/deceptionreport"
	"dr600ab-api/internal/model"
)

type memoryDeceptionReportStore struct {
	items []model.DeceptionReport
}

func (s *memoryDeceptionReportStore) List(options deceptionreport.QueryOptions) ([]model.DeceptionReportSummary, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = len(s.items)
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]model.DeceptionReportSummary, 0, len(s.items))
	for _, item := range s.items {
		if options.Status != "" && item.Status != options.Status {
			continue
		}
		filtered = append(filtered, item.DeceptionReportSummary)
	}
	if offset >= len(filtered) {
		return []model.DeceptionReportSummary{}, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]model.DeceptionReportSummary(nil), filtered[offset:end]...), nil
}

func (s *memoryDeceptionReportStore) Get(id string) (model.DeceptionReport, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return model.DeceptionReport{}, deceptionreport.ErrNotFound
}

func (s *memoryDeceptionReportStore) DeleteFailed(id string) (int64, error) {
	for index, item := range s.items {
		if item.ID != id {
			continue
		}
		if item.Status != model.DeceptionReportStatusFailed {
			return 0, deceptionreport.ErrNotFailed
		}
		s.items = append(s.items[:index], s.items[index+1:]...)
		return 1, nil
	}
	return 0, deceptionreport.ErrNotFound
}

func (s *memoryDeceptionReportStore) CloseRunning(reason string, now time.Time) (int64, error) {
	var closed int64
	for index := range s.items {
		if s.items[index].Status != model.DeceptionReportStatusRunning {
			continue
		}
		s.items[index].Status = model.DeceptionReportStatusAbnormal
		s.items[index].AbnormalReason = reason
		s.items[index].EndedAt = &now
		closed++
	}
	return closed, nil
}

func (s *memoryDeceptionReportStore) Close() error {
	return nil
}

func TestHandleDeceptionReportsFiltersByStatus(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		reports: &memoryDeceptionReportStore{
			items: []model.DeceptionReport{
				{
					DeceptionReportSummary: model.DeceptionReportSummary{
						ID:        "completed-1",
						Status:    model.DeceptionReportStatusCompleted,
						StartedAt: startedAt,
						Mode:      "fixed_point",
						PortName:  "/dev/ttyGNSS0",
					},
					Records: []model.DeceptionRecord{{Direction: "tx", Command: "0x51"}},
				},
				{
					DeceptionReportSummary: model.DeceptionReportSummary{
						ID:        "running-1",
						Status:    model.DeceptionReportStatusRunning,
						StartedAt: startedAt.Add(time.Minute),
						Mode:      "circle",
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/deception-reports?status=completed&limit=1", nil)
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

	var body model.ListResponse[model.DeceptionReportSummary]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Count != 1 {
		t.Fatalf("Count = %d, want 1", body.Count)
	}
	if body.Items[0].ID != "completed-1" || body.Items[0].Status != model.DeceptionReportStatusCompleted {
		t.Fatalf("Items = %#v, want completed report", body.Items)
	}
}

func TestHandleDeceptionReportsReturnsPageInfo(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		reports: &memoryDeceptionReportStore{
			items: []model.DeceptionReport{
				{DeceptionReportSummary: model.DeceptionReportSummary{ID: "report-1", Status: model.DeceptionReportStatusCompleted, StartedAt: startedAt}},
				{DeceptionReportSummary: model.DeceptionReportSummary{ID: "report-2", Status: model.DeceptionReportStatusCompleted, StartedAt: startedAt}},
				{DeceptionReportSummary: model.DeceptionReportSummary{ID: "report-3", Status: model.DeceptionReportStatusCompleted, StartedAt: startedAt}},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/deception-reports?limit=2", nil)
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

	var body model.ListResponse[model.DeceptionReportSummary]
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

func TestHandleDeceptionReportsRejectsInvalidStatus(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		reports:    &memoryDeceptionReportStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/deception-reports?status=deleted", nil)
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

func TestHandleDeceptionReportReturnsDetailAndNotFound(t *testing.T) {
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	server := &Server{
		translator: mustTranslator(t),
		reports: &memoryDeceptionReportStore{
			items: []model.DeceptionReport{
				{
					DeceptionReportSummary: model.DeceptionReportSummary{
						ID:        "report-1",
						Status:    model.DeceptionReportStatusCompleted,
						StartedAt: startedAt,
					},
					Request: model.ScreenDeceptionRequest{Enabled: true, TargetID: "target-1"},
					Records: []model.DeceptionRecord{
						{Direction: "tx", Command: "0x51"},
						{Direction: "rx", Command: "0x51"},
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/deception-reports/report-1", nil)
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
	var detail model.DeceptionReport
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if detail.ID != "report-1" || detail.Request.TargetID != "target-1" || len(detail.Records) != 2 {
		t.Fatalf("detail = %#v, want report detail", detail)
	}

	req, err = http.NewRequest(http.MethodGet, "/api/v1/deception-reports/missing", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err = server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() missing error = %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleDeleteFailedDeceptionReport(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		reports: &memoryDeceptionReportStore{
			items: []model.DeceptionReport{
				{
					DeceptionReportSummary: model.DeceptionReportSummary{
						ID:     "failed-1",
						Status: model.DeceptionReportStatusFailed,
					},
				},
				{
					DeceptionReportSummary: model.DeceptionReportSummary{
						ID:     "completed-1",
						Status: model.DeceptionReportStatusCompleted,
					},
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodDelete, "/api/v1/deception-reports/failed-1", nil)
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
	var body model.DeceptionReportDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Deleted != 1 {
		t.Fatalf("Deleted = %d, want 1", body.Deleted)
	}

	req, err = http.NewRequest(http.MethodDelete, "/api/v1/deception-reports/completed-1", nil)
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

	req, err = http.NewRequest(http.MethodDelete, "/api/v1/deception-reports/missing", nil)
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

func TestHandleDeceptionReportInternalError(t *testing.T) {
	server := &Server{
		translator: mustTranslator(t),
		reports:    failingDeceptionReportStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerDeceptionReportRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/deception-reports/report-1", nil)
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

type failingDeceptionReportStore struct{}

func (failingDeceptionReportStore) List(deceptionreport.QueryOptions) ([]model.DeceptionReportSummary, error) {
	return nil, errors.New("boom")
}

func (failingDeceptionReportStore) Get(string) (model.DeceptionReport, error) {
	return model.DeceptionReport{}, errors.New("boom")
}

func (failingDeceptionReportStore) DeleteFailed(string) (int64, error) {
	return 0, errors.New("boom")
}

func (failingDeceptionReportStore) CloseRunning(string, time.Time) (int64, error) {
	return 0, nil
}

func (failingDeceptionReportStore) Close() error {
	return nil
}
