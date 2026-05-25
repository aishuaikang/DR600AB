// Package deception 管理 GNSS 导航诱骗设备串口会话和大屏诱骗控制。
package deception

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"gnss-spoofer/protocol"
	"serialport"
)

const (
	screenDeceptionEventType       = "screen.deception.updated"
	defaultDeceptionMode           = "fixed_point"
	deceptionModeFixedPoint        = "fixed_point"
	deceptionModeCircle            = "circle"
	deceptionModeLinear            = "linear"
	deceptionStrengthStrong        = "strong"
	deceptionStrengthStandard      = "standard"
	deceptionStrengthWeak          = "weak"
	deceptionStrengthCustom        = "custom"
	deceptionDelayAuto             = "auto"
	deceptionDelayManual           = "manual"
	deceptionDelayOff              = "off"
	defaultDeceptionAttenuationDB  = 15
	defaultCircleRadiusM           = 100
	defaultCirclePeriodSeconds     = 60
	defaultLinearSpeedMPS          = 10
	defaultDeceptionCommandTimeout = 1200 * time.Millisecond
	deceptionQueryBurstIdleTimeout = 150 * time.Millisecond
	maxDeceptionRecords            = 200
	minSimulationAltitudeM         = -500
	maxSimulationAltitudeM         = 10000
)

var (
	errSerialInactive     = errors.New("deception serial session inactive")
	errLocationRequired   = errors.New("deception location required")
	errInvalidMode        = errors.New("invalid deception mode")
	errInvalidSignal      = errors.New("invalid deception signal")
	errInvalidAttenuation = errors.New("invalid deception attenuation")
	errInvalidDelay       = errors.New("invalid deception delay")
	errInvalidCircle      = errors.New("invalid deception circle")
	errInvalidLinear      = errors.New("invalid deception linear")
)

// Options 配置 GNSS 诱骗设备串口默认值。
type Options struct {
	DefaultBaudRate       int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	CommandTimeout        time.Duration
}

// SerialPort 抽象串口读写能力，便于测试替换。
type SerialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
}

// SerialOpener 根据串口配置打开串口。
type SerialOpener func(cfg *serialport.Config) (SerialPort, error)

// SettingsStore 持久化最近一次 GNSS 诱骗串口会话请求。
type SettingsStore interface {
	LoadDeception() (model.DeceptionSessionRequest, bool, error)
	SaveDeception(model.DeceptionSessionRequest) error
}

// ReportStore 持久化诱骗操作证据报告。
type ReportStore interface {
	Create(model.DeceptionReport) (model.DeceptionReport, error)
	CreateRunning(model.DeceptionReport) (model.DeceptionReport, error)
	Update(model.DeceptionReport) error
}

// Service 管理 GNSS 诱骗设备串口会话、命令 ACK 和大屏诱骗状态。
type Service struct {
	mu                sync.RWMutex
	serialOperationMu sync.Mutex

	store      *store.MemoryStore
	translator *i18n.Translator
	settings   SettingsStore
	reports    ReportStore
	options    Options
	openPort   SerialOpener

	current        *session
	sequence       uint64
	records        []model.DeceptionRecord
	activeReport   *model.DeceptionReport
	activeReportID string

	deceptionActive         bool
	deceptionTargetID       string
	deceptionMode           string
	deceptionPoint          *model.GeoPoint
	deceptionAltitudeM      float64
	deceptionSignalMask     uint16
	deceptionStrengthPreset string
	deceptionAttenuationDB  int
	deceptionDelayMode      string
	deceptionDelayNS        float64
	deceptionDistanceM      float64
	deceptionSummary        string
	deceptionCircle         *model.ScreenDeceptionCircleParams
	deceptionLinear         *model.ScreenDeceptionLinearParams
	deceptionRandom         *model.ScreenDeceptionRandomParams
	unsupportedReason       string
	lastAck                 string
	lastError               string
}

type session struct {
	id            string
	request       model.DeceptionSessionRequest
	config        serialport.Config
	client        *serialClient
	startedAt     time.Time
	locale        string
	state         string
	autoReconnect bool
	lastError     string
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewService 创建 GNSS 诱骗服务。
func NewService(
	store *store.MemoryStore,
	translator *i18n.Translator,
	settingsStore SettingsStore,
	options Options,
) *Service {
	return &Service{
		store:                   store,
		translator:              translator,
		settings:                settingsStore,
		options:                 normalizeOptions(options),
		openPort:                openSerialPort,
		deceptionMode:           defaultDeceptionMode,
		deceptionSignalMask:     protocol.SignalAllSupported,
		deceptionStrengthPreset: deceptionStrengthStandard,
		deceptionAttenuationDB:  defaultDeceptionAttenuationDB,
		deceptionDelayMode:      deceptionDelayAuto,
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

// SetReportStore 设置诱骗报告持久化存储。
func (s *Service) SetReportStore(reports ReportStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = reports
}

// Settings 加载已持久化的 GNSS 诱骗串口设置。
func (s *Service) Settings() (model.DeceptionSessionRequest, bool, error) {
	if s.settings == nil {
		return model.DeceptionSessionRequest{}, false, nil
	}
	return s.settings.LoadDeception()
}

// Configured 返回 GNSS 诱骗串口是否已保存有效配置。
func (s *Service) Configured() (model.DeceptionSessionRequest, bool, error) {
	settings, ok, err := s.Settings()
	if err != nil || !ok {
		return model.DeceptionSessionRequest{}, false, err
	}
	return settings, strings.TrimSpace(settings.PortName) != "", nil
}

// ClearSettings 停止当前 GNSS 诱骗串口会话并清空已保存的串口设置。
func (s *Service) ClearSettings(locale string) (model.DeceptionSessionResponse, error) {
	if err := s.saveSettings(model.DeceptionSessionRequest{}); err != nil {
		return model.DeceptionSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
	}
	return s.Stop(locale), nil
}

// Start 保存设置、打开串口并启动帧读取循环。
func (s *Service) Start(req model.DeceptionSessionRequest, locale string) (model.DeceptionSessionResponse, error) {
	req = s.normalizeRequest(req)
	if strings.TrimSpace(req.PortName) == "" {
		return model.DeceptionSessionResponse{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "deception_port_required"))
	}
	req.AutoConnect = true

	if err := s.saveSettings(req); err != nil {
		return model.DeceptionSessionResponse{}, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "internal"), err)
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
		if prev.client != nil {
			prev.client.Close()
		}
	}

	client, err := s.connectOnce(&cfg, locale)
	if err == nil {
		if !s.assignConnectedClient(seq, sess, client) {
			client.Close()
			return s.Current(locale), nil
		}
		response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "deception.session.started"))
		s.publish("deception.session.started", response)
		return response, nil
	}

	localizedErr := localizedDisplayError(locale, err.Error())
	s.setSessionFailure(seq, sess, "connecting", localizedErr)
	response := s.responseForSession(sess, locale, s.translator.T(locale, "common", "deception.session.inactive"))
	response.LastError = localizedErr
	response.Active = false
	s.publish("deception.session.connecting", response)
	return response, nil
}

// Stop 关闭当前 GNSS 诱骗串口会话。
func (s *Service) Stop(locale string) model.DeceptionSessionResponse {
	s.mu.Lock()
	prev := s.current
	s.sequence++
	s.current = nil
	s.clearScreenDeceptionLocked()
	s.mu.Unlock()

	if prev == nil {
		return model.DeceptionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "deception.session.inactive"),
		}
	}

	prev.cancel()
	if prev.client != nil {
		s.stopAllTransmitBestEffort(prev.client, prev.locale)
		prev.client.Close()
	}

	response := s.responseForSession(prev, locale, s.translator.T(locale, "common", "deception.session.stopped"))
	response.Active = false
	response.State = "inactive"
	response.AutoReconnect = false
	s.publish("deception.session.stopped", response)
	s.publishScreenDeception(s.ScreenState())
	return response
}

