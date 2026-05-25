package protocol

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildQueryDeviceStatusFrame(t *testing.T) {
	got, err := BuildQuery(QueryDeviceStatus)
	if err != nil {
		t.Fatalf("BuildQuery() error = %v", err)
	}
	want := []byte{0xEB, 0x90, 0x09, 0xC0, 0x63, 0x44, 0x53, 0xFF, 0x3D}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildQuery() = % X, want % X", got, want)
	}
}

func TestBuildQueryDeviceSignalFrameIncludesSignalMask(t *testing.T) {
	got, err := BuildQuery(QueryDeviceSignal)
	if err != nil {
		t.Fatalf("BuildQuery() error = %v", err)
	}
	want := []byte{0xEB, 0x90, 0x0C, 0xC0, 0x63, 0x44, 0x5D, 0xFF, 0xFF, 0x1F, 0xFF, 0x67}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildQuery() = % X, want % X", got, want)
	}
}

func TestBuildTransmitSwitchFrames(t *testing.T) {
	tests := []struct {
		name string
		mask uint16
		want []byte
	}{
		{
			name: "all supported signals on",
			mask: SignalAllSupported,
			want: []byte{0xEB, 0x90, 0x0B, 0xA0, 0x63, 0x44, 0x52, 0xFF, 0x47, 0x00, 0x65},
		},
		{
			name: "all signals off",
			mask: 0,
			want: []byte{0xEB, 0x90, 0x0B, 0xA0, 0x63, 0x44, 0x52, 0xFF, 0x00, 0x00, 0x1E},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildSetTransmitSwitch(tt.mask)
			if err != nil {
				t.Fatalf("BuildSetTransmitSwitch() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BuildSetTransmitSwitch() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestParseFrameValidatesChecksum(t *testing.T) {
	frame, err := BuildQuery(QueryDevicePosition)
	if err != nil {
		t.Fatalf("BuildQuery() error = %v", err)
	}
	frame[len(frame)-1] ^= 0xFF

	if _, err := ParseFrame(frame); err == nil {
		t.Fatal("ParseFrame() error = nil, want checksum error")
	}
}

func TestScannerResyncsAndExtractsFrames(t *testing.T) {
	first, err := BuildQuery(QueryDeviceStatus)
	if err != nil {
		t.Fatalf("BuildQuery(status) error = %v", err)
	}
	second, err := BuildQuery(QueryDevicePosition)
	if err != nil {
		t.Fatalf("BuildQuery(location) error = %v", err)
	}

	var scanner Scanner
	frames, errs := scanner.Push(append([]byte{0x00, 0x01}, first[:4]...))
	if len(frames) != 0 || len(errs) != 0 {
		t.Fatalf("partial push frames=%d errs=%d, want none", len(frames), len(errs))
	}
	frames, errs = scanner.Push(append(first[4:], second...))
	if len(errs) != 0 {
		t.Fatalf("unexpected scanner errors: %v", errs)
	}
	if len(frames) != 2 {
		t.Fatalf("frames count = %d, want 2", len(frames))
	}
	if frames[0].Frame.Command() != QueryDeviceStatus || frames[1].Frame.Command() != QueryDevicePosition {
		t.Fatalf("commands = 0x%02X 0x%02X, want status/location", frames[0].Frame.Command(), frames[1].Frame.Command())
	}
}

func TestBuildSetSystemTimeUsesUTC(t *testing.T) {
	input := time.Date(2026, 5, 19, 10, 20, 30, 0, time.FixedZone("CST", 8*3600))
	frame, err := BuildSetSystemTime(input)
	if err != nil {
		t.Fatalf("BuildSetSystemTime() error = %v", err)
	}
	decoded, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}
	wantBody := []byte{CmdSystemTime, 26, 5, 19, 2, 20, 30}
	if !reflect.DeepEqual(decoded.Body, wantBody) {
		t.Fatalf("body = % X, want % X", decoded.Body, wantBody)
	}
}

func TestBuildSetSignalDelayUsesLittleEndianFloat(t *testing.T) {
	frame, err := BuildSetSignalDelay(SignalGPSL1CA, 12.5)
	if err != nil {
		t.Fatalf("BuildSetSignalDelay() error = %v", err)
	}
	decoded, err := ParseFrame(frame)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}
	if decoded.Body[0] != CmdSignalDelay {
		t.Fatalf("command = 0x%02X, want 0x%02X", decoded.Body[0], CmdSignalDelay)
	}
	if mask := binary.LittleEndian.Uint16(decoded.Body[2:4]); mask != SignalGPSL1CA {
		t.Fatalf("mask = 0x%04X, want GPS", mask)
	}
}

func TestBuildSetSuppressionAndTimedSearch(t *testing.T) {
	suppressionFrame, err := BuildSetSuppression(1, true)
	if err != nil {
		t.Fatalf("BuildSetSuppression() error = %v", err)
	}
	decoded, err := ParseFrame(suppressionFrame)
	if err != nil {
		t.Fatalf("ParseFrame(suppression) error = %v", err)
	}
	wantSuppressionBody := []byte{CmdSuppression, ReservedByte, 1, 0, 0, 0, 1, 0, 0, 0}
	if !reflect.DeepEqual(decoded.Body, wantSuppressionBody) {
		t.Fatalf("suppression body = % X, want % X", decoded.Body, wantSuppressionBody)
	}

	timedSearchFrame, err := BuildSetTimedSearch(true)
	if err != nil {
		t.Fatalf("BuildSetTimedSearch() error = %v", err)
	}
	decoded, err = ParseFrame(timedSearchFrame)
	if err != nil {
		t.Fatalf("ParseFrame(timed search) error = %v", err)
	}
	wantTimedSearchBody := []byte{CmdTimedSearch, 1}
	if !reflect.DeepEqual(decoded.Body, wantTimedSearchBody) {
		t.Fatalf("timed search body = % X, want % X", decoded.Body, wantTimedSearchBody)
	}
}

func TestParseAck(t *testing.T) {
	raw, err := BuildFrame(ControlAck, DeviceAddress, HostAddress, []byte{CmdTransmitSwitch, 0, 0, 0})
	if err != nil {
		t.Fatalf("BuildFrame() error = %v", err)
	}
	frame, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}
	ack, err := ParseAck(frame)
	if err != nil {
		t.Fatalf("ParseAck() error = %v", err)
	}
	if !ack.Success() || ack.Command != CmdTransmitSwitch {
		t.Fatalf("ack = %+v, want success for transmit switch", ack)
	}
}

func TestDescribeFrameLocaleEnglish(t *testing.T) {
	body := make([]byte, 96)
	body[0] = QueryDeviceStatus
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
	putFloat64(body, 34, 117.123456)
	putFloat64(body, 42, 29.654321)
	putFloat32(body, 50, 88.5)
	binary.LittleEndian.PutUint16(body[94:96], SignalGPSL1CA|SignalBDSB1I)

	description := DescribeFrameLocale(mustReportFrame(t, body), "en-US")
	for _, want := range []string{"report", "device status", "current position", "simulated position", "active signals"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description = %q, want %q", description, want)
		}
	}
	for _, forbidden := range []string{"上报", "设备状态", "当前定位", "模拟位置", "工作信号"} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("description = %q, must not contain Chinese text %q", description, forbidden)
		}
	}
}

