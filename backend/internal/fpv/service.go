// Package fpv receives grayscale FPV image frames and publishes them to screen clients.
package fpv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	defaultHost              = "192.168.8.10"
	defaultPort              = 49600
	defaultBindRetryInterval = time.Second
	defaultMaxFrameBytes     = 32 * 1024 * 1024
	defaultFirstFrameTimeout = 3 * time.Second
	defaultReadIdleTimeout   = 30 * time.Second
	defaultMaxRecordFrames   = 120
)

var (
	ErrInvalidFrequency = errors.New("invalid fpv frequency")
	ErrPlaybackBusy     = errors.New("fpv video busy")
)

// ListenerOpener binds a TCP listener. It is replaceable for tests.
type ListenerOpener func(network string, address string) (net.Listener, error)

// Options configures the FPV image receiver.
type Options struct {
	Host              string
	Port              int
	BindRetryInterval time.Duration
	MaxFrameBytes     int
	FirstFrameTimeout time.Duration
	ReadIdleTimeout   time.Duration
	MaxRecordFrames   int
	OpenListener      ListenerOpener
}

// Message is a pre-encoded SSE payload.
type Message struct {
	Name string
	Data []byte
}

// Frame is the browser-facing FPV image payload.
type Frame struct {
	Num        int     `json:"num"`
	Rows       int     `json:"rows"`
	Cols       int     `json:"cols"`
	PixelCount int     `json:"pixelCount"`
	FrameBytes int64   `json:"frameBytes"`
	RateKB     float64 `json:"rateKB"`
	ReceivedAt string  `json:"receivedAt"`
	Image      string  `json:"image"`
}

// Status describes the current FPV receiver and playback state.
type Status struct {
	Active          bool    `json:"active"`
	Frequency       float64 `json:"frequency,omitempty"`
	BandStart       int     `json:"bandStart,omitempty"`
	BandEnd         int     `json:"bandEnd,omitempty"`
	TCPAddress      string  `json:"tcpAddress"`
	Listening       bool    `json:"listening"`
	ListenError     string  `json:"listenError,omitempty"`
	SourceConnected bool    `json:"sourceConnected"`
	ClientAddress   string  `json:"clientAddress,omitempty"`
	FrameCount      int     `json:"frameCount"`
	UpdatedAt       string  `json:"updatedAt,omitempty"`
}

// Playback holds the active playback token and computed tuning band.
type Playback struct {
	ID        uint64
	Frequency float64
	BandStart int
	BandEnd   int
}

type imageStats struct {
	min uint8
	max uint8
	p2  uint8
	p98 uint8
}

type activePlayback struct {
	Playback
	startedAt time.Time
}

// Service owns the TCP receiver, frame subscribers, and single playback lock.
type Service struct {
	options Options

	mu              sync.RWMutex
	subscribers     map[chan Message]struct{}
	listening       bool
	listenError     string
	sourceConnected bool
	clientAddress   string
	frameCount      int
	updatedAt       time.Time
	lastFrame       *Frame
	recordFrames    []Frame
	playback        *activePlayback
	playbackSeq     uint64
}

// NewService creates an FPV receiver service.
func NewService(options Options) *Service {
	return &Service{
		options:     normalizeOptions(options),
		subscribers: map[chan Message]struct{}{},
	}
}

// Run keeps trying to bind the TCP receiver until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		listener, err := s.options.OpenListener("tcp", s.Address())
		if err != nil {
			s.setListenerState(false, err.Error())
			if !sleepOrDone(ctx, s.options.BindRetryInterval) {
				return
			}
			continue
		}

		s.setListenerState(true, "")
		err = s.serveListener(ctx, listener)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			s.setListenerState(false, err.Error())
		}
		if !sleepOrDone(ctx, s.options.BindRetryInterval) {
			return
		}
	}
}

// Address returns the bound TCP address for image frames.
func (s *Service) Address() string {
	return net.JoinHostPort(s.options.Host, strconv.Itoa(s.options.Port))
}

// BeginPlayback reserves the single playback slot and computes the tuning band.
func (s *Service) BeginPlayback(frequency float64) (Playback, error) {
	if math.IsNaN(frequency) || math.IsInf(frequency, 0) || frequency <= 0 {
		return Playback{}, ErrInvalidFrequency
	}

	center := int(math.Round(frequency))
	if center <= 50 {
		return Playback{}, ErrInvalidFrequency
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.playback != nil {
		return Playback{}, ErrPlaybackBusy
	}

	s.playbackSeq++
	playback := activePlayback{
		Playback: Playback{
			ID:        s.playbackSeq,
			Frequency: float64(center),
			BandStart: center - 50,
			BandEnd:   center + 50,
		},
		startedAt: time.Now(),
	}
	s.playback = &playback
	s.lastFrame = nil
	s.recordFrames = nil
	s.frameCount = 0
	s.updatedAt = playback.startedAt
	s.broadcastLocked(statusMessage(s.snapshotLocked()))
	return playback.Playback, nil
}

// EndPlayback releases the playback slot when token still owns it.
func (s *Service) EndPlayback(token Playback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.playback == nil || s.playback.ID != token.ID {
		return
	}

	s.playback = nil
	s.lastFrame = nil
	s.recordFrames = nil
	s.frameCount = 0
	s.updatedAt = time.Now()
	s.broadcastLocked(statusMessage(s.snapshotLocked()))
}

// Snapshot returns the current receiver and playback state.
func (s *Service) Snapshot() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

// LastFrame returns a copy of the latest published frame.
func (s *Service) LastFrame() *Frame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastFrame == nil {
		return nil
	}
	frame := *s.lastFrame
	return &frame
}

