package protocol

import (
	"encoding/binary"
	"reflect"
	"testing"
	"time"
)

func TestBuildQueryDeviceStatusFrame(t *testing.T) {
	got, err := BuildQuery(QueryDeviceStatus)
	if err != nil {
		t.Fatalf("BuildQuery() error = %v", err)
	}
	want := []byte{0xEB, 0x90, 0x09, 0xC0, 0x63, 0x67, 0x53, 0xFF, 0x60}
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
			want: []byte{0xEB, 0x90, 0x0B, 0xA0, 0x63, 0x67, 0x52, 0xFF, 0x47, 0x00, 0x88},
		},
		{
			name: "all signals off",
			mask: 0,
			want: []byte{0xEB, 0x90, 0x0B, 0xA0, 0x63, 0x67, 0x52, 0xFF, 0x00, 0x00, 0x41},
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
