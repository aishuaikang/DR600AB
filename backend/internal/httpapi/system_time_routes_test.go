package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/systemtime"
)

type fakeSystemTimeService struct {
	info     systemtime.Info
	zones    []string
	ntpState bool
	timezone string
	manual   string
}

func (f *fakeSystemTimeService) GetInfo(context.Context) (systemtime.Info, error) {
	info := f.info
	info.NTPEnabled = f.ntpState
	info.Timezone = f.timezone
	return info, nil
}

func (f *fakeSystemTimeService) ListTimezones(context.Context) ([]string, error) {
	return f.zones, nil
}

func (f *fakeSystemTimeService) SetTimezone(_ context.Context, timezone string) error {
	if strings.ContainsAny(timezone, ";\n") {
		return systemtime.ErrInvalidTimezone
	}
	f.timezone = timezone
	return nil
}

func (f *fakeSystemTimeService) SetNTPEnabled(_ context.Context, enabled bool) error {
	f.ntpState = enabled
	return nil
}

func (f *fakeSystemTimeService) SetManualTime(_ context.Context, value string) error {
	f.manual = value
	f.ntpState = false
	return nil
}

func newSystemTimeTestApp(t *testing.T, service *fakeSystemTimeService) *fiber.App {
	t.Helper()
	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("create translator: %v", err)
	}
	server := &Server{systemTime: service, translator: translator}
	app := fiber.New()
	server.registerSystemTimeRoutes(app)
	return app
}

func TestSystemTimeRoutes(t *testing.T) {
	service := &fakeSystemTimeService{
		info:     systemtime.Info{Platform: "linux", TimeManagementSupported: true, CurrentTime: "2026-07-22 10:20:30", UTCOffset: "+08:00", NTPSynced: true},
		zones:    []string{"Asia/Shanghai", "UTC"},
		ntpState: true,
		timezone: "Asia/Shanghai",
	}
	app := newSystemTimeTestApp(t, service)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/system/time", nil))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("GET /system/time status = %v, err = %v", response.StatusCode, err)
	}
	var info systemtime.Info
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatalf("decode time info: %v", err)
	}
	if info.Timezone != "Asia/Shanghai" || !info.NTPEnabled {
		t.Fatalf("unexpected time info: %#v", info)
	}

	response, err = app.Test(httptest.NewRequest(http.MethodGet, "/system/timezones", nil))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("GET /system/timezones status = %v, err = %v", response.StatusCode, err)
	}

	request := httptest.NewRequest(http.MethodPut, "/system/time/ntp", strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK || service.ntpState {
		t.Fatalf("PUT /system/time/ntp status = %v, err = %v, state = %v", response.StatusCode, err, service.ntpState)
	}

	request = httptest.NewRequest(http.MethodPut, "/system/timezone", strings.NewReader(`{"timezone":"Asia/Shanghai; reboot"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid timezone status = %v, err = %v", response.StatusCode, err)
	}

	request = httptest.NewRequest(http.MethodPut, "/system/time/manual", strings.NewReader(`{"datetime":"2026-07-22 10:20:30"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = app.Test(request)
	if err != nil || response.StatusCode != http.StatusOK || service.manual != "2026-07-22 10:20:30" || service.ntpState {
		t.Fatalf("manual time status = %v, err = %v, value = %q, ntp = %v", response.StatusCode, err, service.manual, service.ntpState)
	}
}
