package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/model"
)

type memoryUserSettingsStore struct {
	settings model.UserSettings
	ok       bool
}

type pruningIntrusionStore struct {
	items []model.IntrusionRecord
}

func (s *memoryUserSettingsStore) LoadUser() (model.UserSettings, bool, error) {
	return s.settings, s.ok, nil
}

func (s *memoryUserSettingsStore) SaveUser(settings model.UserSettings) error {
	s.settings = settings
	s.ok = true
	return nil
}

func (s *memoryUserSettingsStore) SaveEditableUser(settings model.UserSettings) (model.UserSettings, error) {
	settings.DeviceSN = s.settings.DeviceSN
	s.settings = settings
	s.ok = true
	return settings, nil
}

func (s *pruningIntrusionStore) List(options intrusion.QueryOptions) ([]model.IntrusionRecord, error) {
	return s.items, nil
}

func (s *pruningIntrusionStore) Delete(ids []string) (int64, error) {
	return 0, nil
}

func (s *pruningIntrusionStore) PruneRetention(days int, now time.Time) (int64, error) {
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

func (s *pruningIntrusionStore) Close() error {
	return nil
}

func TestHandleUpdateUserSettingsPreservesDeviceSN(t *testing.T) {
	store := &memoryUserSettingsStore{
		settings: model.UserSettings{
			DeviceSN: "10125",
		},
		ok: true,
	}
	server := &Server{
		translator:   mustTranslator(t),
		userSettings: store,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerUserRoutes(api)

	body, err := json.Marshal(model.UserSettings{
		DeviceSN:             "client-sn",
		ManualDeviceLocation: &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, "/api/v1/user/settings", bytes.NewReader(body))
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
	if store.settings.DeviceSN != "10125" {
		t.Fatalf("saved device SN = %q, want preserved 10125", store.settings.DeviceSN)
	}
	if store.settings.ManualDeviceLocation == nil ||
		store.settings.ManualDeviceLocation.Latitude != 23.12911 ||
		store.settings.ManualDeviceLocation.Longitude != 113.264385 {
		t.Fatalf("saved manual location = %+v, want request value", store.settings.ManualDeviceLocation)
	}
	if store.settings.IntrusionRetentionDays == nil || *store.settings.IntrusionRetentionDays != model.DefaultIntrusionRetentionDays {
		t.Fatalf("retention days = %#v, want default", store.settings.IntrusionRetentionDays)
	}
}

func TestHandleUserSettingsReturnsDefaultIntrusionRetention(t *testing.T) {
	server := &Server{
		translator:   mustTranslator(t),
		userSettings: &memoryUserSettingsStore{},
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerUserRoutes(api)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/user/settings", nil)
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
	var body model.UserSettings
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.IntrusionRetentionDays == nil || *body.IntrusionRetentionDays != model.DefaultIntrusionRetentionDays {
		t.Fatalf("retention days = %#v, want default", body.IntrusionRetentionDays)
	}
}

func TestHandleUpdateUserSettingsPrunesIntrusions(t *testing.T) {
	oldArchivedAt := time.Now().AddDate(0, 0, -100)
	recentArchivedAt := time.Now().AddDate(0, 0, -10)
	retentionDays := 90
	intrusions := &pruningIntrusionStore{
		items: []model.IntrusionRecord{
			{ID: "old", TargetID: "old", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: oldArchivedAt, LastSeen: oldArchivedAt, ArchivedAt: oldArchivedAt},
			{ID: "recent", TargetID: "recent", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: recentArchivedAt, LastSeen: recentArchivedAt, ArchivedAt: recentArchivedAt},
		},
	}
	server := &Server{
		translator:   mustTranslator(t),
		userSettings: &memoryUserSettingsStore{},
		intrusions:   intrusions,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerUserRoutes(api)

	body, err := json.Marshal(model.UserSettings{IntrusionRetentionDays: &retentionDays})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, "/api/v1/user/settings", bytes.NewReader(body))
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
	if len(intrusions.items) != 1 || intrusions.items[0].ID != "recent" {
		t.Fatalf("intrusions = %#v, want only recent", intrusions.items)
	}
}

func TestHandleUpdateUserSettingsRetentionZeroKeepsIntrusions(t *testing.T) {
	retentionDays := 0
	intrusions := &pruningIntrusionStore{
		items: []model.IntrusionRecord{
			{ID: "old", TargetID: "old", TargetType: model.IntrusionTargetTypeDetection, FirstSeen: time.Now(), LastSeen: time.Now(), ArchivedAt: time.Now().AddDate(0, 0, -100)},
		},
	}
	server := &Server{
		translator:   mustTranslator(t),
		userSettings: &memoryUserSettingsStore{},
		intrusions:   intrusions,
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerUserRoutes(api)

	body, err := json.Marshal(model.UserSettings{IntrusionRetentionDays: &retentionDays})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, "/api/v1/user/settings", bytes.NewReader(body))
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
	if len(intrusions.items) != 1 {
		t.Fatalf("intrusions count = %d, want 1", len(intrusions.items))
	}
}

func mustTranslator(t *testing.T) *i18n.Translator {
	t.Helper()

	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	return translator
}
