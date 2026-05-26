// Package compass 管理三维电子罗盘串口会话。
package compass

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
)

const (
	DefaultBaudRate      = 9600
	defaultQueryTimeout  = 3 * time.Second
	fallbackPollInterval = 200 * time.Millisecond
	responseBufferSize   = 32
)

// Options 配置三维电子罗盘串口默认值。
type Options struct {
	DefaultBaudRate       int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	QueryTimeout          time.Duration
}

// SerialPort 抽象串口读写能力，便于测试替换。
type SerialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
}

// SerialOpener 根据串口配置打开串口。
type SerialOpener func(cfg *serialport.Config) (SerialPort, error)

// SettingsStore 持久化最近一次三维电子罗盘串口会话请求。
type SettingsStore interface {
	LoadCompass() (model.CompassSessionRequest, bool, error)
	SaveCompass(model.CompassSessionRequest) error
}

// Service 管理三维电子罗盘串口会话和角度记录。
type Service struct {
	mu sync.RWMutex

	store      *store.MemoryStore
	translator *i18n.Translator
	settings   SettingsStore
	options    Options
	openPort   SerialOpener
	current    *session
	sequence   uint64
}

type session struct {
	id             string
	request        model.CompassSessionRequest
	config         serialport.Config
	port           SerialPort
	startedAt      time.Time
	locale         string
	state          string
	autoReconnect  bool
	retryCount     int
	lastError      string
	lastRecord     *model.CompassRecord
	lastPitch      *float64
	lastRoll       *float64
	lastHeading    *float64
	lastRawHex     string
	lastUpdatedAt  *time.Time
	autoOutput     bool
	autoOutputRate int
	ctx            context.Context
	cancel         context.CancelFunc
}

type responseFrame struct {
	raw        []byte
	command    byte
	receivedAt time.Time
}

// NewService 创建三维电子罗盘服务。
func NewService(store *store.MemoryStore, translator *i18n.Translator, settingsStore SettingsStore, options Options) *Service {
	return &Service{
		store:      store,
		translator: translator,
		settings:   settingsStore,
		options:    normalizeOptions(options),
		openPort:   openSerialPort,
	}
}

// SetSerialOpener 替换串口打开函数，主要用于测试。
func (s *Service) SetSerialOpener(open SerialOpener) {
	if open == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openPort = open
}

// Settings 加载已持久化的三维电子罗盘会话请求。
func (s *Service) Settings() (model.CompassSessionRequest, bool, error) {
	if s.settings == nil {
		return model.CompassSessionRequest{}, false, nil
	}
	return s.settings.LoadCompass()
}

// Configured 返回三维电子罗盘串口是否已保存有效配置。
func (s *Service) Configured() (model.CompassSessionRequest, bool, error) {
	settings, ok, err := s.Settings()
	if err != nil || !ok {
		return model.CompassSessionRequest{}, false, err
	}
	return settings, strings.TrimSpace(settings.PortName) != "", nil
}

// ClearSettings 停止当前三维电子罗盘会话并清空已保存的串口设置。
func (s *Service) ClearSettings(locale string) (model.CompassSessionResponse, error) {
	if err := s.saveSettings(model.CompassSessionRequest{}); err != nil {
		return model.CompassSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}
	return s.Stop(locale), nil
}

