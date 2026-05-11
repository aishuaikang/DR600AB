package client

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

type fakeSerialPort struct {
	read       *strings.Reader
	written    bytes.Buffer
	readCalls  int
	closeCount int
}

func newFakeSerialPort(input string) *fakeSerialPort {
	return &fakeSerialPort{read: strings.NewReader(input)}
}

func (p *fakeSerialPort) SetMode(mode *serial.Mode) error { return nil }

func (p *fakeSerialPort) Read(b []byte) (int, error) {
	p.readCalls++
	return p.read.Read(b)
}

func (p *fakeSerialPort) Write(b []byte) (int, error) {
	return p.written.Write(b)
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
	return nil
}

func (p *fakeSerialPort) Break(duration time.Duration) error { return nil }

func TestDuplexSerialClientSendsViaWritePort(t *testing.T) {
	readPort := newFakeSerialPort("")
	writePort := newFakeSerialPort("")
	c := NewDuplexSerialClient(readPort, "/dev/rx", writePort, "/dev/tx", false)

	if err := c.Send("AT+PING"); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if got := readPort.written.String(); got != "" {
		t.Fatalf("read port should not receive writes, got %q", got)
	}
	if got := writePort.written.String(); got != "AT+PING\n" {
		t.Fatalf("unexpected write port payload: %q", got)
	}
	if got := c.ReadPortName(); got != "/dev/rx" {
		t.Fatalf("unexpected read port name: %q", got)
	}
	if got := c.WritePortName(); got != "/dev/tx" {
		t.Fatalf("unexpected write port name: %q", got)
	}
	if got := c.PortName(); got != "/dev/rx" {
		t.Fatalf("unexpected compatible port name: %q", got)
	}
}

func TestDuplexSerialClientReadsViaReadPort(t *testing.T) {
	readPort := newFakeSerialPort("device=10125, model=PAL Analog\n")
	writePort := newFakeSerialPort("")
	c := NewDuplexSerialClient(readPort, "/dev/rx", writePort, "/dev/tx", false)

	line, err := c.ReadLine()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if line != "device=10125, model=PAL Analog" {
		t.Fatalf("unexpected line: %q", line)
	}
	if writePort.readCalls != 0 {
		t.Fatalf("write port should not be read, got %d reads", writePort.readCalls)
	}
}

func TestDuplexSerialClientCloseClosesBothPorts(t *testing.T) {
	readPort := newFakeSerialPort("")
	writePort := newFakeSerialPort("")
	c := NewDuplexSerialClient(readPort, "/dev/rx", writePort, "/dev/tx", false)

	c.Close()

	if readPort.closeCount != 1 {
		t.Fatalf("unexpected read port close count: %d", readPort.closeCount)
	}
	if writePort.closeCount != 1 {
		t.Fatalf("unexpected write port close count: %d", writePort.closeCount)
	}
}

func TestSerialClientCloseClosesSharedPortOnce(t *testing.T) {
	port := newFakeSerialPort("")
	c := NewSerialClient(port, "/dev/shared", false)

	c.Close()

	if port.closeCount != 1 {
		t.Fatalf("unexpected shared port close count: %d", port.closeCount)
	}
}
