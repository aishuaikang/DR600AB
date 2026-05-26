package compass

import (
	"bytes"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/settings"
	"dr600ab-api/internal/store"
	"serialport"
)

type fakeSerialPort struct {
	mu         sync.Mutex
	readCh     chan []byte
	closeCh    chan struct{}
	closeOnce  sync.Once
	pending    []byte
	writes     [][]byte
	onWrite    func([]byte) []byte
	closeCount int
}

func newFakeSerialPort() *fakeSerialPort {
	return &fakeSerialPort{
		readCh:  make(chan []byte, 32),
		closeCh: make(chan struct{}),
	}
}

func (p *fakeSerialPort) Read(b []byte) (int, error) {
	for {
		p.mu.Lock()
		if len(p.pending) > 0 {
			n := copy(b, p.pending)
			p.pending = p.pending[n:]
			p.mu.Unlock()
			return n, nil
		}
		p.mu.Unlock()

		select {
		case data := <-p.readCh:
			p.mu.Lock()
			p.pending = append(p.pending, data...)
			p.mu.Unlock()
		case <-p.closeCh:
			return 0, io.EOF
		}
	}
}

func (p *fakeSerialPort) Write(b []byte) (int, error) {
	data := append([]byte(nil), b...)
	p.mu.Lock()
	p.writes = append(p.writes, data)
	onWrite := p.onWrite
	p.mu.Unlock()

	if onWrite != nil {
		if response := onWrite(data); len(response) > 0 {
			p.send(response)
		}
	}
	return len(b), nil
}

func (p *fakeSerialPort) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closeCount++
		p.mu.Unlock()
		close(p.closeCh)
	})
	return nil
}

func (p *fakeSerialPort) SetReadTimeout(time.Duration) error { return nil }

func (p *fakeSerialPort) send(data []byte) {
	select {
	case p.readCh <- append([]byte(nil), data...):
	case <-p.closeCh:
	}
}

func (p *fakeSerialPort) snapshotWrites() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.writes))
	for i := range p.writes {
		out[i] = append([]byte(nil), p.writes[i]...)
	}
	return out
}

func (p *fakeSerialPort) closedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeCount
}

func TestStartConfiguresAutoOutputAndIngestsAutoFrame(t *testing.T) {
	svc := newTestService(t)
	events, unsubscribe := svc.store.Subscribe(16)
	defer unsubscribe()

	port := newFakeSerialPort()
	port.onWrite = func(data []byte) []byte {
		want, err := buildSetAutoOutputRateCmd(defaultDeviceAddr, autoOutputRate25Hz)
		if err != nil {
			t.Fatalf("buildSetAutoOutputRateCmd() error = %v", err)
		}
		if bytes.Equal(data, want) {
			return buildSetAutoOutputRateResponse(defaultDeviceAddr, true)
		}
		return nil
	}
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
		return port, nil
	})

	resp, err := svc.Start(model.CompassSessionRequest{PortName: "/dev/ttyUSB3"}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !resp.Active {
		t.Fatalf("response active = false, want true: %+v", resp)
	}

	raw, err := buildPitchRollHeadingResponse(defaultDeviceAddr, -1.54, 14.43, 15.20)
	if err != nil {
		t.Fatalf("buildPitchRollHeadingResponse() error = %v", err)
	}
	port.send(raw)

	waitEventually(t, func() bool {
		return len(svc.Records(10)) == 1
	})
	records := svc.Records(10)
	if records[0].Pitch != -1.54 || records[0].Roll != 14.43 || records[0].Heading != 15.20 {
		t.Fatalf("record = %+v, want parsed angles", records[0])
	}
	current := svc.Current("zh-CN")
	if current.LastHeading == nil || *current.LastHeading != 15.20 || !current.AutoOutput {
		t.Fatalf("current = %+v, want last heading and auto output", current)
	}
	sawRecord := false
	waitEventually(t, func() bool {
		sawCompassRecord(events, 15.20, &sawRecord)
		return sawRecord
	})

	_ = svc.Stop("zh-CN")
}

func TestStartUsesCompassDefaultBaudRate(t *testing.T) {
	svc := newTestService(t)
	port := newFakeSerialPort()
	var gotBaudRate int
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
		gotBaudRate = cfg.BaudRate
		return port, nil
	})

	if _, err := svc.Start(model.CompassSessionRequest{PortName: "/dev/ttyUSB3"}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if gotBaudRate != DefaultBaudRate {
		t.Fatalf("baud rate = %d, want %d", gotBaudRate, DefaultBaudRate)
	}

	_ = svc.Stop("zh-CN")
}

