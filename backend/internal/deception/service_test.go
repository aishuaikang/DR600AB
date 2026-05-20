package deception

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"gnss-spoofer/protocol"
	"serialport"
)

type fakePort struct {
	mu      sync.Mutex
	writes  [][]byte
	reads   chan []byte
	closed  bool
	reports map[byte][][]byte
}

func newFakePort() *fakePort {
	return &fakePort{
		reads:   make(chan []byte, 64),
		reports: make(map[byte][][]byte),
	}
}

func (p *fakePort) Read(buf []byte) (int, error) {
	chunk, ok := <-p.reads
	if !ok {
		return 0, io.EOF
	}
	copy(buf, chunk)
	return len(chunk), nil
}

func (p *fakePort) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := append([]byte(nil), data...)
	p.writes = append(p.writes, cp)

	frame, err := protocol.ParseFrame(data)
	if err == nil {
		switch frame.Control {
		case protocol.ControlQuery:
			if bodies := p.reports[frame.Command()]; len(bodies) > 0 {
				for _, body := range bodies {
					report, reportErr := protocol.BuildFrame(protocol.ControlReport, protocol.DeviceAddress, protocol.HostAddress, body)
					if reportErr == nil {
						p.reads <- report
					}
				}
			}
		default:
			ack, ackErr := protocol.BuildFrame(protocol.ControlAck, protocol.DeviceAddress, protocol.HostAddress, []byte{frame.Command(), 0, 0, 0})
			if ackErr == nil {
				p.reads <- ack
			}
		}
	}
	return len(data), nil
}

func (p *fakePort) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	close(p.reads)
	return nil
}

func (p *fakePort) SetReadTimeout(time.Duration) error {
	return nil
}

func (p *fakePort) commands() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, 0, len(p.writes))
	for _, write := range p.writes {
		frame, err := protocol.ParseFrame(write)
		if err == nil {
			out = append(out, frame.Command())
		}
	}
	return out
}

func (p *fakePort) frames() []protocol.Frame {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]protocol.Frame, 0, len(p.writes))
	for _, write := range p.writes {
		frame, err := protocol.ParseFrame(write)
		if err == nil {
			out = append(out, frame)
		}
	}
	return out
}

func (p *fakePort) setReport(command byte, body []byte, more ...[]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	bodies := make([][]byte, 0, 1+len(more))
	bodies = append(bodies, append([]byte(nil), body...))
	for _, item := range more {
		bodies = append(bodies, append([]byte(nil), item...))
	}
	p.reports[command] = bodies
}

func TestSetScreenDeceptionSendsFixedPointCommandSequence(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	lon := 116.994057
	lat := 28.170931
	alt := 120.0
	mask := uint16(protocol.SignalGPSL1CA | protocol.SignalBDSB1I)
	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:        true,
		TargetID:       "target-1",
		Mode:           "fixed_point",
		Longitude:      &lon,
		Latitude:       &lat,
		AltitudeM:      &alt,
		SignalMask:     &mask,
		StrengthPreset: "weak",
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 55, true, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}
	if !state.Active || state.TargetID != "target-1" || state.Point == nil {
		t.Fatalf("state = %+v, want active target", state)
	}
	if state.Mode != "fixed_point" || state.SignalMask != mask || state.AttenuationDB != 30 || state.DelayNS != 0 {
		t.Fatalf("state = %+v, want normalized fixed point params", state)
	}

	wantCommands := []byte{
		protocol.CmdSimulatedPosition,
		protocol.CmdTransmitSwitch,
	}
	if got := port.commands(); !bytes.Equal(got, wantCommands) {
		t.Fatalf("commands = % X, want % X", got, wantCommands)
	}

	frames := port.frames()
	assertMask(t, frames, protocol.CmdTransmitSwitch, mask)
	positionFrame := findFrame(t, frames, protocol.CmdSimulatedPosition)
	if got := readFloat64FromBody(positionFrame.Body[2:10]); got != lon {
		t.Fatalf("simulated longitude = %v, want %v", got, lon)
	}
	if got := readFloat64FromBody(positionFrame.Body[10:18]); got != lat {
		t.Fatalf("simulated latitude = %v, want %v", got, lat)
	}
	if got := int32(binary.LittleEndian.Uint32(positionFrame.Body[18:22])); got != int32(alt) {
		t.Fatalf("simulated altitude = %d, want %d", got, int32(alt))
	}
}