// Current 返回当前 GNSS 诱骗串口会话状态。
func (s *Service) Current(locale string) model.DeceptionSessionResponse {
	s.mu.RLock()
	current := s.current
	s.mu.RUnlock()

	if current == nil {
		return model.DeceptionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: s.translator.T(locale, "common", "deception.session.inactive"),
		}
	}
	return s.responseForSession(current, locale, s.messageForState(current.state, locale))
}

// Records 返回最新 GNSS 诱骗协议交互记录。
func (s *Service) Records(limit int) []model.DeceptionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.records) {
		limit = len(s.records)
	}
	out := make([]model.DeceptionRecord, 0, limit)
	for i := len(s.records) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.records[i])
	}
	return out
}

// Query 查询 GNSS 诱骗设备状态项，用于开发调试反显。
func (s *Service) Query(item string, locale string) (model.DeceptionQueryResponse, error) {
	query, ok := deceptionQueryCommand(strings.TrimSpace(item))
	if !ok {
		return model.DeceptionQueryResponse{}, s.localizedError(locale, "deception_invalid_query")
	}
	client, err := s.activeClient()
	if err != nil {
		s.setScreenLastError(localizedDisplayError(locale, err.Error()))
		return model.DeceptionQueryResponse{}, s.localizedError(locale, "deception_serial_inactive")
	}

	frame, raw, err := client.SendQuery(context.Background(), query)
	if err != nil {
		return model.DeceptionQueryResponse{}, err
	}
	return model.DeceptionQueryResponse{
		Item:        strings.TrimSpace(item),
		Command:     fmt.Sprintf("0x%02X", query),
		RawHex:      protocol.Hex(raw),
		Description: protocol.DescribeFrameLocale(frame, locale),
		Message:     s.translator.T(locale, "common", "deception.query.ok"),
	}, nil
}

// RestoreSavedSettings 在存在已保存设置时自动恢复 GNSS 诱骗串口会话。
func (s *Service) RestoreSavedSettings(locale string) {
	if s.settings == nil {
		return
	}
	req, ok, err := s.settings.LoadDeception()
	if err != nil || !ok || !req.AutoConnect {
		return
	}
	_, _ = s.Start(req, locale)
}

// ScreenState 返回大屏诱骗控制当前状态。
func (s *Service) ScreenState() model.ScreenDeceptionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.screenStateLocked()
}

// ScreenDeviceStatus 查询并返回大屏诱骗设备完整只读状态。
func (s *Service) ScreenDeviceStatus(locale string) model.ScreenDeceptionDeviceStatus {
	status := model.ScreenDeceptionDeviceStatus{
		RawDescriptions: map[string]string{},
		QueryErrors:     map[string]string{},
	}

	client, err := s.activeClient()
	if err != nil {
		status.LastError = s.screenLastError()
		if status.LastError == "" {
			status.LastError = s.translator.T(locale, "errors", "deception_serial_inactive")
		}
		return status
	}

	s.serialOperationMu.Lock()
	defer s.serialOperationMu.Unlock()

	status.SerialActive = true
	updatedAt := time.Now()
	status.UpdatedAt = &updatedAt

	query := func(name string, command byte, apply func(protocol.Frame) error) {
		status.RawDescriptions[name] = formatQueryRawDescription(command, nil, fmt.Errorf("%s", protocol.TextLocale(locale, "waiting_response")), locale)
		frame, raw, err := client.SendQuery(context.Background(), command)
		if err != nil {
			status.QueryErrors[name] = protocol.LocalizeErrorText(err.Error(), locale)
			status.RawDescriptions[name] = formatQueryRawDescription(command, nil, err, locale)
			return
		}
		status.RawDescriptions[name] = formatQueryRawDescription(command, raw, nil, locale)
		if err := apply(frame); err != nil {
			status.QueryErrors[name] = protocol.LocalizeErrorText(err.Error(), locale)
		}
	}
	queryBurst := func(name string, command byte, apply func([]protocol.Frame) error) {
		status.RawDescriptions[name] = formatQueryRawDescription(command, nil, fmt.Errorf("%s", protocol.TextLocale(locale, "waiting_response")), locale)
		frames, raws, err := client.SendQueryBurst(context.Background(), command, deceptionQueryBurstIdleTimeout)
		if err != nil {
			status.QueryErrors[name] = protocol.LocalizeErrorText(err.Error(), locale)
			status.RawDescriptions[name] = formatQueryRawDescription(command, nil, err, locale)
			return
		}
		descriptions := make([]string, 0, len(frames))
		for index := range frames {
			rawKey := name
			if len(frames) > 1 {
				rawKey = fmt.Sprintf("%s_%02d", name, index+1)
			}
			description := ""
			if index < len(raws) {
				description = formatQueryRawDescription(command, raws[index], nil, locale)
			} else {
				description = formatQueryRawDescription(command, nil, nil, locale)
			}
			status.RawDescriptions[rawKey] = description
			descriptions = append(descriptions, description)
		}
		if len(descriptions) > 0 {
			status.RawDescriptions[name] = strings.Join(descriptions, "\n\n")
		}
		if err := apply(frames); err != nil {
			status.QueryErrors[name] = protocol.LocalizeErrorText(err.Error(), locale)
		}
	}

	query("status", protocol.QueryDeviceStatus, func(frame protocol.Frame) error {
		report, err := protocol.ParseDeviceStatusReport(frame)
		if err != nil {
			return err
		}
		applyDeviceStatusReport(&status, report)
		return nil
	})
	query("version", protocol.QueryFirmwareVersion, func(frame protocol.Frame) error {
		report, err := protocol.ParseVersionReport(frame)
		if err != nil {
			return err
		}
		status.Version = &model.ScreenDeceptionVersionStatus{
			Software: report.Software,
			FPGA:     report.FPGA,
			Protocol: report.Protocol,
		}
		return nil
	})
	query("system_time", protocol.QuerySystemTime, func(frame protocol.Frame) error {
		report, err := protocol.ParseSystemTimeReport(frame)
		if err != nil {
			return err
		}
		status.ReportedSystemTime = cloneTime(report)
		return nil
	})
	query("transmit", protocol.QueryTransmitSwitch, func(frame protocol.Frame) error {
		report, err := protocol.ParseTransmitSwitchReport(frame)
		if err != nil {
			return err
		}
		status.TransmitMask = &report.Mask
		status.TransmitSignals = protocol.SignalNames(report.Mask)
		return nil
	})
	query("simulated_position", protocol.QuerySimulatedPosition, func(frame protocol.Frame) error {
		report, err := protocol.ParsePositionReport(frame, protocol.QuerySimulatedPosition)
		if err != nil {
			return err
		}
		status.QueriedSimulatedPosition = statusPointFromProtocol(&report)
		return nil
	})
	query("device_position", protocol.QueryDevicePosition, func(frame protocol.Frame) error {
		report, err := protocol.ParsePositionReport(frame, protocol.QueryDevicePosition)
		if err != nil {
			return err
		}
		status.QueriedDevicePosition = statusPointFromProtocol(&report)
		return nil
	})
	query("target_position", protocol.QueryTargetPosition, func(frame protocol.Frame) error {
		report, err := protocol.ParseTargetPositionReport(frame)
		if err != nil {
			return err
		}
		status.TargetPosition = &model.ScreenDeceptionTargetStatus{
			DistanceM:    report.DistanceM,
			HeightM:      report.HeightM,
			DirectionDeg: report.DirectionDeg,
			HeadingDeg:   report.HeadingDeg,
		}
		return nil
	})
	query("spoof_circle", protocol.QuerySpoofCircle, func(frame protocol.Frame) error {
		report, err := protocol.ParseSpoofCircleReport(frame)
		if err != nil {
			return err
		}
		status.SpoofCircle = &model.ScreenDeceptionSpoofCircleStatus{
			DistanceM:     report.DistanceM,
			HeightM:       report.HeightM,
			DirectionDeg:  report.DirectionDeg,
			HeadingDeg:    report.HeadingDeg,
			RadiusM:       report.RadiusM,
			PeriodSeconds: report.PeriodSeconds,
			Direction:     report.Direction,
		}
		return nil
	})
	query("random", protocol.QueryRandomPosition, func(frame protocol.Frame) error {
		report, err := protocol.ParseRandomPositionReport(frame)
		if err != nil {
			return err
		}
		status.Random = &model.ScreenDeceptionRandomStatus{
			Enabled:        report.Enabled,
			RadiusM:        report.RadiusM,
			RefreshSeconds: report.RefreshSeconds,
		}
		return nil
	})
	query("attenuation", protocol.QueryPowerAttenuation, func(frame protocol.Frame) error {
		report, err := protocol.ParsePowerAttenuationReport(frame)
		if err != nil {
			return err
		}
		status.Attenuation = &model.ScreenDeceptionAttenuationStatus{
			GPS: report.GPS,
			BDS: report.BDS,
			GLO: report.GLO,
			GAL: report.GAL,
		}
		return nil
	})
	query("delay", protocol.QuerySignalDelay, func(frame protocol.Frame) error {
		report, err := protocol.ParseSignalDelayReport(frame)
		if err != nil {
			return err
		}
		status.DelayBySignalNS = &model.ScreenDeceptionDelayStatus{
			GPS: cloneFloat64(report.GPS),
			BDS: cloneFloat64(report.BDS),
			GLO: cloneFloat64(report.GLO),
			GAL: cloneFloat64(report.GAL),
		}
		status.DelayNS = firstSignalDelay(report)
		return nil
	})
	query("timed_search", protocol.QueryTimedSearch, func(frame protocol.Frame) error {
		report, err := protocol.ParseTimedSearchReport(frame)
		if err != nil {
			return err
		}
		status.TimedSearch = &report.Enabled
		return nil
	})
	queryBurst("device_signal", protocol.QueryDeviceSignal, func(frames []protocol.Frame) error {
		reports := make([]protocol.DeviceSignalReport, 0, len(frames))
		for _, frame := range frames {
			report, err := protocol.ParseDeviceSignalReport(frame)
			if err != nil {
				return err
			}
			reports = append(reports, report)
		}
		applyDeviceSignalReports(&status, reports)
		return nil
	})

	return status
}