// Start 保存设置、打开串口并启动三维电子罗盘读取循环。
func (s *Service) Start(req model.CompassSessionRequest, locale string) (model.CompassSessionResponse, error) {
	req = s.normalizeRequest(req)
	if strings.TrimSpace(req.PortName) == "" {
		return model.CompassSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "compass_port_required"))
	}
	req.AutoConnect = true

	if err := s.saveSettings(req); err != nil {
		return model.CompassSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}

	cfg := s.buildConfig(req)

	s.mu.Lock()
	if current := s.current; current != nil && sameRequest(current.request, req) {
		current.locale = locale
		current.autoReconnect = req.AutoConnect
		response := s.responseForSession(current, locale, s.messageForState(current.state, locale))
		s.mu.Unlock()
		return response, nil
	}

	prev := s.current
	ctx, cancel := context.WithCancel(context.Background())
	seq := s.sequence + 1
	s.sequence = seq
	now := time.Now()
	sess := &session{
		id:            fmt.Sprintf("%d", now.UnixNano()),
		request:       req,
		config:        cfg,
		startedAt:     now,
		locale:        locale,
		state:         "connecting",
		autoReconnect: req.AutoConnect,
		ctx:           ctx,
		cancel:        cancel,
	}
	s.current = sess
	s.mu.Unlock()

	if prev != nil {
		prev.cancel()
		if prev.port != nil {
			_ = prev.port.Close()
		}
	}

	port, err := s.connectOnce(&cfg)
	if err == nil {
		if !s.assignConnectedPort(seq, sess, port) {
			_ = port.Close()
			return s.Current(locale), nil
		}
		response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "compass.session.started"))
		s.store.Publish(model.Event{Type: "compass.session.started", Time: time.Now(), Payload: response})
		go s.manageSession(seq, sess, true)
		return response, nil
	}

	s.setSessionFailure(seq, sess, "connecting", err.Error())
	response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "compass.session.connecting"))
	response.LastError = err.Error()
	response.Active = false
	s.store.Publish(model.Event{Type: "compass.session.connecting", Time: time.Now(), Payload: response})
	go s.manageSession(seq, sess, false)
	return response, nil
}

// Stop 关闭当前三维电子罗盘会话并发布停止事件。
func (s *Service) Stop(locale string) model.CompassSessionResponse {
	s.mu.Lock()
	prev := s.current
	s.sequence++
	s.current = nil
	s.mu.Unlock()

	if prev == nil {
		return model.CompassSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "compass.session.inactive"),
		}
	}

	prev.cancel()
	if prev.port != nil {
		_ = prev.port.Close()
	}

	response := s.responseForSession(prev, locale, s.translator.T(locale, "common", "compass.session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "compass.session.stopped", Time: time.Now(), Payload: response})
	return response
}

// Current 返回当前三维电子罗盘会话状态。
func (s *Service) Current(locale string) model.CompassSessionResponse {
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()

	if current == nil {
		return model.CompassSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "compass.session.inactive"),
		}
	}
	return s.responseForSession(current, locale, s.messageForState(current.state, locale))
}

// Records 返回最新三维电子罗盘角度记录。
func (s *Service) Records(limit int) []model.CompassRecord {
	return s.store.ListCompass(limit)
}

// RestoreSavedSettings 在存在已保存设置时自动恢复三维电子罗盘会话。
func (s *Service) RestoreSavedSettings(locale string) {
	if s.settings == nil {
		return
	}
	req, ok, err := s.settings.LoadCompass()
	if err != nil || !ok || !req.AutoConnect || strings.TrimSpace(req.PortName) == "" {
		return
	}
	_, _ = s.Start(req, locale)
}

// Shutdown 释放串口资源。
func (s *Service) Shutdown() {
	_ = s.Stop("zh-CN")
}

