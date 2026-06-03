package interferencereport

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"dr600ab-api/internal/model"
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
	report, err := store.CreateRunning(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			StartedAt: startedAt,
		},
		Request: model.ScreenStrikeRequest{
			Enabled:         true,
			ChannelIDs:      []string{"io1", "io3"},
			DurationSeconds: 60,
		},
		StartState: &model.ScreenStrikeState{
			Active:          true,
			ChannelIDs:      []string{"io1", "io3"},
			DurationSeconds: 60,
			Channels: []model.GpioChannel{
				{ID: "io1", Label: "IOC4", Pin: 20, Bands: []string{"433"}},
				{ID: "io3", Label: "IOC3", Pin: 19, Bands: []string{"2.4"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRunning() error = %v", err)
	}
	if report.ID == "" {
		t.Fatal("report.ID is empty")
	}

	completedAt := startedAt.Add(60 * time.Second)
	report.Status = model.InterferenceReportStatusCompleted
	report.EndedAt = &completedAt
	report.EndState = &model.ScreenStrikeState{Active: false}
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
	if summary.Status != model.InterferenceReportStatusCompleted {
		t.Fatalf("Status = %q, want completed", summary.Status)
	}
	if summary.DurationSeconds != 60 || summary.RequestedDurationSeconds != 60 {
		t.Fatalf("durations = %d/%d, want 60/60", summary.DurationSeconds, summary.RequestedDurationSeconds)
	}
	if len(summary.ChannelLabels) != 2 || summary.ChannelLabels[0] != "433M" || summary.ChannelLabels[1] != "2.4G" {
		t.Fatalf("ChannelLabels = %#v, want 433M/2.4G", summary.ChannelLabels)
	}

	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(detail.Request.ChannelIDs) != 2 || detail.StartState == nil || detail.EndState == nil {
		t.Fatalf("detail = %#v, want persisted request and states", detail)
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
	running, err := store.CreateRunning(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{StartedAt: startedAt},
		Request:                   model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("CreateRunning() error = %v", err)
	}
	_, err = store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:    model.InterferenceReportStatusFailed,
			StartedAt: startedAt.Add(time.Minute),
			LastError: "gpio failed",
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io2"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("Create(failed) error = %v", err)
	}

	failedItems, err := store.List(QueryOptions{Status: model.InterferenceReportStatusFailed})
	if err != nil {
		t.Fatalf("List(failed) error = %v", err)
	}
	if len(failedItems) != 1 || failedItems[0].Status != model.InterferenceReportStatusFailed {
		t.Fatalf("failed items = %#v, want one failed report", failedItems)
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
	if detail.Status != model.InterferenceReportStatusAbnormal {
		t.Fatalf("Status = %q, want abnormal", detail.Status)
	}
	if detail.AbnormalReason != "abnormal_restart" || detail.LastError != "abnormal_restart" {
		t.Fatalf("reason/error = %q/%q, want abnormal_restart", detail.AbnormalReason, detail.LastError)
	}
	if detail.DurationSeconds != 120 {
		t.Fatalf("DurationSeconds = %d, want 120", detail.DurationSeconds)
	}
}

func TestStoreNormalizesLegacyGPIOChannelLabels(t *testing.T) {
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
	report, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:        model.InterferenceReportStatusCompleted,
			StartedAt:     startedAt,
			ChannelIDs:    []string{"io1", "io2", "io3"},
			ChannelLabels: []string{"IOC4", "IOC2", "IOC3"},
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1", "io2", "io3"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	summaries, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []string{"433M/800M/900M/1.4G", "1.2G/1.5G", "2.4G/5.2G/5.8G"}
	if len(summaries) != 1 || !equalStrings(summaries[0].ChannelLabels, want) {
		t.Fatalf("summary.ChannelLabels = %#v, want %#v", summaries, want)
	}
	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !equalStrings(detail.ChannelLabels, want) {
		t.Fatalf("detail.ChannelLabels = %#v, want %#v", detail.ChannelLabels, want)
	}
}

func TestStoreNormalizesLegacySysfsGPIOChannelLabels(t *testing.T) {
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
	report, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:        model.InterferenceReportStatusCompleted,
			StartedAt:     startedAt,
			ChannelIDs:    []string{"io1", "io2", "io3"},
			ChannelLabels: []string{"GPIO20", "GPIO18", "GPIO19"},
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1", "io2", "io3"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	want := []string{"433M/800M/900M/1.4G", "1.2G/1.5G", "2.4G/5.2G/5.8G"}
	if !equalStrings(detail.ChannelLabels, want) {
		t.Fatalf("detail.ChannelLabels = %#v, want %#v", detail.ChannelLabels, want)
	}
}

func TestStoreNormalizesExternalIOChannelLabels(t *testing.T) {
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
	report, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:        model.InterferenceReportStatusCompleted,
			StartedAt:     startedAt,
			ChannelIDs:    []string{"io1", "io2", "io3"},
			ChannelLabels: []string{"IO2", "IO3", "IO1"},
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1", "io2", "io3"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	want := []string{"433M/800M/900M/1.4G", "1.2G/1.5G", "2.4G/5.2G/5.8G"}
	if !equalStrings(detail.ChannelLabels, want) {
		t.Fatalf("detail.ChannelLabels = %#v, want %#v", detail.ChannelLabels, want)
	}
}

func TestStoreNormalizesLegacyGPIOErrorPrefixes(t *testing.T) {
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
	report, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:    model.InterferenceReportStatusFailed,
			StartedAt: startedAt,
			LastError: "更新 GPIO 状态失败: 导出 GPIO20 失败: open /sys/class/gpio/export: no such file or directory",
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1"}, DurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	summaries, err := store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].LastError != "open /sys/class/gpio/export: no such file or directory" {
		t.Fatalf("LastError = %#v, want normalized legacy error", summaries)
	}
	detail, err := store.Get(report.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.LastError != "open /sys/class/gpio/export: no such file or directory" {
		t.Fatalf("detail.LastError = %q, want normalized legacy error", detail.LastError)
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func TestNormalizeReportErrorRemovesGPIOWrappers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "update and write",
			raw:  "更新 GPIO 状态失败: 写入 GPIO20/value 失败: open /sys/class/gpio/gpio20/value: no such file or directory",
			want: "open /sys/class/gpio/gpio20/value: no such file or directory",
		},
		{
			name: "english update and export",
			raw:  "Failed to update GPIO state: 导出 GPIO20 失败: open /sys/class/gpio/export: no such file or directory",
			want: "open /sys/class/gpio/export: no such file or directory",
		},
		{
			name: "direction wrapper",
			raw:  "设置 GPIO20 为输出模式失败: permission denied",
			want: "permission denied",
		},
		{
			name: "external io missing value file",
			raw:  "更新 GPIO 状态失败: 外部 IO0 电平文件不可用: no such file or directory",
			want: "no such file or directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReportError(tt.raw); got != tt.want {
				t.Fatalf("normalizeReportError() = %q, want %q", got, tt.want)
			}
		})
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
	failed, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:    model.InterferenceReportStatusFailed,
			StartedAt: startedAt,
			LastError: "gpio failed",
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io1"}},
	})
	if err != nil {
		t.Fatalf("Create(failed) error = %v", err)
	}
	completed, err := store.Create(model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:    model.InterferenceReportStatusCompleted,
			StartedAt: startedAt.Add(time.Minute),
		},
		Request: model.ScreenStrikeRequest{Enabled: true, ChannelIDs: []string{"io2"}},
	})
	if err != nil {
		t.Fatalf("Create(completed) error = %v", err)
	}

	deleted, err := store.DeleteFailed(completed.ID)
	if !errors.Is(err, ErrNotFailed) {
		t.Fatalf("DeleteFailed(completed) deleted=%d error=%v, want ErrNotFailed", deleted, err)
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

func TestParseStatus(t *testing.T) {
	if status, err := ParseStatus("completed"); err != nil || status != model.InterferenceReportStatusCompleted {
		t.Fatalf("ParseStatus(completed) = %q/%v, want completed/nil", status, err)
	}
	if _, err := ParseStatus("deleted"); err == nil {
		t.Fatal("ParseStatus(deleted) error = nil, want error")
	}
}
