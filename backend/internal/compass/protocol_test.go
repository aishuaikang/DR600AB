package compass

import (
	"bytes"
	"errors"
	"testing"
)

func TestBuildReadPitchRollHeadingCmd(t *testing.T) {
	got := buildReadPitchRollHeadingCmd(defaultDeviceAddr)
	want := []byte{0x77, 0x04, 0x00, 0x04, 0x08}
	if !bytes.Equal(got, want) {
		t.Fatalf("command = % X, want % X", got, want)
	}
}

func TestBuildReadHeadingCmd(t *testing.T) {
	got := buildReadHeadingCmd(defaultDeviceAddr)
	want := []byte{0x77, 0x04, 0x00, 0x03, 0x07}
	if !bytes.Equal(got, want) {
		t.Fatalf("command = % X, want % X", got, want)
	}
}

func TestBuildSetAutoOutputRateCmd(t *testing.T) {
	got, err := buildSetAutoOutputRateCmd(defaultDeviceAddr, autoOutputRateReplyMode)
	if err != nil {
		t.Fatalf("buildSetAutoOutputRateCmd() error = %v", err)
	}
	want := []byte{0x77, 0x05, 0x00, 0x0C, 0x00, 0x11}
	if !bytes.Equal(got, want) {
		t.Fatalf("command = % X, want % X", got, want)
	}
}

func TestBuildSetAutoOutputRateCmdRejectsInvalidRate(t *testing.T) {
	if _, err := buildSetAutoOutputRateCmd(defaultDeviceAddr, 0x06); !errors.Is(err, errInvalidRate) {
		t.Fatalf("error = %v, want %v", err, errInvalidRate)
	}
}

func TestParsePitchRollHeadingResponse(t *testing.T) {
	got, err := parsePitchRollHeadingResponse([]byte{0x77, 0x0D, 0x00, 0x84, 0x10, 0x01, 0x54, 0x00, 0x14, 0x43, 0x00, 0x15, 0x20, 0x82})
	if err != nil {
		t.Fatalf("parsePitchRollHeadingResponse() error = %v", err)
	}
	if got.pitch != -1.54 {
		t.Fatalf("pitch = %.2f, want -1.54", got.pitch)
	}
	if got.roll != 14.43 {
		t.Fatalf("roll = %.2f, want 14.43", got.roll)
	}
	if got.heading != 15.20 {
		t.Fatalf("heading = %.2f, want 15.20", got.heading)
	}
}

func TestParsePitchRollHeadingResponseAvoidsDecimalArtifacts(t *testing.T) {
	got, err := parsePitchRollHeadingResponse([]byte{0x77, 0x0D, 0x00, 0x84, 0x00, 0x00, 0x71, 0x00, 0x00, 0x12, 0x01, 0x32, 0x17, 0x5E})
	if err != nil {
		t.Fatalf("parsePitchRollHeadingResponse() error = %v", err)
	}
	if got.pitch != 0.71 {
		t.Fatalf("pitch = %.17f, want 0.71", got.pitch)
	}
	if got.roll != 0.12 {
		t.Fatalf("roll = %.17f, want 0.12", got.roll)
	}
	if got.heading != 132.17 {
		t.Fatalf("heading = %.17f, want 132.17", got.heading)
	}
}

func TestParseHeadingResponse(t *testing.T) {
	raw, err := buildHeadingResponse(defaultDeviceAddr, -15.25)
	if err != nil {
		t.Fatalf("buildHeadingResponse() error = %v", err)
	}
	got, err := parseHeadingResponse(raw)
	if err != nil {
		t.Fatalf("parseHeadingResponse() error = %v", err)
	}
	if got.heading != -15.25 {
		t.Fatalf("heading = %.2f, want -15.25", got.heading)
	}
}

func TestParseSetAutoOutputRateResponse(t *testing.T) {
	got, err := parseSetAutoOutputRateResponse([]byte{0x77, 0x05, 0x00, 0x8C, 0x00, 0x91})
	if err != nil {
		t.Fatalf("parseSetAutoOutputRateResponse() error = %v", err)
	}
	if got.address != defaultDeviceAddr || got.status != 0x00 || !got.success {
		t.Fatalf("response = %+v, want success", got)
	}
}

func TestParseFrameRejectsInvalidChecksum(t *testing.T) {
	_, err := parseFrame([]byte{0x77, 0x04, 0x00, 0x04, 0x09})
	if !errors.Is(err, errChecksumFailed) {
		t.Fatalf("error = %v, want %v", err, errChecksumFailed)
	}
}

func TestParsePitchRollHeadingResponseRejectsWrongCommand(t *testing.T) {
	frame, err := buildPitchRollHeadingResponse(defaultDeviceAddr, 0, 0, 30)
	if err != nil {
		t.Fatalf("buildPitchRollHeadingResponse() error = %v", err)
	}
	frame[3] = commandReadHeadingResp
	frame[len(frame)-1] = calcChecksum(frame[1 : len(frame)-1])

	_, err = parsePitchRollHeadingResponse(frame)
	if !errors.Is(err, errInvalidCommand) {
		t.Fatalf("error = %v, want %v", err, errInvalidCommand)
	}
}

func TestDecodeAngleBCDRejectsInvalidBCD(t *testing.T) {
	if _, err := decodeAngleBCD([]byte{0x00, 0x1A, 0x00}); !errors.Is(err, errInvalidAngle) {
		t.Fatalf("error = %v, want %v", err, errInvalidAngle)
	}
}