func TestSetScreenDeceptionSendsModeSpecificCommands(t *testing.T) {
	tests := []struct {
		name         string
		request      model.ScreenDeceptionRequest
		wantCommands []byte
	}{
		{
			name: "circle",
			request: model.ScreenDeceptionRequest{
				Mode: "circle",
				Circle: &model.ScreenDeceptionCircleParams{
					RadiusM:       80,
					PeriodSeconds: 45,
					Direction:     "ccw",
				},
				DelayMode: "off",
			},
			wantCommands: []byte{
				protocol.QueryDeviceStatus,
				protocol.CmdSimulatedPosition,
				protocol.CmdSimulatedCircle,
				protocol.CmdTransmitSwitch,
			},
		},
		{
			name: "linear",
			request: model.ScreenDeceptionRequest{
				Mode:      "linear",
				DelayMode: "off",
				Linear: &model.ScreenDeceptionLinearParams{
					SpeedMPS:    12,
					MaxSpeedMPS: 18,
				},
			},
			wantCommands: []byte{
				protocol.QueryDeviceStatus,
				protocol.CmdSimulatedPosition,
				protocol.CmdInitialVelocity,
				protocol.CmdMaxSpeed,
				protocol.CmdTransmitSwitch,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, port := newTestService(t)
			startSession(t, svc)
			port.setReport(protocol.QueryDeviceStatus, testDeviceStatusBody())

			tt.request.Enabled = true

			state, err := svc.SetScreenDeception(
				tt.request,
				model.GeoPoint{},
				0,
				false,
				"zh-CN",
			)
			if err != nil {
				t.Fatalf("SetScreenDeception() error = %v", err)
			}
			if state.Mode != tt.request.Mode {
				t.Fatalf("state.Mode = %q, want %q", state.Mode, tt.request.Mode)
			}
			if got := port.commands(); !bytes.Equal(got, tt.wantCommands) {
				t.Fatalf("commands = % X, want % X", got, tt.wantCommands)
			}
		})
	}
}

func TestSetScreenDeceptionCircleUsesCurrentPositionFromDeviceStatus(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)
	port.setReport(protocol.QueryDeviceStatus, testDeviceStatusBody())

	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled: true,
		Mode:    "circle",
		Circle: &model.ScreenDeceptionCircleParams{
			RadiusM:       80,
			PeriodSeconds: 45,
			Direction:     "ccw",
		},
	}, model.GeoPoint{}, 0, false, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}
	if state.Point == nil || state.Point.Longitude != 116.994057 || state.Point.Latitude != 28.170931 || state.AltitudeM != 12.5 {
		t.Fatalf("state = %+v, want device status current position reflected", state)
	}
	if got := port.commands(); !bytes.Equal(got, []byte{
		protocol.QueryDeviceStatus,
		protocol.CmdSimulatedPosition,
		protocol.CmdSimulatedCircle,
		protocol.CmdTransmitSwitch,
	}) {
		t.Fatalf("commands = % X, want status query, simulated position, circle, transmit", got)
	}

	positionFrame := findFrame(t, port.frames(), protocol.CmdSimulatedPosition)
	if got := readFloat64FromBody(positionFrame.Body[2:10]); got != 116.994057 {
		t.Fatalf("simulated longitude = %v, want status current longitude", got)
	}
	if got := readFloat64FromBody(positionFrame.Body[10:18]); got != 28.170931 {
		t.Fatalf("simulated latitude = %v, want status current latitude", got)
	}
	if got := int32(binary.LittleEndian.Uint32(positionFrame.Body[18:22])); got != 13 {
		t.Fatalf("simulated altitude = %d, want rounded status current altitude", got)
	}
}

