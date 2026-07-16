package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.bug.st/serial"

	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/gps"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
)

func TestGPSReadRoutesAllowLicensedUserWithoutDeveloperSession(t *testing.T) {
	translator := mustTranslator(t)
	memoryStore := store.NewMemoryStore(10, 10)
	gpsSvc := gps.NewService(memoryStore, translator, nil, gps.Options{})
	gpsSvc.IngestLine(
		"gps-user",
		"/dev/ttyGPS0",
		"$GNGGA,013909.00,,,,,0,00,99.99,,,,,,*7A",
	)
	server := &Server{
		app:        fiber.New(),
		translator: translator,
		gps:        gpsSvc,
		detection:  detection.NewService(memoryStore, translator, nil, detection.Options{}),
	}
	server.registerGPSReadRoutes(server.app.Group("/api/v1"))

	tests := []struct {
		name string
		path string
	}{
		{name: "session", path: "/api/v1/gps/session"},
		{name: "records", path: "/api/v1/gps/records"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.path, nil)
			if err != nil {
				t.Fatalf("NewRequest(%q) error = %v", tt.path, err)
			}
			resp, err := server.app.Test(req)
			if err != nil {
				t.Fatalf("app.Test(%q) error = %v", tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", tt.path, resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestHandleSendDetectionCommandWritesToTXPort(t *testing.T) {
	translator := mustTranslator(t)
	developerSvc, token := newTestDeveloperSession(t)
	detectionSvc := detection.NewService(store.NewMemoryStore(10, 10), translator, nil, detection.Options{})

	ports := map[string]*screenFPVSerialPort{}
	detectionSvc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newScreenFPVSerialPort()
		ports[cfg.PortName] = port
		return port, nil
	})
	if _, err := detectionSvc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer detectionSvc.Stop("zh-CN")

	server := &Server{
		app:        fiber.New(),
		translator: translator,
		developer:  developerSvc,
		detection:  detectionSvc,
	}
	api := server.app.Group("/api/v1")
	server.registerDetectionRecordRoutes(api)

	req, err := http.NewRequest(
		http.MethodPost,
		"/api/v1/detection/commands",
		bytes.NewBufferString(`{"command":"  start -freq 1360  "}`),
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body model.DetectionCommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Command != "start -freq 1360" || body.Message == "" {
		t.Fatalf("response = %+v, want trimmed command and message", body)
	}
	assertScreenFPVPortWrites(
		t,
		ports["/dev/tx"],
		"start -freq 1, -pathb 1, -gain 60\r\n",
		"start -freq 1360\r\n",
	)
}

func TestHandleSendDetectionCommandRejectsEmptyCommand(t *testing.T) {
	translator := mustTranslator(t)
	developerSvc, token := newTestDeveloperSession(t)
	server := &Server{
		app:        fiber.New(),
		translator: translator,
		developer:  developerSvc,
		detection:  detection.NewService(store.NewMemoryStore(10, 10), translator, nil, detection.Options{}),
	}
	api := server.app.Group("/api/v1")
	server.registerDetectionRecordRoutes(api)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/detection/commands", bytes.NewBufferString(`{"command":"   "}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Developer-Token", token)

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandleSendDetectionCommandRejectsMultilineCommand(t *testing.T) {
	translator := mustTranslator(t)
	developerSvc, token := newTestDeveloperSession(t)
	server := &Server{
		app:        fiber.New(),
		translator: translator,
		developer:  developerSvc,
		detection:  detection.NewService(store.NewMemoryStore(10, 10), translator, nil, detection.Options{}),
	}
	api := server.app.Group("/api/v1")
	server.registerDetectionRecordRoutes(api)

	req, err := http.NewRequest(http.MethodPost, "/api/v1/detection/commands", bytes.NewBufferString(`{"command":"start -freq 1360\r\nstart -imag 0"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Developer-Token", token)

	resp, err := server.app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var body model.ApiError
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Code != "invalid_request" {
		t.Fatalf("error code = %q, want invalid_request", body.Code)
	}
}
