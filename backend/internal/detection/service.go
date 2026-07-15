// Package detection 管理侦测串口会话和解析后的侦测数据。
package detection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"serialport"
	"tri-detector/client"
	"uav-protocol/diddecrypt"
	protocolmerge "uav-protocol/merge"
	"uav-protocol/parser"
)

// Options 配置串口默认值和重连时间参数。
type Options struct {
	DefaultBaudRate       int
	DefaultRxBaudRate     int
	DefaultTxBaudRate     int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	O3Decrypt             O3DecryptOptions
}

// O3DecryptOptions 配置 O3+/O4 DID 加密报文的 MQTT 解密分支。
type O3DecryptOptions struct {
	Enabled        bool
	Broker         string
	Port           int
	Username       string
	Password       string
	Timeout        time.Duration
	ConnectTimeout time.Duration
}

// SerialOpener 根据串口配置打开串口。
type SerialOpener func(cfg *serialport.Config) (serial.Port, error)

// O3PlusO4Decoder 解密 DID 加密报文并返回定位目标。
type O3PlusO4Decoder interface {
	ParseO3PlusO4PacketMQTT(ctx context.Context, packet parser.DIDEncrypted, deviceSN string, receivedAt time.Time) (model.ScreenPositionTarget, bool)
}

// DirectionSwitch 控制测向链路使用的射频开关。
type DirectionSwitch interface {
	SetDirectionSwitch(enabled bool) error
}

const (
	startDetectionCommand         = "start -freq 1, -pathb 1, -gain 60"
	screenDirectionEventType      = "screen.direction.updated"
	maxDirectionFrequencyMHz      = 10000
	defaultRxBaudRate             = 115200
	defaultTxBaudRate             = 460800
	defaultBaudRate               = defaultRxBaudRate
	defaultO3DecryptWorkers       = 4
	reservedO3InitialDecryptSlots = 1
)

type commandControlMode string

const (
	commandControlModeIdle      commandControlMode = ""
	commandControlModeDirection commandControlMode = "direction"
	commandControlModeFPV       commandControlMode = "fpv"
)

type commandControlResetPolicy int

const (
	commandControlResetAlways commandControlResetPolicy = iota
	commandControlResetIfCurrent
)

var (
	ErrCommandSerialOffline      = errors.New("detection command serial offline")
	ErrCommandModeConflict       = errors.New("detection command mode conflict")
	ErrDirectionTargetRequired   = errors.New("direction target required")
	ErrInvalidDirectionFrequency = errors.New("invalid direction frequency")
)

// SettingsStore 持久化最近一次侦测会话请求和公开用户设置。
type SettingsStore interface {
	Load() (model.DetectionSessionRequest, bool, error)
	Save(model.DetectionSessionRequest) error
}

// Service 管理侦测串口会话并存储解析记录。
type Service struct {
	mu        sync.RWMutex
	commandMu sync.Mutex

	store      *store.MemoryStore
	translator *i18n.Translator
	settings   SettingsStore
	options    Options
	openPort   SerialOpener
	o3Decoder  O3PlusO4Decoder
	o3Mu       sync.Mutex
	o3Active   map[string]struct{}
	o3Slots    chan struct{}
	dirSwitch  DirectionSwitch
	listPorts  func() ([]string, error)
	current    *session
	direction  model.ScreenDirectionState
	mode       commandControlMode
	sequence   uint64
}

