package detection

import (
	"context"
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
	"tri-detector/parser"
)

func TestIngestLineStoresParsedAndDetectionRecords(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
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
	if records[0].Kind != "detect" {
		t.Fatalf("detection kind = %q, want detect", records[0].Kind)
	}
	if records[0].Frequency != 5865 {
		t.Fatalf("frequency = %v, want 5865", records[0].Frequency)
	}
}

func TestIngestLineStoresScreenPositionFromRID(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "RID ssid=RID-1581ABC, serial=1581ABC, model=DJI Mini 4 Pro, UA_type=2, drone_GPS=31.200000,121.400000, pilot_GPS=31.210000,121.410000, speed=12.5, Vspeed=0, direc=90, AltitudeP=20.0, AltitudeG=110.0, Height_AGL=35.5, MAC=60:60:1f:38:98:b9, rssi=-82, freq=2437")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "1581ABC" {
		t.Fatalf("serial = %q, want 1581ABC", items[0].Serial)
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 31.2 || items[0].Drone.Longitude != 121.4 {
		t.Fatalf("unexpected drone point: %#v", items[0].Drone)
	}
	if items[0].Pilot == nil {
		t.Fatalf("expected pilot point")
	}
	if records := svc.Records(10); len(records) != 0 {
		t.Fatalf("detection records count = %d, want 0 for RID", len(records))
	}
}

func TestIngestLineStoresScreenPositionFromRIDWithoutCoordinates(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "RID ssid=RID-1581F6Z9C2412003L1W8, serial=1581F6Z9C2412003L1W8, model=DJI, UA_type=2, drone_GPS=0,0, pilot_GPS=0,0, speed=0, Vspeed=0, direc=361, AltitudeP=93.5, AltitudeG=-1000, Height_AGL=0, MAC=60:60:1f:38:98:b9, rssi=-72, freq=2437")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "1581F6Z9C2412003L1W8" {
		t.Fatalf("serial = %q, want RID serial", items[0].Serial)
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 0 || items[0].Drone.Longitude != 0 {
		t.Fatalf("expected zero drone point, got %#v", items[0].Drone)
	}
	if items[0].Pilot == nil || items[0].Pilot.Latitude != 0 || items[0].Pilot.Longitude != 0 {
		t.Fatalf("expected zero pilot point, got %#v", items[0].Pilot)
	}
}

func TestIngestLineStoresScreenPositionFromDIDPlain(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "num=672/3/1, device=10125, serial=0M6CH6AR0A100L, model=41-Mavic 2, uuid=176344372408408473, drone_GPS=31.200000,121.400000, home_GPS=31.190000,121.390000, pilot_GPS=31.210000,121.410000, Height=50, Altitude=110.0,EastV=3.0, NothV=4.0,UpV=0.0, freq=5796.5, rssi=-78, distance=0.0km,")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Source != string(parser.TypeDIDPlain) {
		t.Fatalf("source = %q, want did_plain", items[0].Source)
	}
	if items[0].Speed == nil || *items[0].Speed != 5 {
		t.Fatalf("speed = %#v, want 5", items[0].Speed)
	}
	if records := svc.Records(10); len(records) != 0 {
		t.Fatalf("detection records count = %d, want 0 for DID plain", len(records))
	}
}

func TestIngestLineStoresFallbackScreenPositionFromDIDEncryptedWithoutDecoder(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Serial != "875bb45f" {
		t.Fatalf("serial = %q, want encrypted id", items[0].Serial)
	}
	if items[0].Model != "DJI-Drone" {
		t.Fatalf("model = %q, want DJI-Drone", items[0].Model)
	}
	if items[0].CorrelationID != "did_encrypted:875bb45f" {
		t.Fatalf("correlation id = %q, want did_encrypted:875bb45f", items[0].CorrelationID)
	}
	if items[0].Cracked {
		t.Fatalf("fallback target should not be marked cracked")
	}
	if items[0].Drone != nil || items[0].Pilot != nil || items[0].Home != nil {
		t.Fatalf("fallback target should not invent coordinates, got drone=%#v pilot=%#v home=%#v", items[0].Drone, items[0].Pilot, items[0].Home)
	}
}