func TestStartOverridesLegacyCompassBaudRate(t *testing.T) {
	svc := newTestService(t)
	port := newFakeSerialPort()
	var gotBaudRate int
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
		gotBaudRate = cfg.BaudRate
		return port, nil
	})

	if _, err := svc.Start(model.CompassSessionRequest{PortName: "/dev/ttyUSB3", BaudRate: 115200}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if gotBaudRate != DefaultBaudRate {
		t.Fatalf("baud rate = %d, want %d", gotBaudRate, DefaultBaudRate)
	}

	settings, ok, err := svc.Settings()
	if err != nil {
		t.Fatalf("Settings() error = %v", err)
	}
	if !ok || settings.BaudRate != DefaultBaudRate {
		t.Fatalf("settings = %+v, ok = %v, want persisted %d", settings, ok, DefaultBaudRate)
	}

	_ = svc.Stop("zh-CN")
}

func TestStartFallsBackToPollingWhenSetRateFails(t *testing.T) {
	svc := newTestService(t)
	port := newFakeSerialPort()
	port.onWrite = func(data []byte) []byte {
		setRate, err := buildSetAutoOutputRateCmd(defaultDeviceAddr, autoOutputRate25Hz)
		if err != nil {
			t.Fatalf("buildSetAutoOutputRateCmd() error = %v", err)
		}
		if bytes.Equal(data, setRate) {
			return buildSetAutoOutputRateResponse(defaultDeviceAddr, false)
		}
		if bytes.Equal(data, buildReadPitchRollHeadingCmd(defaultDeviceAddr)) {
			raw, err := buildPitchRollHeadingResponse(defaultDeviceAddr, 1, 2, 33.25)
			if err != nil {
				t.Fatalf("buildPitchRollHeadingResponse() error = %v", err)
			}
			return raw
		}
		return nil
	}
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
		return port, nil
	})

	if _, err := svc.Start(model.CompassSessionRequest{PortName: "/dev/ttyUSB3"}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitEventually(t, func() bool {
		return len(svc.Records(10)) > 0 && hasWrite(port.snapshotWrites(), buildReadPitchRollHeadingCmd(defaultDeviceAddr))
	})
	current := svc.Current("zh-CN")
	if current.AutoOutput {
		t.Fatalf("auto output = true, want fallback polling")
	}
	if current.LastHeading == nil || *current.LastHeading != 33.25 {
		t.Fatalf("current = %+v, want polled heading", current)
	}

	_ = svc.Stop("zh-CN")
}

func TestStopClosesActivePort(t *testing.T) {
	svc := newTestService(t)
	port := newFakeSerialPort()
	port.onWrite = func(data []byte) []byte {
		return buildSetAutoOutputRateResponse(defaultDeviceAddr, true)
	}
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
		return port, nil
	})

	if _, err := svc.Start(model.CompassSessionRequest{PortName: "/dev/ttyUSB3"}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_ = svc.Stop("zh-CN")

	if port.closedCount() == 0 {
		t.Fatal("close count = 0, want active port closed")
	}
}

func TestRestoreSavedSettingsSkipsClearedSettings(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsStore.SaveCompass(model.CompassSessionRequest{}); err != nil {
		t.Fatalf("SaveCompass() error = %v", err)
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, settingsStore, Options{})

	openCount := 0
	svc.SetSerialOpener(func(cfg *serialport.Config) (SerialPort, error) {
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

func newTestService(t *testing.T) *Service {
	t.Helper()
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	return NewService(
		store.NewMemoryStore(10, 10),
		tr,
		settings.NewStore(filepath.Join(t.TempDir(), "settings.json")),
		Options{
			QueryTimeout:          50 * time.Millisecond,
			ReconnectInitialDelay: time.Hour,
			ReconnectMaxDelay:     time.Hour,
		},
	)
}

func hasWrite(writes [][]byte, want []byte) bool {
	for _, got := range writes {
		if bytes.Equal(got, want) {
			return true
		}
	}
	return false
}

func waitEventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func sawCompassRecord(
	events <-chan model.Event,
	heading float64,
	sawRecord *bool,
) {
	for {
		select {
		case evt := <-events:
			if evt.Type == "compass.record" {
				record, ok := evt.Payload.(model.CompassRecord)
				if ok && record.Heading == heading {
					*sawRecord = true
				}
			}
		default:
			return
		}
	}
}
