package detection

import (
	"context"
	"errors"
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
	"uav-protocol/diddecrypt"
	"uav-protocol/parser"
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
	if records[0].DisplayModel != "Analog PAL" {
		t.Fatalf("display model = %q, want Analog PAL", records[0].DisplayModel)
	}
}

func TestIngestLineStoresScreenPositionFromRID(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "RID ssid=RID-1581ABC, serial=1581ABC, model=DJI Mini 4 Pro, UA_type=2, drone_GPS=121.400000,31.200000, pilot_GPS=121.410000,31.210000, speed=12.5, Vspeed=0, direc=90, AltitudeP=20.0, AltitudeG=110.0, Height_AGL=35.5, MAC=60:60:1f:38:98:b9, rssi=-82, freq=2437")

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

func TestIngestLineStoresScreenPositionFromRIDWithZeroCoordinates(t *testing.T) {
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
		t.Fatalf("expected zero drone point to be retained for display, got %#v", items[0].Drone)
	}
	if items[0].Pilot == nil || items[0].Pilot.Latitude != 0 || items[0].Pilot.Longitude != 0 {
		t.Fatalf("expected zero pilot point to be retained for display, got %#v", items[0].Pilot)
	}
}

func TestIngestLineStoresScreenPositionFromDIDPlain(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "num=672/3/1, device=10125, serial=0M6CH6AR0A100L, model=41-Mavic 2, uuid=176344372408408473, drone_GPS=121.400000,31.200000, home_GPS=121.390000,31.190000, pilot_GPS=121.410000,31.210000, Height=50, Altitude=110.0,EastV=3.0, NothV=4.0,UpV=0.0, freq=5796.5, rssi=-78, distance=0.0km,")

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

func TestIngestLineStoresDIDPlainLongitudeLatitudeGPS(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "num=672/3/1, device=4745, serial=3YTBL320040274, model=66-Air 2S, uuid=186158855762255052, drone_GPS=117.008616,28.192898, home_GPS=117.008255,28.192434, pilot_GPS=117.008450,28.192692, Height=0, Altitude=46,EastV=0,NothV=0,UpV=0, freq=2414.5, rssi=-80, distance=0.0km,")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want 1", len(items))
	}
	if items[0].Model != "Air 2S" {
		t.Fatalf("model = %q, want Air 2S", items[0].Model)
	}
	if items[0].Drone == nil || items[0].Drone.Latitude != 28.192898 || items[0].Drone.Longitude != 117.008616 {
		t.Fatalf("unexpected drone point: %#v", items[0].Drone)
	}
	if items[0].Home == nil || items[0].Home.Latitude != 28.192434 || items[0].Home.Longitude != 117.008255 {
		t.Fatalf("unexpected home point: %#v", items[0].Home)
	}
	if items[0].Pilot == nil || items[0].Pilot.Latitude != 28.192692 || items[0].Pilot.Longitude != 117.008450 {
		t.Fatalf("unexpected pilot point: %#v", items[0].Pilot)
	}
}

func TestIngestLineStoresDIDEncryptedFallbackWithoutDecoder(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want fallback target", len(items))
	}
	if items[0].Serial != "875bb45f" || items[0].Model != "DJI-Drone" || items[0].Cracked {
		t.Fatalf("fallback target = %#v", items[0])
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
	events, unsubscribe := svc.Subscribe(10)
	defer unsubscribe()

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
	if items[0].HitCount != 1 {
		t.Fatalf("hit count = %d, want decoded target after fallback removal", items[0].HitCount)
	}
	if items[0].LastRecord.Device != "10125" {
		t.Fatalf("last record device = %q, want 10125", items[0].LastRecord.Device)
	}
	if records := svc.Records(10); len(records) != 0 {
		t.Fatalf("detection records count = %d, want 0 for DID encrypted", len(records))
	}
	removed := false
	waitUntil(t, time.Second, func() bool {
		for {
			select {
			case evt := <-events:
				if evt.Type != "screen.position.removed" {
					continue
				}
				target, ok := evt.Payload.(model.ScreenPositionTarget)
				if ok && target.CorrelationID == "did_encrypted:875bb45f" && target.Model == "DJI-Drone" && !target.Cracked {
					removed = true
				}
			default:
				return removed
			}
		}
	})
}

