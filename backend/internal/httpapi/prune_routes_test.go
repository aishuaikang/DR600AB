package httpapi

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/model"
)

var errPruneRetention = errors.New("prune retention failed")

type failingIntrusionPruneStore struct {
	memoryIntrusionStore
}

func (s *failingIntrusionPruneStore) PruneRetention(days int, now time.Time) (int64, error) {
	return 0, errPruneRetention
}

type failingFPVVideoRecordPruneStore struct {
	memoryFPVVideoRecordStore
}

func (s *failingFPVVideoRecordPruneStore) PruneRetention(days int, now time.Time) (int64, error) {
	return 0, errPruneRetention
}

func TestHandleFPVVideoRecordsDoesNotPruneIntrusions(t *testing.T) {
	now := time.Now()
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &failingIntrusionPruneStore{},
		fpvRecords: &memoryFPVVideoRecordStore{
			items: []model.FPVVideoRecord{
				{
					ID:        "fpv-record",
					Status:    model.FPVVideoRecordStatusCompleted,
					StartedAt: now,
					EndedAt:   now.Add(time.Second),
				},
			},
		},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerFPVVideoRecordRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/fpv-video-records", nil)
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
}

func TestHandleIntrusionRecordsDoesNotPruneFPVVideoRecords(t *testing.T) {
	now := time.Now()
	server := &Server{
		translator: mustTranslator(t),
		intrusions: &memoryIntrusionStore{
			items: []model.IntrusionRecord{
				{
					ID:         "intrusion-record",
					TargetID:   "target",
					TargetType: model.IntrusionTargetTypeDetection,
					FirstSeen:  now,
					LastSeen:   now,
					ArchivedAt: now,
				},
			},
		},
		fpvRecords: &failingFPVVideoRecordPruneStore{},
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
}
