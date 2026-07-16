// Package gps 管理 GPS NMEA 0183 串口会话。
package gps

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
	"tri-detector/client"
)

const (
	startGPSCommand           = "AT+QGPS=1\r\n"
	startGPSDataCommand       = "AT+QNETDEVCTL=1,1,1\r\n"
	startGPSPowerCommand      = "at+qgpspower=1\r\n"
	startGPSDataCommandDelay  = 2 * time.Second
	startGPSPowerCommandDelay = 3 * time.Second
)

type gpsStartupCommand struct {
	command    string
	delayAfter time.Duration
}

type gpsConnectOptions struct {
	controlPortName     string
	locale              string
	sendStartupCommands bool
}

var gpsStartupCommands = [...]gpsStartupCommand{
	{command: startGPSCommand, delayAfter: startGPSDataCommandDelay},
	{command: startGPSDataCommand, delayAfter: startGPSPowerCommandDelay},
	{command: startGPSPowerCommand},
}

// Options 配置 GPS 串口默认值和重连时间参数。
type Options struct {
	DefaultBaudRate       int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
}

// SerialOpener 根据串口配置打开串口。
type SerialOpener func(cfg *serialport.Config) (serial.Port, error)

// SettingsStore 持久化最近一次 GPS 会话请求。
type SettingsStore interface {
	LoadGPS() (model.GPSSessionRequest, bool, error)
	SaveGPS(model.GPSSessionRequest) error
}

// Service 管理 GPS 串口会话并存储 NMEA 记录。
type Service struct {
	mu sync.RWMutex

	store                  *store.MemoryStore
	translator             *i18n.Translator
	settings               SettingsStore
	options                Options
	openPort               SerialOpener
	waitStartupDelay       func(context.Context, time.Duration) bool
	initializedControlPort string
	current                *session
	sequence               uint64
}

type session struct {
	id            string
	request       model.GPSSessionRequest
	config        serialport.Config
	controlPort   string
	client        *client.SerialClient
	startedAt     time.Time
	locale        string
	state         string
	autoReconnect bool
	retryCount    int
	lastError     string
	lastNMEA      string
	lastFix       *model.GPSFix
	lastRecord    *model.GPSRecord
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewService 创建 GPS 服务。
func NewService(store *store.MemoryStore, translator *i18n.Translator, settingsStore SettingsStore, options Options) *Service {
	return &Service{
		store:            store,
		translator:       translator,
		settings:         settingsStore,
		options:          normalizeOptions(options),
		openPort:         serialport.Open,
		waitStartupDelay: waitForContext,
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

// Settings 加载已持久化的 GPS 会话请求。
func (s *Service) Settings() (model.GPSSessionRequest, bool, error) {
	if s.settings == nil {
		return model.GPSSessionRequest{}, false, nil
	}
	return s.settings.LoadGPS()
}

// ClearSettings 停止当前 GPS 会话并清空已保存的串口设置。
func (s *Service) ClearSettings(locale string) (model.GPSSessionResponse, error) {
	if err := s.saveSettings(model.GPSSessionRequest{}); err != nil {
		return model.GPSSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}
	return s.Stop(locale), nil
}

// Start 保存设置、打开串口并启动 GPS NMEA 读取循环。
func (s *Service) Start(req model.GPSSessionRequest, locale string) (model.GPSSessionResponse, error) {
	req = s.normalizeRequest(req)
	dataPortName, controlPortName := s.resolvePortNames(req)
	if dataPortName == "" {
		return model.GPSSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "gps_data_port_required"))
	}
	if controlPortName == "" {
		return model.GPSSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "gps_control_port_required"))
	}
	req.PortName = dataPortName
	req.DataPortName = dataPortName
	req.ControlPortName = controlPortName
	req.AutoConnect = true

	if err := s.saveSettings(req); err != nil {
		return model.GPSSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}

	cfg := s.buildConfig(req, dataPortName)

	s.mu.Lock()
	if current := s.current; current != nil && sameRequest(current.request, req) {
		current.locale = locale
		current.autoReconnect = req.AutoConnect
		response := s.responseForSession(current, locale, s.messageForState(current.state, locale))
		s.mu.Unlock()
		return response, nil
	}

	prev := s.current
	sendStartupCommands := s.initializedControlPort != controlPortName
	ctx, cancel := context.WithCancel(context.Background())
	seq := s.sequence + 1
	s.sequence = seq
	now := time.Now()
	sess := &session{
		id:            fmt.Sprintf("%d", now.UnixNano()),
		request:       req,
		config:        cfg,
		controlPort:   controlPortName,
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
		if prev.client != nil {
			prev.client.Close()
		}
	}

	gpsClient, startupCommandsSent, err := s.connectOnce(
		sess.ctx,
		&sess.config,
		gpsConnectOptions{
			controlPortName:     sess.controlPort,
			locale:              locale,
			sendStartupCommands: sendStartupCommands,
		},
	)
	if startupCommandsSent {
		s.markStartupCommandsSent(seq, sess)
	}
	if err == nil {
		if !s.assignConnectedClient(seq, sess, gpsClient) {
			gpsClient.Close()
			return s.Current(locale), nil
		}

		response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "gps.session.started"))
		s.store.Publish(model.Event{Type: "gps.session.started", Time: time.Now(), Payload: response})
		go s.manageSession(seq, sess, true)
		return response, nil
	}

	if !s.setSessionFailure(seq, sess, "connecting", err.Error()) {
		return s.Current(locale), nil
	}
	response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "gps.session.connecting"))
	response.LastError = err.Error()
	response.Active = false
	s.store.Publish(model.Event{Type: "gps.session.connecting", Time: time.Now(), Payload: response})
	go s.manageSession(seq, sess, false)
	return response, nil
}

