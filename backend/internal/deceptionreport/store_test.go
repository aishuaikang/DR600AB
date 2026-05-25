package deceptionreport

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"dr600ab-api/internal/model"
	"gnss-spoofer/protocol"
)

func TestStoreCreatesListsAndGetsReport(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	lon := 116.994057
	lat := 28.170931
	mask := uint16(protocol.SignalGPSL1CA | protocol.SignalBDSB1I)
	report, err := store.CreateRunning(model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			StartedAt: startedAt,
			PortName:  "/dev/ttyGNSS0",
		},
		Request: model.ScreenDeceptionRequest{
			Enabled:    true,
			TargetID:   "target-1",
			Mode:       "fixed_point",
			Longitude:  &lon,
			Latitude:   &lat,
			SignalMask: &mask,
		},
		Session: model.DeceptionSessionResponse{
			Active:   true,
			PortName: "/dev/ttyGNSS0",
		},
		StartState: &model.ScreenDeceptionState{
			Active:     true,
			TargetID:   "target-1",
			Mode:       "fixed_point",
			Point:      &model.GeoPoint{Latitude: lat, Longitude: lon},
			SignalMask: mask,
			Summary:    "固定点诱骗",
		},
		StartDeviceStatus: &model.ScreenDeceptionDeviceStatus{
			SerialActive:    true,
			RawDescriptions: map[string]string{"status": "TX status\nRX status"},
		},
		RawDescriptions: map[string]string{"status": "TX status\nRX status"},
		QueryErrors:     map[string]string{"version": "timeout"},
		Records: []model.DeceptionRecord{
			{Time: startedAt.Add(time.Second), Direction: "tx", Command: "0x51", RawHex: "EB90"},
			{Time: startedAt.Add(2 * time.Second), Direction: "rx", Command: "0x51", RawHex: "EB90"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRunning() error = %v", err)
	}
	if report.ID == "" {
		t.Fatalf("report.ID is empty")
	}

	completedAt := startedAt.Add(75 * time.Second)
	report.Status = model.DeceptionReportStatusCompleted
	report.EndedAt = &completedAt
	report.EndState = &model.ScreenDeceptionState{Active: false, Mode: "fixed_point", SignalMask: protocol.SignalAllSupported}
	report.BeforeStopStatus = &model.ScreenDeceptionDeviceStatus{SerialActive: true, RawDescriptions: map[string]string{"status": "before"}}
	report.AfterStopStatus = &model.ScreenDeceptionDeviceStatus{SerialActive: true, RawDescriptions: map[string]string{"status": "after"}}
	if err := store.Update(report); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	summaries, err := store.List(QueryOptions{Limit: 20})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("List() len = %d, want 1", len(summaries))
	}
	summary := summaries[0]
	if summary.Status != model.DeceptionReportStatusCompleted {
		t.Fatalf("Status = %q, want completed", summary.Status)
	}
	if summary.TargetID != "target-1" || summary.Mode != "fixed_point" {
		t.Fatalf("summary = %+v, want normalized target/mode", summary)
	}
	if summary.Point == nil || summary.Point.Longitude != lon || summary.Point.Latitude != lat {
		t.Fatalf("Point = %+v, want start state point", summary.Point)
	}
	if summary.DurationSeconds != 75 {
		t.Fatalf("DurationSeconds = %d, want 75", summary.DurationSeconds)
	}
	if len(summary.SignalNames) != 2 {
		t.Fatalf("SignalNames = %#v, want two names", summary.SignalNames)
	}
	if summary.Summary != "" {
		t.Fatalf("Summary = %q, want empty when only start state summary is set", summary.Summary)
	}

	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.Request.TargetID != "target-1" {
		t.Fatalf("Request.TargetID = %q, want target-1", detail.Request.TargetID)
	}
	if detail.StartDeviceStatus == nil || detail.StartDeviceStatus.RawDescriptions["status"] == "" {
		t.Fatalf("StartDeviceStatus = %+v, want raw descriptions", detail.StartDeviceStatus)
	}
	if detail.RawDescriptions["status"] == "" || detail.QueryErrors["version"] == "" {
		t.Fatalf("raw/query maps = %+v %+v, want persisted maps", detail.RawDescriptions, detail.QueryErrors)
	}
	if len(detail.Records) != 2 || detail.Records[0].Direction != "tx" || detail.Records[1].Direction != "rx" {
		t.Fatalf("Records = %#v, want persisted order", detail.Records)
	}
	if detail.Summary != "" {
		t.Fatalf("detail.Summary = %q, want empty when only start state summary is set", detail.Summary)
	}
}

