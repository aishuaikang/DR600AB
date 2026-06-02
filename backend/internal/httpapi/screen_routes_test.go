package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.bug.st/serial"

	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/fpv"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
)

func TestScreenDetectionCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{},
		false,
		model.DetectionSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive detection", status)
	}
}

func TestScreenDetectionCapabilityStatusKeepsConfiguredOfflineState(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{
			RxPortName: "/dev/rx",
			TxPortName: "/dev/tx",
		},
		true,
		model.DetectionSessionResponse{
			State:     "connecting",
			LastError: "open /dev/rx: no such file",
		},
	)

	if !status.Configured || status.Active || status.State != "connecting" {
		t.Fatalf("status = %+v, want configured offline detection", status)
	}
	if status.RxPortName != "/dev/rx" || status.TxPortName != "/dev/tx" {
		t.Fatalf("status ports = %+v, want configured rx/tx ports", status)
	}
	if status.LastError == "" {
		t.Fatalf("lastError = %q, want configured error", status.LastError)
	}
}

func TestScreenDetectionCapabilityStatusTreatsCurrentSessionAsConfigured(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{},
		false,
		model.DetectionSessionResponse{
			Active:     true,
			State:      "connected",
			RxPortName: "/dev/rx",
			TxPortName: "/dev/tx",
		},
	)

	if !status.Configured || !status.Active || status.State != "connected" {
		t.Fatalf("status = %+v, want configured active detection", status)
	}
	if status.RxPortName != "/dev/rx" || status.TxPortName != "/dev/tx" {
		t.Fatalf("status ports = %+v, want session rx/tx ports", status)
	}
}

func TestScreenDeceptionCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenDeceptionCapabilityStatus(
		model.DeceptionSessionRequest{},
		false,
		model.DeceptionSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive deception", status)
	}
}

func TestScreenCompassCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{},
		false,
		model.CompassSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive compass", status)
	}
}

func TestScreenCompassCapabilityStatusKeepsConfiguredOfflineState(t *testing.T) {
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{PortName: "/dev/ttyUSB3"},
		true,
		model.CompassSessionResponse{
			State:     "connecting",
			LastError: "open /dev/ttyUSB3: no such file",
		},
	)

	if !status.Configured || status.Active || status.State != "connecting" {
		t.Fatalf("status = %+v, want configured offline compass", status)
	}
	if status.PortName != "/dev/ttyUSB3" || status.LastError == "" {
		t.Fatalf("status = %+v, want configured port and error", status)
	}
}

func TestScreenCompassCapabilityStatusTreatsCurrentSessionAsConfigured(t *testing.T) {
	heading := 123.45
	updatedAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{},
		false,
		model.CompassSessionResponse{
			Active:        true,
			State:         "connected",
			PortName:      "/dev/ttyUSB3",
			LastHeading:   &heading,
			LastUpdatedAt: &updatedAt,
		},
	)

	if !status.Configured || !status.Active || status.State != "connected" || status.PortName != "/dev/ttyUSB3" {
		t.Fatalf("status = %+v, want configured active compass", status)
	}
	if status.HeadingDeg == nil || *status.HeadingDeg != heading {
		t.Fatalf("heading = %v, want %.2f", status.HeadingDeg, heading)
	}
	if status.HeadingUpdatedAt == nil || !status.HeadingUpdatedAt.Equal(updatedAt) {
		t.Fatalf("heading updated at = %v, want %v", status.HeadingUpdatedAt, updatedAt)
	}
}

func TestScreenDeviceLocationResponsePrefersGPSOverManual(t *testing.T) {
	updatedAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	response := screenDeviceLocationResponse(
		&model.GPSFix{Latitude: 23.1, Longitude: 113.2, Valid: true},
		&updatedAt,
		model.UserSettings{
			ManualDeviceLocation: &model.GeoPoint{Latitude: 39.9, Longitude: 116.3},
		},
	)

	if response.Source != "gps" || !response.Valid || response.Point == nil {
		t.Fatalf("response = %+v, want GPS point", response)
	}
	if response.Point.Latitude != 23.1 || response.Point.Longitude != 113.2 {
		t.Fatalf("point = %+v, want GPS coordinates", response.Point)
	}
	if response.UpdatedAt == nil || !response.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updatedAt = %v, want %v", response.UpdatedAt, updatedAt)
	}
}