// Stop 关闭当前 GPS 会话并发布停止事件。
func (s *Service) Stop(locale string) model.GPSSessionResponse {
	s.mu.Lock()
	prev := s.current
	s.sequence++
	s.current = nil
	s.mu.Unlock()

	if prev == nil {
		return model.GPSSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "gps.session.inactive"),
		}
	}

	prev.cancel()
	if prev.client != nil {
		prev.client.Close()
	}

	response := s.responseForSession(prev, locale, s.translator.T(locale, "common", "gps.session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "gps.session.stopped", Time: time.Now(), Payload: response})
	return response
}

// Current 返回当前 GPS 会话状态，并按语言本地化提示文本。
func (s *Service) Current(locale string) model.GPSSessionResponse {
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()

	if current == nil {
		return model.GPSSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "gps.session.inactive"),
		}
	}
	return s.responseForSession(current, locale, s.messageForState(current.state, locale))
}

// LatestFix 返回当前会话最后一次有效 GPS 定位的副本。
func (s *Service) LatestFix() (*model.GPSFix, *time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil || s.current.lastFix == nil {
		return nil, nil
	}

	fix := *s.current.lastFix
	var updatedAt *time.Time
	if s.current.lastRecord != nil {
		value := s.current.lastRecord.ReceivedAt
		updatedAt = &value
	}
	return &fix, updatedAt
}

// Records 返回最新 GPS NMEA 记录。
func (s *Service) Records(limit int) []model.GPSRecord {
	return s.store.ListGPS(limit)
}

// RestoreSavedSettings 在存在已保存设置时自动恢复 GPS 会话。
func (s *Service) RestoreSavedSettings(locale string) {
	if s.settings == nil {
		return
	}
	req, ok, err := s.settings.LoadGPS()
	if err != nil || !ok {
		return
	}
	dataPortName, controlPortName := s.resolvePortNames(req)
	if !req.AutoConnect || dataPortName == "" || controlPortName == "" {
		return
	}
	_, _ = s.Start(req, locale)
}

