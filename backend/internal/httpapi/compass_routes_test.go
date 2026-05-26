package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/compass"
	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/settings"
	"dr600ab-api/internal/store"
)

const testDeveloperSecret = "JBSWY3DPEHPK3PXP"

func TestHandleUpdateCompassSettingsClearsEmptyPort(t *testing.T) {
	translator := mustTranslator(t)
	developerSvc, token := newTestDeveloperSession(t)
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsStore.SaveCompass(model.CompassSessionRequest{PortName: "/dev/ttyUSB3"}); err != nil {
		t.Fatalf("SaveCompass() error = %v", err)
	}
	server := &Server{
		translator: translator,
		developer:  developerSvc,
		compass: compass.NewService(
			store.NewMemoryStore(10, 10),
			translator,
			settingsStore,
			compass.Options{},
		),
	}
	server.app = fiber.New()
	api := server.app.Group("/api/v1")
	server.registerCompassRoutes(api)

	req, err := http.NewRequest(
		http.MethodPut,
		"/api/v1/compass/settings",
		bytes.NewBufferString(`{"portName":"   "}`),
	)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Developer-Token", token)

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.CompassSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Active || body.State != "inactive" {
		t.Fatalf("response = %+v, want inactive compass session", body)
	}
	saved, ok, err := settingsStore.LoadCompass()
	if err != nil {
		t.Fatalf("LoadCompass() error = %v", err)
	}
	if !ok || saved.PortName != "" {
		t.Fatalf("saved compass settings = %+v, ok = %v, want empty port", saved, ok)
	}
}

func newTestDeveloperSession(t *testing.T) (*developer.Service, string) {
	t.Helper()

	service, err := developer.NewService(testDeveloperSecret, time.Minute)
	if err != nil {
		t.Fatalf("developer.NewService() error = %v", err)
	}
	token, _, err := service.Login(testTOTPCode(t, testDeveloperSecret, time.Now()))
	if err != nil {
		t.Fatalf("developer.Login() error = %v", err)
	}
	return service, token
}

func testTOTPCode(t *testing.T, secret string, now time.Time) string {
	t.Helper()

	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}

	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(now.Unix()/30))
	hash := hmac.New(sha1.New, key)
	_, _ = hash.Write(buffer[:])
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code := value % 1_000_000
	return fmt.Sprintf("%06d", code)
}