func TestIngestLineSkipsDIDEncryptedFallbackAfterCorrelationCracked(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})
	decoder := &oneShotO3Decoder{}
	svc.SetO3PlusO4Decoder(decoder)
	line := "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9"

	svc.IngestLine("session-1", "/dev/ttyUSB0", line)
	var items []model.ScreenPositionTarget
	waitUntil(t, time.Second, func() bool {
		items = svc.ScreenPositions(10)
		return len(items) == 1 && items[0].Serial == "o3-sn"
	})

	svc.IngestLine("session-1", "/dev/ttyUSB0", line)
	waitUntil(t, time.Second, func() bool {
		return decoder.Calls() >= 2
	})

	items = svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want only cracked target after repeated DID encrypted", len(items))
	}
	if items[0].Serial != "o3-sn" || items[0].Model == "DJI-Drone" || !items[0].Cracked {
		t.Fatalf("target after repeated DID encrypted = %#v", items[0])
	}
}

func TestIngestLineStoresFallbackScreenPositionFromDIDEncrypted(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9")

	items := svc.ScreenPositions(10)
	if len(items) != 1 {
		t.Fatalf("screen positions count = %d, want fallback target", len(items))
	}
	if items[0].Serial != "875bb45f" || items[0].Model != "DJI-Drone" || items[0].Cracked {
		t.Fatalf("fallback target = %#v", items[0])
	}
	if items[0].Drone != nil || items[0].Pilot != nil || items[0].Home != nil {
		t.Fatalf("fallback coordinates = %#v/%#v/%#v, want nil", items[0].Drone, items[0].Pilot, items[0].Home)
	}
	if items[0].Frequency != 2429.5 || items[0].RSSI != -64 {
		t.Fatalf("fallback radio = %#v", items[0])
	}
	if items[0].LastRecord.Type != string(parser.TypeDIDEncrypted) || items[0].LastRecord.Serial != "875bb45f" {
		t.Fatalf("fallback last record = %#v", items[0].LastRecord)
	}
}