func TestParseDeviceStatusReport(t *testing.T) {
	body := make([]byte, 96)
	body[0] = QueryDeviceStatus
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
	body[93] = 0
	binary.LittleEndian.PutUint16(body[94:96], SignalGPSL1CA|SignalGALE1)

	raw, err := BuildFrame(ControlReport, DeviceAddress, HostAddress, body)
	if err != nil {
		t.Fatalf("BuildFrame() error = %v", err)
	}
	frame, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}

	report, err := ParseDeviceStatusReport(frame)
	if err != nil {
		t.Fatalf("ParseDeviceStatusReport() error = %v", err)
	}
	if report.SystemTime == nil || !report.SystemTime.Equal(time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("SystemTime = %v, want protocol time", report.SystemTime)
	}
	if report.SyncStatus == nil || !report.SyncStatus.ReceiverWorking || !report.SyncStatus.AntennaOK || !report.SyncStatus.TimeSynced {
		t.Fatalf("SyncStatus = %+v, want decoded bits", report.SyncStatus)
	}
	if report.OscillatorState != "locked" {
		t.Fatalf("OscillatorState = %q, want locked", report.OscillatorState)
	}
	if report.CurrentPosition == nil || report.CurrentPosition.Longitude != 116.994057 || report.CurrentPosition.Latitude != 28.170931 {
		t.Fatalf("CurrentPosition = %+v, want decoded lon/lat", report.CurrentPosition)
	}
	if report.SimulatedPosition == nil || report.SimulatedPosition.Longitude != 117.123456 || report.SimulatedPosition.Latitude != 29.654321 {
		t.Fatalf("SimulatedPosition = %+v, want decoded lon/lat", report.SimulatedPosition)
	}
	if report.TemperatureC == nil || *report.TemperatureC != 42.25 {
		t.Fatalf("TemperatureC = %v, want 42.25", report.TemperatureC)
	}
	if report.UptimeSeconds == nil || *report.UptimeSeconds != 3600 {
		t.Fatalf("UptimeSeconds = %v, want 3600", report.UptimeSeconds)
	}
	if report.Motion == nil || report.Motion.CircleDirection != "ccw" || report.Motion.CircleRadiusM == nil || *report.Motion.CircleRadiusM != 120 {
		t.Fatalf("Motion = %+v, want decoded motion", report.Motion)
	}
	if report.AmplifierOn == nil || !*report.AmplifierOn {
		t.Fatalf("AmplifierOn = %v, want true", report.AmplifierOn)
	}
	if report.SignalMask == nil || *report.SignalMask != SignalGPSL1CA|SignalGALE1 {
		t.Fatalf("SignalMask = %v, want GPS/GAL", report.SignalMask)
	}
}

