package parser

import (
	"encoding/json"
	"testing"
)

func TestParseDIDEncrypted(t *testing.T) {
	line := "#=632/3/1, device=10125, Encypted Mavic_O4_ID=875bb45f, freq=2429.5, rssi=-64, byte,15,1b,9b,58,f0,d9"
	msg, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	d := msg.Data.(*DIDEncrypted)
	b, _ := json.MarshalIndent(msg, "", "  ")
	t.Logf("result: %s", b)
	_ = d
}

func TestParseRID(t *testing.T) {
	line := "RID ssid=RID-1581F6Z9C2412003L1W8, serial=1581F6Z9C2412003L1W8, model=DJI Mini 4 pro, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=0.000000,0.000000, speed=0.0, Vspeed=0, direc=361, AltitudeP=-38.5, AltitudeG=-1000.0, Height_AGL=0, MAC=60:60:1f:38:98:b9, rssi=-82, freq=2437"
	msg, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	d := msg.Data.(*RID)
	b, _ := json.MarshalIndent(msg, "", "  ")
	t.Logf("result: %s", b)
	_ = d
}

func TestParseRIDRejectsIncompleteLine(t *testing.T) {
	line := "RID ssid=RID-1581F6N8C238400"

	if _, err := ParseLine(line); err == nil {
		t.Fatal("expected incomplete RID to be rejected")
	}
}

func TestParseDIDPlain(t *testing.T) {
	line := "num=672/3/1, device=10125, serial=0M6CH6AR0A100L, model=41-Mavic 2, uuid=176344372408408473, drone_GPS=0.000000,0.000000, home_GPS=0.000000,0.000000, pilot_GPS=0.000000,0.000000, Height=0, Altitude=65519.0,EastV=-0.0, NothV=-0.0,UpV=0.0, freq=5796.5, rssi=-78, distance=0.0km,"
	msg, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	d := msg.Data.(*DIDPlain)
	b, _ := json.MarshalIndent(msg, "", "  ")
	t.Logf("result: %s", b)
	_ = d
}

func TestParseDetect(t *testing.T) {
	line := "device=10125, model=PAL Analog, freq=5865.0, rssi=-56.9"
	msg, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	d := msg.Data.(*Detect)
	b, _ := json.MarshalIndent(msg, "", "  ")
	t.Logf("result: %s", b)
	_ = d
}

func TestParseHeartbeat(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		device string
		seq    string
	}{
		{
			name:   "s2 heartbeat",
			line:   "#=10, device=4747, Heart Beat, 815,  22",
			device: "4747",
			seq:    "815",
		},
		{
			name:   "s1 heartbeat with com prefix",
			line:   "com #=10, device=4747, Heart Beat, 815,  22",
			device: "4747",
			seq:    "815",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseLine(tt.line)
			if err != nil {
				t.Fatal(err)
			}

			d := msg.Data.(*Heartbeat)
			if d.Device != tt.device {
				t.Fatalf("unexpected device: got %q want %q", d.Device, tt.device)
			}
			if d.Seq != tt.seq {
				t.Fatalf("unexpected seq: got %q want %q", d.Seq, tt.seq)
			}

			b, _ := json.MarshalIndent(msg, "", "  ")
			t.Logf("result: %s", b)
		})
	}
}