// SetScreenDeception 更新大屏诱骗控制状态。
func (s *Service) SetScreenDeception(
	req model.ScreenDeceptionRequest,
	devicePoint model.GeoPoint,
	deviceAltitudeM float64,
	hasDevicePoint bool,
	locale string,
) (model.ScreenDeceptionState, error) {
	if !req.Enabled {
		return s.stopScreenDeception(locale)
	}
	return s.startScreenDeception(req, devicePoint, deviceAltitudeM, hasDevicePoint, locale)
}

// Shutdown 关闭发射并释放串口。
func (s *Service) Shutdown() {
	s.CloseActiveReportAbnormal("service_shutdown")

	s.mu.Lock()
	prev := s.current
	s.sequence++
	s.current = nil
	s.clearScreenDeceptionLocked()
	s.mu.Unlock()

	if prev != nil {
		prev.cancel()
		if prev.client != nil {
			s.stopAllTransmitBestEffort(prev.client, prev.locale)
			prev.client.Close()
		}
	}
}

// CloseActiveReportAbnormal marks the in-memory running report abnormal.
func (s *Service) CloseActiveReportAbnormal(reason string) {
	report := s.currentActiveReport()
	if report.ID == "" {
		return
	}
	endedAt := time.Now()
	state := s.ScreenState()
	report.Status = model.DeceptionReportStatusAbnormal
	report.EndedAt = &endedAt
	report.EndState = cloneScreenState(&state)
	report.AbnormalReason = strings.TrimSpace(reason)
	if report.AbnormalReason == "" {
		report.AbnormalReason = "abnormal"
	}
	if report.LastError == "" {
		report.LastError = report.AbnormalReason
	}
	if records := s.activeReportRecords(report.ID); records != nil {
		report.Records = records
	}
	report.RecordCount = len(report.Records)
	_ = s.updateReport(report)
	s.clearActiveReportID(report.ID)
}

func (s *Service) startScreenDeception(
	req model.ScreenDeceptionRequest,
	devicePoint model.GeoPoint,
	deviceAltitudeM float64,
	hasDevicePoint bool,
	locale string,
) (model.ScreenDeceptionState, error) {
	config, err := normalizeScreenDeceptionRequest(req, devicePoint, hasDevicePoint, locale)
	if err != nil {
		return s.ScreenState(), s.localizedError(locale, errorCodeForNormalizeError(err))
	}

	client, err := s.activeClient()
	if err != nil {
		s.setScreenLastError(localizedDisplayError(locale, err.Error()))
		return s.ScreenState(), s.localizedError(locale, "deception_serial_inactive")
	}

	startedAt := time.Now()
	recordStart := s.recordCount()
	sessionResponse := s.Current(locale)

	s.serialOperationMu.Lock()
	var commands []commandFrame
	if requiresQueriedStatusPosition(config.mode) {
		config.devicePosition, err = queryCurrentPositionForMotion(client, locale)
		if err == nil {
			config.longitude = config.devicePosition.Longitude
			config.latitude = config.devicePosition.Latitude
			config.altitudeM = config.devicePosition.AltitudeM
		}
	}
	if err == nil {
		commands, err = buildStartCommands(config, locale)
	}
	var lastAck string
	if err == nil {
		lastAck, err = s.sendCommandSequence(client, commands, locale)
	}
	s.serialOperationMu.Unlock()
	if err != nil {
		records := s.recordsSince(recordStart)
		s.createFailedReport(req, sessionResponse, startedAt, config.summary, records, err, locale)
		if len(commands) > 0 {
			s.serialOperationMu.Lock()
			_, _ = s.sendStopForMode(client, config.mode, locale)
			s.serialOperationMu.Unlock()
		}
		s.setScreenLastError(localizedDisplayError(locale, err.Error()))
		return s.ScreenState(), err
	}

	var point *model.GeoPoint
	if config.mode == deceptionModeFixedPoint {
		point = &model.GeoPoint{Latitude: config.latitude, Longitude: config.longitude}
	} else if requiresQueriedStatusPosition(config.mode) && config.devicePosition != nil {
		point = &model.GeoPoint{Latitude: config.latitude, Longitude: config.longitude}
	}

	s.mu.Lock()
	s.deceptionActive = true
	s.deceptionTargetID = strings.TrimSpace(config.targetID)
	s.deceptionMode = config.mode
	s.deceptionPoint = point
	s.deceptionAltitudeM = config.altitudeM
	s.deceptionSignalMask = config.signalMask
	s.deceptionStrengthPreset = config.strengthPreset
	s.deceptionAttenuationDB = config.attenuationDB
	s.deceptionDelayMode = config.delayMode
	s.deceptionDelayNS = config.delayNS
	s.deceptionDistanceM = config.distanceM
	s.deceptionSummary = ""
	s.deceptionCircle = cloneCircleParams(config.circle)
	s.deceptionLinear = cloneLinearParams(config.linear)
	s.deceptionRandom = nil
	s.unsupportedReason = ""
	s.lastAck = lastAck
	s.lastError = ""
	state := s.screenStateLocked()
	s.mu.Unlock()

	report := s.createRunningReport(req, sessionResponse, state, startedAt, config.summary, recordStart, locale)
	if report.ID != "" {
		startStatus := s.ScreenDeviceStatus(locale)
		report.StartDeviceStatus = cloneDeviceStatus(&startStatus)
		report.RawDescriptions = mergeStringMaps(report.RawDescriptions, startStatus.RawDescriptions)
		report.QueryErrors = mergeStringMaps(report.QueryErrors, startStatus.QueryErrors)
		report.Records = s.recordsSince(recordStart)
		report.RecordCount = len(report.Records)
		if err := s.updateReport(report); err == nil {
			s.setActiveReport(report)
		}
	}

	s.publishScreenDeception(state)
	return state, nil
}