// IngestLine 保存一条 NMEA 0183 数据。
func (s *Service) IngestLine(sessionID, portName, line string) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return
	}

	record := model.GPSRecord{
		SessionID:  sessionID,
		PortName:   portName,
		ReceivedAt: time.Now(),
		Type:       nmeaSentenceType(raw),
		Raw:        raw,
		Fix:        parseNMEA(raw),
		Details:    parseNMEADetails(raw),
	}
	s.store.AddGPS(record)
	s.updateLastRecord(sessionID, record)
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
			sendStartupCommands := s.shouldSendStartupCommands(seq, sess)
			gpsClient, sent, err := s.connectOnce(
				sess.ctx,
				&sess.config,
				gpsConnectOptions{
					controlPortName:     sess.controlPort,
					locale:              sess.locale,
					sendStartupCommands: sendStartupCommands,
				},
			)
			if sent {
				s.markStartupCommandsSent(seq, sess)
			}
			if err != nil {
				state := sess.state
				if state == "" {
					state = "connecting"
				}
				if !s.setSessionFailure(seq, sess, state, err.Error()) {
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

			if !s.assignConnectedClient(seq, sess, gpsClient) {
				gpsClient.Close()
				return
			}

			delay = s.options.ReconnectInitialDelay
			connected = true
			response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "gps.session.started"))
			s.store.Publish(model.Event{Type: "gps.session.started", Time: time.Now(), Payload: response})
		}

		sess.client.ReadLoop(func(line string) {
			s.IngestLine(sess.id, sess.config.PortName, line)
		})
		sess.client.Close()
		sess.client = nil

		if !s.isCurrentSession(seq, sess) {
			return
		}
		if !sess.autoReconnect {
			s.finalizeStopped(seq, sess)
			return
		}

		if !s.setSessionFailure(
			seq,
			sess,
			"reconnecting",
			s.translator.T(sess.locale, "common", "gps.session.disconnected"),
		) {
			return
		}
		response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "gps.session.reconnecting"))
		response.LastError = sess.lastError
		response.RetryCount = sess.retryCount
		response.Active = false
		s.store.Publish(model.Event{Type: "gps.session.reconnecting", Time: time.Now(), Payload: response})
		connected = false
		if !s.sleepOrDone(sess.ctx, delay) {
			return
		}
		delay = s.nextBackoff(delay)
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

	response := s.responseForSession(prev, sess.locale, s.translator.T(sess.locale, "common", "gps.session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "gps.session.stopped", Time: time.Now(), Payload: response})
}

func (s *Service) messageForState(state, locale string) string {
	switch state {
	case "connected":
		return s.translator.T(locale, "common", "gps.session.started")
	case "connecting":
		return s.translator.T(locale, "common", "gps.session.connecting")
	case "reconnecting":
		return s.translator.T(locale, "common", "gps.session.reconnecting")
	case "inactive":
		return s.translator.T(locale, "common", "gps.session.inactive")
	default:
		return s.translator.T(locale, "common", "gps.session.inactive")
	}
}

func (s *Service) responseForSession(sess *session, locale, message string) model.GPSSessionResponse {
	if sess == nil {
		return model.GPSSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: message,
		}
	}

	active := sess.state == "connected"
	return model.GPSSessionResponse{
		Active:          active,
		SessionID:       sess.id,
		PortName:        sess.config.PortName,
		DataPortName:    sess.config.PortName,
		ControlPortName: sess.controlPort,
		BaudRate:        sess.config.BaudRate,
		DataBits:        sess.config.DataBits,
		StopBits:        sess.config.StopBits,
		Parity:          sess.config.Parity,
		StartedAt:       sess.startedAt,
		State:           sess.state,
		AutoReconnect:   sess.autoReconnect,
		LastError:       sess.lastError,
		RetryCount:      sess.retryCount,
		LastNMEA:        sess.lastNMEA,
		LastFix:         sess.lastFix,
		LastRecord:      sess.lastRecord,
		Message:         message,
	}
}

