package client

import (
	"bufio"
	"bytes"
	"reflect"
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

func TestScanSerialRecords(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "crlf records",
			input: "received: start -freq 1\r\nsample_rate=61440000\r\n",
			want:  []string{"received: start -freq 1", "sample_rate=61440000"},
		},
		{
			name: "detect records without separators",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-74.6",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-77.0",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-74.6",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-77.0",
			},
		},
		{
			name:  "unknown payload remains intact until eof",
			input: "alpha betagamma delta",
			want:  []string{"alpha betagamma delta"},
		},
		{
			name: "unknown prefix releases before detect records",
			input: strings.Join([]string{
				"#=0/0/0, d",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-67.1",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-66.6",
			}, ""),
			want: []string{
				"#=0/0/0, d",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-67.1",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-66.6",
			},
		},
		{
			name: "detect releases before unknown tail and next record",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-66.5",
				"com #=75,",
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-73.2",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-66.5",
				"com #=75,",
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-73.2",
			},
		},
		{
			name: "s1 heartbeat remains one record before detect",
			input: strings.Join([]string{
				"com #=10, device=4747, Heart Beat, 815,  22",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-74.8",
			}, ""),
			want: []string{
				"com #=10, device=4747, Heart Beat, 815,  22",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-74.8",
			},
		},
		{
			name: "detects release before s1 heartbeat with newline",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-68.3",
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-68.7",
				"com #=132, device=4747, Heart Beat, 1069,  44\n",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-68.3",
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-68.7",
				"com #=132, device=4747, Heart Beat, 1069,  44",
			},
		},
		{
			name: "detect releases before encrypted record with newline",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-61.5",
				"#=0/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=2429.5, rssi=-72, byte,aa,bb\n",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-61.5",
				"#=0/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=2429.5, rssi=-72, byte,aa,bb",
			},
		},
		{
			name: "detect does not absorb did plain numeric prefix",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-54.9",
				"4747, serial=163DF7C0015853, uuid=abc",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-54.9",
				"4747, serial=163DF7C0015853, uuid=abc",
			},
		},
		{
			name: "detect and numeric did plain split in continuous stream",
			input: strings.Join([]string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-54.9",
				"4747, serial=163DF7C0015853, uuid=abc",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-70.0",
			}, ""),
			want: []string{
				"device=4747, model=PAL Analog, freq=1255.0, rssi=-54.9",
				"4747, serial=163DF7C0015853, uuid=abc",
				"device=4747, model=PAL Analog, freq=1260.0, rssi=-70.0",
			},
		},
		{
			name: "rid releases before encrypted record",
			input: strings.Join([]string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256403, speed=0.0, Vspeed=0, direc=361, AltitudeP=78.0, AltitudeG=-1000.0, Height_AGL=0, MAC=60:60:1f:71:a2:54, rssi=-80, freq=5805",
				"#=6/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=5796.5, rssi=-71, byte,ee,4a",
			}, ""),
			want: []string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256403, speed=0.0, Vspeed=0, direc=361, AltitudeP=78.0, AltitudeG=-1000.0, Height_AGL=0, MAC=60:60:1f:71:a2:54, rssi=-80, freq=5805",
				"#=6/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=5796.5, rssi=-71, byte,ee,4a",
			},
		},
		{
			name: "partial rid releases before encrypted record",
			input: strings.Join([]string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256404, speed=0.0",
				"#=0/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=5816.5, rssi=-71, byte,a0,15",
			}, ""),
			want: []string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256404, speed=0.0",
				"#=0/0/0, device=4747, Encypted Mavic_O4_ID=557777f5, freq=5816.5, rssi=-71, byte,a0,15",
			},
		},
		{
			name: "rid releases before s1 heartbeat",
			input: strings.Join([]string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256404, speed=0.0, Vspeed=0, direc=361, AltitudeP=78.5, AltitudeG=-1000.0, Height_AGL=0, MAC=60:60:1f:71:a2:54, rssi=-81, freq=5805",
				"com #=255, device=4747, Heart Beat, 671,  42",
			}, ""),
			want: []string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA, model=DJI AIR 3, UA_type=2, drone_GPS=0.000000,0.000000, pilot_GPS=116.882180,28.256404, speed=0.0, Vspeed=0, direc=361, AltitudeP=78.5, AltitudeG=-1000.0, Height_AGL=0, MAC=60:60:1f:71:a2:54, rssi=-81, freq=5805",
				"com #=255, device=4747, Heart Beat, 671,  42",
			},
		},
		{
			name: "partial rid releases before s1 heartbeat",
			input: strings.Join([]string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA",
				"com #=260, device=4747, Heart Beat, 973,  56",
			}, ""),
			want: []string{
				"RID ssid=RID-1581F6N8C23840032ZDA, serial=1581F6N8C23840032ZDA",
				"com #=260, device=4747, Heart Beat, 973,  56",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			scanner.Split(scanSerialRecords)

			got := []string{}
			for scanner.Scan() {
				got = append(got, scanner.Text())
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan failed: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected records: got %#v want %#v", got, tt.want)
			}
		})
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