func (s *Service) stopScreenDeception(locale string) (model.ScreenDeceptionState, error) {
	client, err := s.activeClient()
	var stopErr error
	var lastAck string
	report := s.currentActiveReport()
	if report.ID != "" {
		beforeStatus := s.ScreenDeviceStatus(locale)
		report.BeforeStopStatus = cloneDeviceStatus(&beforeStatus)
		report.RawDescriptions = mergeStringMaps(report.RawDescriptions, prefixStringMap("before_stop.", beforeStatus.RawDescriptions))
		report.QueryErrors = mergeStringMaps(report.QueryErrors, prefixStringMap("before_stop.", beforeStatus.QueryErrors))
	}
	if err == nil {
		mode := s.currentScreenDeceptionMode()
		s.serialOperationMu.Lock()
		lastAck, stopErr = s.sendStopForMode(client, mode, locale)
		s.serialOperationMu.Unlock()
	} else if !errors.Is(err, errSerialInactive) {
		stopErr = err
	}

	s.mu.Lock()
	s.clearScreenDeceptionLocked()
	if lastAck != "" {
		s.lastAck = lastAck
	}
	if stopErr != nil {
		s.lastError = localizedDisplayError(locale, stopErr.Error())
	}
	state := s.screenStateLocked()
	s.mu.Unlock()

	if report.ID != "" {
		report.EndState = cloneScreenState(&state)
		endedAt := time.Now()
		report.EndedAt = &endedAt
		report.Status = model.DeceptionReportStatusCompleted
		if stopErr != nil {
			report.LastError = localizedDisplayError(locale, stopErr.Error())
		}
		if err == nil {
			afterStatus := s.ScreenDeviceStatus(locale)
			report.AfterStopStatus = cloneDeviceStatus(&afterStatus)
			report.RawDescriptions = mergeStringMaps(report.RawDescriptions, prefixStringMap("after_stop.", afterStatus.RawDescriptions))
			report.QueryErrors = mergeStringMaps(report.QueryErrors, prefixStringMap("after_stop.", afterStatus.QueryErrors))
		}
		if records := s.activeReportRecords(report.ID); records != nil {
			report.Records = records
		}
		report.RecordCount = len(report.Records)
		_ = s.updateReport(report)
		s.clearActiveReportID(report.ID)
	}

	s.publishScreenDeception(state)
	return state, stopErr
}

func (s *Service) connectOnce(cfg *serialport.Config, locale string) (*serialClient, error) {
	s.mu.RLock()
	openPort := s.openPort
	s.mu.RUnlock()

	port, err := openPort(cfg)
	if err != nil {
		return nil, err
	}
	client := newSerialClient(port, cfg.PortName, s.options.CommandTimeout, locale, s.record)
	client.Start()
	return client, nil
}

func (s *Service) assignConnectedClient(seq uint64, sess *session, c *serialClient) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return false
	}
	sess.client = c
	sess.state = "connected"
	sess.lastError = ""
	return true
}

func (s *Service) activeClient() (*serialClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil || s.current.client == nil || s.current.state != "connected" {
		return nil, errSerialInactive
	}
	return s.current.client, nil
}

func (s *Service) setSessionFailure(seq uint64, sess *session, state, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sequence != seq || s.current != sess {
		return
	}
	sess.state = state
	sess.lastError = lastErr
}

func (s *Service) setScreenLastError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = message
}

func (s *Service) screenLastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *Service) saveSettings(req model.DeceptionSessionRequest) error {
	if s.settings == nil {
		return nil
	}
	return s.settings.SaveDeception(req)
}