func (s *Service) connectOnce(
	ctx context.Context,
	cfg *serialport.Config,
	options gpsConnectOptions,
) (*client.SerialClient, bool, error) {
	s.mu.RLock()
	openPort := s.openPort
	s.mu.RUnlock()

	if options.controlPortName == "" || options.controlPortName == cfg.PortName {
		readPort, err := openPort(cfg)
		if err != nil {
			return nil, false, err
		}
		gpsClient := client.NewSerialClient(readPort, cfg.PortName, false)
		if options.sendStartupCommands {
			if err := sendGPSStartCommands(ctx, readPort, s.waitStartupDelay); err != nil {
				gpsClient.Close()
				return nil, false, fmt.Errorf(
					"%s: %w",
					s.translator.T(options.locale, "errors", "gps_start_command_failed"),
					err,
				)
			}
		}
		return gpsClient, options.sendStartupCommands, nil
	}

	controlCfg := *cfg
	controlCfg.PortName = options.controlPortName
	startupCommandsSent := false
	if options.sendStartupCommands {
		if err := s.sendGPSStartCommands(ctx, openPort, &controlCfg); err != nil {
			return nil, false, fmt.Errorf(
				"%s: %w",
				s.translator.T(options.locale, "errors", "gps_start_command_failed"),
				err,
			)
		}
		startupCommandsSent = true
	}

	readPort, err := openPort(cfg)
	if err != nil {
		return nil, startupCommandsSent, err
	}
	return client.NewSerialClient(readPort, cfg.PortName, false), startupCommandsSent, nil
}

func (s *Service) sendGPSStartCommands(
	ctx context.Context,
	openPort SerialOpener,
	cfg *serialport.Config,
) error {
	port, err := openPort(cfg)
	if err != nil {
		return err
	}
	defer port.Close()
	return sendGPSStartCommands(ctx, port, s.waitStartupDelay)
}

func sendGPSStartCommands(
	ctx context.Context,
	port serial.Port,
	wait func(context.Context, time.Duration) bool,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("GPS 启动已取消: %w", err)
	}
	if err := port.ResetInputBuffer(); err != nil {
		return fmt.Errorf("清空输入缓冲失败: %w", err)
	}
	if err := port.ResetOutputBuffer(); err != nil {
		return fmt.Errorf("清空输出缓冲失败: %w", err)
	}
	if wait == nil {
		wait = waitForContext
	}
	for _, step := range gpsStartupCommands {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("GPS 启动已取消: %w", err)
		}
		n, err := port.Write([]byte(step.command))
		if err != nil {
			return fmt.Errorf("发送 GPS 指令 %q 失败: %w", strings.TrimSpace(step.command), err)
		}
		if n != len(step.command) {
			return fmt.Errorf(
				"GPS 指令 %q 发送不完整: 期望%d字节, 实际%d字节",
				strings.TrimSpace(step.command),
				len(step.command),
				n,
			)
		}
		if err := port.Drain(); err != nil {
			return fmt.Errorf("等待 GPS 指令 %q 发送完成失败: %w", strings.TrimSpace(step.command), err)
		}
		if step.delayAfter > 0 && !wait(ctx, step.delayAfter) {
			err := ctx.Err()
			if err == nil {
				err = context.Canceled
			}
			return fmt.Errorf("GPS 启动已取消: %w", err)
		}
	}
	return nil
}

func (s *Service) assignConnectedClient(seq uint64, sess *session, c *client.SerialClient) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return false
	}
	sess.client = c
	sess.state = "connected"
	sess.retryCount = 0
	sess.lastError = ""
	return true
}

func (s *Service) setSessionFailure(seq uint64, sess *session, state, lastErr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return false
	}
	sess.state = state
	sess.lastError = lastErr
	sess.retryCount++
	return true
}

func (s *Service) updateLastRecord(sessionID string, record model.GPSRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil || s.current.id != sessionID {
		return
	}
	s.current.lastNMEA = record.Raw
	if record.Fix != nil {
		s.current.lastFix = record.Fix
	}
	next := record
	s.current.lastRecord = &next
}

