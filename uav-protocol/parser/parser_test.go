package parser

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"uav-protocol/model"
	"uav-protocol/spectrum"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantType model.MessageType
		check    func(t *testing.T, msg *model.Message)
	}{
		{
			name:     "detect",
			line:     "device=2904, model=O3+_ofdm_datalink, freq=5730.0, rssi=-65.9, seq=5500, gpio=6, bw=20M,",
			wantType: model.TypeDetect,
			check: func(t *testing.T, msg *model.Message) {
				t.Helper()
				data := msg.Data.(*model.Detect)
				if data.Device != "2904" || data.Model != "O3+_ofdm_datalink" || data.Freq != 5730.0 || data.RSSI != -65.9 {
					t.Fatalf("detect = %+v", data)
				}
				if data.Seq != 5500 || data.GPIO != 6 || data.Bandwidth != 20 {
					t.Fatalf("detect extension fields = %+v", data)
				}
			},
		},
		{
			name:     "rid",
			line:     "RID ssid=RID-1581ABC, serial=1581ABC, model=DJI Mini 4 Pro, UA_type=2, drone_GPS=121.400000,31.200000, pilot_GPS=121.410000,31.210000, speed=12.5, Vspeed=0, direc=90, AltitudeP=20.0, AltitudeG=110.0, Height_AGL=35.5, MAC=60:60:1f:38:98:b9, rssi=-82, freq=2437",
			wantType: model.TypeRID,
			check: func(t *testing.T, msg *model.Message) {
				t.Helper()
				data := msg.Data.(*model.RID)
				if data.Serial != "1581ABC" || data.DroneGPS.Lat != 31.2 || data.DroneGPS.Lng != 121.4 {
					t.Fatalf("rid = %+v", data)
				}
			},
		},
		{
			name:     "empty",
			line:     "Empty packet, freq=5530.6, rssi=-60",
			wantType: model.TypeEmpty,
			check: func(t *testing.T, msg *model.Message) {
				t.Helper()
				data := msg.Data.(*model.Empty)
				if data.Freq != 5530.6 || data.RSSI != -60 {
					t.Fatalf("empty = %+v", data)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseLine(tt.line)
			if err != nil {
				t.Fatalf("ParseLine() error = %v", err)
			}
			if msg.Type != tt.wantType {
				t.Fatalf("Type = %s, want %s", msg.Type, tt.wantType)
			}
			tt.check(t, msg)
		})
	}
}

func TestMessageTypeAliases(t *testing.T) {
	if MessageType(TypeSpectrum) != model.TypeSpectrum {
		t.Fatalf("TypeSpectrum = %s, want %s", TypeSpectrum, model.TypeSpectrum)
	}
}

func TestParseLineSpectrumHex(t *testing.T) {
	now := time.Unix(1700000010, 0)
	parser := New(Options{Now: func() time.Time { return now }})
	data := testSpectrumFrameBytes()
	line := hex.EncodeToString(data)

	msg, err := parser.ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine() error = %v", err)
	}
	if msg.Type != model.TypeSpectrum || !msg.Time.Equal(now) || msg.Raw != line {
		t.Fatalf("message = %+v, want spectrum at injected time with raw hex", msg)
	}
	frame, ok := msg.Data.(*spectrum.Frame)
	if !ok || frame.CenterFreqHz != 3600_000_000 || frame.FFTSize != 2 {
		t.Fatalf("frame = %#v, ok = %v", msg.Data, ok)
	}
}

func TestParseBytesSpectrumHex(t *testing.T) {
	raw := []byte("0x00 0x36 0xee 0x80, 0x46, 0x50, 0x46, 0xb4")

	msg, isSpectrum, err := ParseBytes(raw)
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if !isSpectrum || msg.Type != model.TypeSpectrum {
		t.Fatalf("message = %+v, isSpectrum = %v, want spectrum", msg, isSpectrum)
	}
	frame := msg.Data.(*spectrum.Frame)
	if frame.CenterFreqHz != 3600_000_000 || frame.Values[0] != 0 || frame.Values[1] != 1 {
		t.Fatalf("frame = %+v", frame)
	}
}

func TestParseBytesTextBeforeSpectrumProbe(t *testing.T) {
	line := "device=2904, model=O3+_ofdm_datalink, freq=5730.0, rssi=-65.9, seq=5500, gpio=6, bw=20M,"
	if len(line)%2 != 0 {
		t.Fatalf("test fixture must have even byte length, got %d", len(line))
	}

	msg, isSpectrum, err := ParseBytes([]byte(line))
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if isSpectrum || msg.Type != model.TypeDetect {
		t.Fatalf("message type = %s, isSpectrum = %v, want detect text", msg.Type, isSpectrum)
	}
}

func testSpectrumFrameBytes() []byte {
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], 3600_000)
	binary.BigEndian.PutUint16(data[4:6], uint16(18000))
	binary.BigEndian.PutUint16(data[6:8], uint16(18100))
	return data
}