func TestSetScreenDeceptionLinearUsesCurrentPositionFromDeviceStatus(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)
	port.setReport(protocol.QueryDeviceStatus, testDeviceStatusBody())

	direction := 135.0
	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled: true,
		Mode:    "linear",
		Linear: &model.ScreenDeceptionLinearParams{
			SpeedMPS:     12,
			DirectionDeg: &direction,
			MaxSpeedMPS:  18,
		},
	}, model.GeoPoint{}, 0, false, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}
	if state.Point == nil || state.Point.Longitude != 116.994057 || state.Point.Latitude != 28.170931 || state.AltitudeM != 12.5 {
		t.Fatalf("state = %+v, want device status current position reflected", state)
	}
	if got := port.commands(); !bytes.Equal(got, []byte{
		protocol.QueryDeviceStatus,
		protocol.CmdSimulatedPosition,
		protocol.CmdInitialVelocity,
		protocol.CmdMaxSpeed,
		protocol.CmdTransmitSwitch,
	}) {
		t.Fatalf("commands = % X, want status query, simulated position, velocity, max speed, transmit", got)
	}

	frames := port.frames()
	positionFrame := findFrame(t, frames, protocol.CmdSimulatedPosition)
	if got := readFloat64FromBody(positionFrame.Body[2:10]); got != 116.994057 {
		t.Fatalf("simulated longitude = %v, want status current longitude", got)
	}
	if got := readFloat64FromBody(positionFrame.Body[10:18]); got != 28.170931 {
		t.Fatalf("simulated latitude = %v, want status current latitude", got)
	}
	if got := int32(binary.LittleEndian.Uint32(positionFrame.Body[18:22])); got != 13 {
		t.Fatalf("simulated altitude = %d, want rounded status current altitude", got)
	}
	maxSpeedFrame := findFrame(t, frames, protocol.CmdMaxSpeed)
	if got := readFloat32FromBody(maxSpeedFrame.Body[2:6]); got != 18 {
		t.Fatalf("max speed = %v, want 18", got)
	}
}

func TestSetScreenDeceptionRejectsInvalidModeWithoutTransmit(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	lon := 116.994057
	lat := 28.170931
	_, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:   true,
		Mode:      "one_key",
		Longitude: &lon,
		Latitude:  &lat,
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 0, true, "zh-CN")
	if ErrorCode(err) != "deception_invalid_mode" {
		t.Fatalf("error = %v code=%q, want invalid mode", err, ErrorCode(err))
	}
	if got := port.commands(); len(got) != 0 {
		t.Fatalf("commands = % X, want none", got)
	}
}

func TestSetScreenDeceptionRejectsInvalidInput(t *testing.T) {
	svc, _ := newTestService(t)
	startSession(t, svc)

	lon := 181.0
	lat := 28.170931
	_, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:   true,
		Longitude: &lon,
		Latitude:  &lat,
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 0, true, "zh-CN")
	if ErrorCode(err) != "deception_location_required" {
		t.Fatalf("error = %v code=%q, want location required", err, ErrorCode(err))
	}
}

func TestSetScreenDeceptionRejectsMissingFixedPointCoordinate(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	_, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled: true,
		Mode:    "fixed_point",
	}, model.GeoPoint{}, 0, false, "zh-CN")
	if ErrorCode(err) != "deception_location_required" {
		t.Fatalf("error = %v code=%q, want location required", err, ErrorCode(err))
	}
	if got := port.commands(); len(got) != 0 {
		t.Fatalf("commands = % X, want none", got)
	}
}

func TestSetScreenDeceptionNormalizesOutOfRangeAltitude(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	lon := 116.994057
	lat := 28.170931
	alt := 10041.0
	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:   true,
		Longitude: &lon,
		Latitude:  &lat,
		AltitudeM: &alt,
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 0, true, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}
	if state.AltitudeM != 0 {
		t.Fatalf("state.AltitudeM = %v, want normalized 0", state.AltitudeM)
	}

	frame := findFrame(t, port.frames(), protocol.CmdSimulatedPosition)
	gotAltitude := int32(binary.LittleEndian.Uint32(frame.Body[18:22]))
	if gotAltitude != 0 {
		t.Fatalf("simulated altitude = %d, want 0", gotAltitude)
	}
}