func TestScreenDeviceLocationResponseFallsBackToManual(t *testing.T) {
	response := screenDeviceLocationResponse(
		nil,
		nil,
		model.UserSettings{
			ManualDeviceLocation: &model.GeoPoint{Latitude: 39.9, Longitude: 116.3},
		},
	)

	if response.Source != "manual" || !response.Valid || response.Point == nil {
		t.Fatalf("response = %+v, want manual point", response)
	}
	if response.Point.Latitude != 39.9 || response.Point.Longitude != 116.3 {
		t.Fatalf("point = %+v, want manual coordinates", response.Point)
	}
}

func TestScreenDeviceLocationResponseReturnsNoneWithoutValidPoint(t *testing.T) {
	response := screenDeviceLocationResponse(
		&model.GPSFix{Latitude: 91, Longitude: 113.2, Valid: true},
		nil,
		model.UserSettings{},
	)

	if response.Source != "none" || response.Valid || response.Point != nil {
		t.Fatalf("response = %+v, want none", response)
	}
}

func TestValidGeoPointRejectsInvalidCoordinates(t *testing.T) {
	for _, point := range []*model.GeoPoint{
		nil,
		{Latitude: -91, Longitude: 0},
		{Latitude: 91, Longitude: 0},
		{Latitude: 0, Longitude: -181},
		{Latitude: 0, Longitude: 181},
		{Latitude: 0, Longitude: 0},
	} {
		if validGeoPoint(point) {
			t.Fatalf("validGeoPoint(%+v) = true, want false", point)
		}
	}
}

func TestScreenFPVVideoReturnsSerialOffline(t *testing.T) {
	detectionSvc := newScreenFPVDetectionService(t)
	fpvSvc := fpv.NewService(fpv.Options{Host: "192.168.8.10", Port: 49600})
	server := newScreenFPVTestServer(t, detectionSvc, fpvSvc)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/screen/fpv/video?frequency=1360", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var payload model.ApiError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "detection_serial_offline" {
		t.Fatalf("error code = %q, want detection_serial_offline", payload.Code)
	}
	if fpvSvc.Snapshot().Active {
		t.Fatalf("FPV playback stayed active after serial offline failure")
	}
}

func TestScreenFPVVideoRejectsBusyPlayback(t *testing.T) {
	detectionSvc := newScreenFPVDetectionService(t)
	fpvSvc := fpv.NewService(fpv.Options{Host: "192.168.8.10", Port: 49600})
	playback, err := fpvSvc.BeginPlayback(1280)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	defer fpvSvc.EndPlayback(playback)

	server := newScreenFPVTestServer(t, detectionSvc, fpvSvc)
	req, err := http.NewRequest(http.MethodGet, "/api/v1/screen/fpv/video?frequency=1360", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	var payload model.ApiError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "fpv_video_busy" {
		t.Fatalf("error code = %q, want fpv_video_busy", payload.Code)
	}
}

func TestScreenFPVPlaybackCommandOrder(t *testing.T) {
	detectionSvc := newScreenFPVDetectionService(t)
	ports := map[string]*screenFPVSerialPort{}
	detectionSvc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newScreenFPVSerialPort()
		ports[cfg.PortName] = port
		return port, nil
	})

	_, err := detectionSvc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/fpv-rx",
		TxPortName: "/dev/fpv-tx",
		RxBaudRate: 115200,
		TxBaudRate: 460800,
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer detectionSvc.Stop("zh-CN")

	fpvSvc := fpv.NewService(fpv.Options{Host: "192.168.8.10", Port: 49600})
	playback, err := fpvSvc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	defer fpvSvc.EndPlayback(playback)

	server := &Server{detection: detectionSvc, fpv: fpvSvc}
	if err := server.startFPVPlayback(playback); err != nil {
		t.Fatalf("startFPVPlayback() error = %v", err)
	}
	if err := server.stopFPVPlayback(); err != nil {
		t.Fatalf("stopFPVPlayback() error = %v", err)
	}

	assertScreenFPVPortWrites(t, ports["/dev/fpv-tx"],
		"start -freq 1\n",
		"start -imag 192.168.8.10:49600\r\n",
		"start -band 1310,1410\r\n",
		"start -imag 0\r\n",
		"start -freq 1\r\n",
	)
}

