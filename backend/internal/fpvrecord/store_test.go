package fpvrecord

import (
	"path/filepath"
	"testing"
	"time"

	"dr600ab-api/internal/model"
)

func TestStoreInsertListDelete(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "fpv-video-records.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(12 * time.Second)
	lastFrameAt := startedAt.Add(3 * time.Second)
	record := model.FPVVideoRecord{
		ID:            "fpv-1",
		TargetID:      "target-1",
		Serial:        "SN123",
		Model:         "DJI_O3+",
		DisplayModel:  "DJI O3+",
		Device:        "detector",
		Frequency:     1360,
		RSSI:          -57.5,
		StartedAt:     startedAt,
		EndedAt:       endedAt,
		Status:        model.FPVVideoRecordStatusCompleted,
		FrameCount:    4,
		LastFrameRows: 120,
		LastFrameCols: 160,
		LastFrameAt:   &lastFrameAt,
		LastRecord:    model.ScreenDetectionLastRecord{ID: "record-1", Model: "DJI_O3+", Frequency: 1360},
		Frames: []model.FPVVideoRecordFrame{
			{Num: 1, Rows: 120, Cols: 160, PixelCount: 19200, Image: "data:image/png;base64,test"},
		},
		CreatedAt:       startedAt,
		DurationSeconds: 99,
	}
	if err := store.Insert(record); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	items, err := store.List(QueryOptions{Limit: 10, Status: model.FPVVideoRecordStatusCompleted})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	item := items[0]
	if item.ID != record.ID || item.TargetID != record.TargetID || item.Serial != record.Serial {
		t.Fatalf("item identity = %+v, want record identity", item)
	}
	if item.DurationSeconds != 12 {
		t.Fatalf("DurationSeconds = %d, want normalized 12", item.DurationSeconds)
	}
	if item.LastFrameAt == nil || !item.LastFrameAt.Equal(lastFrameAt) {
		t.Fatalf("LastFrameAt = %v, want %v", item.LastFrameAt, lastFrameAt)
	}
	if item.LastRecord == nil {
		t.Fatalf("LastRecord = nil, want stored JSON")
	}
	if len(item.Frames) != 0 {
		t.Fatalf("list item Frames = %d, want omitted summary frames", len(item.Frames))
	}
	detail, ok, err := store.Get("fpv-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatalf("Get() ok = false, want true")
	}
	if len(detail.Frames) != 1 || detail.Frames[0].Image != record.Frames[0].Image {
		t.Fatalf("detail Frames = %#v, want stored frame", detail.Frames)
	}

	deleted, err := store.Delete([]string{"fpv-1", "fpv-1", ""})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	items, err = store.List(QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() after delete error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) after delete = %d, want 0", len(items))
	}
}

func TestStoreListFiltersStatusAndPages(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "fpv-video-records.db"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	startedAt := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	records := []model.FPVVideoRecord{
		{ID: "completed-new", StartedAt: startedAt.Add(2 * time.Minute), EndedAt: startedAt.Add(2*time.Minute + time.Second), Status: model.FPVVideoRecordStatusCompleted},
		{ID: "failed", StartedAt: startedAt.Add(time.Minute), EndedAt: startedAt.Add(time.Minute + time.Second), Status: model.FPVVideoRecordStatusFailed},
		{ID: "completed-old", StartedAt: startedAt, EndedAt: startedAt.Add(time.Second), Status: model.FPVVideoRecordStatusCompleted},
	}
	for _, record := range records {
		if err := store.Insert(record); err != nil {
			t.Fatalf("Insert(%s) error = %v", record.ID, err)
		}
	}

	items, err := store.List(QueryOptions{
		Limit:  1,
		Offset: 1,
		Status: model.FPVVideoRecordStatusCompleted,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != "completed-old" {
		t.Fatalf("items = %#v, want second completed record", items)
	}
}

func TestParseStatus(t *testing.T) {
	if status, err := ParseStatus("completed"); err != nil || status != model.FPVVideoRecordStatusCompleted {
		t.Fatalf("ParseStatus(completed) = %q, %v; want completed, nil", status, err)
	}
	if status, err := ParseStatus(""); err != nil || status != "" {
		t.Fatalf("ParseStatus(empty) = %q, %v; want empty, nil", status, err)
	}
	if _, err := ParseStatus("running"); err == nil {
		t.Fatalf("ParseStatus(running) error = nil, want error")
	}
}
