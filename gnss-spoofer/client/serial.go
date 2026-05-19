package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"go.bug.st/serial"

	"gnss-spoofer/protocol"
)

type ReceivedFrame struct {
	Frame protocol.Frame
	Raw   []byte
	Err   error
	Time  time.Time
}

type waiter struct {
	match func(protocol.Frame) bool
	ch    chan ReceivedFrame
}

type SerialClient struct {
	port     serial.Port
	portName string
	verbose  bool
	output   io.Writer

	writeMu sync.Mutex
	waitMu  sync.Mutex
	waitSeq int
	waiters map[int]waiter

	frames    chan ReceivedFrame
	closeOnce sync.Once
	done      chan struct{}
}

func NewSerialClient(port serial.Port, portName string, verbose bool) *SerialClient {
	return &SerialClient{
		port:     port,
		portName: portName,
		verbose:  verbose,
		output:   os.Stdout,
		waiters:  make(map[int]waiter),
		frames:   make(chan ReceivedFrame, 64),
		done:     make(chan struct{}),
	}
}

func (c *SerialClient) SetOutput(w io.Writer) {
	if w == nil {
		c.output = os.Stdout
		return
	}
	c.output = w
}

func (c *SerialClient) PortName() string {
	return c.portName
}

func (c *SerialClient) Frames() <-chan ReceivedFrame {
	return c.frames
}

func (c *SerialClient) Start() {
	go c.readLoop()
}

func (c *SerialClient) Send(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.verbose {
		fmt.Fprintf(c.output, "[TX RAW] %s\n", protocol.Hex(data))
	}
	n, err := c.port.Write(data)
	if err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("发送不完整: 期望 %d 字节，实际 %d 字节", len(data), n)
	}
	return nil
}

func (c *SerialClient) SendAndWait(
	ctx context.Context,
	data []byte,
	match func(protocol.Frame) bool,
) (ReceivedFrame, error) {
	ch, unregister := c.registerWaiter(match)
	defer unregister()

	if err := c.Send(data); err != nil {
		return ReceivedFrame{}, err
	}

	select {
	case <-ctx.Done():
		return ReceivedFrame{}, ctx.Err()
	case rec := <-ch:
		if rec.Err != nil {
			return ReceivedFrame{}, rec.Err
		}
		return rec, nil
	}
}

func (c *SerialClient) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.port != nil {
			_ = c.port.Close()
		}
	})
}

func (c *SerialClient) registerWaiter(match func(protocol.Frame) bool) (<-chan ReceivedFrame, func()) {
	if match == nil {
		match = func(protocol.Frame) bool { return true }
	}
	ch := make(chan ReceivedFrame, 1)

	c.waitMu.Lock()
	c.waitSeq++
	id := c.waitSeq
	c.waiters[id] = waiter{match: match, ch: ch}
	c.waitMu.Unlock()

	return ch, func() {
		c.waitMu.Lock()
		delete(c.waiters, id)
		c.waitMu.Unlock()
	}
}

func (c *SerialClient) readLoop() {
	var scanner protocol.Scanner
	buf := make([]byte, 512)
	for {
		select {
		case <-c.done:
			return
		default:
		}

		n, err := c.port.Read(buf)
		if n > 0 {
			frames, errs := scanner.Push(buf[:n])
			for _, parseErr := range errs {
				c.publish(ReceivedFrame{Err: parseErr, Time: time.Now()})
			}
			for _, parsed := range frames {
				rec := ReceivedFrame{
					Frame: parsed.Frame,
					Raw:   parsed.Raw,
					Time:  time.Now(),
				}
				if c.verbose {
					fmt.Fprintf(c.output, "[RX RAW] %s\n", protocol.Hex(rec.Raw))
				}
				c.dispatch(rec)
			}
		}
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			c.publish(ReceivedFrame{Err: fmt.Errorf("读取串口失败: %w", err), Time: time.Now()})
			return
		}
	}
}

func (c *SerialClient) dispatch(rec ReceivedFrame) {
	c.waitMu.Lock()
	for _, waiter := range c.waiters {
		if waiter.match(rec.Frame) {
			select {
			case waiter.ch <- rec:
			default:
			}
		}
	}
	c.waitMu.Unlock()

	c.publish(rec)
}

func (c *SerialClient) publish(rec ReceivedFrame) {
	select {
	case c.frames <- rec:
	default:
	}
}