func TestScreenFPVPlaybackRollsBackImageOutputWhenBandCommandFails(t *testing.T) {
	detectionSvc := newScreenFPVDetectionService(t)
	ports := map[string]*screenFPVSerialPort{}
	detectionSvc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newScreenFPVSerialPort()
		ports[cfg.PortName] = port
		return port, nil
	})

	_, err := detectionSvc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/fpv-rx",
		TxPortName: "/dev/fpv-tx",
		RxBaudRate: 115200,
		TxBaudRate: 460800,
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer detectionSvc.Stop("zh-CN")

	txPort := ports["/dev/fpv-tx"]
	txPort.failOnceContains = "start -band"

	fpvSvc := fpv.NewService(fpv.Options{Host: "192.168.8.10", Port: 49600})
	playback, err := fpvSvc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	defer fpvSvc.EndPlayback(playback)

	server := &Server{detection: detectionSvc, fpv: fpvSvc}
	if err := server.startFPVPlayback(playback); err == nil {
		t.Fatal("startFPVPlayback() error = nil, want band command failure")
	}

	assertScreenFPVPortWrites(t, txPort,
		"start -freq 1\n",
		"start -imag 192.168.8.10:49600\r\n",
		"start -imag 0\r\n",
		"start -freq 1\r\n",
	)
}

func newScreenFPVTestServer(t *testing.T, detectionSvc *detection.Service, fpvSvc *fpv.Service) *Server {
	t.Helper()
	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	server := &Server{
		app:        fiber.New(),
		translator: translator,
		detection:  detectionSvc,
		fpv:        fpvSvc,
	}
	server.app.Get("/api/v1/screen/fpv/video", server.handleScreenFPVVideo)
	return server
}

func newScreenFPVDetectionService(t *testing.T) *detection.Service {
	t.Helper()
	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	return detection.NewService(store.NewMemoryStore(10, 10), translator, nil, detection.Options{})
}

type screenFPVSerialPort struct {
	mu               sync.Mutex
	writes           []string
	closeCh          chan struct{}
	closed           bool
	failOnceContains string
}

func newScreenFPVSerialPort() *screenFPVSerialPort {
	return &screenFPVSerialPort{closeCh: make(chan struct{})}
}

func (p *screenFPVSerialPort) SetMode(mode *serial.Mode) error { return nil }

func (p *screenFPVSerialPort) Read(b []byte) (int, error) {
	<-p.closeCh
	return 0, io.EOF
}

func (p *screenFPVSerialPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failOnceContains != "" && strings.Contains(string(b), p.failOnceContains) {
		p.failOnceContains = ""
		return 0, errors.New("forced write failure")
	}
	p.writes = append(p.writes, string(b))
	return len(b), nil
}

func (p *screenFPVSerialPort) Drain() error { return nil }

func (p *screenFPVSerialPort) ResetInputBuffer() error { return nil }

func (p *screenFPVSerialPort) ResetOutputBuffer() error { return nil }

func (p *screenFPVSerialPort) SetDTR(dtr bool) error { return nil }

func (p *screenFPVSerialPort) SetRTS(rts bool) error { return nil }

func (p *screenFPVSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (p *screenFPVSerialPort) SetReadTimeout(timeout time.Duration) error { return nil }

func (p *screenFPVSerialPort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.closeCh)
	}
	return nil
}

func (p *screenFPVSerialPort) Break(duration time.Duration) error { return nil }

func assertScreenFPVPortWrites(t *testing.T, port *screenFPVSerialPort, want ...string) {
	t.Helper()
	if port == nil {
		t.Fatalf("port is nil")
	}

	port.mu.Lock()
	got := append([]string(nil), port.writes...)
	port.mu.Unlock()

	if len(got) != len(want) {
		t.Fatalf("writes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("writes[%d] = %q, want %q; all writes = %#v", i, got[i], want[i], got)
		}
	}
}