func (s *Service) manageSession(seq uint64, sess *session, connected bool) {
	delay := s.options.ReconnectInitialDelay
	if delay <= 0 {
		delay = time.Second
	}

	for {
		if !s.isCurrentSession(seq, sess) {
			return
		}

		if !connected {
			port, err := s.connectOnce(&sess.config)
			if err != nil {
				state := sess.state
				if state == "" {
					state = "connecting"
				}
				s.setSessionFailure(seq, sess, state, err.Error())
				if !s.isCurrentSession(seq, sess) {
					return
				}
				response := s.responseForSession(sess, sess.locale, s.messageForState(state, sess.locale))
				response.LastError = err.Error()
				response.RetryCount = sess.retryCount
				response.Active = false
				s.store.Publish(model.Event{
					Type:    sessionEventType(state),
					Time:    time.Now(),
					Payload: response,
				})
				if !s.sleepOrDone(sess.ctx, delay) {
					return
				}
				delay = s.nextBackoff(delay)
				continue
			}

			if !s.assignConnectedPort(seq, sess, port) {
				_ = port.Close()
				return
			}
			delay = s.options.ReconnectInitialDelay
			connected = true
			response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "compass.session.started"))
			s.store.Publish(model.Event{Type: "compass.session.started", Time: time.Now(), Payload: response})
		}

		err := s.runConnected(seq, sess)
		if !s.isCurrentSession(seq, sess) {
			return
		}
		if !sess.autoReconnect {
			s.finalizeStopped(seq, sess)
			return
		}

		lastErr := s.translator.T(sess.locale, "common", "compass.session.disconnected")
		if err != nil && !isClosedError(err) {
			lastErr = err.Error()
		}
		s.setSessionFailure(seq, sess, "reconnecting", lastErr)
		response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "compass.session.reconnecting"))
		response.LastError = sess.lastError
		response.RetryCount = sess.retryCount
		response.Active = false
		s.store.Publish(model.Event{Type: "compass.session.reconnecting", Time: time.Now(), Payload: response})
		connected = false
		if !s.sleepOrDone(sess.ctx, delay) {
			return
		}
		delay = s.nextBackoff(delay)
	}
}

func (s *Service) runConnected(seq uint64, sess *session) error {
	s.mu.RLock()
	port := sess.port
	s.mu.RUnlock()
	if port == nil {
		return io.ErrClosedPipe
	}
	defer func() {
		_ = port.Close()
	}()

	respCh := make(chan responseFrame, responseBufferSize)
	readErrCh := make(chan error, 1)
	readCtx, readCancel := context.WithCancel(sess.ctx)
	defer readCancel()

	go s.readLoop(readCtx, seq, sess, port, respCh, readErrCh)

	autoOutputEnabled := false
	if err := s.configureAutoOutputRate(sess.ctx, port, respCh, autoOutputRate25Hz); err == nil {
		autoOutputEnabled = true
		s.setAutoOutput(seq, sess, true, int(autoOutputRate25Hz), "")
	} else {
		s.setAutoOutput(seq, sess, false, 0, err.Error())
	}

	pollTicker := time.NewTicker(fallbackPollInterval)
	defer pollTicker.Stop()
	if !autoOutputEnabled {
		_ = s.queryPitchRollHeading(sess.ctx, port, respCh)
	}

	for {
		select {
		case <-sess.ctx.Done():
			return nil
		case err := <-readErrCh:
			return err
		case <-pollTicker.C:
			if autoOutputEnabled {
				continue
			}
			if err := s.queryPitchRollHeading(sess.ctx, port, respCh); err != nil {
				s.setSessionLastError(seq, sess, err.Error())
			}
		}
	}
}

func (s *Service) configureAutoOutputRate(ctx context.Context, port SerialPort, respCh <-chan responseFrame, rate byte) error {
	cmd, err := buildSetAutoOutputRateCmd(defaultDeviceAddr, rate)
	if err != nil {
		return err
	}
	sentAt := time.Now()
	if err := sendCommand(port, cmd); err != nil {
		return err
	}
	raw, err := waitResponse(ctx, respCh, commandSetAutoOutputRateResp, sentAt, s.options.QueryTimeout)
	if err != nil {
		return err
	}
	resp, err := parseSetAutoOutputRateResponse(raw)
	if err != nil {
		return err
	}
	if !resp.success {
		return fmt.Errorf("set auto output rate rejected by device")
	}
	return nil
}

func (s *Service) queryPitchRollHeading(ctx context.Context, port SerialPort, respCh <-chan responseFrame) error {
	cmd := buildReadPitchRollHeadingCmd(defaultDeviceAddr)
	sentAt := time.Now()
	if err := sendCommand(port, cmd); err != nil {
		return err
	}
	raw, err := waitResponse(ctx, respCh, commandPitchRollHeadingResp, sentAt, s.options.QueryTimeout)
	if err != nil {
		return err
	}
	_, err = parsePitchRollHeadingResponse(raw)
	return err
}