func TestO3DecryptResultKeepsZeroCoordinatesForDisplay(t *testing.T) {
	receivedAt := time.Now()

	target := screenPositionFromProtocolTarget(diddecrypt.TargetFromDecryptResult(parser.DIDEncrypted{
		Device:      "4745",
		EncryptedID: "86ca8046",
		Freq:        5776.5,
		RSSI:        -76,
	}, diddecrypt.DecryptResult{
		SN:       "o3-sn",
		Model:    "DJI O3",
		Lat:      0,
		Lon:      0,
		PilotLat: 0,
		PilotLon: 0,
		HomeLat:  0,
		HomeLon:  0,
	}, receivedAt, true))

	if target.Drone == nil || target.Drone.Latitude != 0 || target.Drone.Longitude != 0 {
		t.Fatalf("expected zero drone point to be retained for display, got %#v", target.Drone)
	}
	if target.Pilot == nil || target.Pilot.Latitude != 0 || target.Pilot.Longitude != 0 {
		t.Fatalf("expected zero pilot point to be retained for display, got %#v", target.Pilot)
	}
	if target.Home == nil || target.Home.Latitude != 0 || target.Home.Longitude != 0 {
		t.Fatalf("expected zero home point to be retained for display, got %#v", target.Home)
	}
	if target.TrajectorySpeed == nil || *target.TrajectorySpeed != 0 {
		t.Fatalf("expected zero trajectory speed to be retained, got %#v", target.TrajectorySpeed)
	}
	if target.TrajectoryHeight == nil || *target.TrajectoryHeight != 0 {
		t.Fatalf("expected zero trajectory height to be retained, got %#v", target.TrajectoryHeight)
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
	return fakeO3DecodedTarget(packet, receivedAt), true
}

type oneShotO3Decoder struct {
	mu    sync.Mutex
	calls int
}

func (d *oneShotO3Decoder) ParseO3PlusO4PacketMQTT(_ context.Context, packet parser.DIDEncrypted, deviceSN string, receivedAt time.Time) (model.ScreenPositionTarget, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.calls > 1 || deviceSN != "10125" {
		return model.ScreenPositionTarget{}, false
	}
	return fakeO3DecodedTarget(packet, receivedAt), true
}

func (d *oneShotO3Decoder) Calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func fakeO3DecodedTarget(packet parser.DIDEncrypted, receivedAt time.Time) model.ScreenPositionTarget {
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
	}
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

func TestSendCommandsWritesToActiveTXPort(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	ports := map[string]*fakeSerialPort{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		ports[cfg.PortName] = port
		return port, nil
	})

	if _, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer svc.Stop("zh-CN")

	if err := svc.SendCommands(
		"start -imag 192.168.8.10:49600\r\n",
		"start -band 1310,1410\r\n",
	); err != nil {
		t.Fatalf("SendCommands() error = %v", err)
	}

	assertPortWrites(
		t,
		ports["/dev/tx"],
		startDetectionCommand+"\n",
		"start -imag 192.168.8.10:49600\r\n",
		"start -band 1310,1410\r\n",
	)
	if got := ports["/dev/rx"].writes; len(got) != 0 {
		t.Fatalf("rx writes = %v, want none", got)
	}
}

func TestSendCommandsRequiresConnectedSession(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, nil, Options{})

	err = svc.SendCommands("start -imag 0\r\n")
	if !errors.Is(err, ErrCommandSerialOffline) {
		t.Fatalf("SendCommands() error = %v, want ErrCommandSerialOffline", err)
	}
}

func TestStartSessionSupportsSeparateReceiveAndSendBaudRates(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string]serialport.Config{}
	ports := map[string]*fakeSerialPort{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		opened[cfg.PortName] = *cfg
		port := newFakeSerialPort()
		ports[cfg.PortName] = port
		return port, nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		RxBaudRate: 460800,
		TxBaudRate: 115200,
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if opened["/dev/rx"].BaudRate != 460800 {
		t.Fatalf("rx baud rate = %d, want 460800", opened["/dev/rx"].BaudRate)
	}
	if opened["/dev/tx"].BaudRate != 115200 {
		t.Fatalf("tx baud rate = %d, want 115200", opened["/dev/tx"].BaudRate)
	}
	if resp.BaudRate != 460800 || resp.RxBaudRate != 460800 || resp.TxBaudRate != 115200 {
		t.Fatalf("unexpected response baud rates: %+v", resp)
	}
	assertPortWrites(t, ports["/dev/tx"], startDetectionCommand+"\n")

	_ = svc.Stop("zh-CN")
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

func TestStartSessionUsesDetectionDefaultBaudRate(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	var openedConfig serialport.Config
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		openedConfig = *cfg
		return newFakeSerialPort(), nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		PortName: "/dev/detection",
		DataBits: 8,
		StopBits: 1,
		Parity:   "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if openedConfig.BaudRate != defaultBaudRate {
		t.Fatalf("opened baud rate = %d, want %d", openedConfig.BaudRate, defaultBaudRate)
	}
	if resp.BaudRate != defaultBaudRate {
		t.Fatalf("response baud rate = %d, want %d", resp.BaudRate, defaultBaudRate)
	}

	_ = svc.Stop("zh-CN")
}

