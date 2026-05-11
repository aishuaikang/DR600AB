package detection

import (
	"io"
	"path/filepath"
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

func TestIngestLineStoresDetectionAndFPVRecords(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "device=10125, model=PAL Analog, freq=5865.0, rssi=-56.9")

	parsed := svc.Parsed(10)
	if len(parsed) != 1 {
		t.Fatalf("parsed count = %d, want 1", len(parsed))
	}
	if got := parsed[0].Type; got != "detect" {
		t.Fatalf("parsed type = %q, want detect", got)
	}

	records := svc.Records(10)
	if len(records) != 1 {
		t.Fatalf("detection count = %d, want 1", len(records))
	}
	if !records[0].IsFPV {
		t.Fatal("expected detection record to be classified as FPV")
	}
	if records[0].FPVBand != "5.8" {
		t.Fatalf("fpv band = %q, want 5.8", records[0].FPVBand)
	}

	fpv := svc.FPV(10)
	if len(fpv) != 1 {
		t.Fatalf("fpv count = %d, want 1", len(fpv))
	}
	if fpv[0].Band != "5.8" {
		t.Fatalf("fpv band = %q, want 5.8", fpv[0].Band)
	}
}

type fakeSerialPort struct {
	closeCount int
	closeCh    chan struct{}
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
	p.closeCount++
	select {
	case <-p.closeCh:
	default:
		close(p.closeCh)
	}
	return nil
}

func (p *fakeSerialPort) Break(duration time.Duration) error { return nil }

func TestStartSessionSupportsSeparateReceiveAndSendPorts(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string]*fakeSerialPort{}
	svc.SetPortLister(func() ([]string, error) {
		return []string{"/dev/rx", "/dev/tx", "/dev/other"}, nil
	})
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		opened[cfg.PortName] = port
		return port, nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		BaudRate:   57600,
		DataBits:   7,
		StopBits:   2,
		Parity:     "even",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !resp.Active {
		t.Fatal("expected session to be active")
	}
	if resp.State != "connected" {
		t.Fatalf("state = %q, want connected", resp.State)
	}
	if resp.RxPortName != "/dev/rx" {
		t.Fatalf("rx port = %q, want /dev/rx", resp.RxPortName)
	}
	if resp.TxPortName != "/dev/tx" {
		t.Fatalf("tx port = %q, want /dev/tx", resp.TxPortName)
	}
	if resp.PortName != "/dev/rx" {
		t.Fatalf("port name = %q, want /dev/rx", resp.PortName)
	}
	if len(opened) != 2 {
		t.Fatalf("opened ports = %d, want 2", len(opened))
	}

	ports, err := svc.ListPorts()
	if err != nil {
		t.Fatalf("ListPorts() error = %v", err)
	}
	if !ports[0].Active || !ports[1].Active {
		t.Fatalf("expected rx and tx ports to be active, got %+v", ports)
	}

	stopped := svc.Stop("zh-CN")
	if stopped.Active {
		t.Fatal("expected stopped session to be inactive")
	}
	if stopped.RxPortName != "/dev/rx" || stopped.TxPortName != "/dev/tx" {
		t.Fatalf("unexpected stopped response: %+v", stopped)
	}
}

func TestStartSessionFallsBackToLegacyPortName(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	openCount := 0
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		openCount++
		return newFakeSerialPort(), nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		PortName: "/dev/legacy",
		BaudRate: 115200,
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if openCount != 1 {
		t.Fatalf("open count = %d, want 1", openCount)
	}
	if resp.RxPortName != "/dev/legacy" {
		t.Fatalf("rx port = %q, want /dev/legacy", resp.RxPortName)
	}
	if resp.TxPortName != "/dev/legacy" {
		t.Fatalf("tx port = %q, want /dev/legacy", resp.TxPortName)
	}

	_ = svc.Stop("zh-CN")
}

func TestRestoreSavedSettingsAutoConnectsOnStartup(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	st := store.NewMemoryStore(10, 10, 10)
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	req := model.DetectionSessionRequest{
		RxPortName:  "/dev/rx",
		TxPortName:  "/dev/tx",
		BaudRate:    115200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "none",
		AutoConnect: true,
	}
	if err := settingsStore.Save(req); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := NewService(st, tr, settingsStore, Options{
		ReconnectInitialDelay: 10 * time.Millisecond,
		ReconnectMaxDelay:     20 * time.Millisecond,
	})

	openCount := 0
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		openCount++
		return newFakeSerialPort(), nil
	})

	svc.RestoreSavedSettings("zh-CN")

	waitForCondition(t, 2*time.Second, func() bool {
		return svc.Current("zh-CN").Active
	})

	if openCount != 2 {
		t.Fatalf("open count = %d, want 2", openCount)
	}

	current := svc.Current("zh-CN")
	if !current.Active {
		t.Fatal("expected restored session to be active")
	}
	if current.RxPortName != "/dev/rx" || current.TxPortName != "/dev/tx" {
		t.Fatalf("unexpected restored session: %+v", current)
	}

	_ = svc.Stop("zh-CN")
}

func TestReconnectsAfterPortClosesAutomatically(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	st := store.NewMemoryStore(10, 10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{
		ReconnectInitialDelay: 10 * time.Millisecond,
		ReconnectMaxDelay:     20 * time.Millisecond,
	})

	var mu sync.Mutex
	var ports []*fakeSerialPort
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		mu.Lock()
		ports = append(ports, port)
		mu.Unlock()
		return port, nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/rx",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !resp.Active {
		t.Fatal("expected session to be active")
	}

	mu.Lock()
	if len(ports) != 1 {
		count := len(ports)
		mu.Unlock()
		t.Fatalf("expected 1 opened port, got %d", count)
	}
	firstPort := ports[0]
	mu.Unlock()
	firstPort.Close()

	waitForCondition(t, 2*time.Second, func() bool {
		mu.Lock()
		count := len(ports)
		mu.Unlock()
		return count >= 2 && svc.Current("zh-CN").Active
	})

	current := svc.Current("zh-CN")
	if !current.Active {
		t.Fatal("expected session to reconnect")
	}
	if current.State != "connected" {
		t.Fatalf("state = %q, want connected", current.State)
	}
	mu.Lock()
	openCount := len(ports)
	mu.Unlock()
	if openCount < 2 {
		t.Fatalf("reconnect opened %d ports, want at least 2", openCount)
	}

	_ = svc.Stop("zh-CN")
}

func TestIngestLineKeepsHeartbeatInParsedRecordsOnly(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=84, device=10125, Heart Beat, 879,  0")

	if got := len(svc.Parsed(10)); got != 1 {
		t.Fatalf("parsed count = %d, want 1", got)
	}
	if got := len(svc.Records(10)); got != 0 {
		t.Fatalf("detection count = %d, want 0", got)
	}
	if got := len(svc.FPV(10)); got != 0 {
		t.Fatalf("fpv count = %d, want 0", got)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