func (s *Service) readLoop(
	ctx context.Context,
	seq uint64,
	sess *session,
	port SerialPort,
	respCh chan<- responseFrame,
	errCh chan<- error,
) {
	for {
		if ctx.Err() != nil {
			return
		}

		raw, err := readFrame(ctx, port)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case errCh <- err:
			default:
			}
			return
		}

		parsed, err := parseFrame(raw)
		if err != nil {
			continue
		}
		resp := responseFrame{
			raw:        append([]byte(nil), raw...),
			command:    parsed.command,
			receivedAt: time.Now(),
		}

		if parsed.command == commandPitchRollHeadingResp {
			if err := s.ingestPitchRollHeading(seq, sess, raw, resp.receivedAt); err != nil {
				s.setSessionLastError(seq, sess, err.Error())
			}
		}

		select {
		case respCh <- resp:
		default:
		}
	}
}

func (s *Service) ingestPitchRollHeading(seq uint64, sess *session, raw []byte, receivedAt time.Time) error {
	resp, err := parsePitchRollHeadingResponse(raw)
	if err != nil {
		return err
	}
	record := model.CompassRecord{
		SessionID:  sess.id,
		PortName:   sess.config.PortName,
		ReceivedAt: receivedAt,
		Pitch:      resp.pitch,
		Roll:       resp.roll,
		Heading:    resp.heading,
		RawHex:     formatHex(raw),
	}
	s.store.AddCompass(record)
	s.updateLastRecord(seq, sess, record)
	return nil
}

func sendCommand(port SerialPort, data []byte) error {
	n, err := port.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("partial send: wrote %d of %d bytes", n, len(data))
	}
	return nil
}

func waitResponse(
	ctx context.Context,
	respCh <-chan responseFrame,
	command byte,
	sentAt time.Time,
	timeout time.Duration,
) ([]byte, error) {
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, context.DeadlineExceeded
		case resp := <-respCh:
			if resp.command == command && !resp.receivedAt.Before(sentAt) {
				return append([]byte(nil), resp.raw...), nil
			}
		}
	}
}

func readFrame(ctx context.Context, reader io.Reader) ([]byte, error) {
	for {
		start, err := readByte(ctx, reader)
		if err != nil {
			return nil, err
		}
		if start != frameStart {
			continue
		}

		length, err := readByte(ctx, reader)
		if err != nil {
			return nil, err
		}
		if length < 4 || length > maxFrameLength {
			continue
		}

		raw := []byte{start, length}
		remaining := int(length) - 1
		for range remaining {
			value, err := readByte(ctx, reader)
			if err != nil {
				return nil, err
			}
			raw = append(raw, value)
		}
		if _, err := parseFrame(raw); err != nil {
			continue
		}
		return raw, nil
	}
}

func readByte(ctx context.Context, reader io.Reader) (byte, error) {
	buf := []byte{0}
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		n, err := reader.Read(buf)
		if n > 0 {
			return buf[0], nil
		}
		if err != nil {
			return 0, err
		}
	}
}