func TestIngestLineStoresScreenPositionFromDIDEncryptedDecoder(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})
	svc.SetO3PlusO4Decoder(fakeO3Decoder{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9")

	var items []model.ScreenPositionTarget
	waitUntil(t, time.Second, func() bool {
		items = svc.ScreenPositions(10)
		return len(items) == 1 && items[0].Serial == "o3-sn"
	})
	if items[0].Serial != "o3-sn" {
		t.Fatalf("serial = %q, want o3-sn", items[0].Serial)
	}
	if items[0].Model != "DJI O4" {
		t.Fatalf("model = %q, want DJI O4", items[0].Model)
	}
	if items[0].CorrelationID != "did_encrypted:875bb45f" {
		t.Fatalf("correlation id = %q, want did_encrypted:875bb45f", items[0].CorrelationID)
	}
	if !items[0].Cracked {
		t.Fatalf("expected decoded target to be marked cracked")
	}
	if items[0].HitCount != 2 {
		t.Fatalf("hit count = %d, want fallback plus decoded updates", items[0].HitCount)
	}
	if items[0].LastRecord.Device != "10125" {
		t.Fatalf("last record device = %q, want 10125", items[0].LastRecord.Device)
	}
	if records := svc.Records(10); len(records) != 0 {
		t.Fatalf("detection records count = %d, want 0 for DID encrypted", len(records))
	}
}

func TestO3DecryptResultKeepsZeroCoordinates(t *testing.T) {
	decoder := &mqttO3PlusO4Decoder{}
	receivedAt := time.Now()

	target, ok := decoder.positionFromDecryptResult(parser.DIDEncrypted{
		Device:      "4745",
		EncryptedID: "86ca8046",
		Freq:        5776.5,
		RSSI:        -76,
	}, o3DecryptAlert{
		SN:       "o3-sn",
		Model:    "DJI O3",
		Lat:      0,
		Lon:      0,
		PilotLat: 0,
		PilotLon: 0,
		HomeLat:  0,
		HomeLon:  0,
	}, receivedAt)

	if !ok {
		t.Fatalf("expected zero coordinates to produce a screen position target")
	}
	if target.Drone == nil || target.Drone.Latitude != 0 || target.Drone.Longitude != 0 {
		t.Fatalf("expected zero drone point, got %#v", target.Drone)
	}
	if target.Pilot == nil || target.Pilot.Latitude != 0 || target.Pilot.Longitude != 0 {
		t.Fatalf("expected zero pilot point, got %#v", target.Pilot)
	}
	if target.Home == nil || target.Home.Latitude != 0 || target.Home.Longitude != 0 {
		t.Fatalf("expected zero home point, got %#v", target.Home)
	}
}

type fakeSerialPort struct {
	closeCount int
	closeCh    chan struct{}
	writes     []string
}

type fakeO3Decoder struct{}

func (fakeO3Decoder) ParseO3PlusO4PacketMQTT(_ context.Context, packet parser.DIDEncrypted, deviceSN string, receivedAt time.Time) (model.ScreenPositionTarget, bool) {
	if deviceSN != "10125" {
		return model.ScreenPositionTarget{}, false
	}
	return model.ScreenPositionTarget{
		Serial:    "o3-sn",
		Model:     "DJI O4",
		Source:    string(parser.TypeDIDEncrypted),
		Frequency: packet.Freq,
		RSSI:      packet.RSSI,
		Device:    packet.Device,
		Drone:     &model.ScreenPositionPoint{Latitude: 31.2, Longitude: 121.4},
		Cracked:   true,
		FirstSeen: receivedAt,
		LastSeen:  receivedAt,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       string(parser.TypeDIDEncrypted),
			ReceivedAt: receivedAt,
			Device:     packet.Device,
			Serial:     "o3-sn",
			Model:      "DJI O4",
			Frequency:  packet.Freq,
			RSSI:       packet.RSSI,
			Cracked:    true,
		},
	}, true
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !condition() {
		t.Fatalf("condition not met within %s", timeout)
	}
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
	st := store.NewMemoryStore(10, 10)
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
	if got := opened["/dev/rx"].writes; len(got) != 0 {
		t.Fatalf("rx writes = %v, want none", got)
	}
	assertPortWrites(t, opened["/dev/tx"], startDetectionCommand+"\n")

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
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	openCount := 0
	var opened *fakeSerialPort
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		openCount++
		opened = newFakeSerialPort()
		return opened, nil
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
	current := svc.Current("zh-CN")
	if !current.Active {
		t.Fatal("expected legacy session to be active")
	}
	assertPortWrites(t, opened, startDetectionCommand+"\n")

	_ = svc.Stop("zh-CN")
}

func TestStartSessionSendsStartCommandAfterSwitchingTxPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string][]*fakeSerialPort{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		opened[cfg.PortName] = append(opened[cfg.PortName], port)
		return port, nil
	})

	_, err = svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx1",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() first error = %v", err)
	}

	_, err = svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx2",
		BaudRate:   115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() second error = %v", err)
	}

	if got := len(opened["/dev/tx1"]); got != 1 {
		t.Fatalf("tx1 open count = %d, want 1", got)
	}
	if got := len(opened["/dev/tx2"]); got != 1 {
		t.Fatalf("tx2 open count = %d, want 1", got)
	}
	assertPortWrites(t, opened["/dev/tx1"][0], startDetectionCommand+"\n")
	assertPortWrites(t, opened["/dev/tx2"][0], startDetectionCommand+"\n")

	current := svc.Current("zh-CN")
	if current.TxPortName != "/dev/tx2" {
		t.Fatalf("current tx port = %q, want /dev/tx2", current.TxPortName)
	}

	_ = svc.Stop("zh-CN")
}

func TestRestoreSavedSettingsAutoConnectsOnStartup(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	st := store.NewMemoryStore(10, 10)
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

	st := store.NewMemoryStore(10, 10)
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
	assertPortWrites(t, firstPort, startDetectionCommand+"\n")
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
	mu.Lock()
	reconnectedPort := ports[openCount-1]
	mu.Unlock()
	assertPortWrites(t, reconnectedPort, startDetectionCommand+"\n")

	_ = svc.Stop("zh-CN")
}

func TestIngestLineStoresHeartbeatAsParsedOnly(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=84, device=10125, Heart Beat, 879,  0")

	if got := len(svc.Parsed(10)); got != 1 {
		t.Fatalf("parsed count = %d, want 1", got)
	}
	records := svc.Records(10)
	if got := len(records); got != 0 {
		t.Fatalf("detection count = %d, want 0", got)
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

func assertPortWrites(t *testing.T, port *fakeSerialPort, want ...string) {
	t.Helper()
	if port == nil {
		t.Fatal("port is nil")
	}
	if len(port.writes) != len(want) {
		t.Fatalf("writes = %v, want %v", port.writes, want)
	}
	for i, got := range port.writes {
		if got != want[i] {
			t.Fatalf("write[%d] = %q, want %q", i, got, want[i])
		}
	}
}