type session struct {
	id            string
	request       model.DetectionSessionRequest
	config        serialport.Config
	txPortName    string
	txBaudRate    int
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

// NewService 创建带串口默认值和存储依赖的侦测服务。
func NewService(store *store.MemoryStore, translator *i18n.Translator, settingsStore SettingsStore, options Options) *Service {
	service := &Service{
		store:      store,
		translator: translator,
		settings:   settingsStore,
		options:    normalizeOptions(options),
		openPort:   serialport.Open,
		listPorts:  serialport.ListPorts,
		o3Active:   make(map[string]struct{}),
		o3Slots:    make(chan struct{}, defaultO3DecryptWorkers),
	}
	service.o3Decoder = NewMQTTO3PlusO4Decoder(service.options.O3Decrypt)
	return service
}

// Store returns the runtime state store used by the service.
func (s *Service) Store() *store.MemoryStore {
	if s == nil {
		return nil
	}
	return s.store
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

// SetO3PlusO4Decoder 替换 O3+/O4 解密器，主要用于测试。
func (s *Service) SetO3PlusO4Decoder(decoder O3PlusO4Decoder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.o3Decoder = decoder
}

// SetDirectionSwitch 替换测向射频开关控制器，主要用于接入 GPIO 服务和测试。
func (s *Service) SetDirectionSwitch(directionSwitch DirectionSwitch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirSwitch = directionSwitch
}

// SetPortLister 替换串口枚举函数，主要用于测试。
func (s *Service) SetPortLister(list func() ([]string, error)) {
	if list == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listPorts = list
}

// Settings 加载已持久化的侦测会话请求。
func (s *Service) Settings() (model.DetectionSessionRequest, bool, error) {
	if s.settings == nil {
		return model.DetectionSessionRequest{}, false, nil
	}
	return s.settings.Load()
}

// Configured 返回侦测串口是否已保存有效配置。
func (s *Service) Configured() (model.DetectionSessionRequest, bool, error) {
	settings, ok, err := s.Settings()
	if err != nil || !ok {
		return model.DetectionSessionRequest{}, false, err
	}
	rxPortName, _ := s.resolvePortNames(settings)
	return settings, rxPortName != "", nil
}

// ClearSettings 停止当前侦测会话并清空已保存的串口设置。
func (s *Service) ClearSettings(locale string) (model.DetectionSessionResponse, error) {
	if err := s.saveSettings(model.DetectionSessionRequest{}); err != nil {
		return model.DetectionSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}
	return s.Stop(locale), nil
}

// ListPorts 返回串口列表，并标记当前会话占用的串口。
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

// Start 保存设置、打开串口并启动侦测读取循环。
func (s *Service) Start(req model.DetectionSessionRequest, locale string) (model.DetectionSessionResponse, error) {
	req = s.normalizeRequest(req)
	rxPortName, txPortName := s.resolvePortNames(req)
	if rxPortName == "" {
		return model.DetectionSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "port_required"))
	}
	req.PortName = rxPortName
	req.RxPortName = rxPortName
	req.TxPortName = txPortName
	req.BaudRate = req.RxBaudRate
	req.AutoConnect = true

	if err := s.saveSettings(req); err != nil {
		return model.DetectionSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}

	cfg := s.buildConfig(req, rxPortName)

	s.mu.Lock()
	if current := s.current; current != nil && sameRequest(current.request, req) {
		current.locale = locale
		current.autoReconnect = req.AutoConnect
		response := s.responseForSessionLocked(current, locale, s.messageForState(current.state, locale))
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
		txBaudRate:    req.TxBaudRate,
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
		s.closeSessionClient(prev, commandControlResetAlways)
	}

	client, err := s.connectOnce(&sess.config, sess.txPortName, sess.txBaudRate, locale)
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

// Stop 关闭当前侦测会话并发布停止事件。
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
	s.closeSessionClient(prev, commandControlResetAlways)

	response := s.responseForSession(prev, locale, s.translator.T(locale, "common", "session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.store.Publish(model.Event{Type: "session.stopped", Time: time.Now(), Payload: response})
	return response
}

// Current 返回当前侦测会话状态，并按语言本地化提示文本。
func (s *Service) Current(locale string) model.DetectionSessionResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	current := s.current

	if current == nil {
		return model.DetectionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "session.inactive"),
		}
	}
	return s.responseForSessionLocked(current, locale, s.messageForState(current.state, locale))
}

// Records 返回最新的标准化侦测记录。
func (s *Service) Records(limit int) []model.DetectionRecord {
	return s.store.ListDetections(limit)
}

// ScreenDetections 返回大屏使用的合并侦测目标。
func (s *Service) ScreenDetections(limit int) []model.ScreenDetectionTarget {
	return s.store.ListScreenDetections(limit)
}

// ScreenPositions 返回大屏使用的合并定位目标。
func (s *Service) ScreenPositions(limit int) []model.ScreenPositionTarget {
	return s.store.ListScreenPositions(limit)
}

