package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
)

type memoryUserSettingsStore struct {
	settings model.UserSettings
	ok       bool
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
}

func mustTranslator(t *testing.T) *i18n.Translator {
	t.Helper()

	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	return translator
}