func TestSetScreenDeceptionUsesManualDelayAndKeepsCustomAttenuationState(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	lon := 116.994057
	lat := 28.170931
	delayNS := 42.5
	attenuationDB := 7
	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:        true,
		Longitude:      &lon,
		Latitude:       &lat,
		StrengthPreset: "custom",
		AttenuationDB:  &attenuationDB,
		DelayMode:      "manual",
		DelayNS:        &delayNS,
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 0, true, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}
	if state.AttenuationDB != attenuationDB || state.DelayNS != delayNS {
		t.Fatalf("state = %+v, want custom attenuation and manual delay", state)
	}
	frames := port.frames()
	if hasFrame(frames, protocol.CmdPowerAttenuation) {
		t.Fatalf("power attenuation command was sent, want removed from start sequence")
	}
	if hasFrame(frames, protocol.CmdSignalDelay) {
		t.Fatalf("signal delay command was sent, want removed from fixed point start sequence")
	}
}

func TestSetScreenDeceptionManualStopTurnsOffTransmit(t *testing.T) {
	svc, port := newTestService(t)
	startSession(t, svc)

	lon := 116.994057
	lat := 28.170931
	_, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{
		Enabled:   true,
		Longitude: &lon,
		Latitude:  &lat,
	}, model.GeoPoint{Latitude: 28.1, Longitude: 116.9}, 0, true, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception() error = %v", err)
	}

	state, err := svc.SetScreenDeception(model.ScreenDeceptionRequest{Enabled: false}, model.GeoPoint{}, 0, false, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenDeception(stop) error = %v", err)
	}
	if state.Active {
		t.Fatalf("state.Active = true, want stopped")
	}
	commands := port.commands()
	if len(commands) == 0 || commands[len(commands)-1] != protocol.CmdTransmitSwitch {
		t.Fatalf("last command = % X, want transmit switch off", commands)
	}
}

func TestScreenDeviceStatusReturnsInactiveWithoutSerial(t *testing.T) {
	svc, _ := newTestService(t)

	status := svc.ScreenDeviceStatus("zh-CN")
	if status.SerialActive {
		t.Fatalf("SerialActive = true, want false")
	}
	if status.RawDescriptions == nil {
		t.Fatalf("RawDescriptions = nil, want empty map")
	}
	if status.LastError == "" {
		t.Fatalf("LastError is empty, want inactive message")
	}
}