// ScreenPositionsWithDeviceLocation 返回大屏定位目标，并按指定设备位置计算距离关系。
func (s *Service) ScreenPositionsWithDeviceLocation(limit int, deviceLocation *model.ScreenDeviceLocationResponse) []model.ScreenPositionTarget {
	return s.store.ListScreenPositionsWithDeviceLocation(limit, deviceLocation)
}

// ScreenDirectionState 返回当前大屏测向锁频状态。
func (s *Service) ScreenDirectionState() model.ScreenDirectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneScreenDirectionState(s.direction)
}

// SetScreenDirection 更新大屏测向锁频状态，并向侦测 TX 串口发送锁频或复位命令。
func (s *Service) SetScreenDirection(req model.ScreenDirectionRequest) (model.ScreenDirectionState, error) {
	if !req.Enabled {
		return s.stopScreenDirection()
	}

	targetID := strings.TrimSpace(req.TargetID)
	if targetID == "" {
		return s.ScreenDirectionState(), ErrDirectionTargetRequired
	}
	frequency, err := normalizeDirectionFrequency(req.Frequency)
	if err != nil {
		return s.ScreenDirectionState(), err
	}

	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	if s.mode != commandControlModeIdle {
		return s.ScreenDirectionState(), ErrCommandModeConflict
	}
	if err := s.setDirectionSwitch(true); err != nil {
		return s.setScreenDirectionError(err), err
	}
	if err := s.sendCommandsLocked(fmt.Sprintf("start -freq %d", frequency)); err != nil {
		if switchErr := s.setDirectionSwitch(false); switchErr != nil {
			err = errors.Join(err, switchErr)
		}
		return s.setScreenDirectionError(err), err
	}
	s.mode = commandControlModeDirection

	startedAt := time.Now()
	return s.setScreenDirectionState(model.ScreenDirectionState{
		Active:    true,
		TargetID:  targetID,
		Frequency: float64(frequency),
		StartedAt: &startedAt,
	}, true), nil
}

func (s *Service) stopScreenDirection() (model.ScreenDirectionState, error) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	if s.mode == commandControlModeFPV {
		return s.ScreenDirectionState(), ErrCommandModeConflict
	}
	if err := s.setDirectionSwitch(false); err != nil {
		return s.setScreenDirectionError(err), err
	}
	if err := s.sendCommandsLocked(startDetectionCommand); err != nil {
		return s.setScreenDirectionError(err), err
	}
	s.mode = commandControlModeIdle
	return s.clearScreenDirectionState(true), nil
}

// SendCommands writes text commands to the active detection TX serial port in order.
func (s *Service) SendCommands(commands ...string) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	if s.mode != commandControlModeIdle {
		return ErrCommandModeConflict
	}
	return s.sendCommandsLocked(commands...)
}

// BeginScreenFPVPlayback switches the detector into FPV image mode.
func (s *Service) BeginScreenFPVPlayback(imageAddress string, bandStart, bandEnd int) error {
	imageAddress = strings.TrimSpace(imageAddress)
	if imageAddress == "" {
		return fmt.Errorf("fpv image address is required")
	}
	if bandStart <= 0 || bandEnd <= 0 || bandEnd < bandStart {
		return fmt.Errorf("invalid fpv band: %d,%d", bandStart, bandEnd)
	}

	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	if s.mode != commandControlModeIdle {
		return ErrCommandModeConflict
	}
	s.mode = commandControlModeFPV
	if err := s.sendCommandsLocked(fmt.Sprintf("start -imag %s\r\n", imageAddress)); err != nil {
		s.mode = commandControlModeIdle
		return err
	}
	if err := s.sendCommandsLocked(fmt.Sprintf("start -band %d,%d\r\n", bandStart, bandEnd)); err != nil {
		if stopErr := s.endScreenFPVPlaybackLocked(); stopErr != nil {
			return errors.Join(err, stopErr)
		}
		return err
	}
	return nil
}

// EndScreenFPVPlayback restores the detector to the default frequency mode.
func (s *Service) EndScreenFPVPlayback() error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	if s.mode == commandControlModeDirection {
		return ErrCommandModeConflict
	}
	return s.endScreenFPVPlaybackLocked()
}

