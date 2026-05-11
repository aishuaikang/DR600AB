package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
	"tri-detector/client"
	"tri-detector/parser"
)

type Options struct {
	DefaultBaudRate       int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
}

type SerialOpener func(cfg *serialport.Config) (serial.Port, error)

type SettingsStore interface {
	Load() (model.DetectionSessionRequest, bool, error)
	Save(model.DetectionSessionRequest) error
}

type Service struct {
	mu sync.RWMutex

	store      *store.MemoryStore
	translator *i18n.Translator
	settings   SettingsStore
	options    Options
	openPort   SerialOpener
	listPorts  func() ([]string, error)
	current    *session
	sequence   uint64
}

type session struct {
	id            string
	request       model.DetectionSessionRequest
	config        serialport.Config
	txPortName    string
	client        *client.SerialClient
	startedAt     time.Time
	locale        string
	state         string
	autoReconnect bool
	retryCount    int
	lastError     string
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewService(store *store.MemoryStore, translator *i18n.Translator, settingsStore SettingsStore, options Options) *Service {
	return &Service{
		store:      store,
		translator: translator,
		settings:   settingsStore,
		options:    normalizeOptions(options),
		openPort:   serialport.Open,
		listPorts:  serialport.ListPorts,
	}
}

func (s *Service) SetSerialOpener(open SerialOpener) {
	if open == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openPort = open
}

func (s *Service) SetPortLister(list func() ([]string, error)) {
	if list == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listPorts = list
}

func (s *Service) Settings() (model.DetectionSessionRequest, bool, error) {
	if s.settings == nil {
		return model.DetectionSessionRequest{}, false, nil
	}
	return s.settings.Load()
}

func (s *Service) ListPorts() ([]model.PortInfo, error) {
	s.mu.RLock()
	listPorts := s.listPorts
	current := s.current
	s.mu.RUnlock()

	ports, err := listPorts()
	if err != nil {
		return nil, err
	}

	active := current != nil && current.state == "connected"
	result := make([]model.PortInfo, 0, len(ports))
	for _, name := range ports {
		result = append(result, model.PortInfo{
			Name:   name,
			Active: active && (current.config.PortName == name || current.txPortName == name),
		})
	}
	return result, nil
}

func (s *Service) Start(req model.DetectionSessionRequest, locale string) (model.DetectionSessionResponse, error) {
	req = s.normalizeRequest(req)
	rxPortName, txPortName := s.resolvePortNames(req)
	if rxPortName == "" {
		return model.DetectionSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "port_required"))
	}
	req.PortName = rxPortName
	req.RxPortName = rxPortName
	req.TxPortName = txPortName
	req.AutoConnect = true

	if err := s.saveSettings(req); err != nil {
		return model.DetectionSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}

	cfg := s.buildConfig(req, rxPortName)

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
		txPortName:    txPortName,
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

	client, err := s.connectOnce(&sess.config, sess.txPortName)
	if err == nil {
		if !s.assignConnectedClient(seq, sess, client) {
			client.Close()
			return s.Current(locale), nil
		}

		response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "session.started"))
		s.store.Publish(model.Event{Type: "session.started", Time: time.Now(), Payload: response})
		go s.manageSession(seq, sess, true)
		return response, nil
	}

	s.setSessionFailure(seq, sess, "connecting", err.Error())
	response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "session.connecting"))
	response.LastError = err.Error()
	response.Active = false
	s.store.Publish(model.Event{Type: "session.connecting", Time: time.Now(), Payload: response})
	go s.manageSession(seq, sess, false)
	return response, nil
}

func (s *Service) Stop(locale string) model.DetectionSessionResponse {
	s.mu.Lock()
	prev := s.current
	s.sequence++
	s.current = nil
	s.mu.Unlock()

	if prev == nil {
		return model.DetectionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "session.inactive"),
		}
	}

	prev.cancel()
	if prev.client != nil {
		prev.client.Close()
	}

	response := s.responseForSession(prev, locale, s.translator.T(locale, "common", "session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "session.stopped", Time: time.Now(), Payload: response})
	return response
}

func (s *Service) Current(locale string) model.DetectionSessionResponse {
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()

	if current == nil {
		return model.DetectionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "session.inactive"),
		}
	}
	return s.responseForSession(current, locale, s.messageForState(current.state, locale))
}

