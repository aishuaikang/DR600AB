package gps

import (
	"io"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"go.bug.st/serial"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/settings"
	"dr600ab-api/internal/store"
	"serialport"
)

type fakeSerialPort struct {
	mu         sync.Mutex
	closeCount int
	closeCh    chan struct{}
	writes     []string
}

func newFakeSerialPort() *fakeSerialPort {
	return &fakeSerialPort{closeCh: make(chan struct{})}
}

func (p *fakeSerialPort) SetMode(mode *serial.Mode) error { return nil }

func (p *fakeSerialPort) Read(b []byte) (int, error) {
	<-p.closeCh
	return 0, io.EOF
}

func (p *fakeSerialPort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, string(b))
	return len(b), nil
}

func (p *fakeSerialPort) Drain() error { return nil }

func (p *fakeSerialPort) ResetInputBuffer() error { return nil }

func (p *fakeSerialPort) ResetOutputBuffer() error { return nil }

func (p *fakeSerialPort) SetDTR(dtr bool) error { return nil }

func (p *fakeSerialPort) SetRTS(rts bool) error { return nil }

func (p *fakeSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}

func (p *fakeSerialPort) SetReadTimeout(timeout time.Duration) error { return nil }

func (p *fakeSerialPort) Close() error {
	p.mu.Lock()
	p.closeCount++
	p.mu.Unlock()
	select {
	case <-p.closeCh:
	default:
		close(p.closeCh)
	}
	return nil
}

func (p *fakeSerialPort) Break(duration time.Duration) error { return nil }

func TestStartSendsQGPSCommandToControlPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string]*fakeSerialPort{}
	var openOrder []string
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		opened[cfg.PortName] = port
		openOrder = append(openOrder, cfg.PortName)
		return port, nil
	})

	resp, err := svc.Start(model.GPSSessionRequest{
		DataPortName:    "/dev/ttyUSB1",
		ControlPortName: "/dev/ttyUSB2",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		Parity:          "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !resp.Active {
		t.Fatal("expected GPS session to be active")
	}
	if resp.DataPortName != "/dev/ttyUSB1" || resp.ControlPortName != "/dev/ttyUSB2" {
		t.Fatalf("unexpected ports: %+v", resp)
	}
	if want := []string{"/dev/ttyUSB2", "/dev/ttyUSB1"}; !slices.Equal(openOrder, want) {
		t.Fatalf("open order = %v, want %v", openOrder, want)
	}
	assertPortWrites(t, opened["/dev/ttyUSB1"])
	assertPortWrites(t, opened["/dev/ttyUSB2"], startGPSCommand)
	assertCloseCount(t, opened["/dev/ttyUSB2"], 1)
	assertCloseCount(t, opened["/dev/ttyUSB1"], 0)

	_ = svc.Stop("zh-CN")
	assertCloseCount(t, opened["/dev/ttyUSB1"], 1)
}

func TestStartSendsQGPSCommandOnSharedGPSPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string]*fakeSerialPort{}
	var openOrder []string
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		opened[cfg.PortName] = port
		openOrder = append(openOrder, cfg.PortName)
		return port, nil
	})

	resp, err := svc.Start(model.GPSSessionRequest{
		DataPortName:    "/dev/ttyUSB0",
		ControlPortName: "/dev/ttyUSB0",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		Parity:          "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !resp.Active {
		t.Fatal("expected GPS session to be active")
	}
	if want := []string{"/dev/ttyUSB0"}; !slices.Equal(openOrder, want) {
		t.Fatalf("open order = %v, want %v", openOrder, want)
	}
	assertPortWrites(t, opened["/dev/ttyUSB0"], startGPSCommand)
	assertCloseCount(t, opened["/dev/ttyUSB0"], 0)

	_ = svc.Stop("zh-CN")
	assertCloseCount(t, opened["/dev/ttyUSB0"], 1)
}

func TestIngestLineParsesGGAAndRMC(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("gps-1", "/dev/ttyUSB1", "$GPGGA,092750.000,5321.6802,N,00630.3372,W,1,8,1.03,61.7,M,55.2,M,,*76")
	svc.IngestLine("gps-1", "/dev/ttyUSB1", "$GPRMC,092751.000,A,5321.6803,N,00630.3373,W,0.13,309.62,120598,,,A*10")

	records := svc.Records(10)
	if len(records) != 2 {
		t.Fatalf("GPS record count = %d, want 2", len(records))
	}
	if records[1].Type != "GGA" || records[1].Fix == nil || !records[1].Fix.Valid {
		t.Fatalf("GGA record = %+v, want valid fix", records[1])
	}
	if records[0].Type != "RMC" || records[0].Fix == nil || !records[0].Fix.Valid {
		t.Fatalf("RMC record = %+v, want valid fix", records[0])
	}
	assertClose(t, records[0].Fix.Latitude, 53.3613383333)
	assertClose(t, records[0].Fix.Longitude, -6.5056216667)
}

func TestSettingsStoreKeepsDetectionAndGPS(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "settings.json")
	settingsStore := settings.NewStore(storePath)

	detectionReq := model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}
	gpsReq := model.GPSSessionRequest{
		DataPortName:    "/dev/ttyUSB1",
		ControlPortName: "/dev/ttyUSB2",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		Parity:          "none",
	}

	if err := settingsStore.Save(detectionReq); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := settingsStore.SaveGPS(gpsReq); err != nil {
		t.Fatalf("SaveGPS() error = %v", err)
	}

	gotDetection, ok, err := settingsStore.Load()
	if err != nil || !ok {
		t.Fatalf("Load() = %+v, %v, %v", gotDetection, ok, err)
	}
	gotGPS, ok, err := settingsStore.LoadGPS()
	if err != nil || !ok {
		t.Fatalf("LoadGPS() = %+v, %v, %v", gotGPS, ok, err)
	}
	if gotDetection.RxPortName != detectionReq.RxPortName || gotDetection.TxPortName != detectionReq.TxPortName {
		t.Fatalf("detection settings = %+v, want %+v", gotDetection, detectionReq)
	}
	if gotGPS.DataPortName != gpsReq.DataPortName || gotGPS.ControlPortName != gpsReq.ControlPortName {
		t.Fatalf("gps settings = %+v, want %+v", gotGPS, gpsReq)
	}
}

func TestRestoreSavedSettingsSkipsClearedSettings(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsStore.SaveGPS(model.GPSSessionRequest{}); err != nil {
		t.Fatalf("SaveGPS() error = %v", err)
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, settingsStore, Options{})

	openCount := 0
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		openCount++
		return newFakeSerialPort(), nil
	})

	svc.RestoreSavedSettings("zh-CN")

	if openCount != 0 {
		t.Fatalf("open count = %d, want 0", openCount)
	}
	if current := svc.Current("zh-CN"); current.Active {
		t.Fatalf("restored session active = true, want inactive: %+v", current)
	}
}

func assertPortWrites(t *testing.T, port *fakeSerialPort, want ...string) {
	t.Helper()
	if port == nil {
		t.Fatal("port is nil")
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if len(port.writes) != len(want) {
		t.Fatalf("writes = %v, want %v", port.writes, want)
	}
	for i, got := range port.writes {
		if got != want[i] {
			t.Fatalf("write[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func assertCloseCount(t *testing.T, port *fakeSerialPort, want int) {
	t.Helper()
	if port == nil {
		t.Fatal("port is nil")
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.closeCount != want {
		t.Fatalf("close count = %d, want %d", port.closeCount, want)
	}
}

func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.000001 {
		t.Fatalf("value = %.10f, want %.10f", got, want)
	}
}