func (s *Service) normalizeRequest(req model.DeceptionSessionRequest) model.DeceptionSessionRequest {
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

func (s *Service) buildConfig(req model.DeceptionSessionRequest) serialport.Config {
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

func (s *Service) responseForSession(sess *session, locale, message string) model.DeceptionSessionResponse {
	if sess == nil {
		return model.DeceptionSessionResponse{
			Active:  false,
			State:   "inactive",
			Message: message,
		}
	}
	active := sess.state == "connected"
	return model.DeceptionSessionResponse{
		Active:        active,
		SessionID:     sess.id,
		PortName:      sess.config.PortName,
		BaudRate:      sess.config.BaudRate,
		DataBits:      sess.config.DataBits,
		StopBits:      sess.config.StopBits,
		Parity:        sess.config.Parity,
		StartedAt:     sess.startedAt,
		State:         sess.state,
		AutoReconnect: sess.autoReconnect,
		LastError:     sess.lastError,
		Message:       message,
	}
}

func (s *Service) messageForState(state, locale string) string {
	switch state {
	case "connected":
		return s.translator.T(locale, "common", "deception.session.started")
	case "connecting":
		return s.translator.T(locale, "common", "deception.session.connecting")
	case "inactive":
		return s.translator.T(locale, "common", "deception.session.inactive")
	default:
		return s.translator.T(locale, "common", "deception.session.inactive")
	}
}

func (s *Service) sendCommandSequence(client *serialClient, commands []commandFrame, locale string) (string, error) {
	lastAck := ""
	for _, command := range commands {
		ack, err := client.SendAndWaitAck(context.Background(), command.code, command.frame)
		if err != nil {
			return lastAck, fmt.Errorf("%s", protocol.TextLocale(locale, "command_failed", command.name, protocol.LocalizeErrorText(err.Error(), locale)))
		}
		lastAck = protocol.TextLocale(locale, "ack_result", command.name, ack.ReturnValue, ack.ErrorCode)
		if !ack.Success() {
			return lastAck, fmt.Errorf("%s", protocol.TextLocale(locale, "command_ack_failed", command.name, protocol.AckErrorTextLocale(ack.ErrorCode, locale)))
		}
	}
	return lastAck, nil
}

func queryCurrentPositionForMotion(client *serialClient, locale string) (*protocol.PositionReport, error) {
	frame, _, err := client.SendQuery(context.Background(), protocol.QueryDeviceStatus)
	if err != nil {
		return nil, fmt.Errorf("%s", protocol.TextLocale(locale, "query_status_failed", protocol.LocalizeErrorText(err.Error(), locale)))
	}
	report, err := protocol.ParseDeviceStatusReport(frame)
	if err != nil {
		return nil, fmt.Errorf("%s", protocol.TextLocale(locale, "parse_status_failed", protocol.LocalizeErrorText(err.Error(), locale)))
	}
	if report.CurrentPosition == nil {
		return nil, fmt.Errorf("%s", protocol.TextLocale(locale, "status_missing_pos"))
	}
	if !validLocation(report.CurrentPosition.Longitude, report.CurrentPosition.Latitude) {
		return nil, fmt.Errorf("%s", protocol.TextLocale(locale, "status_invalid_pos"))
	}
	return report.CurrentPosition, nil
}

func formatQueryRawDescription(command byte, responseRaw []byte, err error, locale string) string {
	queryFrame, buildErr := protocol.BuildQuery(command)
	parts := []string{}
	if buildErr == nil {
		parts = append(parts, fmt.Sprintf("TX %s", protocol.Hex(queryFrame)))
	} else {
		parts = append(parts, protocol.TextLocale(locale, "tx_build_failed", protocol.LocalizeErrorText(buildErr.Error(), locale)))
	}
	if len(responseRaw) > 0 {
		if frame, parseErr := protocol.ParseFrame(responseRaw); parseErr == nil {
			parts = append(parts, fmt.Sprintf("RX %s", protocol.Hex(responseRaw)))
			parts = append(parts, protocol.DescribeFrameLocale(frame, locale))
		} else {
			parts = append(parts, fmt.Sprintf("RX %s", protocol.Hex(responseRaw)))
			parts = append(parts, protocol.TextLocale(locale, "parse_failed", protocol.LocalizeErrorText(parseErr.Error(), locale)))
		}
	}
	if err != nil {
		parts = append(parts, fmt.Sprintf("ERR %s", protocol.LocalizeErrorText(err.Error(), locale)))
	}
	return strings.Join(parts, "\n")
}

func requiresQueriedStatusPosition(mode string) bool {
	return mode == deceptionModeCircle || mode == deceptionModeLinear
}

func (s *Service) sendStopTransmit(client *serialClient, locale string) (string, error) {
	frame, err := protocol.BuildSetTransmitSwitch(0)
	if err != nil {
		return "", err
	}
	ack, err := client.SendAndWaitAck(context.Background(), protocol.CmdTransmitSwitch, frame)
	if err != nil {
		return "", err
	}
	commandName := protocol.TextLocale(locale, "stop_transmit")
	result := protocol.TextLocale(locale, "ack_result", commandName, ack.ReturnValue, ack.ErrorCode)
	if !ack.Success() {
		return result, fmt.Errorf("%s", protocol.TextLocale(locale, "command_ack_failed", commandName, protocol.AckErrorTextLocale(ack.ErrorCode, locale)))
	}
	return result, nil
}

func (s *Service) sendStopForMode(client *serialClient, mode string, locale string) (string, error) {
	return s.sendStopTransmit(client, locale)
}

func (s *Service) stopAllTransmitBestEffort(client *serialClient, locale string) {
	if client == nil {
		return
	}
	s.serialOperationMu.Lock()
	defer s.serialOperationMu.Unlock()
	_, _ = s.sendStopTransmit(client, locale)
}

func (s *Service) currentScreenDeceptionMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.deceptionMode
}

func (s *Service) screenStateLocked() model.ScreenDeceptionState {
	point := cloneGeoPoint(s.deceptionPoint)
	state := model.ScreenDeceptionState{
		Active:            s.deceptionActive,
		TargetID:          s.deceptionTargetID,
		Mode:              firstNonEmpty(s.deceptionMode, defaultDeceptionMode),
		Point:             point,
		AltitudeM:         s.deceptionAltitudeM,
		SignalMask:        s.deceptionSignalMask,
		StrengthPreset:    firstNonEmpty(s.deceptionStrengthPreset, deceptionStrengthStandard),
		AttenuationDB:     s.deceptionAttenuationDB,
		DelayMode:         firstNonEmpty(s.deceptionDelayMode, deceptionDelayOff),
		DelayNS:           s.deceptionDelayNS,
		DistanceM:         s.deceptionDistanceM,
		Summary:           s.deceptionSummary,
		UnsupportedReason: s.unsupportedReason,
		Circle:            cloneCircleParams(s.deceptionCircle),
		Linear:            cloneLinearParams(s.deceptionLinear),
		Random:            cloneRandomParams(s.deceptionRandom),
		SerialActive:      s.current != nil && s.current.state == "connected",
		LastAck:           s.lastAck,
		LastError:         s.lastError,
	}
	return state
}

func (s *Service) clearScreenDeceptionLocked() {
	s.deceptionActive = false
	s.deceptionTargetID = ""
	s.deceptionMode = defaultDeceptionMode
	s.deceptionPoint = nil
	s.deceptionAltitudeM = 0
	s.deceptionSignalMask = protocol.SignalAllSupported
	s.deceptionStrengthPreset = deceptionStrengthStandard
	s.deceptionAttenuationDB = defaultDeceptionAttenuationDB
	s.deceptionDelayMode = deceptionDelayAuto
	s.deceptionDelayNS = 0
	s.deceptionDistanceM = 0
	s.deceptionSummary = ""
	s.deceptionCircle = nil
	s.deceptionLinear = nil
	s.deceptionRandom = nil
	s.unsupportedReason = ""
}

func cloneScreenState(state *model.ScreenDeceptionState) *model.ScreenDeceptionState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Point = cloneGeoPoint(state.Point)
	cloned.Circle = cloneCircleParams(state.Circle)
	cloned.Linear = cloneLinearParams(state.Linear)
	cloned.Random = cloneRandomParams(state.Random)
	return &cloned
}

func cloneDeviceStatus(status *model.ScreenDeceptionDeviceStatus) *model.ScreenDeceptionDeviceStatus {
	if status == nil {
		return nil
	}
	data, err := json.Marshal(status)
	if err != nil {
		return nil
	}
	var cloned model.ScreenDeceptionDeviceStatus
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return &cloned
}

func cloneReport(report model.DeceptionReport) model.DeceptionReport {
	data, err := json.Marshal(report)
	if err != nil {
		return report
	}
	var cloned model.DeceptionReport
	if err := json.Unmarshal(data, &cloned); err != nil {
		return report
	}
	return cloned
}

func mergeStringMaps(base map[string]string, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func prefixStringMap(prefix string, values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[prefix+key] = value
	}
	return out
}

func (s *Service) publishScreenDeception(state model.ScreenDeceptionState) {
	s.publish(screenDeceptionEventType, state)
}

func (s *Service) publish(eventType string, payload any) {
	if s.store == nil {
		return
	}
	s.store.Publish(model.Event{Type: eventType, Time: time.Now(), Payload: payload})
}

func (s *Service) recordCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

func (s *Service) recordsSince(index int) []model.DeceptionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 {
		index = 0
	}
	if index > len(s.records) {
		index = len(s.records)
	}
	out := make([]model.DeceptionRecord, len(s.records[index:]))
	copy(out, s.records[index:])
	return out
}

func (s *Service) currentReportStore() ReportStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reports
}

func (s *Service) createRunningReport(
	req model.ScreenDeceptionRequest,
	session model.DeceptionSessionResponse,
	state model.ScreenDeceptionState,
	startedAt time.Time,
	summary string,
	recordStart int,
	locale string,
) model.DeceptionReport {
	reports := s.currentReportStore()
	if reports == nil {
		return model.DeceptionReport{}
	}
	records := s.recordsSince(recordStart)
	report := model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			Status:    model.DeceptionReportStatusRunning,
			StartedAt: startedAt,
			PortName:  session.PortName,
			Summary:   strings.TrimSpace(summary),
		},
		Request:     req,
		Session:     session,
		StartState:  cloneScreenState(&state),
		Records:     records,
		RecordCount: len(records),
	}
	report.RawDescriptions = map[string]string{}
	report.QueryErrors = map[string]string{}
	created, err := reports.CreateRunning(report)
	if err != nil {
		s.setScreenLastError(localizedDisplayError(locale, err.Error()))
		return model.DeceptionReport{}
	}
	return created
}

