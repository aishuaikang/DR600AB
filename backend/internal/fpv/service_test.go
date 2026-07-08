package fpv

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"net"
	"strings"
	"testing"
	"time"
)

func TestBeginPlaybackComputesBandAndRejectsBusy(t *testing.T) {
	svc := NewService(Options{})

	playback, err := svc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	if playback.Frequency != 1360 || playback.BandStart != 1310 || playback.BandEnd != 1410 {
		t.Fatalf("playback = %+v, want 1360 with 1310-1410 band", playback)
	}

	if _, err := svc.BeginPlayback(1280); !errors.Is(err, ErrPlaybackBusy) {
		t.Fatalf("second BeginPlayback() error = %v, want ErrPlaybackBusy", err)
	}

	svc.EndPlayback(playback)
	next, err := svc.BeginPlayback(1280)
	if err != nil {
		t.Fatalf("BeginPlayback() after EndPlayback() error = %v", err)
	}
	if next.BandStart != 1230 || next.BandEnd != 1330 {
		t.Fatalf("next playback = %+v, want 1230-1330 band", next)
	}
}

func TestEndPlaybackClearsLastFrame(t *testing.T) {
	svc := NewService(Options{})
	playback, err := svc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}

	svc.publishFrame(Frame{
		Num:        1,
		Rows:       1,
		Cols:       1,
		PixelCount: 1,
		FrameBytes: 5,
		ReceivedAt: time.Now().Format(time.RFC3339Nano),
		Image:      "data:image/png;base64,AA==",
	})
	if svc.LastFrame() == nil {
		t.Fatal("LastFrame() is nil after active publish")
	}

	svc.EndPlayback(playback)
	if frame := svc.LastFrame(); frame != nil {
		t.Fatalf("LastFrame() = %+v, want nil after EndPlayback", frame)
	}
	if status := svc.Snapshot(); status.FrameCount != 0 {
		t.Fatalf("FrameCount = %d, want 0 after EndPlayback", status.FrameCount)
	}

	svc.publishFrame(Frame{
		Num:        2,
		Rows:       1,
		Cols:       1,
		PixelCount: 1,
		FrameBytes: 5,
		ReceivedAt: time.Now().Format(time.RFC3339Nano),
		Image:      "data:image/png;base64,AA==",
	})
	if frame := svc.LastFrame(); frame != nil {
		t.Fatalf("LastFrame() = %+v, want nil after inactive publish", frame)
	}
}

func TestRecordedFramesKeepsRecentFrames(t *testing.T) {
	svc := NewService(Options{MaxRecordFrames: 2})
	playback, err := svc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	defer svc.EndPlayback(playback)

	for num := 1; num <= 3; num++ {
		svc.publishFrame(Frame{
			Num:        num,
			Rows:       1,
			Cols:       1,
			PixelCount: 1,
			FrameBytes: 5,
			ReceivedAt: time.Now().Format(time.RFC3339Nano),
			Image:      "data:image/png;base64,AA==",
		})
	}

	frames := svc.RecordedFrames()
	if len(frames) != 2 || frames[0].Num != 2 || frames[1].Num != 3 {
		t.Fatalf("RecordedFrames() = %+v, want frames 2 and 3", frames)
	}
}

func TestRunRetriesBindAndPublishesFrame(t *testing.T) {
	addrCh := make(chan string, 1)
	var attempts int
	svc := NewService(Options{
		Host:              "127.0.0.1",
		Port:              0,
		BindRetryInterval: 10 * time.Millisecond,
		OpenListener: func(network string, address string) (net.Listener, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("bind failed")
			}
			listener, err := net.Listen(network, "127.0.0.1:0")
			if err != nil {
				return nil, err
			}
			addrCh <- listener.Addr().String()
			return listener, nil
		},
	})

	events, unsubscribe := svc.Subscribe(8)
	defer unsubscribe()
	playback, err := svc.BeginPlayback(1360)
	if err != nil {
		t.Fatalf("BeginPlayback() error = %v", err)
	}
	defer svc.EndPlayback(playback)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(time.Second):
		t.Fatal("listener did not bind after retry")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial fpv listener: %v", err)
	}
	defer conn.Close()

	var header [4]byte
	binary.BigEndian.PutUint16(header[0:2], 2)
	binary.BigEndian.PutUint16(header[2:4], 2)
	if _, err := conn.Write(append(header[:], 0, 64, 128, 255)); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	frame := waitForFrame(t, events)
	if frame.Rows != 2 || frame.Cols != 2 || frame.PixelCount != 4 {
		t.Fatalf("frame = %+v, want 2x2 image", frame)
	}
	if !strings.HasPrefix(frame.Image, "data:image/png;base64,") {
		t.Fatalf("frame image = %q, want PNG data URL", frame.Image)
	}
}

func TestHandleConnectionSkipsEncodingWhenInactive(t *testing.T) {
	called := false
	previous := encodePNGDataURLFunc
	encodePNGDataURLFunc = func(image.Image) (string, error) {
		called = true
		return "", errors.New("unexpected encode")
	}
	t.Cleanup(func() {
		encodePNGDataURLFunc = previous
	})

	svc := NewService(Options{
		FirstFrameTimeout: time.Second,
		ReadIdleTimeout:   time.Second,
	})
	server, client := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer client.Close()
		defer cancel()
		var header [4]byte
		binary.BigEndian.PutUint16(header[0:2], 2)
		binary.BigEndian.PutUint16(header[2:4], 2)
		_, _ = client.Write(append(header[:], 0, 64, 128, 255))
	}()

	if err := svc.handleConnection(ctx, server); err != nil {
		t.Fatalf("handleConnection() error = %v", err)
	}
	if called {
		t.Fatal("encodePNGDataURL was called without active playback")
	}
	if frame := svc.LastFrame(); frame != nil {
		t.Fatalf("LastFrame() = %+v, want nil without active playback", frame)
	}
}

func TestHandleConnectionRejectsOversizedFrame(t *testing.T) {
	svc := NewService(Options{
		MaxFrameBytes:     3,
		FirstFrameTimeout: time.Second,
		ReadIdleTimeout:   time.Second,
	})
	server, client := net.Pipe()
	defer server.Close()

	go func() {
		defer client.Close()
		var header [4]byte
		binary.BigEndian.PutUint16(header[0:2], 2)
		binary.BigEndian.PutUint16(header[2:4], 2)
		_, _ = client.Write(header[:])
	}()

	err := svc.handleConnection(context.Background(), server)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("handleConnection() error = %v, want oversized frame error", err)
	}
}

func waitForFrame(t *testing.T, events <-chan Message) Frame {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case message := <-events:
			if message.Name != "frame" {
				continue
			}
			var frame Frame
			if err := json.Unmarshal(message.Data, &frame); err != nil {
				t.Fatalf("decode frame: %v", err)
			}
			return frame
		case <-timeout:
			t.Fatal("timed out waiting for frame")
		}
	}
}
