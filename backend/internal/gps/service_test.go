package gps

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
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
	writeCount int
	writeErrAt int
	writeErr   error
	onWrite    func(string)
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
	p.writeCount++
	if p.writeCount == p.writeErrAt && p.writeErr != nil {
		p.mu.Unlock()
		return 0, p.writeErr
	}
	command := string(b)
	p.writes = append(p.writes, command)
	onWrite := p.onWrite
	p.mu.Unlock()
	if onWrite != nil {
		onWrite(command)
	}
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

func TestStartSendsGPSStartupCommandsToControlPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})
	var delays []time.Duration
	svc.waitStartupDelay = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return true
	}

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
	assertPortWrites(
		t,
		opened["/dev/ttyUSB2"],
		startGPSCommand,
		startGPSDataCommand,
		startGPSPowerCommand,
	)
	if want := []time.Duration{startGPSDataCommandDelay, startGPSPowerCommandDelay}; !slices.Equal(delays, want) {
		t.Fatalf("command delays = %v, want %v", delays, want)
	}
	assertCloseCount(t, opened["/dev/ttyUSB2"], 1)
	assertCloseCount(t, opened["/dev/ttyUSB1"], 0)

	_ = svc.Stop("zh-CN")
	assertCloseCount(t, opened["/dev/ttyUSB1"], 1)
}

func TestStartSendsGPSStartupCommandsOnSharedGPSPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})
	var delays []time.Duration
	svc.waitStartupDelay = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return true
	}

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
	assertPortWrites(
		t,
		opened["/dev/ttyUSB0"],
		startGPSCommand,
		startGPSDataCommand,
		startGPSPowerCommand,
	)
	if want := []time.Duration{startGPSDataCommandDelay, startGPSPowerCommandDelay}; !slices.Equal(delays, want) {
		t.Fatalf("command delays = %v, want %v", delays, want)
	}
	assertCloseCount(t, opened["/dev/ttyUSB0"], 0)

	_ = svc.Stop("zh-CN")
	assertCloseCount(t, opened["/dev/ttyUSB0"], 1)
}

func TestStopCancelsGPSStartupCommandSequence(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	svc := NewService(
		store.NewMemoryStore(10, 10),
		tr,
		settings.NewStore(filepath.Join(t.TempDir(), "settings.json")),
		Options{},
	)
	waitStarted := make(chan struct{})
	var waitStartedOnce sync.Once
	svc.waitStartupDelay = func(ctx context.Context, _ time.Duration) bool {
		waitStartedOnce.Do(func() {
			close(waitStarted)
		})
		<-ctx.Done()
		return false
	}
	port := newFakeSerialPort()
	svc.SetSerialOpener(func(_ *serialport.Config) (serial.Port, error) {
		return port, nil
	})

	type startResult struct {
		response model.GPSSessionResponse
		err      error
	}
	startDone := make(chan startResult, 1)
	go func() {
		response, startErr := svc.Start(model.GPSSessionRequest{
			DataPortName:    "/dev/ttyUSB0",
			ControlPortName: "/dev/ttyUSB0",
			BaudRate:        115200,
			DataBits:        8,
			StopBits:        1,
			Parity:          "none",
		}, "zh-CN")
		startDone <- startResult{response: response, err: startErr}
	}()

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for GPS startup delay")
	}
	_ = svc.Stop("zh-CN")
	select {
	case result := <-startDone:
		if result.err != nil {
			t.Fatalf("Start() error = %v", result.err)
		}
		if result.response.State != "inactive" || result.response.Active {
			t.Fatalf("Start() response after cancellation = %+v, want inactive", result.response)
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after GPS session cancellation")
	}

	assertPortWrites(t, port, startGPSCommand)
	assertCloseCount(t, port, 1)
}

func TestStartSendsGPSStartupCommandsOnlyWhenControlPortChanges(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	svc := NewService(
		store.NewMemoryStore(10, 10),
		tr,
		settings.NewStore(filepath.Join(t.TempDir(), "settings.json")),
		Options{},
	)
	svc.waitStartupDelay = func(_ context.Context, _ time.Duration) bool {
		return true
	}

	var openedMu sync.Mutex
	opened := map[string][]*fakeSerialPort{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		openedMu.Lock()
		opened[cfg.PortName] = append(opened[cfg.PortName], port)
		openedMu.Unlock()
		return port, nil
	})

	first := model.GPSSessionRequest{
		DataPortName:    "/dev/ttyUSB1",
		ControlPortName: "/dev/ttyUSB2",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		Parity:          "none",
	}
	if _, err := svc.Start(first, "zh-CN"); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	sameControlPort := first
	sameControlPort.DataPortName = "/dev/ttyUSB3"
	if _, err := svc.Start(sameControlPort, "zh-CN"); err != nil {
		t.Fatalf("same-control Start() error = %v", err)
	}

	newControlPort := sameControlPort
	newControlPort.ControlPortName = "/dev/ttyUSB4"
	if _, err := svc.Start(newControlPort, "zh-CN"); err != nil {
		t.Fatalf("new-control Start() error = %v", err)
	}

	openedMu.Lock()
	controlPorts := append([]*fakeSerialPort(nil), opened["/dev/ttyUSB2"]...)
	newControlPorts := append([]*fakeSerialPort(nil), opened["/dev/ttyUSB4"]...)
	dataPorts := append([]*fakeSerialPort(nil), opened["/dev/ttyUSB3"]...)
	openedMu.Unlock()
	if len(controlPorts) != 1 {
		t.Fatalf("original control port open count = %d, want 1", len(controlPorts))
	}
	if len(newControlPorts) != 1 {
		t.Fatalf("new control port open count = %d, want 1", len(newControlPorts))
	}
	assertPortWrites(t, controlPorts[0], startGPSCommand, startGPSDataCommand, startGPSPowerCommand)
	assertPortWrites(t, newControlPorts[0], startGPSCommand, startGPSDataCommand, startGPSPowerCommand)
	for _, port := range dataPorts {
		assertPortWrites(t, port)
	}

	_ = svc.Stop("zh-CN")
}