func (s *Service) createFailedReport(
	req model.ScreenDeceptionRequest,
	session model.DeceptionSessionResponse,
	startedAt time.Time,
	summary string,
	records []model.DeceptionRecord,
	cause error,
	locale string,
) {
	reports := s.currentReportStore()
	if reports == nil || cause == nil {
		return
	}
	endedAt := time.Now()
	report := model.DeceptionReport{
		DeceptionReportSummary: model.DeceptionReportSummary{
			Status:    model.DeceptionReportStatusFailed,
			StartedAt: startedAt,
			EndedAt:   &endedAt,
			PortName:  session.PortName,
			Summary:   strings.TrimSpace(summary),
			LastError: localizedDisplayError(locale, cause.Error()),
		},
		Request:     req,
		Session:     session,
		Records:     records,
		RecordCount: len(records),
	}
	created, err := reports.Create(report)
	if err != nil {
		s.setScreenLastError(localizedDisplayError(locale, err.Error()))
		return
	}
	_ = created
}

func (s *Service) updateReport(report model.DeceptionReport) error {
	reports := s.currentReportStore()
	if reports == nil || report.ID == "" {
		return nil
	}
	return reports.Update(report)
}

func (s *Service) setActiveReport(report model.DeceptionReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeReportID = report.ID
	cloned := cloneReport(report)
	s.activeReport = &cloned
}

func (s *Service) currentActiveReport() model.DeceptionReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.activeReport == nil {
		return model.DeceptionReport{}
	}
	return cloneReport(*s.activeReport)
}

func (s *Service) clearActiveReportID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeReportID == id {
		s.activeReportID = ""
		s.activeReport = nil
	}
}

func (s *Service) activeReportRecords(id string) []model.DeceptionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.activeReportID != id || s.activeReport == nil {
		return nil
	}
	out := make([]model.DeceptionRecord, len(s.activeReport.Records))
	copy(out, s.activeReport.Records)
	return out
}

func applyDeviceStatusReport(status *model.ScreenDeceptionDeviceStatus, report protocol.DeviceStatusReport) {
	status.SystemTime = cloneTime(report.SystemTime)
	status.OscillatorState = report.OscillatorState
	status.CurrentPosition = statusPointFromProtocol(report.CurrentPosition)
	status.SimulatedPosition = statusPointFromProtocol(report.SimulatedPosition)
	status.TemperatureC = cloneFloat64(report.TemperatureC)
	status.TimePrecisionNS = cloneFloat64(report.TimePrecisionNS)
	status.UptimeSeconds = cloneUint32(report.UptimeSeconds)
	status.AmplifierOn = cloneBool(report.AmplifierOn)
	status.AutoTransmit = cloneBool(report.AutoTransmit)
	status.FirstTimeSynced = cloneBool(report.FirstTimeSynced)
	if report.SyncStatus != nil {
		status.SyncStatus = &model.ScreenDeceptionSyncStatus{
			ReceiverWorking:    report.SyncStatus.ReceiverWorking,
			ReceiverPositioned: report.SyncStatus.ReceiverPositioned,
			LeapSecondValid:    report.SyncStatus.LeapSecondValid,
			TimeSynced:         report.SyncStatus.TimeSynced,
			AntennaOK:          report.SyncStatus.AntennaOK,
		}
	}
	if report.Motion != nil {
		status.Motion = &model.ScreenDeceptionMotionStatus{
			MaxSpeedMPS:              cloneFloat64(report.Motion.MaxSpeedMPS),
			InitialSpeedMPS:          cloneFloat64(report.Motion.InitialSpeedMPS),
			InitialDirectionDeg:      cloneFloat64(report.Motion.InitialDirectionDeg),
			AccelerationMPS2:         cloneFloat64(report.Motion.AccelerationMPS2),
			AccelerationDirectionDeg: cloneFloat64(report.Motion.AccelerationDirectionDeg),
			CircleRadiusM:            cloneFloat64(report.Motion.CircleRadiusM),
			CirclePeriodSeconds:      cloneFloat64(report.Motion.CirclePeriodSeconds),
			CircleDirection:          report.Motion.CircleDirection,
		}
	}
	if report.SignalMask != nil && status.TransmitMask == nil {
		mask := *report.SignalMask
		status.TransmitMask = &mask
		status.TransmitSignals = protocol.SignalNames(mask)
	}
}

func applyDeviceSignalReport(status *model.ScreenDeceptionDeviceStatus, report protocol.DeviceSignalReport) {
	converted := screenDeviceSignalFromProtocol(report)
	status.DeviceSignal = &converted
	status.DeviceSignals = []model.ScreenDeceptionDeviceSignalStatus{converted}
}

func applyDeviceSignalReports(status *model.ScreenDeceptionDeviceStatus, reports []protocol.DeviceSignalReport) {
	if len(reports) == 0 {
		return
	}
	signals := make([]model.ScreenDeceptionDeviceSignalStatus, 0, len(reports))
	var aggregateMask uint16
	transmitSwitch := false
	for _, report := range reports {
		converted := screenDeviceSignalFromProtocol(report)
		signals = append(signals, converted)
		aggregateMask |= converted.SignalMask
		transmitSwitch = transmitSwitch || converted.TransmitSwitch
	}
	aggregate := signals[0]
	aggregate.SignalMask = aggregateMask
	aggregate.SignalNames = protocol.SignalNames(aggregateMask)
	aggregate.TransmitSwitch = transmitSwitch
	status.DeviceSignal = &aggregate
	status.DeviceSignals = signals
}

func screenDeviceSignalFromProtocol(report protocol.DeviceSignalReport) model.ScreenDeceptionDeviceSignalStatus {
	return model.ScreenDeceptionDeviceSignalStatus{
		SystemTime:  cloneTime(report.SystemTime),
		SignalMask:  report.SignalMask,
		SignalNames: append([]string{}, report.SignalNames...),
		DelayNS:     report.DelayNS,
		WorkStatus: model.ScreenDeceptionSignalWorkStatus{
			ClockOK:         report.WorkStatus.ClockOK,
			EphemerisValid:  report.WorkStatus.EphemerisValid,
			RFModuleOK:      report.WorkStatus.RFModuleOK,
			SignalTransmit:  report.WorkStatus.SignalTransmit,
			TransmitChannel: report.WorkStatus.TransmitChannel,
			FPGAOK:          report.WorkStatus.FPGAOK,
			Raw:             report.WorkStatus.Raw,
		},
		TransmitSwitch:         report.TransmitSwitch,
		AttenuationDB:          report.AttenuationDB,
		ReceivedSatelliteCount: report.ReceivedSatelliteCount,
		ReceivedPRNs:           append([]int{}, report.ReceivedPRNs...),
		ReceivedCN0:            append([]int{}, report.ReceivedCN0...),
		TransmittedCount:       report.TransmittedCount,
		TransmittedPRNs:        append([]int{}, report.TransmittedPRNs...),
	}
}

func statusPointFromProtocol(point *protocol.PositionReport) *model.ScreenDeceptionStatusPoint {
	if point == nil {
		return nil
	}
	return &model.ScreenDeceptionStatusPoint{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
		AltitudeM: point.AltitudeM,
	}
}

func firstSignalDelay(report protocol.SignalDelayReport) *float64 {
	for _, value := range []*float64{report.GPS, report.BDS, report.GLO, report.GAL} {
		if value != nil {
			return cloneFloat64(value)
		}
	}
	return nil
}

func (s *Service) record(record model.DeceptionRecord) {
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	s.mu.Lock()
	s.records = appendBounded(s.records, record, maxDeceptionRecords)
	if s.activeReport != nil {
		s.activeReport.Records = append(s.activeReport.Records, record)
		s.activeReport.RecordCount = len(s.activeReport.Records)
	}
	s.mu.Unlock()
	s.publish("deception.record", record)
}