func TestScreenDeviceStatusQueriesStructuredReports(t *testing.T) {
	svc, port := newTestService(t)
	port.setReport(protocol.QueryDeviceStatus, testDeviceStatusBody())
	port.setReport(protocol.QueryFirmwareVersion, []byte{protocol.QueryFirmwareVersion, 0, 0x03, 0x08, 0x10, 0x00, 0x05, 0x0C, 0x20, 0x00, 0x07, 0x10, 0x30, 0x00})
	port.setReport(protocol.QuerySystemTime, []byte{protocol.QuerySystemTime, 26, 5, 20, 1, 2, 3})
	port.setReport(protocol.QueryTransmitSwitch, []byte{protocol.QueryTransmitSwitch, 0, 0x47, 0x00})
	port.setReport(protocol.QueryDeviceSignal, testDeviceSignalBody(protocol.SignalGPSL1CA, 0x7D))
	port.setReport(protocol.QuerySimulatedPosition, testPositionBody(protocol.QuerySimulatedPosition, 28.170931, 116.994057, 120))
	port.setReport(protocol.QueryDevicePosition, testPositionBody(protocol.QueryDevicePosition, 29.654321, 117.123456, 88))
	port.setReport(protocol.QueryTargetPosition, testTargetPositionBody())
	port.setReport(protocol.QuerySpoofCircle, testSpoofCircleBody())
	port.setReport(protocol.QueryRandomPosition, []byte{protocol.QueryRandomPosition, 0, 1, 0, 0, 0, 100, 0, 0, 0, 3, 0, 0, 0})
	port.setReport(protocol.QueryPowerAttenuation, []byte{protocol.QueryPowerAttenuation, 1, 2, 3, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	delayBody := make([]byte, 30)
	delayBody[0] = protocol.QuerySignalDelay
	putFloat32(delayBody, 2, 11)
	putFloat32(delayBody, 6, 12)
	putFloat32(delayBody, 10, 13)
	putFloat32(delayBody, 26, 14)
	port.setReport(protocol.QuerySignalDelay, delayBody)
	port.setReport(protocol.QueryTimedSearch, []byte{protocol.QueryTimedSearch, 1})
	startSession(t, svc)

	status := svc.ScreenDeviceStatus("zh-CN")
	if !status.SerialActive {
		t.Fatalf("SerialActive = false, want true")
	}
	if status.LastError != "" {
		t.Fatalf("LastError = %q, want empty", status.LastError)
	}
	if status.CurrentPosition == nil || status.CurrentPosition.Longitude != 116.994057 || status.CurrentPosition.Latitude != 28.170931 {
		t.Fatalf("CurrentPosition = %+v, want decoded current location", status.CurrentPosition)
	}
	if status.SimulatedPosition == nil || status.SimulatedPosition.Longitude != 117.123456 || status.SimulatedPosition.Latitude != 29.654321 {
		t.Fatalf("SimulatedPosition = %+v, want decoded simulated location", status.SimulatedPosition)
	}
	if status.Version == nil || status.Version.Software != "1.2.3" || status.Version.FPGA != "2.3.5" || status.Version.Protocol != "3.4.7" {
		t.Fatalf("Version = %+v, want decoded versions", status.Version)
	}
	if status.ReportedSystemTime == nil || !status.ReportedSystemTime.Equal(time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("ReportedSystemTime = %v, want decoded system time", status.ReportedSystemTime)
	}
	if status.QueriedSimulatedPosition == nil || status.QueriedSimulatedPosition.Longitude != 116.994057 || status.QueriedSimulatedPosition.Latitude != 28.170931 {
		t.Fatalf("QueriedSimulatedPosition = %+v, want decoded queried simulated position", status.QueriedSimulatedPosition)
	}
	if status.QueriedDevicePosition == nil || status.QueriedDevicePosition.Longitude != 117.123456 || status.QueriedDevicePosition.Latitude != 29.654321 {
		t.Fatalf("QueriedDevicePosition = %+v, want decoded queried device position", status.QueriedDevicePosition)
	}
	if status.TargetPosition == nil || status.TargetPosition.DistanceM != 100 || status.TargetPosition.HeightM != 50 {
		t.Fatalf("TargetPosition = %+v, want decoded target position", status.TargetPosition)
	}
	if status.SpoofCircle == nil || status.SpoofCircle.RadiusM != 120 || status.SpoofCircle.Direction != "ccw" {
		t.Fatalf("SpoofCircle = %+v, want decoded circle", status.SpoofCircle)
	}
	if status.Random == nil || !status.Random.Enabled || status.Random.RadiusM != 100 || status.Random.RefreshSeconds != 3 {
		t.Fatalf("Random = %+v, want decoded random position", status.Random)
	}
	if status.TimedSearch == nil || !*status.TimedSearch {
		t.Fatalf("TimedSearch = %v, want true", status.TimedSearch)
	}
	if status.TransmitMask == nil || *status.TransmitMask != protocol.SignalAllSupported {
		t.Fatalf("TransmitMask = %v, want all supported", status.TransmitMask)
	}
	if len(status.TransmitSignals) != 4 {
		t.Fatalf("TransmitSignals = %v, want 4 signals", status.TransmitSignals)
	}
	if status.Attenuation == nil || status.Attenuation.GPS != 1 || status.Attenuation.GAL != 4 {
		t.Fatalf("Attenuation = %+v, want decoded attenuation", status.Attenuation)
	}
	if status.DelayNS == nil || *status.DelayNS != 11 || status.DelayBySignalNS == nil || status.DelayBySignalNS.GAL == nil || *status.DelayBySignalNS.GAL != 14 {
		t.Fatalf("Delay = %v %+v, want decoded delay", status.DelayNS, status.DelayBySignalNS)
	}
	if status.DeviceSignal == nil || status.DeviceSignal.ReceivedSatelliteCount != 3 || status.DeviceSignal.TransmittedCount != 2 {
		t.Fatalf("DeviceSignal = %+v, want decoded pseudo satellite status", status.DeviceSignal)
	}
	if status.DeviceSignal == nil || !status.DeviceSignal.WorkStatus.ClockOK || !status.DeviceSignal.WorkStatus.FPGAOK {
		t.Fatalf("DeviceSignal.WorkStatus = %+v, want decoded work status", status.DeviceSignal)
	}
	if len(status.RawDescriptions) != 13 {
		t.Fatalf("RawDescriptions len = %d, want 13", len(status.RawDescriptions))
	}
	if got := status.RawDescriptions["status"]; !strings.Contains(got, "TX ") || !strings.Contains(got, "RX ") || !strings.Contains(got, "当前定位") {
		t.Fatalf("RawDescriptions[status] = %q, want tx/rx and parsed detail", got)
	}
	if len(status.QueryErrors) != 0 {
		t.Fatalf("QueryErrors = %+v, want empty", status.QueryErrors)
	}
}

func TestScreenDeviceStatusIncludesRawDescriptionOnQueryTimeout(t *testing.T) {
	svc, _ := newTestService(t)
	startSession(t, svc)

	status := svc.ScreenDeviceStatus("zh-CN")
	if status.RawDescriptions["status"] == "" {
		t.Fatalf("RawDescriptions[status] is empty, want query tx/error detail")
	}
	if !strings.Contains(status.RawDescriptions["status"], "TX ") || !strings.Contains(status.RawDescriptions["status"], "ERR ") {
		t.Fatalf("RawDescriptions[status] = %q, want tx and error detail", status.RawDescriptions["status"])
	}
	if status.QueryErrors["status"] == "" {
		t.Fatalf("QueryErrors[status] is empty, want timeout error")
	}
}

func TestScreenDeviceStatusCollectsDeviceSignalBurst(t *testing.T) {
	svc, port := newTestService(t)
	port.setReport(protocol.QueryDeviceStatus, testDeviceStatusBody())
	port.setReport(protocol.QueryDeviceSignal, testDeviceSignalBody(protocol.SignalGPSL1CA, 0x7D), testDeviceSignalBody(protocol.SignalBDSB1I, 0x78))
	startSession(t, svc)

	status := svc.ScreenDeviceStatus("zh-CN")
	if status.DeviceSignal == nil {
		t.Fatalf("DeviceSignal = nil, want aggregate")
	}
	if status.DeviceSignal.SignalMask != protocol.SignalGPSL1CA|protocol.SignalBDSB1I {
		t.Fatalf("DeviceSignal.SignalMask = 0x%04X, want GPS+BDS", status.DeviceSignal.SignalMask)
	}
	if len(status.DeviceSignals) != 2 {
		t.Fatalf("DeviceSignals len = %d, want 2", len(status.DeviceSignals))
	}
	if _, ok := status.RawDescriptions["device_signal_02"]; !ok {
		t.Fatalf("RawDescriptions missing device_signal_02: %+v", status.RawDescriptions)
	}
}

func newTestService(t *testing.T) (*Service, *fakePort) {
	t.Helper()
	translator, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	port := newFakePort()
	svc := NewService(store.NewMemoryStore(10, 10), translator, nil, Options{
		CommandTimeout: 500 * time.Millisecond,
	})
	svc.SetSerialOpener(func(*serialport.Config) (SerialPort, error) {
		return port, nil
	})
	return svc, port
}

func startSession(t *testing.T, svc *Service) {
	t.Helper()
	response, err := svc.Start(model.DeceptionSessionRequest{
		PortName:      "/dev/ttyGNSS0",
		BaudRate:      115200,
		DataBits:      8,
		StopBits:      1,
		Parity:        "none",
		ReadTimeoutMs: 1000,
	}, "zh-CN")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !response.Active {
		t.Fatalf("Start() active = false response=%+v", response)
	}
}

func assertMask(t *testing.T, frames []protocol.Frame, command byte, want uint16) {
	t.Helper()
	frame := findFrame(t, frames, command)
	if len(frame.Body) < 4 {
		t.Fatalf("command 0x%02X body too short: % X", command, frame.Body)
	}
	if got := binary.LittleEndian.Uint16(frame.Body[2:4]); got != want {
		t.Fatalf("command 0x%02X mask = 0x%04X, want 0x%04X", command, got, want)
	}
}

func findFrame(t *testing.T, frames []protocol.Frame, command byte) protocol.Frame {
	t.Helper()
	for _, frame := range frames {
		if frame.Command() == command {
			return frame
		}
	}
	t.Fatalf("command 0x%02X was not sent", command)
	return protocol.Frame{}
}

func hasFrame(frames []protocol.Frame, command byte) bool {
	for _, frame := range frames {
		if frame.Command() == command {
			return true
		}
	}
	return false
}

func readFloat32FromBody(body []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(body))
}

func readFloat64FromBody(body []byte) float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(body))
}