func TestStoreFiltersByStatusAndClosesRunningReports(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	running, err := store.CreateRunning(model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{StartedAt: startedAt},
		StartState:             &model.ScreenDeceptionState{Active: true, Mode: "fixed_point", SignalMask: protocol.SignalGPSL1CA},
	})
	if err != nil {
		t.Fatalf("CreateRunning() running error = %v", err)
	}
	_, err = store.Create(model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			Status:    model.DeceptionReportStatusFailed,
			StartedAt: startedAt.Add(time.Minute),
			LastError: "command failed",
		},
		Request: model.ScreenDeceptionRequest{
			Enabled:  true,
			TargetID: "target-failed",
			Mode:     "fixed_point",
		},
		Records: []model.DeceptionRecord{{Direction: "tx", Command: "0x51"}},
	})
	if err != nil {
		t.Fatalf("Create() failed error = %v", err)
	}

	failedItems, err := store.List(QueryOptions{Status: model.DeceptionReportStatusFailed})
	if err != nil {
		t.Fatalf("List(failed) error = %v", err)
	}
	if len(failedItems) != 1 || failedItems[0].Status != model.DeceptionReportStatusFailed {
		t.Fatalf("failed items = %#v, want one failed report", failedItems)
	}
	if failedItems[0].TargetID != "target-failed" || failedItems[0].Mode != "fixed_point" {
		t.Fatalf("failed summary = %+v, want request-derived target and mode", failedItems[0])
	}

	closedAt := startedAt.Add(2 * time.Minute)
	closed, err := store.CloseRunning("abnormal_restart", closedAt)
	if err != nil {
		t.Fatalf("CloseRunning() error = %v", err)
	}
	if closed != 1 {
		t.Fatalf("CloseRunning() = %d, want 1", closed)
	}

	detail, err := store.Get(running.ID)
	if err != nil {
		t.Fatalf("Get(running) error = %v", err)
	}
	if detail.Status != model.DeceptionReportStatusAbnormal {
		t.Fatalf("Status = %q, want abnormal", detail.Status)
	}
	if detail.AbnormalReason != "abnormal_restart" || detail.LastError != "abnormal_restart" {
		t.Fatalf("reason/error = %q/%q, want abnormal_restart", detail.AbnormalReason, detail.LastError)
	}
	if detail.EndedAt == nil || !detail.EndedAt.Equal(closedAt) {
		t.Fatalf("EndedAt = %v, want %v", detail.EndedAt, closedAt)
	}
	if detail.DurationSeconds != 120 {
		t.Fatalf("DurationSeconds = %d, want 120", detail.DurationSeconds)
	}
}

func TestStoreGetUnknownReportReturnsNotFound(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if _, err := store.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteFailedReportOnly(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "reports.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	failed, err := store.Create(model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			Status:    model.DeceptionReportStatusFailed,
			StartedAt: startedAt,
			LastError: "command failed",
		},
		Request: model.ScreenDeceptionRequest{Enabled: true, TargetID: "failed-target"},
	})
	if err != nil {
		t.Fatalf("Create(failed) error = %v", err)
	}
	completed, err := store.Create(model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			Status:    model.DeceptionReportStatusCompleted,
			StartedAt: startedAt.Add(time.Minute),
		},
		Request: model.ScreenDeceptionRequest{Enabled: true, TargetID: "completed-target"},
	})
	if err != nil {
		t.Fatalf("Create(completed) error = %v", err)
	}

	deleted, err := store.DeleteFailed(completed.ID)
	if !errors.Is(err, ErrNotFailed) {
		t.Fatalf("DeleteFailed(completed) deleted=%d error=%v, want ErrNotFailed", deleted, err)
	}
	if _, err := store.Get(completed.ID); err != nil {
		t.Fatalf("Get(completed) error = %v, want still present", err)
	}

	deleted, err = store.DeleteFailed(failed.ID)
	if err != nil {
		t.Fatalf("DeleteFailed(failed) error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteFailed(failed) = %d, want 1", deleted)
	}
	if _, err := store.Get(failed.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted failed) error = %v, want ErrNotFound", err)
	}
	if deleted, err := store.DeleteFailed("missing"); deleted != 0 || !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteFailed(missing) deleted=%d error=%v, want 0/ErrNotFound", deleted, err)
	}
}