func (s *Service) localizedError(locale string, code string) error {
	return &codedError{
		code:    code,
		message: s.translator.T(locale, "errors", code),
	}
}

func localizedDisplayError(locale string, message string) string {
	return protocol.LocalizeErrorText(message, locale)
}

type codedError struct {
	code    string
	message string
}

func (e *codedError) Error() string {
	return e.message
}

// ErrorCode 返回 Service 产生的可识别错误码。
func ErrorCode(err error) string {
	var coded *codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ""
}

type commandFrame struct {
	name  string
	code  byte
	frame []byte
}

type screenDeceptionConfig struct {
	targetID       string
	mode           string
	longitude      float64
	latitude       float64
	altitudeM      float64
	devicePosition *protocol.PositionReport
	signalMask     uint16
	strengthPreset string
	attenuationDB  int
	delayMode      string
	delayNS        float64
	distanceM      float64
	summary        string
	circle         *model.ScreenDeceptionCircleParams
	linear         *model.ScreenDeceptionLinearParams
}

func buildStartCommands(config screenDeceptionConfig, locale string) ([]commandFrame, error) {
	commands := make([]commandFrame, 0, 5)

	if config.mode == deceptionModeFixedPoint {
		frame, err := protocol.BuildSetSimulatedPosition(
			config.longitude,
			config.latitude,
			int32(math.Round(config.altitudeM)),
		)
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdSimulatedPosition, locale),
			code:  protocol.CmdSimulatedPosition,
			frame: frame,
		})
	}

	switch config.mode {
	case deceptionModeCircle:
		frame, err := buildSimulatedPositionFromDevicePosition(config.devicePosition, locale)
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdSimulatedPosition, locale),
			code:  protocol.CmdSimulatedPosition,
			frame: frame,
		})

		direction, err := rotateDirectionValue(config.circle.Direction)
		if err != nil {
			return nil, err
		}
		frame, err = protocol.BuildSetSimulatedCircle(
			float32(config.circle.RadiusM),
			float32(config.circle.PeriodSeconds),
			direction,
		)
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdSimulatedCircle, locale),
			code:  protocol.CmdSimulatedCircle,
			frame: frame,
		})
	case deceptionModeLinear:
		frame, err := buildSimulatedPositionFromDevicePosition(config.devicePosition, locale)
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdSimulatedPosition, locale),
			code:  protocol.CmdSimulatedPosition,
			frame: frame,
		})

		frame, err = protocol.BuildSetInitialVelocity(
			float32(config.linear.SpeedMPS),
			float32(*config.linear.DirectionDeg),
		)
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdInitialVelocity, locale),
			code:  protocol.CmdInitialVelocity,
			frame: frame,
		})

		frame, err = protocol.BuildSetMaxSpeed(float32(config.linear.MaxSpeedMPS))
		if err != nil {
			return nil, err
		}
		commands = append(commands, commandFrame{
			name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdMaxSpeed, locale),
			code:  protocol.CmdMaxSpeed,
			frame: frame,
		})
	}

	frame, err := protocol.BuildSetTransmitSwitch(config.signalMask)
	if err != nil {
		return nil, err
	}
	commands = append(commands, commandFrame{
		name:  protocol.CommandNameLocale(protocol.ControlSet, protocol.CmdTransmitSwitch, locale),
		code:  protocol.CmdTransmitSwitch,
		frame: frame,
	})
	return commands, nil
}

func buildSimulatedPositionFromDevicePosition(position *protocol.PositionReport, locale string) ([]byte, error) {
	if position == nil {
		return nil, fmt.Errorf("%s", protocol.TextLocale(locale, "status_pos_required"))
	}
	return protocol.BuildSetSimulatedPosition(
		position.Longitude,
		position.Latitude,
		int32(math.Round(position.AltitudeM)),
	)
}

func normalizeScreenDeceptionRequest(
	req model.ScreenDeceptionRequest,
	devicePoint model.GeoPoint,
	hasDevicePoint bool,
	locale string,
) (screenDeceptionConfig, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = defaultDeceptionMode
	}
	if !supportedMode(mode) {
		return screenDeceptionConfig{}, errInvalidMode
	}

	signalMask := protocol.SignalAllSupported
	if req.SignalMask != nil {
		signalMask = *req.SignalMask
	}
	if signalMask == 0 || signalMask&^protocol.SignalAllSupported != 0 {
		return screenDeceptionConfig{}, errInvalidSignal
	}

	strengthPreset := strings.TrimSpace(req.StrengthPreset)
	if strengthPreset == "" && req.AttenuationDB != nil {
		strengthPreset = deceptionStrengthCustom
	}
	if strengthPreset == "" {
		strengthPreset = deceptionStrengthStandard
	}
	attenuationDB, err := attenuationForPreset(strengthPreset, req.AttenuationDB)
	if err != nil {
		return screenDeceptionConfig{}, err
	}

	delayMode := strings.TrimSpace(req.DelayMode)
	if delayMode == "" {
		delayMode = deceptionDelayOff
	}

	var longitude float64
	var latitude float64
	var altitudeM float64
	if mode == deceptionModeFixedPoint {
		if req.Longitude == nil || req.Latitude == nil || !validLocation(*req.Longitude, *req.Latitude) {
			return screenDeceptionConfig{}, errLocationRequired
		}
		longitude = *req.Longitude
		latitude = *req.Latitude
		if req.AltitudeM != nil && finite(*req.AltitudeM) {
			altitudeM = normalizeSimulationAltitude(*req.AltitudeM)
		}
	}

	delayNS, err := normalizeDelay(delayMode, req.DelayNS, 0)
	if err != nil {
		return screenDeceptionConfig{}, err
	}

	config := screenDeceptionConfig{
		targetID:       strings.TrimSpace(req.TargetID),
		mode:           mode,
		longitude:      longitude,
		latitude:       latitude,
		altitudeM:      altitudeM,
		signalMask:     signalMask,
		strengthPreset: strengthPreset,
		attenuationDB:  attenuationDB,
		delayMode:      delayMode,
		delayNS:        delayNS,
	}

	switch mode {
	case deceptionModeCircle:
		circle, err := normalizeCircleParams(req.Circle)
		if err != nil {
			return screenDeceptionConfig{}, err
		}
		config.circle = circle
	case deceptionModeLinear:
		linear, err := normalizeLinearParams(req.Linear)
		if err != nil {
			return screenDeceptionConfig{}, err
		}
		config.linear = linear
	}
	config.summary = buildDeceptionSummary(config, locale)
	return config, nil
}

func supportedMode(mode string) bool {
	switch mode {
	case deceptionModeFixedPoint, deceptionModeCircle, deceptionModeLinear:
		return true
	default:
		return false
	}
}

func attenuationForPreset(preset string, custom *int) (int, error) {
	switch preset {
	case deceptionStrengthStrong:
		return 0, nil
	case deceptionStrengthStandard:
		return defaultDeceptionAttenuationDB, nil
	case deceptionStrengthWeak:
		return 30, nil
	case deceptionStrengthCustom:
		if custom == nil {
			return defaultDeceptionAttenuationDB, nil
		}
		if *custom < 0 || *custom > 80 {
			return 0, errInvalidAttenuation
		}
		return *custom, nil
	default:
		return 0, errInvalidAttenuation
	}
}

func normalizeDelay(mode string, value *float64, distanceM float64) (float64, error) {
	switch mode {
	case deceptionDelayAuto:
		return 0, nil
	case deceptionDelayManual:
		if value == nil || !finite(*value) || *value < 0 {
			return 0, errInvalidDelay
		}
		return *value, nil
	case deceptionDelayOff:
		return 0, nil
	default:
		return 0, errInvalidDelay
	}
}