func testDeviceStatusBody() []byte {
	body := make([]byte, 96)
	body[0] = protocol.QueryDeviceStatus
	body[2] = 26
	body[3] = 5
	body[4] = 20
	body[5] = 1
	body[6] = 2
	body[7] = 3
	body[8] = 0x8F
	body[9] = 3
	putFloat64(body, 10, 116.994057)
	putFloat64(body, 18, 28.170931)
	putFloat32(body, 26, 12.5)
	putFloat32(body, 30, 42.25)
	putFloat64(body, 34, 117.123456)
	putFloat64(body, 42, 29.654321)
	putFloat32(body, 50, 88.5)
	putFloat32(body, 54, 3.5)
	binary.LittleEndian.PutUint32(body[58:62], 3600)
	putFloat32(body, 62, 30)
	putFloat32(body, 66, 10)
	putFloat32(body, 70, 180)
	putFloat32(body, 74, 1.5)
	putFloat32(body, 78, 90)
	putFloat32(body, 82, 120)
	putFloat32(body, 86, 60)
	body[90] = 1
	body[91] = 1
	body[92] = 1
	binary.LittleEndian.PutUint16(body[94:96], protocol.SignalAllSupported)
	return body
}

func testDeviceSignalBody(mask uint16, workStatus byte) []byte {
	body := make([]byte, 91)
	body[0] = protocol.QueryDeviceSignal
	body[2] = 26
	body[3] = 5
	body[4] = 20
	body[5] = 1
	body[6] = 2
	body[7] = 3
	binary.LittleEndian.PutUint16(body[8:10], mask)
	putFloat32(body, 10, 12.5)
	body[14] = workStatus
	body[15] = 1
	body[16] = 18
	body[17] = 3
	body[18] = 3
	body[19] = 7
	body[20] = 11
	body[42] = 36
	body[43] = 39
	body[44] = 41
	body[66] = 2
	body[67] = 3
	body[68] = 7
	return body
}

