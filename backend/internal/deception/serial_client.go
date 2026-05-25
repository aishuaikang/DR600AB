package deception

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dr600ab-api/internal/model"
	"gnss-spoofer/protocol"
)

type receivedFrame struct {
	frame protocol.Frame
	raw   []byte
	err   error
	time  time.Time
}

type frameWaiter struct {
	match func(protocol.Frame) bool
	ch    chan receivedFrame
}

type serialClient struct {
	port           SerialPort
	portName       string
	commandTimeout time.Duration
	locale         string
	record         func(model.DeceptionRecord)

	requestMu sync.Mutex
	writeMu   sync.Mutex
	waitMu    sync.Mutex
	waitSeq   int
	waiters   map[int]frameWaiter

	closeOnce sync.Once
	done      chan struct{}
}

func newSerialClient(
	port SerialPort,
	portName string,
	commandTimeout time.Duration,
	locale string,
	record func(model.DeceptionRecord),
) *serialClient {
	if commandTimeout <= 0 {
		commandTimeout = defaultDeceptionCommandTimeout
	}
	return &serialClient{
		port:           port,
		portName:       portName,
		commandTimeout: commandTimeout,
		locale:         locale,
		record:         record,
		waiters:        make(map[int]frameWaiter),
		done:           make(chan struct{}),
	}
}

func (c *serialClient) Start() {
	go c.readLoop()
}

func (c *serialClient) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.port != nil {
			_ = c.port.Close()
		}
	})
}

func (c *serialClient) SendAndWaitAck(ctx context.Context, command byte, data []byte) (protocol.Ack, error) {
	if len(data) == 0 {
		return protocol.Ack{}, fmt.Errorf("%s", protocol.TextLocale(c.locale, "empty_command_frame"))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	rec, err := c.sendAndWait(timeoutCtx, data, func(frame protocol.Frame) bool {
		return frame.Control == protocol.ControlAck && frame.Command() == command
	})
	if err != nil {
		return protocol.Ack{}, err
	}
	ack, err := protocol.ParseAck(rec.frame)
	if err != nil {
		return protocol.Ack{}, err
	}
	return ack, nil
}

func (c *serialClient) SendQuery(ctx context.Context, query byte) (protocol.Frame, []byte, error) {
	data, err := protocol.BuildQuery(query)
	if err != nil {
		return protocol.Frame{}, nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	rec, err := c.sendAndWait(timeoutCtx, data, func(frame protocol.Frame) bool {
		return frame.Control == protocol.ControlReport && frame.Command() == query
	})
	if err != nil {
		return protocol.Frame{}, nil, err
	}
	return rec.frame, rec.raw, nil
}

func (c *serialClient) SendQueryBurst(ctx context.Context, query byte, idleTimeout time.Duration) ([]protocol.Frame, [][]byte, error) {
	data, err := protocol.BuildQuery(query)
	if err != nil {
		return nil, nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if idleTimeout <= 0 {
		idleTimeout = 100 * time.Millisecond
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, c.commandTimeout)
	defer cancel()

	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	ch, unregister := c.registerWaiter(func(frame protocol.Frame) bool {
		return frame.Control == protocol.ControlReport && frame.Command() == query
	})
	defer unregister()

	if err := c.send(data); err != nil {
		return nil, nil, err
	}

	var frames []protocol.Frame
	var raws [][]byte
	for {
		if len(frames) == 0 {
			select {
			case <-timeoutCtx.Done():
				return nil, nil, timeoutCtx.Err()
			case rec := <-ch:
				if rec.err != nil {
					return nil, nil, rec.err
				}
				frames = append(frames, rec.frame)
				raws = append(raws, rec.raw)
			}
			continue
		}

		idle := time.NewTimer(idleTimeout)
		select {
		case <-timeoutCtx.Done():
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			return frames, raws, nil
		case rec := <-ch:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			if rec.err != nil {
				return nil, nil, rec.err
			}
			frames = append(frames, rec.frame)
			raws = append(raws, rec.raw)
		case <-idle.C:
			return frames, raws, nil
		}
	}
}

func (c *serialClient) sendAndWait(
	ctx context.Context,
	data []byte,
	match func(protocol.Frame) bool,
) (receivedFrame, error) {
	ch, unregister := c.registerWaiter(match)
	defer unregister()

	if err := c.send(data); err != nil {
		return receivedFrame{}, err
	}

	select {
	case <-ctx.Done():
		return receivedFrame{}, ctx.Err()
	case rec := <-ch:
		if rec.err != nil {
			return receivedFrame{}, rec.err
		}
		return rec, nil
	}
}

func (c *serialClient) send(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	n, err := c.port.Write(data)
	c.recordFrame("tx", data, protocol.Frame{}, err)
	if err != nil {
		return fmt.Errorf("%s", protocol.TextLocale(c.locale, "send_failed", protocol.LocalizeErrorText(err.Error(), c.locale)))
	}
	if n != len(data) {
		return fmt.Errorf("%s", protocol.TextLocale(c.locale, "partial_send", len(data), n))
	}
	return nil
}

func (c *serialClient) registerWaiter(match func(protocol.Frame) bool) (<-chan receivedFrame, func()) {
	if match == nil {
		match = func(protocol.Frame) bool { return true }
	}
	ch := make(chan receivedFrame, 128)

	c.waitMu.Lock()
	c.waitSeq++
	id := c.waitSeq
	c.waiters[id] = frameWaiter{match: match, ch: ch}
	c.waitMu.Unlock()

	return ch, func() {
		c.waitMu.Lock()
		delete(c.waiters, id)
		c.waitMu.Unlock()
	}
}

func (c *serialClient) readLoop() {
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
				c.recordFrame("rx", nil, protocol.Frame{}, parseErr)
			}
			for _, parsed := range frames {
				rec := receivedFrame{
					frame: parsed.Frame,
					raw:   parsed.Raw,
					time:  time.Now(),
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
			c.dispatch(receivedFrame{err: fmt.Errorf("%s", protocol.TextLocale(c.locale, "read_failed", protocol.LocalizeErrorText(err.Error(), c.locale))), time: time.Now()})
			return
		}
	}
}

func (c *serialClient) dispatch(rec receivedFrame) {
	if rec.err == nil {
		c.recordFrame("rx", rec.raw, rec.frame, nil)
	} else {
		c.recordFrame("rx", nil, protocol.Frame{}, rec.err)
	}

	c.waitMu.Lock()
	for _, waiter := range c.waiters {
		if rec.err != nil || waiter.match(rec.frame) {
			select {
			case waiter.ch <- rec:
			default:
			}
		}
	}
	c.waitMu.Unlock()
}

func (c *serialClient) recordFrame(direction string, raw []byte, frame protocol.Frame, err error) {
	if c.record == nil {
		return
	}
	record := model.DeceptionRecord{
		Time:      time.Now(),
		Direction: direction,
		RawHex:    protocol.Hex(raw),
	}
	if err != nil {
		record.Error = protocol.LocalizeErrorText(err.Error(), c.locale)
	} else if len(raw) > 0 {
		if frame.Control == 0 && len(frame.Body) == 0 {
			parsed, parseErr := protocol.ParseFrame(raw)
			if parseErr == nil {
				frame = parsed
			}
		}
		record.Command = fmt.Sprintf("0x%02X", frame.Command())
		record.Control = protocol.ControlNameLocale(frame.Control, c.locale)
		record.Description = protocol.DescribeFrameLocale(frame, c.locale)
	}
	c.record(record)
}