func (s *Service) endScreenFPVPlaybackLocked() error {
	if err := s.sendCommandsLocked(
		"start -imag 0\r\n",
		startDetectionCommand+"\r\n",
	); err != nil {
		return err
	}
	s.mode = commandControlModeIdle
	return nil
}

func (s *Service) sendCommandsLocked(commands ...string) error {
	s.mu.RLock()
	current := s.current
	var serialClient *client.SerialClient
	if current != nil && current.state == "connected" {
		serialClient = current.client
	}
	s.mu.RUnlock()

	if serialClient == nil {
		return ErrCommandSerialOffline
	}

	for _, command := range commands {
		if err := serialClient.Send(command); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sessionClient(sess *session) *client.SerialClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sess.client
}

func (s *Service) closeSessionClient(sess *session, resetPolicy commandControlResetPolicy) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()

	s.mu.Lock()
	serialClient := sess.client
	sess.client = nil
	resetControl := resetPolicy == commandControlResetAlways ||
		(resetPolicy == commandControlResetIfCurrent && s.current == sess)
	resetDirectionSwitch := resetControl && s.mode == commandControlModeDirection
	publishDirectionClear := false
	if resetControl {
		s.mode = commandControlModeIdle
		publishDirectionClear = s.clearScreenDirectionStateLocked()
	}
	s.mu.Unlock()

	if serialClient != nil {
		serialClient.Close()
	}
	if resetDirectionSwitch {
		_ = s.setDirectionSwitch(false)
	}
	if publishDirectionClear {
		s.publishScreenDirection(model.ScreenDirectionState{})
	}
}

// Parsed 返回最新解析结果，包含无法识别的原始行。
func (s *Service) Parsed(limit int) []model.ParsedMessage {
	return s.store.ListParsed(limit)
}

// Subscribe 注册带缓冲的事件订阅者，并返回取消订阅函数。
func (s *Service) Subscribe(buffer int) (<-chan model.Event, func()) {
	return s.store.Subscribe(buffer)
}

// RestoreSavedSettings 在存在已保存设置时自动恢复会话。
func (s *Service) RestoreSavedSettings(locale string) {
	if s.settings == nil {
		return
	}
	req, ok, err := s.settings.Load()
	if err != nil || !ok {
		return
	}
	rxPortName, _ := s.resolvePortNames(req)
	if !req.AutoConnect || rxPortName == "" {
		return
	}
	_, _ = s.Start(req, locale)
}

// IngestLine 解析一行串口数据，并写入解析和侦测记录。
func (s *Service) IngestLine(sessionID, portName, line string) {
	s.ingestLine(context.Background(), sessionID, portName, line)
}

func (s *Service) ingestLine(ctx context.Context, sessionID, portName, line string) {
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
	s.ingestScreenPosition(ctx, parsed, msg)

	record, ok := detectionRecordFromMessage(sessionID, portName, parsed, msg)
	if !ok {
		return
	}
	s.store.AddDetection(record)
}

// manageSession 保持单个串口会话运行，直到被停止或替换。
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
			client, err := s.connectOnce(&sess.config, sess.txPortName, sess.txBaudRate, sess.locale)
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

		serialClient := s.sessionClient(sess)
		if serialClient == nil {
			return
		}
		serialClient.ReadLoop(func(line string) {
			s.ingestLine(sess.ctx, sess.id, sess.config.PortName, line)
		})
		s.closeSessionClient(sess, commandControlResetIfCurrent)

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

// finalizeStopped 在读取循环退出且不重连时清理当前会话。
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

// messageForState 将会话状态映射为本地化操作提示。
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

// responseForSession 将内部会话状态转换为 API 响应结构。
func (s *Service) responseForSession(sess *session, locale, message string) model.DetectionSessionResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.responseForSessionLocked(sess, locale, message)
}