func (s *Service) isCurrentSession(seq uint64, sess *session) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence == seq && s.current == sess
}

func (s *Service) shouldSendStartupCommands(seq uint64, sess *session) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence == seq &&
		s.current == sess &&
		s.initializedControlPort != sess.controlPort
}

func (s *Service) markStartupCommandsSent(seq uint64, sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	s.initializedControlPort = sess.controlPort
}

func (s *Service) saveSettings(req model.GPSSessionRequest) error {
	if s.settings == nil {
		return nil
	}
	return s.settings.SaveGPS(req)
}

func (s *Service) buildConfig(req model.GPSSessionRequest, dataPortName string) serialport.Config {
	cfg := serialport.Config{
		PortName:    dataPortName,
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

func (s *Service) normalizeRequest(req model.GPSSessionRequest) model.GPSSessionRequest {
	req.AutoConnect = true
	if req.BaudRate == 0 {
		req.BaudRate = s.options.DefaultBaudRate
	}
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

func (s *Service) resolvePortNames(req model.GPSSessionRequest) (string, string) {
	dataPortName := strings.TrimSpace(req.DataPortName)
	if dataPortName == "" {
		dataPortName = strings.TrimSpace(req.PortName)
	}

	controlPortName := strings.TrimSpace(req.ControlPortName)
	if controlPortName == "" {
		controlPortName = dataPortName
	}

	return dataPortName, controlPortName
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
	return waitForContext(ctx, delay)
}

func waitForContext(ctx context.Context, delay time.Duration) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if delay <= 0 {
		return true
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

func sameRequest(a, b model.GPSSessionRequest) bool {
	return a.PortName == b.PortName &&
		a.DataPortName == b.DataPortName &&
		a.ControlPortName == b.ControlPortName &&
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
		return "gps.session.connecting"
	case "reconnecting":
		return "gps.session.reconnecting"
	case "connected":
		return "gps.session.started"
	case "inactive":
		return "gps.session.stopped"
	default:
		return "gps.session.connecting"
	}
}

func nmeaSentenceType(raw string) string {
	raw = strings.TrimPrefix(raw, "$")
	if len(raw) < 5 {
		return "NMEA"
	}
	if index := strings.Index(raw, ","); index > 0 {
		raw = raw[:index]
	}
	if star := strings.Index(raw, "*"); star > 0 {
		raw = raw[:star]
	}
	if len(raw) >= 3 {
		return raw[len(raw)-3:]
	}
	return raw
}

func parseNMEA(raw string) *model.GPSFix {
	if !hasValidNMEAChecksum(raw) {
		return nil
	}
	fields := splitNMEAFields(raw)
	if len(fields) == 0 {
		return nil
	}

	switch nmeaSentenceType(raw) {
	case "GGA":
		return parseGGA(fields)
	case "RMC":
		return parseRMC(fields)
	default:
		return nil
	}
}

func hasValidNMEAChecksum(raw string) bool {
	details := &model.NMEADetails{}
	parseNMEAChecksum(raw, details)
	return details.ChecksumValid == nil || *details.ChecksumValid
}

func parseNMEADetails(raw string) *model.NMEADetails {
	fields := splitNMEAFields(raw)
	if len(fields) == 0 {
		return nil
	}

	header := strings.TrimSpace(fields[0])
	sentence := nmeaSentenceType(raw)
	talker := ""
	if len(header) > len(sentence) {
		talker = header[:len(header)-len(sentence)]
	}
	details := &model.NMEADetails{
		Talker:       talker,
		Sentence:     sentence,
		SatelliteIDs: []string{},
		Satellites:   []model.NMEASatellite{},
	}
	parseNMEAChecksum(raw, details)

	switch sentence {
	case "RMC":
		parseRMCDetails(fields, details)
	case "GGA":
		parseGGADetails(fields, details)
	case "GSA":
		parseGSADetails(fields, details)
	case "GSV":
		parseGSVDetails(fields, details)
	case "VTG":
		parseVTGDetails(fields, details)
	}
	return details
}

func parseRMCDetails(fields []string, details *model.NMEADetails) {
	details.UTCTime = fieldAt(fields, 1)
	details.Status = fieldAt(fields, 2)
	details.Latitude, details.Longitude = parseCoordinatePointers(
		fieldAt(fields, 3),
		fieldAt(fields, 4),
		fieldAt(fields, 5),
		fieldAt(fields, 6),
	)
	details.SpeedKnots = parseOptionalFloat(fieldAt(fields, 7))
	details.CourseTrue = parseOptionalFloat(fieldAt(fields, 8))
	details.UTCDate = fieldAt(fields, 9)
	details.Mode = fieldAt(fields, 12)
	details.NavigationStatus = fieldAt(fields, 13)
}

func parseGGADetails(fields []string, details *model.NMEADetails) {
	details.UTCTime = fieldAt(fields, 1)
	details.Latitude, details.Longitude = parseCoordinatePointers(
		fieldAt(fields, 2),
		fieldAt(fields, 3),
		fieldAt(fields, 4),
		fieldAt(fields, 5),
	)
	details.FixQuality = parseOptionalInt(fieldAt(fields, 6))
	details.SatellitesUsed = parseOptionalInt(fieldAt(fields, 7))
	details.HDOP = parseOptionalFloat(fieldAt(fields, 8))
	details.AltitudeM = parseOptionalFloat(fieldAt(fields, 9))
	details.GeoidSeparationM = parseOptionalFloat(fieldAt(fields, 11))
	details.DifferentialAgeSec = parseOptionalFloat(fieldAt(fields, 13))
	details.DifferentialStation = fieldAt(fields, 14)
}

func parseGSADetails(fields []string, details *model.NMEADetails) {
	details.Mode = fieldAt(fields, 1)
	details.FixType = parseOptionalInt(fieldAt(fields, 2))
	for index := 3; index <= 14; index++ {
		if id := fieldAt(fields, index); id != "" {
			details.SatelliteIDs = append(details.SatelliteIDs, id)
		}
	}
	details.PDOP = parseOptionalFloat(fieldAt(fields, 15))
	details.HDOP = parseOptionalFloat(fieldAt(fields, 16))
	details.VDOP = parseOptionalFloat(fieldAt(fields, 17))
	details.SignalID = fieldAt(fields, 18)
}

func parseGSVDetails(fields []string, details *model.NMEADetails) {
	details.TotalMessages = parseOptionalInt(fieldAt(fields, 1))
	details.MessageNumber = parseOptionalInt(fieldAt(fields, 2))
	details.TotalSatellites = parseOptionalInt(fieldAt(fields, 3))
	lastSatelliteField := len(fields)
	if remainder := (len(fields) - 4) % 4; remainder == 1 {
		lastSatelliteField--
		details.SignalID = fieldAt(fields, len(fields)-1)
	}
	for index := 4; index+3 < lastSatelliteField; index += 4 {
		id := fieldAt(fields, index)
		if id == "" {
			continue
		}
		details.Satellites = append(details.Satellites, model.NMEASatellite{
			ID:         id,
			Elevation:  parseOptionalInt(fieldAt(fields, index+1)),
			Azimuth:    parseOptionalInt(fieldAt(fields, index+2)),
			SignalDBHz: parseOptionalInt(fieldAt(fields, index+3)),
		})
	}
}

func parseVTGDetails(fields []string, details *model.NMEADetails) {
	details.CourseTrue = parseOptionalFloat(fieldAt(fields, 1))
	details.CourseMagnetic = parseOptionalFloat(fieldAt(fields, 3))
	details.SpeedKnots = parseOptionalFloat(fieldAt(fields, 5))
	details.SpeedKPH = parseOptionalFloat(fieldAt(fields, 7))
	details.Mode = fieldAt(fields, 9)
}

func parseNMEAChecksum(raw string, details *model.NMEADetails) {
	line := strings.TrimSpace(strings.TrimPrefix(raw, "$"))
	star := strings.LastIndex(line, "*")
	if star < 0 || star+1 >= len(line) {
		return
	}
	details.Checksum = strings.ToUpper(strings.TrimSpace(line[star+1:]))
	var checksum byte
	for index := range star {
		checksum ^= line[index]
	}
	details.CalculatedChecksum = fmt.Sprintf("%02X", checksum)
	valid := details.Checksum == details.CalculatedChecksum
	details.ChecksumValid = &valid
}

func parseCoordinatePointers(latValue, latDirection, lngValue, lngDirection string) (*float64, *float64) {
	lat, okLat := parseNMEACoordinate(latValue, latDirection)
	lng, okLng := parseNMEACoordinate(lngValue, lngDirection)
	if !okLat || !okLng {
		return nil, nil
	}
	return &lat, &lng
}

func fieldAt(fields []string, index int) string {
	if index < 0 || index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func parseOptionalInt(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseOptionalFloat(value string) *float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseGGA(fields []string) *model.GPSFix {
	if len(fields) < 10 {
		return nil
	}
	lat, okLat := parseNMEACoordinate(fields[2], fields[3])
	lng, okLng := parseNMEACoordinate(fields[4], fields[5])
	fixQuality := parseInt(fields[6])
	if !okLat || !okLng || fixQuality <= 0 {
		return nil
	}

	return &model.GPSFix{
		Latitude:   lat,
		Longitude:  lng,
		FixQuality: fixQuality,
		Satellites: parseInt(fields[7]),
		AltitudeM:  parseFloat(fields[9]),
		Valid:      true,
	}
}

func parseRMC(fields []string) *model.GPSFix {
	if len(fields) < 9 || fields[2] != "A" {
		return nil
	}
	lat, okLat := parseNMEACoordinate(fields[3], fields[4])
	lng, okLng := parseNMEACoordinate(fields[5], fields[6])
	if !okLat || !okLng {
		return nil
	}

	return &model.GPSFix{
		Latitude:     lat,
		Longitude:    lng,
		SpeedKnots:   parseFloat(fields[7]),
		CourseDegree: parseFloat(fields[8]),
		Valid:        true,
	}
}

func splitNMEAFields(raw string) []string {
	line := strings.TrimSpace(raw)
	line = strings.TrimPrefix(line, "$")
	if star := strings.Index(line, "*"); star >= 0 {
		line = line[:star]
	}
	if line == "" {
		return []string{}
	}
	return strings.Split(line, ",")
}

func parseNMEACoordinate(value, direction string) (float64, bool) {
	value = strings.TrimSpace(value)
	direction = strings.ToUpper(strings.TrimSpace(direction))
	if value == "" || direction == "" {
		return 0, false
	}

	raw, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	degrees := int(raw / 100)
	minutes := raw - float64(degrees*100)
	decimal := float64(degrees) + minutes/60
	if direction == "S" || direction == "W" {
		decimal = -decimal
	}
	if direction != "N" && direction != "S" && direction != "E" && direction != "W" {
		return 0, false
	}
	return decimal, true
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

func normalizeOptions(options Options) Options {
	if options.DefaultBaudRate == 0 {
		options.DefaultBaudRate = 115200
	}
	if options.DefaultDataBits == 0 {
		options.DefaultDataBits = 8
	}
	if options.DefaultStopBits == 0 {
		options.DefaultStopBits = 1
	}
	if options.DefaultParity == "" {
		options.DefaultParity = "none"
	}
	if options.DefaultReadTimeout == 0 {
		options.DefaultReadTimeout = time.Second
	}
	if options.ReconnectInitialDelay == 0 {
		options.ReconnectInitialDelay = time.Second
	}
	if options.ReconnectMaxDelay == 0 {
		options.ReconnectMaxDelay = 15 * time.Second
	}
	if options.ReconnectMaxDelay < options.ReconnectInitialDelay {
		options.ReconnectMaxDelay = options.ReconnectInitialDelay
	}
	return options
}

func firstNonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
