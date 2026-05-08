package handler

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteReceivedLineRawOutputsUnknownLine(t *testing.T) {
	var out bytes.Buffer

	WriteReceivedLine(&out, "unparsed payload", OutputRaw)

	got := out.String()
	if got != "[RAW] unparsed payload\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestWriteReceivedLineBothOutputsRawAndParsedLine(t *testing.T) {
	var out bytes.Buffer

	WriteReceivedLine(&out, "device=10125, model=PAL Analog, freq=5865.0, rssi=-56.9", OutputBoth)

	got := out.String()
	if !strings.Contains(got, "[RAW] device=10125") {
		t.Fatalf("missing raw output: %q", got)
	}
	if !strings.Contains(got, "[PARSED]") {
		t.Fatalf("missing parsed output: %q", got)
	}
}

func TestWriteReceivedLineParsedSkipsUnknownLine(t *testing.T) {
	var out bytes.Buffer

	WriteReceivedLine(&out, "unparsed payload", OutputParsed)

	if got := out.String(); got != "" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestParseOutputModeRejectsUnknownValue(t *testing.T) {
	if _, err := ParseOutputMode("everything"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHandleLocalCommandSwitchesOutputMode(t *testing.T) {
	var out bytes.Buffer
	state := NewOutputModeState(OutputRaw)

	handled := HandleLocalCommand(&out, "/mode both", state)

	if !handled {
		t.Fatal("expected local command to be handled")
	}
	if got := state.Get(); got != OutputBoth {
		t.Fatalf("unexpected mode: %s", got)
	}
	if !strings.Contains(out.String(), "输出模式已切换为: both") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestHandleLocalCommandIgnoresDeviceCommand(t *testing.T) {
	var out bytes.Buffer
	state := NewOutputModeState(OutputRaw)

	handled := HandleLocalCommand(&out, "AT+TEST", state)

	if handled {
		t.Fatal("expected device command to be ignored")
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