func (s *Service) Records(limit int) []model.DetectionRecord {
	return s.store.ListDetections(limit)
}

func (s *Service) Parsed(limit int) []model.ParsedMessage {
	return s.store.ListParsed(limit)
}

func (s *Service) FPV(limit int) []model.FpvRecord {
	return s.store.ListFPV(limit)
}

func (s *Service) Subscribe(buffer int) (<-chan model.Event, func()) {
	return s.store.Subscribe(buffer)
}

func (s *Service) Publish(evt model.Event) {
	s.store.Publish(evt)
}

func (s *Service) RestoreSavedSettings(locale string) {
	if s.settings == nil {
		return
	}
	req, ok, err := s.settings.Load()
	if err != nil || !ok {
		return
	}
	_, _ = s.Start(req, locale)
}

func (s *Service) IngestLine(sessionID, portName, line string) {
	msg, err := parser.ParseLine(line)
	if err != nil {
		parsed := model.ParsedMessage{
			Type: "raw",
			Time: time.Now(),
			Raw:  strings.TrimSpace(line),
			Data: json.RawMessage("null"),
		}
		s.store.AddParsed(parsed)
		return
	}

	parsed := toParsedMessage(msg)
	s.store.AddParsed(parsed)

	record, ok := detectionRecordFromMessage(sessionID, portName, parsed, msg)
	if !ok {
		return
	}
	band, label, fpv := classifyFPV(record.Frequency, record.Model, parsed.Raw)
	record.IsFPV = fpv
	record.FPVBand = band
	s.store.AddDetection(record)

	if fpv {
		s.store.AddFPV(model.FpvRecord{
			ID:          record.ID + "-fpv",
			DetectionID: record.ID,
			Band:        band,
			Label:       label,
			PortName:    record.PortName,
			Device:      record.Device,
			Model:       record.Model,
			Frequency:   record.Frequency,
			RSSI:        record.RSSI,
			ReceivedAt:  record.ReceivedAt,
			SourceKind:  record.Kind,
		})
	}
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
			client, err := s.connectOnce(&sess.config, sess.txPortName)
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

			if !s.assignConnectedClient(seq, sess, client) {
				client.Close()
				return
			}

			delay = s.options.ReconnectInitialDelay
			connected = true
			response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "session.started"))
			s.store.Publish(model.Event{Type: "session.started", Time: time.Now(), Payload: response})
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

		s.setSessionFailure(seq, sess, "reconnecting", s.translator.T(sess.locale, "common", "session.disconnected"))
		response := s.responseForSession(sess, sess.locale, s.translator.T(sess.locale, "common", "session.reconnecting"))
		response.LastError = sess.lastError
		response.RetryCount = sess.retryCount
		response.Active = false
		s.store.Publish(model.Event{Type: "session.reconnecting", Time: time.Now(), Payload: response})
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

	response := s.responseForSession(prev, sess.locale, s.translator.T(sess.locale, "common", "session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "session.stopped", Time: time.Now(), Payload: response})
}

func (s *Service) messageForState(state, locale string) string {
	switch state {
	case "connected":
		return s.translator.T(locale, "common", "session.started")
	case "connecting":
		return s.translator.T(locale, "common", "session.connecting")
	case "reconnecting":
		return s.translator.T(locale, "common", "session.reconnecting")
	case "inactive":
		return s.translator.T(locale, "common", "session.inactive")
	default:
		return s.translator.T(locale, "common", "session.inactive")
	}
}

func (s *Service) responseForSession(sess *session, locale, message string) model.DetectionSessionResponse {
	if sess == nil {
		return model.DetectionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: message,
		}
	}

	active := sess.state == "connected"
	return model.DetectionSessionResponse{
		Active:        active,
		SessionID:     sess.id,
		PortName:      sess.config.PortName,
		RxPortName:    sess.config.PortName,
		TxPortName:    sess.txPortName,
		BaudRate:      sess.config.BaudRate,
		DataBits:      sess.config.DataBits,
		StopBits:      sess.config.StopBits,
		Parity:        sess.config.Parity,
		StartedAt:     sess.startedAt,
		State:         sess.state,
		AutoReconnect: sess.autoReconnect,
		LastError:     sess.lastError,
		RetryCount:    sess.retryCount,
		Message:       message,
	}
}