func testPositionBody(command byte, latitude float64, longitude float64, altitudeM int32) []byte {
	body := make([]byte, 22)
	body[0] = command
	putFloat64(body, 2, latitude)
	putFloat64(body, 10, longitude)
	binary.LittleEndian.PutUint32(body[18:22], uint32(altitudeM))
	return body
}

func testTargetPositionBody() []byte {
	body := make([]byte, 18)
	body[0] = protocol.QueryTargetPosition
	binary.LittleEndian.PutUint32(body[2:6], uint32(100))
	binary.LittleEndian.PutUint32(body[6:10], uint32(50))
	putFloat32(body, 10, 90)
	putFloat32(body, 14, 180)
	return body
}

func testSpoofCircleBody() []byte {
	body := make([]byte, 30)
	body[0] = protocol.QuerySpoofCircle
	binary.LittleEndian.PutUint32(body[2:6], uint32(200))
	binary.LittleEndian.PutUint32(body[6:10], uint32(80))
	putFloat32(body, 10, 45)
	putFloat32(body, 14, 90)
	putFloat32(body, 18, 120)
	putFloat32(body, 22, 60)
	binary.LittleEndian.PutUint32(body[26:30], uint32(1))
	return body
}

func putFloat32(body []byte, offset int, value float32) {
	binary.LittleEndian.PutUint32(body[offset:offset+4], math.Float32bits(value))
}

func putFloat64(body []byte, offset int, value float64) {
	binary.LittleEndian.PutUint64(body[offset:offset+8], math.Float64bits(value))
}