func TestParseSupplementalReports(t *testing.T) {
	versionFrame := mustReportFrame(t, []byte{QueryFirmwareVersion, 0, 0x03, 0x08, 0x10, 0x00, 0x05, 0x0C, 0x20, 0x00, 0x07, 0x10, 0x30, 0x00})
	version, err := ParseVersionReport(versionFrame)
	if err != nil {
		t.Fatalf("ParseVersionReport() error = %v", err)
	}
	if version.Software != "1.2.3" || version.FPGA != "2.3.5" || version.Protocol != "3.4.7" {
		t.Fatalf("version = %+v, want decoded versions", version)
	}

	timeFrame := mustReportFrame(t, []byte{QuerySystemTime, 26, 5, 20, 1, 2, 3})
	systemTime, err := ParseSystemTimeReport(timeFrame)
	if err != nil {
		t.Fatalf("ParseSystemTimeReport() error = %v", err)
	}
	if systemTime == nil || !systemTime.Equal(time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("systemTime = %v, want decoded time", systemTime)
	}

	txFrame := mustReportFrame(t, []byte{QueryTransmitSwitch, 0, 0x47, 0x00})
	tx, err := ParseTransmitSwitchReport(txFrame)
	if err != nil {
		t.Fatalf("ParseTransmitSwitchReport() error = %v", err)
	}
	if tx.Mask != SignalAllSupported {
		t.Fatalf("tx.Mask = 0x%04X, want all supported", tx.Mask)
	}

	powerFrame := mustReportFrame(t, []byte{QueryPowerAttenuation, 1, 2, 3, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	power, err := ParsePowerAttenuationReport(powerFrame)
	if err != nil {
		t.Fatalf("ParsePowerAttenuationReport() error = %v", err)
	}
	if power.GPS != 1 || power.BDS != 2 || power.GLO != 3 || power.GAL != 4 {
		t.Fatalf("power = %+v, want 1/2/3/4", power)
	}

	delayBody := make([]byte, 30)
	delayBody[0] = QuerySignalDelay
	putFloat32(delayBody, 2, 11)
	putFloat32(delayBody, 6, 12)
	putFloat32(delayBody, 10, 13)
	putFloat32(delayBody, 26, 14)
	delayFrame := mustReportFrame(t, delayBody)
	delay, err := ParseSignalDelayReport(delayFrame)
	if err != nil {
		t.Fatalf("ParseSignalDelayReport() error = %v", err)
	}
	if delay.GPS == nil || *delay.GPS != 11 || delay.BDS == nil || *delay.BDS != 12 || delay.GLO == nil || *delay.GLO != 13 || delay.GAL == nil || *delay.GAL != 14 {
		t.Fatalf("delay = %+v, want 11/12/13/14", delay)
	}

	positionBody := make([]byte, 22)
	positionBody[0] = QueryDevicePosition
	putFloat64(positionBody, 2, 28.170931)
	putFloat64(positionBody, 10, 116.994057)
	binary.LittleEndian.PutUint32(positionBody[18:22], uint32(120))
	position, err := ParsePositionReport(mustReportFrame(t, positionBody), QueryDevicePosition)
	if err != nil {
		t.Fatalf("ParsePositionReport() error = %v", err)
	}
	if position.Latitude != 28.170931 || position.Longitude != 116.994057 || position.AltitudeM != 120 {
		t.Fatalf("position = %+v, want decoded position", position)
	}

	targetFrame := mustReportFrame(t, []byte{QueryTargetPosition, 0, 0x64, 0, 0, 0, 0x32, 0, 0, 0, 0, 0, 0xB4, 0x42, 0, 0, 0x34, 0x43})
	target, err := ParseTargetPositionReport(targetFrame)
	if err != nil {
		t.Fatalf("ParseTargetPositionReport() error = %v", err)
	}
	if target.DistanceM != 100 || target.HeightM != 50 || target.DirectionDeg != 90 || target.HeadingDeg != 180 {
		t.Fatalf("target = %+v, want decoded target", target)
	}

	circleBody := make([]byte, 30)
	circleBody[0] = QuerySpoofCircle
	binary.LittleEndian.PutUint32(circleBody[2:6], uint32(200))
	binary.LittleEndian.PutUint32(circleBody[6:10], uint32(80))
	putFloat32(circleBody, 10, 45)
	putFloat32(circleBody, 14, 90)
	putFloat32(circleBody, 18, 120)
	putFloat32(circleBody, 22, 60)
	binary.LittleEndian.PutUint32(circleBody[26:30], 1)
	circle, err := ParseSpoofCircleReport(mustReportFrame(t, circleBody))
	if err != nil {
		t.Fatalf("ParseSpoofCircleReport() error = %v", err)
	}
	if circle.DistanceM != 200 || circle.HeightM != 80 || circle.RadiusM != 120 || circle.PeriodSeconds != 60 || circle.Direction != "ccw" {
		t.Fatalf("circle = %+v, want decoded circle", circle)
	}

	suppressionFrame := mustReportFrame(t, []byte{QuerySuppression, 0, 1, 0, 0, 0, 1, 0, 0, 0})
	suppression, err := ParseSuppressionReport(suppressionFrame)
	if err != nil {
		t.Fatalf("ParseSuppressionReport() error = %v", err)
	}
	if suppression.WaveformMask != 1 || !suppression.TransmitOn {
		t.Fatalf("suppression = %+v, want waveform 1 and transmit on", suppression)
	}

	randomFrame := mustReportFrame(t, []byte{QueryRandomPosition, 0, 1, 0, 0, 0, 100, 0, 0, 0, 3, 0, 0, 0})
	random, err := ParseRandomPositionReport(randomFrame)
	if err != nil {
		t.Fatalf("ParseRandomPositionReport() error = %v", err)
	}
	if !random.Enabled || random.RadiusM != 100 || random.RefreshSeconds != 3 {
		t.Fatalf("random = %+v, want decoded random position", random)
	}

	timedSearchFrame := mustReportFrame(t, []byte{QueryTimedSearch, 1})
	timedSearch, err := ParseTimedSearchReport(timedSearchFrame)
	if err != nil {
		t.Fatalf("ParseTimedSearchReport() error = %v", err)
	}
	if !timedSearch.Enabled {
		t.Fatalf("timedSearch.Enabled = false, want true")
	}
}

func TestParseDeviceSignalReport(t *testing.T) {
	body := make([]byte, 91)
	body[0] = QueryDeviceSignal
	body[2] = 26
	body[3] = 5
	body[4] = 20
	body[5] = 1
	body[6] = 2
	body[7] = 3
	binary.LittleEndian.PutUint16(body[8:10], SignalGPSL1CA|SignalBDSB1I)
	putFloat32(body, 10, 12.5)
	body[14] = 0x7D
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

	report, err := ParseDeviceSignalReport(mustReportFrame(t, body))
	if err != nil {
		t.Fatalf("ParseDeviceSignalReport() error = %v", err)
	}
	if report.SystemTime == nil || !report.SystemTime.Equal(time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)) {
		t.Fatalf("SystemTime = %v, want protocol time", report.SystemTime)
	}
	if report.SignalMask != SignalGPSL1CA|SignalBDSB1I || len(report.SignalNames) != 2 {
		t.Fatalf("SignalMask = 0x%04X names=%v, want GPS/BDS", report.SignalMask, report.SignalNames)
	}
	if report.DelayNS != 12.5 || report.AttenuationDB != 18 || !report.TransmitSwitch {
		t.Fatalf("report = %+v, want delay/attenuation/transmit decoded", report)
	}
	if !report.WorkStatus.ClockOK || !report.WorkStatus.EphemerisValid || !report.WorkStatus.RFModuleOK ||
		!report.WorkStatus.SignalTransmit || !report.WorkStatus.TransmitChannel || !report.WorkStatus.FPGAOK {
		t.Fatalf("WorkStatus = %+v, want decoded bits", report.WorkStatus)
	}
	if report.ReceivedSatelliteCount != 3 || len(report.ReceivedPRNs) != 3 || len(report.ReceivedCN0) != 3 {
		t.Fatalf("received = count %d PRN %v CN0 %v, want three received satellites", report.ReceivedSatelliteCount, report.ReceivedPRNs, report.ReceivedCN0)
	}
	if report.TransmittedCount != 2 || len(report.TransmittedPRNs) != 2 {
		t.Fatalf("transmitted = count %d PRN %v, want two transmitted satellites", report.TransmittedCount, report.TransmittedPRNs)
	}
}

func mustReportFrame(t *testing.T, body []byte) Frame {
	t.Helper()
	raw, err := BuildFrame(ControlReport, DeviceAddress, HostAddress, body)
	if err != nil {
		t.Fatalf("BuildFrame() error = %v", err)
	}
	frame, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame() error = %v", err)
	}
	return frame
}

func putFloat32(body []byte, offset int, value float32) {
	binary.LittleEndian.PutUint32(body[offset:offset+4], math.Float32bits(value))
}

func putFloat64(body []byte, offset int, value float64) {
	binary.LittleEndian.PutUint64(body[offset:offset+8], math.Float64bits(value))
}
