package spectrum

import (
	"encoding/binary"
	"testing"
)

func TestParseFrameAndApplyFrame(t *testing.T) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[:4], 3600_000)
	binary.BigEndian.PutUint16(data[4:6], uint16(18000))
	binary.BigEndian.PutUint16(data[6:8], uint16(18100))

	frame, ok := ParseFrame(data)
	if !ok {
		t.Fatal("expected spectrum frame to parse")
	}
	if frame.CenterFreqHz != 3600_000_000 || frame.FFTSize != 2 {
		t.Fatalf("frame = %+v", frame)
	}
	if frame.Values[0] != 0 || frame.Values[1] != 1 {
		t.Fatalf("values = %v", frame.Values)
	}

	snapshot := NewSnapshot(3600, 3601, DefaultFreqStepMHz)
	updated, result := ApplyFrame(snapshot, frame, DefaultFreqStepMHz)
	if !result.Updated {
		t.Fatalf("expected update, got %+v", result)
	}
	if updated.Values[0] != 0 || updated.Values[1] != 1 {
		t.Fatalf("updated values = %v", updated.Values)
	}
}

func TestParseFrameRejectsTextBytes(t *testing.T) {
	line := []byte("device=2904, model=O3+_ofdm_datalink, freq=5730.0, rssi=-65.9, seq=5500, gpio=6, bw=20M,")
	if _, ok := ParseFrame(line); ok {
		t.Fatal("expected text bytes to be rejected as spectrum frame")
	}
}