func TestReconnectDoesNotResendGPSStartupCommandsAfterSuccessfulInitialization(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	svc := NewService(
		store.NewMemoryStore(10, 10),
		tr,
		settings.NewStore(filepath.Join(t.TempDir(), "settings.json")),
		Options{ReconnectInitialDelay: time.Millisecond, ReconnectMaxDelay: time.Millisecond},
	)
	svc.waitStartupDelay = func(_ context.Context, _ time.Duration) bool {
		return true
	}

	var openerMu sync.Mutex
	var controlPorts []*fakeSerialPort
	dataOpenCount := 0
	reconnected := make(chan *fakeSerialPort, 1)
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		switch cfg.PortName {
		case "/dev/ttyUSB2":
			port := newFakeSerialPort()
			openerMu.Lock()
			controlPorts = append(controlPorts, port)
			openerMu.Unlock()
			return port, nil
		case "/dev/ttyUSB1":
			openerMu.Lock()
			dataOpenCount++
			attempt := dataOpenCount
			openerMu.Unlock()
			if attempt == 1 {
				return nil, errors.New("data port unavailable")
			}
			port := newFakeSerialPort()
			reconnected <- port
			return port, nil
		default:
			return nil, errors.New("unexpected port")
		}
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
	if resp.Active {
		t.Fatal("expected initial data-port connection to fail")
	}

	select {
	case <-reconnected:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for data-port reconnect")
	}
	openerMu.Lock()
	gotControlPorts := append([]*fakeSerialPort(nil), controlPorts...)
	gotDataOpenCount := dataOpenCount
	openerMu.Unlock()
	if len(gotControlPorts) != 1 {
		t.Fatalf("control port open count = %d, want 1", len(gotControlPorts))
	}
	if gotDataOpenCount != 2 {
		t.Fatalf("data port open count = %d, want 2", gotDataOpenCount)
	}
	assertPortWrites(t, gotControlPorts[0], startGPSCommand, startGPSDataCommand, startGPSPowerCommand)

	_ = svc.Stop("zh-CN")
}

func TestSendGPSStartCommandsFollowsConfiguredSequence(t *testing.T) {
	port := newFakeSerialPort()
	events := []string{}
	port.onWrite = func(command string) {
		events = append(events, "write:"+command)
	}

	err := sendGPSStartCommands(context.Background(), port, func(_ context.Context, delay time.Duration) bool {
		events = append(events, "sleep:"+delay.String())
		return true
	})
	if err != nil {
		t.Fatalf("sendGPSStartCommands() error = %v", err)
	}

	want := []string{
		"write:AT+QGPS=1\r\n",
		"sleep:2s",
		"write:AT+QNETDEVCTL=1,1,1\r\n",
		"sleep:3s",
		"write:at+qgpspower=1\r\n",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("startup sequence = %q, want %q", events, want)
	}
}

func TestSendGPSStartCommandsStopsAfterWriteFailure(t *testing.T) {
	port := newFakeSerialPort()
	writeErr := errors.New("write failed")
	port.writeErrAt = 2
	port.writeErr = writeErr
	delays := []time.Duration{}

	err := sendGPSStartCommands(context.Background(), port, func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return true
	})
	if !errors.Is(err, writeErr) {
		t.Fatalf("sendGPSStartCommands() error = %v, want %v", err, writeErr)
	}
	if !strings.Contains(err.Error(), strings.TrimSpace(startGPSDataCommand)) {
		t.Fatalf("sendGPSStartCommands() error = %q, want failed command", err)
	}
	assertPortWrites(t, port, startGPSCommand)
	if want := []time.Duration{startGPSDataCommandDelay}; !slices.Equal(delays, want) {
		t.Fatalf("command delays = %v, want %v", delays, want)
	}
}

func TestSendGPSStartCommandsStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := newFakeSerialPort()
	port.onWrite = func(command string) {
		if command == startGPSCommand {
			cancel()
		}
	}

	err := sendGPSStartCommands(ctx, port, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendGPSStartCommands() error = %v, want context cancellation", err)
	}
	assertPortWrites(t, port, startGPSCommand)
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