func (s *Service) connectOnce(cfg *serialport.Config, txPortName string) (*client.SerialClient, error) {
	s.mu.RLock()
	openPort := s.openPort
	s.mu.RUnlock()

	readPort, err := openPort(cfg)
	if err != nil {
		return nil, err
	}

	if txPortName == "" || txPortName == cfg.PortName {
		return client.NewSerialClient(readPort, cfg.PortName, false), nil
	}

	txCfg := *cfg
	txCfg.PortName = txPortName
	writePort, err := openPort(&txCfg)
	if err != nil {
		_ = readPort.Close()
		return nil, err
	}
	return client.NewDuplexSerialClient(readPort, cfg.PortName, writePort, txPortName, false), nil
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

func (s *Service) setSessionFailure(seq uint64, sess *session, state, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	sess.state = state
	sess.lastError = lastErr
	sess.retryCount++
}

func (s *Service) isCurrentSession(seq uint64, sess *session) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence == seq && s.current == sess
}

func (s *Service) saveSettings(req model.DetectionSessionRequest) error {
	if s.settings == nil {
		return nil
	}
	return s.settings.Save(req)
}

func (s *Service) buildConfig(req model.DetectionSessionRequest, rxPortName string) serialport.Config {
	cfg := serialport.Config{
		PortName:    rxPortName,
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

func (s *Service) normalizeRequest(req model.DetectionSessionRequest) model.DetectionSessionRequest {
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

func (s *Service) resolvePortNames(req model.DetectionSessionRequest) (string, string) {
	rxPortName := strings.TrimSpace(req.RxPortName)
	if rxPortName == "" {
		rxPortName = strings.TrimSpace(req.PortName)
	}

	txPortName := strings.TrimSpace(req.TxPortName)
	if txPortName == "" {
		txPortName = rxPortName
	}

	return rxPortName, txPortName
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

func sameRequest(a, b model.DetectionSessionRequest) bool {
	return a.PortName == b.PortName &&
		a.RxPortName == b.RxPortName &&
		a.TxPortName == b.TxPortName &&
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
		return "session.connecting"
	case "reconnecting":
		return "session.reconnecting"
	case "connected":
		return "session.started"
	case "inactive":
		return "session.stopped"
	default:
		return "session.connecting"
	}
}

func detectionRecordFromMessage(sessionID, portName string, parsed model.ParsedMessage, msg *parser.Message) (model.DetectionRecord, bool) {
	record := model.DetectionRecord{
		ID:         fmt.Sprintf("%s-%d", sessionID, parsed.Time.UnixNano()),
		SessionID:  sessionID,
		PortName:   portName,
		Kind:       parsed.Type,
		ReceivedAt: parsed.Time,
		Parsed:     parsed,
	}

	switch data := msg.Data.(type) {
	case *parser.Detect:
		record.Device = data.Device
		record.Model = data.Model
		record.Frequency = data.Freq
		record.RSSI = data.RSSI
	case *parser.DIDPlain:
		record.Device = data.Device
		record.Model = data.Model
		record.Frequency = data.Freq
		record.RSSI = data.RSSI
	case *parser.DIDEncrypted:
		record.Device = data.Device
		record.Model = data.EncryptedID
		record.Frequency = data.Freq
		record.RSSI = data.RSSI
	case *parser.RID:
		record.Device = data.SSID
		record.Model = data.Model
		record.Frequency = data.Freq
		record.RSSI = data.RSSI
	default:
		return model.DetectionRecord{}, false
	}

	record.Summary = buildSummary(record)
	return record, true
}

func buildSummary(record model.DetectionRecord) string {
	parts := make([]string, 0, 4)
	if record.Device != "" {
		parts = append(parts, record.Device)
	}
	if record.Model != "" {
		parts = append(parts, record.Model)
	}
	if record.Frequency != 0 {
		parts = append(parts, fmt.Sprintf("%.1f MHz", record.Frequency))
	}
	if record.RSSI != 0 {
		parts = append(parts, fmt.Sprintf("%.1f dBm", record.RSSI))
	}
	if len(parts) == 0 {
		return record.Kind
	}
	return strings.Join(parts, " / ")
}

func toParsedMessage(msg *parser.Message) model.ParsedMessage {
	data, err := json.Marshal(msg.Data)
	if err != nil {
		data = []byte("null")
	}
	return model.ParsedMessage{
		Type: string(msg.Type),
		Time: msg.Time,
		Raw:  msg.Raw,
		Data: data,
	}
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