func (s *Service) responseForSessionLocked(sess *session, locale, message string) model.DetectionSessionResponse {
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
		RxBaudRate:    sess.config.BaudRate,
		TxBaudRate:    firstNonZero(sess.txBaudRate, sess.config.BaudRate),
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

// connectOnce 打开接收和发送串口，并发送侦测启动命令。
func (s *Service) connectOnce(cfg *serialport.Config, txPortName string, txBaudRate int, locale string) (*client.SerialClient, error) {
	s.mu.RLock()
	openPort := s.openPort
	s.mu.RUnlock()

	readPort, err := openPort(cfg)
	if err != nil {
		return nil, err
	}

	var serialClient *client.SerialClient
	if txPortName == "" || txPortName == cfg.PortName {
		serialClient = client.NewSerialClient(readPort, cfg.PortName, false)
	} else {
		txCfg := *cfg
		txCfg.PortName = txPortName
		txCfg.BaudRate = firstNonZero(txBaudRate, cfg.BaudRate)
		writePort, err := openPort(&txCfg)
		if err != nil {
			_ = readPort.Close()
			return nil, err
		}
		serialClient = client.NewDuplexSerialClient(readPort, cfg.PortName, writePort, txPortName, false)
	}

	if err := serialClient.Send(startDetectionCommand); err != nil {
		serialClient.Close()
		return nil, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "detection_start_command_failed"), err)
	}
	return serialClient, nil
}

// assignConnectedClient 在会话仍然有效时绑定已连接客户端。
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

// setSessionFailure 记录当前会话最近一次连接错误。
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

func (s *Service) setDirectionSwitch(enabled bool) error {
	s.mu.RLock()
	directionSwitch := s.dirSwitch
	s.mu.RUnlock()
	if directionSwitch == nil {
		return nil
	}
	return directionSwitch.SetDirectionSwitch(enabled)
}

func normalizeDirectionFrequency(value float64) (int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, ErrInvalidDirectionFrequency
	}
	frequency := int(math.Round(value))
	if frequency <= 0 || frequency > maxDirectionFrequencyMHz {
		return 0, ErrInvalidDirectionFrequency
	}
	return frequency, nil
}

func (s *Service) setScreenDirectionError(err error) model.ScreenDirectionState {
	if err == nil {
		return s.ScreenDirectionState()
	}

	s.mu.Lock()
	state := cloneScreenDirectionState(s.direction)
	if state.Active {
		state.LastError = err.Error()
		s.direction = cloneScreenDirectionState(state)
	}
	s.mu.Unlock()

	if state.Active {
		s.publishScreenDirection(state)
	}
	return state
}

func (s *Service) setScreenDirectionState(state model.ScreenDirectionState, publish bool) model.ScreenDirectionState {
	state = cloneScreenDirectionState(state)
	s.mu.Lock()
	s.direction = cloneScreenDirectionState(state)
	s.mu.Unlock()

	if publish {
		s.publishScreenDirection(state)
	}
	return state
}

func (s *Service) clearScreenDirectionState(publish bool) model.ScreenDirectionState {
	s.mu.Lock()
	changed := s.clearScreenDirectionStateLocked()
	s.mu.Unlock()

	state := model.ScreenDirectionState{}
	if publish && changed {
		s.publishScreenDirection(state)
	}
	return state
}

func (s *Service) clearScreenDirectionStateLocked() bool {
	changed := s.direction.Active || s.direction.TargetID != "" || s.direction.Frequency != 0 || s.direction.LastError != ""
	s.direction = model.ScreenDirectionState{}
	return changed
}

func (s *Service) publishScreenDirection(state model.ScreenDirectionState) {
	if s.store == nil {
		return
	}
	s.store.Publish(model.Event{Type: screenDirectionEventType, Time: time.Now(), Payload: state})
}

func cloneScreenDirectionState(state model.ScreenDirectionState) model.ScreenDirectionState {
	if state.StartedAt == nil {
		return state
	}
	startedAt := *state.StartedAt
	state.StartedAt = &startedAt
	return state
}

// isCurrentSession 判断序号和会话指针是否仍对应当前会话。
func (s *Service) isCurrentSession(seq uint64, sess *session) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence == seq && s.current == sess
}

// saveSettings 在配置存储存在时持久化标准化会话请求。
func (s *Service) saveSettings(req model.DetectionSessionRequest) error {
	if s.settings == nil {
		return nil
	}
	return s.settings.Save(req)
}