func TestStartSessionUsesSeparateDefaultBaudRates(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	svc := NewService(st, tr, settings.NewStore(filepath.Join(t.TempDir(), "settings.json")), Options{})

	opened := map[string]serialport.Config{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		opened[cfg.PortName] = *cfg
		return newFakeSerialPort(), nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
		DataBits:   8,
		StopBits:   1,
		Parity:     "none",
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if opened["/dev/rx"].BaudRate != defaultRxBaudRate {
		t.Fatalf("rx baud rate = %d, want %d", opened["/dev/rx"].BaudRate, defaultRxBaudRate)
	}
	if opened["/dev/tx"].BaudRate != defaultTxBaudRate {
		t.Fatalf("tx baud rate = %d, want %d", opened["/dev/tx"].BaudRate, defaultTxBaudRate)
	}
	if resp.RxBaudRate != defaultRxBaudRate || resp.TxBaudRate != defaultTxBaudRate {
		t.Fatalf("unexpected response baud rates: %+v", resp)
	}

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

func TestRestoreSavedSettingsSkipsClearedSettings(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsStore.Save(model.DetectionSessionRequest{}); err != nil {
		t.Fatalf("Save() error = %v", err)
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

func TestReconnectSendsStartCommandToSeparateTxPort(t *testing.T) {
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
	opened := map[string][]*fakeSerialPort{}
	svc.SetSerialOpener(func(cfg *serialport.Config) (serial.Port, error) {
		port := newFakeSerialPort()
		mu.Lock()
		opened[cfg.PortName] = append(opened[cfg.PortName], port)
		mu.Unlock()
		return port, nil
	})

	resp, err := svc.Start(model.DetectionSessionRequest{
		RxPortName: "/dev/rx",
		TxPortName: "/dev/tx",
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
	if len(opened["/dev/rx"]) != 1 || len(opened["/dev/tx"]) != 1 {
		rxCount, txCount := len(opened["/dev/rx"]), len(opened["/dev/tx"])
		mu.Unlock()
		t.Fatalf("opened rx/tx counts = %d/%d, want 1/1", rxCount, txCount)
	}
	firstRX := opened["/dev/rx"][0]
	firstTX := opened["/dev/tx"][0]
	mu.Unlock()
	assertPortWrites(t, firstRX)
	assertPortWrites(t, firstTX, startDetectionCommand+"\n")
	firstRX.Close()

	waitForCondition(t, 2*time.Second, func() bool {
		mu.Lock()
		rxCount, txCount := len(opened["/dev/rx"]), len(opened["/dev/tx"])
		mu.Unlock()
		return rxCount >= 2 && txCount >= 2 && svc.Current("zh-CN").Active
	})

	mu.Lock()
	reconnectedRX := opened["/dev/rx"][len(opened["/dev/rx"])-1]
	reconnectedTX := opened["/dev/tx"][len(opened["/dev/tx"])-1]
	mu.Unlock()
	assertPortWrites(t, reconnectedRX)
	assertPortWrites(t, reconnectedTX, startDetectionCommand+"\n")

	_ = svc.Stop("zh-CN")
}

func TestIngestLineStoresHeartbeatAsParsedOnly(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	st := store.NewMemoryStore(10, 10)
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	svc := NewService(st, tr, settingsStore, Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "#=84, device=10125, Heart Beat, 879,  0")

	if got := len(svc.Parsed(10)); got != 1 {
		t.Fatalf("parsed count = %d, want 1", got)
	}
	records := svc.Records(10)
	if got := len(records); got != 0 {
		t.Fatalf("detection count = %d, want 0", got)
	}
	userSettings, ok, err := settingsStore.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", userSettings, ok, err)
	}
	if userSettings.DeviceSN != settings.StandardDeviceSN("10125") {
		t.Fatalf("device SN = %q, want %q", userSettings.DeviceSN, settings.StandardDeviceSN("10125"))
	}
	if userSettings.DeviceHardwareID != "10125" {
		t.Fatalf("hardware ID = %q, want 10125", userSettings.DeviceHardwareID)
	}
}

func TestIngestLineUpdatesDeviceSNFromDeviceMessages(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	initial := model.UserSettings{
		DeviceSN:                  "old-sn",
		DeviceHardwareID:          "old-hardware-id",
		ManualDeviceLocation:      &model.GeoPoint{Latitude: 23.12911, Longitude: 113.264385},
		ScreenStrikeChannelLabels: []string{"2.4G", "5.2G"},
	}
	if err := settingsStore.SaveUser(initial); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, settingsStore, Options{})

	tests := []struct {
		name string
		line string
		want string
		raw  string
	}{
		{
			name: "detect",
			line: "device=10125, model=PAL Analog, freq=5865.0, rssi=-56.9",
			want: settings.StandardDeviceSN("10125"),
			raw:  "10125",
		},
		{
			name: "did plain",
			line: "num=672/3/1, device=20250, serial=0M6CH6AR0A100L, model=41-Mavic 2, uuid=176344372408408473, drone_GPS=121.400000,31.200000, home_GPS=121.390000,31.190000, pilot_GPS=121.410000,31.210000, Height=50, Altitude=110.0,EastV=3.0, NothV=4.0,UpV=0.0, freq=5796.5, rssi=-78, distance=0.0km,",
			want: settings.StandardDeviceSN("20250"),
			raw:  "20250",
		},
		{
			name: "did encrypted",
			line: "#=632/3/1, device=30375, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9",
			want: settings.StandardDeviceSN("30375"),
			raw:  "30375",
		},
		{
			name: "heartbeat",
			line: "#=84, device=40400, Heart Beat, 879,  0",
			want: settings.StandardDeviceSN("40400"),
			raw:  "40400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.IngestLine("session-1", "/dev/ttyUSB0", tt.line)

			got, ok, err := settingsStore.LoadUser()
			if err != nil || !ok {
				t.Fatalf("LoadUser() = %+v, %v, %v", got, ok, err)
			}
			if got.DeviceSN != tt.want {
				t.Fatalf("device SN = %q, want %q", got.DeviceSN, tt.want)
			}
			if got.DeviceHardwareID != tt.raw {
				t.Fatalf("hardware ID = %q, want %q", got.DeviceHardwareID, tt.raw)
			}
			if got.ManualDeviceLocation == nil ||
				got.ManualDeviceLocation.Latitude != initial.ManualDeviceLocation.Latitude ||
				got.ManualDeviceLocation.Longitude != initial.ManualDeviceLocation.Longitude {
				t.Fatalf("manual location = %+v, want preserved", got.ManualDeviceLocation)
			}
			if len(got.ScreenStrikeChannelLabels) != len(initial.ScreenStrikeChannelLabels) ||
				got.ScreenStrikeChannelLabels[0] != initial.ScreenStrikeChannelLabels[0] ||
				got.ScreenStrikeChannelLabels[1] != initial.ScreenStrikeChannelLabels[1] {
				t.Fatalf("strike labels = %+v, want preserved", got.ScreenStrikeChannelLabels)
			}
		})
	}
}

func TestIngestLineDoesNotClearDeviceSNFromEmptyDevice(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := settingsStore.SaveUser(model.UserSettings{DeviceSN: "10125"}); err != nil {
		t.Fatalf("SaveUser() error = %v", err)
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, settingsStore, Options{})

	svc.IngestLine("session-1", "/dev/ttyUSB0", "model=PAL Analog, freq=5865.0, rssi=-56.9")

	got, ok, err := settingsStore.LoadUser()
	if err != nil || !ok {
		t.Fatalf("LoadUser() = %+v, %v, %v", got, ok, err)
	}
	if got.DeviceSN != settings.StandardDeviceSN("10125") {
		t.Fatalf("device SN = %q, want preserved %q", got.DeviceSN, settings.StandardDeviceSN("10125"))
	}
	if got.DeviceHardwareID != "10125" {
		t.Fatalf("hardware ID = %q, want preserved 10125", got.DeviceHardwareID)
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