// RecordedFrames returns a copy of recently published frames for the active playback.
func (s *Service) RecordedFrames() []Frame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.recordFrames) == 0 {
		return nil
	}
	frames := make([]Frame, len(s.recordFrames))
	copy(frames, s.recordFrames)
	return frames
}

// Subscribe registers a frame/status subscriber.
func (s *Service) Subscribe(buffer int) (<-chan Message, func()) {
	if buffer <= 0 {
		buffer = 1
	}
	ch := make(chan Message, buffer)

	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, unsubscribe
}

func (s *Service) serveListener(ctx context.Context, listener net.Listener) error {
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept fpv connection: %w", err)
		}
		if err := s.handleConnection(ctx, conn); err != nil && ctx.Err() == nil {
			s.setListenerState(true, err.Error())
		}
	}
}

func (s *Service) handleConnection(ctx context.Context, conn net.Conn) error {
	clientAddress := conn.RemoteAddr().String()
	s.setSourceConnection(true, clientAddress)

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	defer func() {
		close(done)
		_ = conn.Close()
		s.setSourceConnection(false, clientAddress)
	}()

	startedAt := time.Now()
	var frameCount int
	var totalBytes int64

	for {
		var header [4]byte
		timeout := s.options.ReadIdleTimeout
		if frameCount == 0 {
			timeout = s.options.FirstFrameTimeout
		}

		if err := readFullWithTimeout(conn, header[:], timeout); err != nil {
			if ctx.Err() != nil || isExpectedReadEnd(err) {
				return nil
			}
			return fmt.Errorf("read fpv header: %w", err)
		}

		rows := int(binary.BigEndian.Uint16(header[0:2]))
		cols := int(binary.BigEndian.Uint16(header[2:4]))
		imageDataSize := rows * cols
		if rows <= 0 || cols <= 0 {
			return fmt.Errorf("invalid fpv image size: %d x %d", rows, cols)
		}
		if imageDataSize > s.options.MaxFrameBytes {
			return fmt.Errorf(
				"fpv image data too large: %d bytes, max %d bytes",
				imageDataSize,
				s.options.MaxFrameBytes,
			)
		}

		imageData := make([]byte, imageDataSize)
		if err := readFullWithTimeout(conn, imageData, s.options.ReadIdleTimeout); err != nil {
			if ctx.Err() != nil || isExpectedReadEnd(err) {
				return nil
			}
			return fmt.Errorf("read fpv image: %w", err)
		}

		frameCount++
		frameBytes := int64(4 + imageDataSize)
		totalBytes += frameBytes

		var rateKB float64
		if elapsed := time.Since(startedAt).Seconds(); elapsed > 0 {
			rateKB = float64(totalBytes) / elapsed / 1024
		}

		img := buildDisplayImage(imageData, rows, cols)
		imageURL, err := encodePNGDataURL(img)
		if err != nil {
			return fmt.Errorf("encode fpv image: %w", err)
		}

		s.publishFrame(Frame{
			Num:        frameCount,
			Rows:       rows,
			Cols:       cols,
			PixelCount: imageDataSize,
			FrameBytes: frameBytes,
			RateKB:     rateKB,
			ReceivedAt: time.Now().Format(time.RFC3339Nano),
			Image:      imageURL,
		})
	}
}

func (s *Service) setListenerState(listening bool, listenError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listening = listening
	s.listenError = listenError
	s.updatedAt = time.Now()
	s.broadcastLocked(statusMessage(s.snapshotLocked()))
}

func (s *Service) setSourceConnection(connected bool, clientAddress string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sourceConnected = connected
	if connected {
		s.clientAddress = clientAddress
		s.frameCount = 0
		s.lastFrame = nil
	} else if s.clientAddress == clientAddress {
		s.clientAddress = ""
	}
	s.updatedAt = time.Now()
	s.broadcastLocked(statusMessage(s.snapshotLocked()))
}

