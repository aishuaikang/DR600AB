package uavprotocol

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"uav-protocol/model"
	"uav-protocol/spectrum"
)

func TestParserParseTextUsesInjectedTime(t *testing.T) {
	now := time.Unix(1700000000, 0)
	parser := NewParser(Options{Now: func() time.Time { return now }})

	msg, err := parser.ParseText("device=2904, model=O3+_ofdm_datalink, freq=5730.0, rssi=-65.9,")
	if err != nil {
		t.Fatalf("ParseText() error = %v", err)
	}
	if msg.Type != model.TypeDetect || !msg.Time.Equal(now) {
		t.Fatalf("message = %+v, want detect at injected time", msg)
	}
}

func TestParserParseSpectrumWrapsMessage(t *testing.T) {
	now := time.Unix(1700000001, 0)
	parser := NewParser(Options{Now: func() time.Time { return now }})
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], 3600_000)
	binary.BigEndian.PutUint16(data[4:6], uint16(18000))
	binary.BigEndian.PutUint16(data[6:8], uint16(18100))

	msg, ok := parser.ParseSpectrum(data)
	if !ok {
		t.Fatal("expected spectrum packet")
	}
	if msg.Type != model.TypeSpectrum || !msg.Time.Equal(now) {
		t.Fatalf("message = %+v, want spectrum at injected time", msg)
	}
	frame, ok := msg.Data.(*spectrum.Frame)
	if !ok || frame.CenterFreqHz != 3600_000_000 || frame.FFTSize != 2 {
		t.Fatalf("frame = %#v, ok = %v", msg.Data, ok)
	}
}

func TestParserParseBytesRejectsUnknownBinary(t *testing.T) {
	parser := NewParser(Options{})
	_, _, err := parser.ParseBytes([]byte{0xff, 0x00, 0x01})
	if err == nil {
		t.Fatal("expected unknown binary packet error")
	}
}

func TestParserParseBytesAcceptsSpectrumHex(t *testing.T) {
	parser := NewParser(Options{})
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], 3600_000)
	binary.BigEndian.PutUint16(data[4:6], uint16(18000))
	binary.BigEndian.PutUint16(data[6:8], uint16(18100))

	msg, isSpectrum, err := parser.ParseBytes([]byte(hex.EncodeToString(data)))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if !isSpectrum || msg.Type != model.TypeSpectrum {
		t.Fatalf("message = %+v, isSpectrum = %v, want spectrum", msg, isSpectrum)
	}
}

func TestParserParseHex(t *testing.T) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], 3600_000)
	binary.BigEndian.PutUint16(data[4:6], uint16(18000))
	binary.BigEndian.PutUint16(data[6:8], uint16(18100))

	msg, err := ParseHex(hex.EncodeToString(data))
	if err != nil {
		t.Fatalf("ParseHex() error = %v", err)
	}
	if msg.Type != model.TypeSpectrum {
		t.Fatalf("message = %+v, want spectrum", msg)
	}
}