func normalizeCircleParams(params *model.ScreenDeceptionCircleParams) (*model.ScreenDeceptionCircleParams, error) {
	circle := model.ScreenDeceptionCircleParams{
		RadiusM:       defaultCircleRadiusM,
		PeriodSeconds: defaultCirclePeriodSeconds,
		Direction:     "cw",
	}
	if params != nil {
		if params.RadiusM != 0 {
			circle.RadiusM = params.RadiusM
		}
		if params.PeriodSeconds != 0 {
			circle.PeriodSeconds = params.PeriodSeconds
		}
		if strings.TrimSpace(params.Direction) != "" {
			circle.Direction = strings.TrimSpace(params.Direction)
		}
	}
	if !finite(circle.RadiusM) || circle.RadiusM <= 0 ||
		!finite(circle.PeriodSeconds) || circle.PeriodSeconds <= 0 {
		return nil, errInvalidCircle
	}
	if _, err := rotateDirectionValue(circle.Direction); err != nil {
		return nil, errInvalidCircle
	}
	return &circle, nil
}

func normalizeLinearParams(params *model.ScreenDeceptionLinearParams) (*model.ScreenDeceptionLinearParams, error) {
	defaultDirection := 0.0
	linear := model.ScreenDeceptionLinearParams{
		SpeedMPS:     defaultLinearSpeedMPS,
		DirectionDeg: &defaultDirection,
		MaxSpeedMPS:  defaultLinearSpeedMPS,
	}
	if params != nil {
		if params.SpeedMPS != 0 {
			linear.SpeedMPS = params.SpeedMPS
		}
		if params.DirectionDeg != nil {
			direction := normalizeDegrees(*params.DirectionDeg)
			linear.DirectionDeg = &direction
		}
		if params.MaxSpeedMPS != 0 {
			linear.MaxSpeedMPS = params.MaxSpeedMPS
		}
	}
	if !finite(linear.SpeedMPS) || linear.SpeedMPS < 0 ||
		!finite(linear.MaxSpeedMPS) || linear.MaxSpeedMPS <= 0 ||
		linear.DirectionDeg == nil || !finite(*linear.DirectionDeg) {
		return nil, errInvalidLinear
	}
	return &linear, nil
}

func rotateDirectionValue(direction string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "", "cw", "0", "clockwise":
		return 0, nil
	case "ccw", "1", "counterclockwise":
		return 1, nil
	default:
		return 0, errInvalidCircle
	}
}

func buildDeceptionSummary(config screenDeceptionConfig, locale string) string {
	signals := protocol.FormatSignalsLocale(config.signalMask, locale)
	switch config.mode {
	case deceptionModeCircle:
		return fmt.Sprintf(
			"%.0fm / %.0fs / %s / %ddB / %s",
			config.circle.RadiusM,
			config.circle.PeriodSeconds,
			strings.ToUpper(config.circle.Direction),
			config.attenuationDB,
			signals,
		)
	case deceptionModeLinear:
		return fmt.Sprintf(
			"%.1fm/s / %.0fdeg / %ddB / %s",
			config.linear.SpeedMPS,
			*config.linear.DirectionDeg,
			config.attenuationDB,
			signals,
		)
	default:
		return fmt.Sprintf("%.6f, %.6f / %.0fm / %ddB / %s", config.longitude, config.latitude, config.altitudeM, config.attenuationDB, signals)
	}
}

func errorCodeForNormalizeError(err error) string {
	switch {
	case errors.Is(err, errLocationRequired):
		return "deception_location_required"
	case errors.Is(err, errInvalidMode):
		return "deception_invalid_mode"
	case errors.Is(err, errInvalidSignal):
		return "deception_invalid_signal"
	case errors.Is(err, errInvalidAttenuation):
		return "deception_invalid_attenuation"
	case errors.Is(err, errInvalidDelay):
		return "deception_invalid_delay"
	case errors.Is(err, errInvalidCircle):
		return "deception_invalid_circle"
	case errors.Is(err, errInvalidLinear):
		return "deception_invalid_linear"
	default:
		return "internal"
	}
}

func openSerialPort(cfg *serialport.Config) (SerialPort, error) {
	return serialport.Open(cfg)
}

func validLocation(longitude float64, latitude float64) bool {
	return finite(longitude) &&
		finite(latitude) &&
		longitude >= -180 &&
		longitude <= 180 &&
		latitude >= -90 &&
		latitude <= 90
}

func normalizeSimulationAltitude(value float64) float64 {
	if !finite(value) || value < minSimulationAltitudeM || value > maxSimulationAltitudeM {
		return 0
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func cloneGeoPoint(point *model.GeoPoint) *model.GeoPoint {
	if point == nil {
		return nil
	}
	next := *point
	return &next
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func cloneUint32(value *uint32) *uint32 {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	next := *value
	return &next
}

func firstMapValue(values map[string]string) string {
	for _, value := range values {
		return value
	}
	return ""
}

func cloneCircleParams(params *model.ScreenDeceptionCircleParams) *model.ScreenDeceptionCircleParams {
	if params == nil {
		return nil
	}
	next := *params
	return &next
}

func cloneLinearParams(params *model.ScreenDeceptionLinearParams) *model.ScreenDeceptionLinearParams {
	if params == nil {
		return nil
	}
	next := *params
	if params.DirectionDeg != nil {
		direction := *params.DirectionDeg
		next.DirectionDeg = &direction
	}
	return &next
}

func cloneRandomParams(params *model.ScreenDeceptionRandomParams) *model.ScreenDeceptionRandomParams {
	if params == nil {
		return nil
	}
	next := *params
	return &next
}

func ceilSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	return int((duration + time.Second - 1) / time.Second)
}

func normalizeDegrees(value float64) float64 {
	if !finite(value) {
		return 0
	}
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	return normalized
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func deceptionQueryCommand(item string) (byte, bool) {
	switch strings.ToLower(strings.TrimSpace(item)) {
	case "status", "device_status":
		return protocol.QueryDeviceStatus, true
	case "tx", "transmit":
		return protocol.QueryTransmitSwitch, true
	case "power", "attenuation":
		return protocol.QueryPowerAttenuation, true
	case "simulated_position", "simpos":
		return protocol.QuerySimulatedPosition, true
	case "device_position", "location":
		return protocol.QueryDevicePosition, true
	case "random":
		return protocol.QueryRandomPosition, true
	case "circle":
		return protocol.QuerySpoofCircle, true
	case "suppression":
		return protocol.QuerySuppression, true
	case "delay":
		return protocol.QuerySignalDelay, true
	case "timed_search", "timedsearch":
		return protocol.QueryTimedSearch, true
	default:
		return 0, false
	}
}

func appendBounded[T any](items []T, item T, maxItems int) []T {
	items = append(items, item)
	if maxItems <= 0 || len(items) <= maxItems {
		return items
	}
	return items[len(items)-maxItems:]
}

func sameRequest(a, b model.DeceptionSessionRequest) bool {
	return a.PortName == b.PortName &&
		a.BaudRate == b.BaudRate &&
		a.DataBits == b.DataBits &&
		a.StopBits == b.StopBits &&
		strings.TrimSpace(a.Parity) == strings.TrimSpace(b.Parity) &&
		a.ReadTimeoutMs == b.ReadTimeoutMs &&
		a.AutoConnect == b.AutoConnect
}

func firstNonZero(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
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
	if options.CommandTimeout == 0 {
		options.CommandTimeout = defaultDeceptionCommandTimeout
	}
	return options
}

var _ SerialPort = serial.Port(nil)