func (s *Service) publishFrame(frame Frame) {
	s.mu.RLock()
	active := s.playback != nil
	s.mu.RUnlock()
	if !active {
		return
	}

	data, err := json.Marshal(frame)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.playback == nil {
		return
	}
	s.frameCount = frame.Num
	s.updatedAt = parseFrameTime(frame.ReceivedAt)
	s.lastFrame = &frame
	if s.options.MaxRecordFrames > 0 {
		s.recordFrames = append(s.recordFrames, frame)
		if overflow := len(s.recordFrames) - s.options.MaxRecordFrames; overflow > 0 {
			copy(s.recordFrames, s.recordFrames[overflow:])
			s.recordFrames = s.recordFrames[:len(s.recordFrames)-overflow]
		}
	}
	s.broadcastLocked(Message{Name: "frame", Data: data})
}

func (s *Service) snapshotLocked() Status {
	status := Status{
		TCPAddress:      s.Address(),
		Listening:       s.listening,
		ListenError:     s.listenError,
		SourceConnected: s.sourceConnected,
		ClientAddress:   s.clientAddress,
		FrameCount:      s.frameCount,
	}
	if !s.updatedAt.IsZero() {
		status.UpdatedAt = s.updatedAt.Format(time.RFC3339Nano)
	}
	if s.playback != nil {
		status.Active = true
		status.Frequency = s.playback.Frequency
		status.BandStart = s.playback.BandStart
		status.BandEnd = s.playback.BandEnd
	}
	return status
}

func (s *Service) broadcastLocked(message Message) {
	for ch := range s.subscribers {
		select {
		case ch <- message:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- message:
			default:
			}
		}
	}
}

func statusMessage(status Status) Message {
	data, err := json.Marshal(status)
	if err != nil {
		data = []byte("{}")
	}
	return Message{Name: "status", Data: data}
}

func buildDisplayImage(imageData []byte, rows int, cols int) *image.Gray {
	var histogram [256]int
	stats := imageStats{
		min: 255,
		max: 0,
	}

	for _, value := range imageData {
		histogram[value]++
		if value < stats.min {
			stats.min = value
		}
		if value > stats.max {
			stats.max = value
		}
	}

	stats.p2 = percentileValue(histogram, len(imageData), 0.02)
	stats.p98 = percentileValue(histogram, len(imageData), 0.98)

	displayPixels := make([]byte, len(imageData))
	if stats.p98 > stats.p2 {
		denominator := float64(stats.p98 - stats.p2)
		for i, value := range imageData {
			scaled := float64(int(value)-int(stats.p2)) / denominator * 255
			displayPixels[i] = clampByte(scaled)
		}
	} else {
		copy(displayPixels, imageData)
	}

	return &image.Gray{
		Pix:    displayPixels,
		Stride: cols,
		Rect:   image.Rect(0, 0, cols, rows),
	}
}

func percentileValue(histogram [256]int, total int, percentile float64) uint8 {
	if total <= 0 {
		return 0
	}

	target := int(math.Ceil(percentile * float64(total)))
	if target < 1 {
		target = 1
	}

	var count int
	for value, bucketSize := range histogram {
		count += bucketSize
		if count >= target {
			return uint8(value)
		}
	}
	return 255
}

func clampByte(value float64) uint8 {
	switch {
	case value <= 0:
		return 0
	case value >= 255:
		return 255
	default:
		return uint8(math.Round(value))
	}
}

func encodePNGDataURL(img image.Image) (string, error) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func readFullWithTimeout(conn net.Conn, buffer []byte, timeout time.Duration) error {
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	_, err := io.ReadFull(conn, buffer)
	return err
}

func isExpectedReadEnd(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func normalizeOptions(options Options) Options {
	if options.Host == "" {
		options.Host = defaultHost
	}
	if options.Port == 0 {
		options.Port = defaultPort
	}
	if options.BindRetryInterval <= 0 {
		options.BindRetryInterval = defaultBindRetryInterval
	}
	if options.MaxFrameBytes <= 0 {
		options.MaxFrameBytes = defaultMaxFrameBytes
	}
	if options.FirstFrameTimeout < 0 {
		options.FirstFrameTimeout = 0
	}
	if options.FirstFrameTimeout == 0 {
		options.FirstFrameTimeout = defaultFirstFrameTimeout
	}
	if options.ReadIdleTimeout < 0 {
		options.ReadIdleTimeout = 0
	}
	if options.ReadIdleTimeout == 0 {
		options.ReadIdleTimeout = defaultReadIdleTimeout
	}
	if options.MaxRecordFrames < 0 {
		options.MaxRecordFrames = 0
	}
	if options.MaxRecordFrames == 0 {
		options.MaxRecordFrames = defaultMaxRecordFrames
	}
	if options.OpenListener == nil {
		options.OpenListener = net.Listen
	}
	return options
}

func sleepOrDone(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func parseFrameTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Now()
	}
	return parsed
}