func (s *Service) finalizeStopped(seq uint64, sess *session) {
	s.mu.Lock()
	if s.sequence != seq || s.current != sess {
		s.mu.Unlock()
		return
	}
	prev := s.current
	s.current = nil
	s.mu.Unlock()

	response := s.responseForSession(prev, sess.locale, s.translator.T(sess.locale, "common", "compass.session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "compass.session.stopped", Time: time.Now(), Payload: response})
}

func (s *Service) responseForSession(sess *session, locale, message string) model.CompassSessionResponse {
	if sess == nil {
		return model.CompassSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: message,
		}
	}

	active := sess.state == "connected"
	return model.CompassSessionResponse{
		Active:         active,
		SessionID:      sess.id,
		PortName:       sess.config.PortName,
		BaudRate:       sess.config.BaudRate,
		DataBits:       sess.config.DataBits,
		StopBits:       sess.config.StopBits,
		Parity:         sess.config.Parity,
		StartedAt:      sess.startedAt,
		State:          sess.state,
		AutoReconnect:  sess.autoReconnect,
		LastError:      sess.lastError,
		RetryCount:     sess.retryCount,
		LastRecord:     cloneCompassRecord(sess.lastRecord),
		LastPitch:      cloneFloat64Ptr(sess.lastPitch),
		LastRoll:       cloneFloat64Ptr(sess.lastRoll),
		LastHeading:    cloneFloat64Ptr(sess.lastHeading),
		LastRawHex:     sess.lastRawHex,
		LastUpdatedAt:  cloneTimePtr(sess.lastUpdatedAt),
		AutoOutput:     sess.autoOutput,
		AutoOutputRate: sess.autoOutputRate,
		Message:        message,
	}
}

func (s *Service) messageForState(state, locale string) string {
	switch state {
	case "connected":
		return s.translator.T(locale, "common", "compass.session.started")
	case "connecting":
		return s.translator.T(locale, "common", "compass.session.connecting")
	case "reconnecting":
		return s.translator.T(locale, "common", "compass.session.reconnecting")
	case "inactive":
		return s.translator.T(locale, "common", "compass.session.inactive")
	default:
		return s.translator.T(locale, "common", "compass.session.inactive")
	}
}

func (s *Service) connectOnce(cfg *serialport.Config) (SerialPort, error) {
	s.mu.RLock()
	openPort := s.openPort
	s.mu.RUnlock()
	return openPort(cfg)
}

func (s *Service) assignConnectedPort(seq uint64, sess *session, port SerialPort) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return false
	}
	sess.port = port
	sess.state = "connected"
	sess.retryCount = 0
	sess.lastError = ""
	return true
}

func (s *Service) setSessionFailure(seq uint64, sess *session, state, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	sess.state = state
	sess.lastError = lastErr
	sess.retryCount++
	sess.port = nil
}

func (s *Service) setSessionLastError(seq uint64, sess *session, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	sess.lastError = lastErr
}

func (s *Service) setAutoOutput(seq uint64, sess *session, enabled bool, rate int, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	sess.autoOutput = enabled
	sess.autoOutputRate = rate
	if lastErr != "" {
		sess.lastError = lastErr
	}
}

func (s *Service) updateLastRecord(seq uint64, sess *session, record model.CompassRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	next := record
	sess.lastRecord = &next
	sess.lastPitch = float64Ptr(record.Pitch)
	sess.lastRoll = float64Ptr(record.Roll)
	sess.lastHeading = float64Ptr(record.Heading)
	sess.lastRawHex = record.RawHex
	updatedAt := record.ReceivedAt
	sess.lastUpdatedAt = &updatedAt
	sess.lastError = ""
}

func (s *Service) isCurrentSession(seq uint64, sess *session) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence == seq && s.current == sess
}

func (s *Service) saveSettings(req model.CompassSessionRequest) error {
	if s.settings == nil {
		return nil
	}
	return s.settings.SaveCompass(req)
}

func (s *Service) buildConfig(req model.CompassSessionRequest) serialport.Config {
	cfg := serialport.Config{
		PortName:    strings.TrimSpace(req.PortName),
		BaudRate:    firstNonZero(req.BaudRate, s.options.DefaultBaudRate),
		DataBits:    firstNonZero(req.DataBits, s.options.DefaultDataBits),
		StopBits:    firstNonZero(req.StopBits, s.options.DefaultStopBits),
		Parity:      strings.TrimSpace(req.Parity),
		ReadTimeout: s.options.DefaultReadTimeout,
	}
	if cfg.Parity == "" {
		cfg.Parity = s.options.DefaultParity
	}
	if req.ReadTimeoutMs > 0 {
		cfg.ReadTimeout = time.Duration(req.ReadTimeoutMs) * time.Millisecond
	}
	return cfg
}