// buildConfig 将 API 请求转换为串口配置。
func (s *Service) buildConfig(req model.DetectionSessionRequest, rxPortName string) serialport.Config {
	baudRate := firstNonZero(req.BaudRate, s.options.DefaultRxBaudRate)
	cfg := serialport.Config{
		PortName:    rxPortName,
		BaudRate:    firstNonZero(req.RxBaudRate, baudRate),
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

// normalizeRequest 使用服务默认值补齐缺省串口参数。
func (s *Service) normalizeRequest(req model.DetectionSessionRequest) model.DetectionSessionRequest {
	req.AutoConnect = true
	rxDefaultBaudRate := s.options.DefaultRxBaudRate
	txDefaultBaudRate := s.options.DefaultTxBaudRate
	if req.BaudRate != 0 {
		rxDefaultBaudRate = req.BaudRate
		txDefaultBaudRate = req.BaudRate
	}
	req.RxBaudRate = firstNonZero(req.RxBaudRate, rxDefaultBaudRate)
	req.TxBaudRate = firstNonZero(req.TxBaudRate, txDefaultBaudRate)
	req.BaudRate = req.RxBaudRate
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

// resolvePortNames 同时兼容旧单串口请求和收发分离请求。
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

// nextBackoff 按倍增策略计算下一次重连延迟，并限制最大值。
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

// sleepOrDone 等待重连延迟，或在会话结束时提前返回。
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

// sameRequest 判断两个会话请求是否指向同一套串口配置。
func sameRequest(a, b model.DetectionSessionRequest) bool {
	return a.PortName == b.PortName &&
		a.RxPortName == b.RxPortName &&
		a.TxPortName == b.TxPortName &&
		a.BaudRate == b.BaudRate &&
		a.RxBaudRate == b.RxBaudRate &&
		a.TxBaudRate == b.TxBaudRate &&
		a.DataBits == b.DataBits &&
		a.StopBits == b.StopBits &&
		strings.TrimSpace(a.Parity) == strings.TrimSpace(b.Parity) &&
		a.ReadTimeoutMs == b.ReadTimeoutMs &&
		a.AutoConnect == b.AutoConnect
}

// sessionEventType 将会话状态映射为服务端事件名称。
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

// detectionRecordFromMessage 从 detect 解析结果中提取可列表展示的侦测字段。
func detectionRecordFromMessage(sessionID, portName string, parsed model.ParsedMessage, msg *parser.Message) (model.DetectionRecord, bool) {
	data, ok := msg.Data.(*parser.Detect)
	if !ok {
		return model.DetectionRecord{}, false
	}
	record := model.DetectionRecord{
		ID:         fmt.Sprintf("%s-%d", sessionID, parsed.Time.UnixNano()),
		SessionID:  sessionID,
		PortName:   portName,
		Kind:       parsed.Type,
		ReceivedAt: parsed.Time,
		Parsed:     parsed,
	}
	record.Device = data.Device
	record.Model = data.Model
	record.Frequency = data.Freq
	record.RSSI = data.RSSI
	record.Summary = buildSummary(record)
	return record, true
}

func (s *Service) ingestScreenPosition(ctx context.Context, parsed model.ParsedMessage, msg *parser.Message) {
	switch data := msg.Data.(type) {
	case *parser.RID:
		if target, ok := screenPositionFromRID(parsed, data); ok {
			s.store.AddScreenPosition(target)
		}
	case *parser.DIDPlain:
		if target, ok := screenPositionFromDIDPlain(parsed, data); ok {
			s.store.AddScreenPosition(target)
		}
	case *parser.DIDEncrypted:
		did := *data
		correlationID := didEncryptedCorrelationID(&did)
		cracked := correlationID != "" && s.store.HasCrackedScreenPositionByCorrelationID(correlationID)
		if target, ok := screenPositionFromDIDEncryptedFallback(parsed, &did); ok &&
			!cracked {
			target.CorrelationID = correlationID
			s.store.AddScreenPosition(target)
		}
		s.mu.RLock()
		decoder := s.o3Decoder
		s.mu.RUnlock()
		if decoder == nil {
			return
		}
		release, ok := s.acquireO3Decrypt(ctx, correlationID, cracked)
		if !ok {
			return
		}
		deviceSN := did.Device
		go func() {
			defer release()
			target, ok := decoder.ParseO3PlusO4PacketMQTT(ctx, did, deviceSN, parsed.Time)
			if !ok || !target.Cracked {
				return
			}
			target.CorrelationID = correlationID
			target.LastRecord.Type = parsed.Type
			target.LastRecord.ReceivedAt = parsed.Time
			if target.LastRecord.Device == "" {
				target.LastRecord.Device = did.Device
			}
			if target.LastRecord.Serial == "" {
				target.LastRecord.Serial = target.Serial
			}
			if target.LastRecord.Model == "" {
				target.LastRecord.Model = target.Model
			}
			if target.LastRecord.Frequency == 0 {
				target.LastRecord.Frequency = did.Freq
			}
			if target.LastRecord.RSSI == 0 {
				target.LastRecord.RSSI = did.RSSI
			}
			s.store.RemoveUncrackedDIDScreenPositionByCorrelationID(correlationID)
			s.store.AddScreenPosition(target)
		}()
	}
}

func (s *Service) acquireO3Decrypt(ctx context.Context, correlationID string, refresh bool) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if correlationID == "" {
		return nil, false
	}
	select {
	case <-ctx.Done():
		return nil, false
	default:
	}

	s.o3Mu.Lock()
	defer s.o3Mu.Unlock()
	if _, ok := s.o3Active[correlationID]; ok {
		return nil, false
	}
	refreshLimit := cap(s.o3Slots) - reservedO3InitialDecryptSlots
	if refresh && len(s.o3Slots) >= refreshLimit {
		return nil, false
	}
	select {
	case s.o3Slots <- struct{}{}:
		s.o3Active[correlationID] = struct{}{}
	default:
		return nil, false
	}

	return func() {
		s.o3Mu.Lock()
		delete(s.o3Active, correlationID)
		s.o3Mu.Unlock()
		<-s.o3Slots
	}, true
}

func didEncryptedCorrelationID(data *parser.DIDEncrypted) string {
	if data == nil {
		return ""
	}
	return protocolmerge.DIDEncryptedCorrelationID(data.EncryptedID)
}

func screenPositionFromDIDEncryptedFallback(parsed model.ParsedMessage, data *parser.DIDEncrypted) (model.ScreenPositionTarget, bool) {
	if data == nil || strings.TrimSpace(data.EncryptedID) == "" {
		return model.ScreenPositionTarget{}, false
	}
	target := screenPositionFromProtocolTarget(diddecrypt.TargetFromDecryptResult(
		*data,
		diddecrypt.DecryptResult{Model: diddecrypt.FallbackModel},
		parsed.Time,
		false,
	))
	clearUncrackedDIDFallbackCoordinates(&target)
	target.LastRecord.Type = parsed.Type
	target.LastRecord.ReceivedAt = parsed.Time
	if target.LastRecord.Device == "" {
		target.LastRecord.Device = data.Device
	}
	if target.LastRecord.Serial == "" {
		target.LastRecord.Serial = target.Serial
	}
	if target.LastRecord.Model == "" {
		target.LastRecord.Model = target.Model
	}
	if target.LastRecord.Frequency == 0 {
		target.LastRecord.Frequency = data.Freq
	}
	if target.LastRecord.RSSI == 0 {
		target.LastRecord.RSSI = data.RSSI
	}
	return target, true
}

func screenPositionFromRID(parsed model.ParsedMessage, data *parser.RID) (model.ScreenPositionTarget, bool) {
	if data == nil || strings.TrimSpace(data.Serial) == "" {
		return model.ScreenPositionTarget{}, false
	}
	target := model.ScreenPositionTarget{
		Serial:           strings.TrimSpace(data.Serial),
		Model:            strings.TrimSpace(data.Model),
		Source:           string(parser.TypeRID),
		Frequency:        data.Freq,
		RSSI:             data.RSSI,
		Device:           strings.TrimSpace(data.SSID),
		Drone:            screenPointFromGPS(data.DroneGPS),
		Pilot:            screenPointFromGPS(data.PilotGPS),
		Speed:            nonZeroFloatPtr(data.Speed),
		Altitude:         nonZeroFloatPtr(data.AltitudeG),
		Height:           nonZeroFloatPtr(data.HeightAGL),
		TrajectorySpeed:  float64Ptr(data.Speed),
		TrajectoryHeight: float64Ptr(data.HeightAGL),
		FirstSeen:        parsed.Time,
		LastSeen:         parsed.Time,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       parsed.Type,
			ReceivedAt: parsed.Time,
			Device:     data.SSID,
			Serial:     data.Serial,
			Model:      data.Model,
			Frequency:  data.Freq,
			RSSI:       data.RSSI,
		},
	}
	return target, true
}

func screenPositionFromDIDPlain(parsed model.ParsedMessage, data *parser.DIDPlain) (model.ScreenPositionTarget, bool) {
	if data == nil || strings.TrimSpace(data.Serial) == "" {
		return model.ScreenPositionTarget{}, false
	}
	target := model.ScreenPositionTarget{
		Serial:           strings.TrimSpace(data.Serial),
		Model:            strings.TrimSpace(data.Model),
		Source:           string(parser.TypeDIDPlain),
		Frequency:        data.Freq,
		RSSI:             data.RSSI,
		Device:           strings.TrimSpace(data.Device),
		Drone:            screenPointFromGPS(data.DroneGPS),
		Pilot:            screenPointFromGPS(data.PilotGPS),
		Home:             screenPointFromGPS(data.HomeGPS),
		Speed:            nonZeroFloatPtr(calculateFlightSpeed(data.EastV, data.NorthV, data.UpV)),
		Altitude:         nonZeroFloatPtr(data.Altitude),
		Height:           nonZeroFloatPtr(data.Height),
		TrajectorySpeed:  float64Ptr(calculateFlightSpeed(data.EastV, data.NorthV, data.UpV)),
		TrajectoryHeight: float64Ptr(data.Height),
		FirstSeen:        parsed.Time,
		LastSeen:         parsed.Time,
		LastRecord: model.ScreenPositionLastRecord{
			Type:       parsed.Type,
			ReceivedAt: parsed.Time,
			Device:     data.Device,
			Serial:     data.Serial,
			Model:      data.Model,
			Frequency:  data.Freq,
			RSSI:       data.RSSI,
		},
	}
	return target, true
}

func screenPointFromGPS(gps parser.GPS) *model.ScreenPositionPoint {
	if !displayableScreenCoordinate(gps.Lat, gps.Lng) {
		return nil
	}
	return &model.ScreenPositionPoint{
		Latitude:  gps.Lat,
		Longitude: gps.Lng,
	}
}

func displayableScreenCoordinate(lat, lng float64) bool {
	return !math.IsNaN(lat) &&
		!math.IsInf(lat, 0) &&
		!math.IsNaN(lng) &&
		!math.IsInf(lng, 0) &&
		lat >= -90 &&
		lat <= 90 &&
		lng >= -180 &&
		lng <= 180
}

func screenPositionHasCoordinate(target model.ScreenPositionTarget) bool {
	return target.Drone != nil || target.Pilot != nil || target.Home != nil
}

func nonZeroFloatPtr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func calculateFlightSpeed(east, north, up float64) float64 {
	return math.Sqrt(east*east + north*north + up*up)
}

// buildSummary 创建侦测列表中展示的简短摘要。
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

// toParsedMessage 将解析器专用数据序列化为通用解析结果结构。
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

// normalizeOptions 使用生产默认值补齐未设置的服务参数。
func normalizeOptions(options Options) Options {
	if options.DefaultBaudRate == 0 {
		options.DefaultBaudRate = defaultRxBaudRate
	}
	if options.DefaultRxBaudRate == 0 {
		options.DefaultRxBaudRate = options.DefaultBaudRate
	}
	if options.DefaultTxBaudRate == 0 {
		options.DefaultTxBaudRate = defaultTxBaudRate
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

// firstNonZero 优先返回已设置的值，否则返回默认值。
func firstNonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