func (s *Service) normalizeRequest(req model.CompassSessionRequest) model.CompassSessionRequest {
	req.PortName = strings.TrimSpace(req.PortName)
	req.AutoConnect = true
	req.BaudRate = s.options.DefaultBaudRate
	if req.DataBits == 0 {
		req.DataBits = s.options.DefaultDataBits
	}
	if req.StopBits == 0 {
		req.StopBits = s.options.DefaultStopBits
	}
	if strings.TrimSpace(req.Parity) == "" {
		req.Parity = s.options.DefaultParity
	}
	if req.ReadTimeoutMs == 0 && s.options.DefaultReadTimeout > 0 {
		req.ReadTimeoutMs = int(s.options.DefaultReadTimeout / time.Millisecond)
	}
	return req
}

func (s *Service) nextBackoff(current time.Duration) time.Duration {
	maxDelay := s.options.ReconnectMaxDelay
	if maxDelay <= 0 {
		maxDelay = 15 * time.Second
	}
	if current <= 0 {
		return s.options.ReconnectInitialDelay
	}
	next := current * 2
	if next > maxDelay {
		return maxDelay
	}
	return next
}

func (s *Service) sleepOrDone(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func openSerialPort(cfg *serialport.Config) (SerialPort, error) {
	return serialport.Open(cfg)
}

func normalizeOptions(options Options) Options {
	if options.DefaultBaudRate == 0 {
		options.DefaultBaudRate = DefaultBaudRate
	}
	if options.DefaultDataBits == 0 {
		options.DefaultDataBits = 8
	}
	if options.DefaultStopBits == 0 {
		options.DefaultStopBits = 1
	}
	if strings.TrimSpace(options.DefaultParity) == "" {
		options.DefaultParity = "none"
	}
	if options.DefaultReadTimeout <= 0 {
		options.DefaultReadTimeout = time.Second
	}
	if options.ReconnectInitialDelay <= 0 {
		options.ReconnectInitialDelay = time.Second
	}
	if options.ReconnectMaxDelay <= 0 {
		options.ReconnectMaxDelay = 15 * time.Second
	}
	if options.QueryTimeout <= 0 {
		options.QueryTimeout = defaultQueryTimeout
	}
	return options
}

func sameRequest(a, b model.CompassSessionRequest) bool {
	return a.PortName == b.PortName &&
		a.BaudRate == b.BaudRate &&
		a.DataBits == b.DataBits &&
		a.StopBits == b.StopBits &&
		strings.TrimSpace(a.Parity) == strings.TrimSpace(b.Parity) &&
		a.ReadTimeoutMs == b.ReadTimeoutMs &&
		a.AutoConnect == b.AutoConnect
}

func sessionEventType(state string) string {
	switch state {
	case "connecting":
		return "compass.session.connecting"
	case "reconnecting":
		return "compass.session.reconnecting"
	case "connected":
		return "compass.session.started"
	case "inactive":
		return "compass.session.stopped"
	default:
		return "compass.session.connecting"
	}
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "use of closed") ||
		strings.Contains(lower, "file already closed") ||
		strings.Contains(lower, "port closed")
}

func cloneCompassRecord(record *model.CompassRecord) *model.CompassRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	return &cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func float64Ptr(value float64) *float64 {
	return &value
}

func firstNonZero(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func formatHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	dst := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(dst, data)

	var builder strings.Builder
	builder.Grow(len(data)*3 - 1)
	for i := 0; i < len(dst); i += 2 {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteByte(toUpperHex(dst[i]))
		builder.WriteByte(toUpperHex(dst[i+1]))
	}
	return builder.String()
}

func toUpperHex(value byte) byte {
	if value >= 'a' && value <= 'f' {
		return value - ('a' - 'A')
	}
	return value
}
