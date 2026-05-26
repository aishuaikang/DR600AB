// Package interference 控制用于 GPIO 输出的通道。
package interference

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"gpio-controller/board"
	"gpio-controller/gpio"
)

const (
	screenStrikeEventType          = "screen.strike.updated"
	screenStrikeMinDurationSeconds = 10
	screenStrikeMaxDurationSeconds = 60
)

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

// GPIOPin 是 GPIO 控制服务依赖的引脚操作接口。
type GPIOPin interface {
	Setup() error
	SetHigh() error
	SetLow() error
	GetValue() (int, error)
	Cleanup()
}

// PinFactory 根据 Linux GPIO 编号创建引脚。
type PinFactory func(number int) GPIOPin

// ReportStore 持久化大屏干扰操作证据报告。
type ReportStore interface {
	Create(model.InterferenceReport) (model.InterferenceReport, error)
	CreateRunning(model.InterferenceReport) (model.InterferenceReport, error)
	Update(model.InterferenceReport) error
}

// UserSettingsStore 读取用户可配置的大屏干扰频段文案。
type UserSettingsStore interface {
	LoadUser() (model.UserSettings, bool, error)
}

// ChannelDefinition 声明一个可控制的 GPIO 输出通道。
type ChannelDefinition struct {
	ID       string
	Label    string
	Pin      int
	Bands    []string
	Reserved bool
}

// Service 管理 GPIO 通道状态，并发布通道事件。
type Service struct {
	mu sync.RWMutex

	channels   map[string]*channelState
	order      []string
	pinFactory PinFactory
	store      *store.MemoryStore
	translator *i18n.Translator
	reports    ReportStore
	settings   UserSettingsStore

	strikeTimer           *time.Timer
	strikeSeq             uint64
	strikeActive          bool
	strikeChannelIDs      []string
	strikeDurationSeconds int
	strikeStartedAt       time.Time
	strikeEndsAt          time.Time
	activeReport          *model.InterferenceReport
	activeReportID        string
}

type channelState struct {
	def          ChannelDefinition
	pin          GPIOPin
	initialized  bool
	enabled      bool
	actualLevel  string
	desiredLevel string
	status       string
	lastError    string
}

// NewService 根据通道定义创建 GPIO 控制服务。
func NewService(store *store.MemoryStore, translator *i18n.Translator, definitions []ChannelDefinition, pinFactory PinFactory) *Service {
	if pinFactory == nil {
		pinFactory = func(number int) GPIOPin {
			return gpio.NewPin(number)
		}
	}
	if len(definitions) == 0 {
		definitions = DefaultChannels()
	}

	channels := make(map[string]*channelState, len(definitions))
	order := make([]string, 0, len(definitions))
	for _, def := range definitions {
		channels[def.ID] = &channelState{
			def:          def,
			actualLevel:  "unknown",
			desiredLevel: "low",
			status:       initialStatus(def),
		}
		order = append(order, def.ID)
	}
	sort.Strings(order)

	return &Service{
		channels:   channels,
		order:      order,
		pinFactory: pinFactory,
		store:      store,
		translator: translator,
	}
}

// SetReportStore 设置干扰报告持久化存储。
func (s *Service) SetReportStore(reports ReportStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = reports
}

// SetUserSettingsStore 设置用户设置存储，用于干扰报告频段显示。
func (s *Service) SetUserSettingsStore(settings UserSettingsStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
}

// DefaultChannels 返回设备使用的 GPIO 通道映射。
func DefaultChannels() []ChannelDefinition {
	pins := board.DefaultPins()
	definitions := make([]ChannelDefinition, 0, len(pins))
	for _, pin := range pins {
		bands := make([]string, len(pin.Bands))
		copy(bands, pin.Bands)
		definitions = append(definitions, ChannelDefinition{
			ID:       pin.ID,
			Label:    pin.Label,
			Pin:      pin.Number,
			Bands:    bands,
			Reserved: pin.Reserved,
		})
	}
	return definitions
}

// ListChannels 按稳定展示顺序返回通道状态。
func (s *Service) ListChannels() []model.GpioChannel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.GpioChannel, 0, len(s.order))
	for _, id := range s.order {
		result = append(result, s.dtoWithActual(s.channels[id]))
	}
	return result
}

// SetState 将通道置为高电平或低电平，并返回更新后的通道状态。
func (s *Service) SetState(id string, enabled bool, locale string) (model.GpioChannel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channel, err := s.setStateLocked(id, enabled, locale)
	if err == nil && s.isScreenStrikeChannelIDLocked(id) {
		if !s.screenStrikeHasHighChannelLocked() {
			s.strikeSeq++
			if s.strikeTimer != nil {
				s.strikeTimer.Stop()
				s.strikeTimer = nil
			}
			s.clearScreenStrikeLocked()
			s.finishActiveReportLocked(model.InterferenceReportStatusCompleted, "", nil, time.Now(), s.screenStrikeStateLocked(time.Now()))
		}
		s.publishScreenStrikeLocked(s.screenStrikeStateLocked(time.Now()))
	}
	return channel, err
}

// ScreenStrikeState 返回大屏干扰控制当前状态。
func (s *Service) ScreenStrikeState() model.ScreenStrikeState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.screenStrikeStateLocked(time.Now())
}

// SetScreenStrike 更新大屏干扰控制状态。启用时只允许控制前三个非预留 GPIO 通道。
func (s *Service) SetScreenStrike(req model.ScreenStrikeRequest, locale string) (model.ScreenStrikeState, error) {
	durationSeconds := req.DurationSeconds
	duration := time.Duration(durationSeconds) * time.Second
	return s.applyScreenStrike(req.Enabled, req.ChannelIDs, duration, durationSeconds, locale)
}

func (s *Service) applyScreenStrike(
	enabled bool,
	channelIDs []string,
	duration time.Duration,
	durationSeconds int,
	locale string,
) (model.ScreenStrikeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !enabled {
		s.strikeSeq++
		if s.strikeTimer != nil {
			s.strikeTimer.Stop()
			s.strikeTimer = nil
		}
		err := s.stopScreenStrikeChannelsLocked(locale)
		s.clearScreenStrikeLocked()
		endedAt := time.Now()
		state := s.screenStrikeStateLocked(endedAt)
		s.finishActiveReportLocked(model.InterferenceReportStatusCompleted, "", err, endedAt, state)
		s.publishScreenStrikeLocked(state)
		return state, err
	}

	startedAt := time.Now()
	req := model.ScreenStrikeRequest{
		Enabled:         enabled,
		ChannelIDs:      append([]string(nil), channelIDs...),
		DurationSeconds: durationSeconds,
	}
	selected, err := s.validateScreenStrikeChannelsLocked(channelIDs, locale)
	if err != nil {
		return s.screenStrikeStateLocked(time.Now()), err
	}
	if duration <= 0 || durationSeconds < screenStrikeMinDurationSeconds || durationSeconds > screenStrikeMaxDurationSeconds {
		return s.screenStrikeStateLocked(time.Now()), s.localizedError(locale, "strike_invalid_duration")
	}
	if s.screenStrikeHasHighChannelLocked() {
		return s.screenStrikeStateLocked(time.Now()), s.localizedError(locale, "strike_already_active")
	}

	s.strikeSeq++
	if s.strikeTimer != nil {
		s.strikeTimer.Stop()
		s.strikeTimer = nil
	}

	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}
	for _, id := range s.screenStrikeChannelIDsLocked() {
		if _, err := s.setStateLocked(id, selectedSet[id], locale); err != nil {
			_ = s.stopScreenStrikeChannelsLocked(locale)
			s.clearScreenStrikeLocked()
			state := s.screenStrikeStateLocked(time.Now())
			s.createFailedReportLocked(req, selected, startedAt, err, state)
			s.publishScreenStrikeLocked(state)
			return state, err
		}
	}

	now := time.Now()
	endsAt := now.Add(duration)
	s.strikeActive = true
	s.strikeChannelIDs = append([]string(nil), selected...)
	s.strikeDurationSeconds = durationSeconds
	s.strikeStartedAt = startedAt
	s.strikeEndsAt = endsAt

	seq := s.strikeSeq
	s.strikeTimer = time.AfterFunc(duration, func() {
		s.stopScreenStrikeOnTimeout(seq)
	})

	state := s.screenStrikeStateLocked(now)
	s.createRunningReportLocked(req, selected, startedAt, state)
	s.publishScreenStrikeLocked(state)
	return state, nil
}

func (s *Service) stopScreenStrikeOnTimeout(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if seq != s.strikeSeq || !s.strikeActive {
		return
	}
	s.strikeSeq++
	s.strikeTimer = nil
	_ = s.stopScreenStrikeChannelsLocked("")
	s.clearScreenStrikeLocked()
	endedAt := time.Now()
	state := s.screenStrikeStateLocked(endedAt)
	s.finishActiveReportLocked(model.InterferenceReportStatusCompleted, "", nil, endedAt, state)
	s.publishScreenStrikeLocked(state)
}

func (s *Service) setStateLocked(id string, enabled bool, locale string) (model.GpioChannel, error) {
	state, ok := s.channels[id]
	if !ok {
		return model.GpioChannel{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "channel_not_found"))
	}

	if state.pin == nil {
		state.pin = s.pinFactory(state.def.Pin)
	}

	if enabled {
		if !state.initialized {
			if err := state.pin.Setup(); err != nil {
				return s.markError(state, err)
			}
			state.initialized = true
		}
		if err := state.pin.SetHigh(); err != nil {
			return s.markError(state, err)
		}
		state.enabled = true
		state.actualLevel = "high"
		state.desiredLevel = "high"
		state.status = "active"
		state.lastError = ""
	} else {
		if state.pin == nil {
			pin := s.pinFactory(state.def.Pin)
			if value, err := pin.GetValue(); err == nil && value != 0 {
				state.pin = pin
			}
		}
		if state.pin != nil {
			if err := state.pin.SetLow(); err != nil {
				return s.markError(state, err)
			}
			state.pin.Cleanup()
			state.pin = nil
			state.initialized = false
		}
		state.enabled = false
		state.actualLevel = "low"
		state.desiredLevel = "low"
		state.status = "idle"
		state.lastError = ""
	}

	channel := s.dtoWithActual(state)
	s.store.Publish(model.Event{Type: "gpio.channel.updated", Time: time.Now(), Payload: channel})
	return channel, nil
}

// Shutdown 将所有已初始化 GPIO 引脚置为低电平并释放资源。
func (s *Service) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.strikeSeq++
	if s.strikeTimer != nil {
		s.strikeTimer.Stop()
		s.strikeTimer = nil
	}
	s.clearScreenStrikeLocked()

	for _, state := range s.channels {
		if state.pin == nil {
			continue
		}
		_ = state.pin.SetLow()
		state.pin.Cleanup()
		state.pin = nil
		state.initialized = false
		state.enabled = false
		state.actualLevel = "low"
		state.desiredLevel = "low"
		state.status = initialStatus(state.def)
	}
	endedAt := time.Now()
	s.finishActiveReportLocked(model.InterferenceReportStatusAbnormal, "service_shutdown", nil, endedAt, s.screenStrikeStateLocked(endedAt))
}

func (s *Service) validateScreenStrikeChannelsLocked(ids []string, locale string) ([]string, error) {
	if len(ids) == 0 {
		return nil, s.localizedError(locale, "strike_channels_required")
	}

	requested := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		requested[id] = struct{}{}
	}
	if len(requested) == 0 {
		return nil, s.localizedError(locale, "strike_channels_required")
	}

	selected := make([]string, 0, len(requested))
	for _, id := range s.screenStrikeChannelIDsLocked() {
		if _, ok := requested[id]; ok {
			selected = append(selected, id)
			delete(requested, id)
		}
	}
	if len(requested) > 0 {
		return nil, s.localizedError(locale, "strike_invalid_channels")
	}
	return selected, nil
}

func (s *Service) screenStrikeChannelIDsLocked() []string {
	ids := make([]string, 0, 3)
	for _, id := range s.order {
		state := s.channels[id]
		if state == nil || state.def.Reserved {
			continue
		}
		ids = append(ids, id)
		if len(ids) == 3 {
			break
		}
	}
	return ids
}

func (s *Service) isScreenStrikeChannelIDLocked(id string) bool {
	for _, strikeID := range s.screenStrikeChannelIDsLocked() {
		if strikeID == id {
			return true
		}
	}
	return false
}

func (s *Service) screenStrikeHasHighChannelLocked() bool {
	for _, id := range s.screenStrikeChannelIDsLocked() {
		if s.dtoWithActual(s.channels[id]).Enabled {
			return true
		}
	}
	return false
}

func (s *Service) stopScreenStrikeChannelsLocked(locale string) error {
	var firstErr error
	for _, id := range s.screenStrikeChannelIDsLocked() {
		if _, err := s.setStateLocked(id, false, locale); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) clearScreenStrikeLocked() {
	s.strikeActive = false
	s.strikeChannelIDs = nil
	s.strikeDurationSeconds = 0
	s.strikeStartedAt = time.Time{}
	s.strikeEndsAt = time.Time{}
}

func (s *Service) screenStrikeStateLocked(now time.Time) model.ScreenStrikeState {
	if now.IsZero() {
		now = time.Now()
	}

	channels := make([]model.GpioChannel, 0, 3)
	activeChannelIDs := make([]string, 0, 3)
	for _, id := range s.screenStrikeChannelIDsLocked() {
		channel := s.dtoWithActual(s.channels[id])
		channels = append(channels, channel)
		if channel.Enabled {
			activeChannelIDs = append(activeChannelIDs, channel.ID)
		}
	}

	active := len(activeChannelIDs) > 0
	state := model.ScreenStrikeState{
		Active:           active,
		ChannelIDs:       append([]string(nil), activeChannelIDs...),
		DurationSeconds:  s.strikeDurationSeconds,
		RemainingSeconds: 0,
		Channels:         channels,
	}
	if active && s.strikeActive {
		startedAt := s.strikeStartedAt
		endsAt := s.strikeEndsAt
		state.StartedAt = &startedAt
		state.EndsAt = &endsAt
		state.RemainingSeconds = ceilSeconds(endsAt.Sub(now))
	}
	return state
}

func (s *Service) publishScreenStrikeLocked(state model.ScreenStrikeState) {
	if s.store == nil {
		return
	}
	s.store.Publish(model.Event{Type: screenStrikeEventType, Time: time.Now(), Payload: state})
}

func (s *Service) createRunningReportLocked(
	req model.ScreenStrikeRequest,
	selected []string,
	startedAt time.Time,
	state model.ScreenStrikeState,
) {
	if s.reports == nil {
		return
	}
	labels, pins := s.reportChannelMetadataLocked(selected)
	report := model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:                   model.InterferenceReportStatusRunning,
			StartedAt:                startedAt,
			RequestedDurationSeconds: req.DurationSeconds,
			ChannelIDs:               append([]string(nil), selected...),
			ChannelLabels:            labels,
			ChannelPins:              pins,
			Summary:                  interferenceReportSummary(labels, req.DurationSeconds),
		},
		Request:    cloneStrikeRequest(req),
		StartState: cloneStrikeState(&state),
	}
	created, err := s.reports.CreateRunning(report)
	if err != nil {
		return
	}
	s.activeReportID = created.ID
	cloned := cloneInterferenceReport(created)
	s.activeReport = &cloned
}

func (s *Service) createFailedReportLocked(
	req model.ScreenStrikeRequest,
	selected []string,
	startedAt time.Time,
	cause error,
	endState model.ScreenStrikeState,
) {
	if s.reports == nil || cause == nil {
		return
	}
	endedAt := time.Now()
	labels, pins := s.reportChannelMetadataLocked(selected)
	report := model.InterferenceReport{
		InterferenceReportSummary: model.InterferenceReportSummary{
			Status:                   model.InterferenceReportStatusFailed,
			StartedAt:                startedAt,
			EndedAt:                  &endedAt,
			RequestedDurationSeconds: req.DurationSeconds,
			ChannelIDs:               append([]string(nil), selected...),
			ChannelLabels:            labels,
			ChannelPins:              pins,
			Summary:                  interferenceReportSummary(labels, req.DurationSeconds),
			LastError:                cause.Error(),
		},
		Request:  cloneStrikeRequest(req),
		EndState: cloneStrikeState(&endState),
	}
	_, _ = s.reports.Create(report)
}

func (s *Service) finishActiveReportLocked(
	status model.InterferenceReportStatus,
	reason string,
	cause error,
	endedAt time.Time,
	endState model.ScreenStrikeState,
) {
	if s.reports == nil || s.activeReport == nil || s.activeReportID == "" {
		return
	}
	report := cloneInterferenceReport(*s.activeReport)
	if report.ID == "" {
		return
	}
	report.Status = status
	report.EndedAt = &endedAt
	report.EndState = cloneStrikeState(&endState)
	if cause != nil {
		report.LastError = cause.Error()
	}
	if strings.TrimSpace(reason) != "" {
		report.AbnormalReason = strings.TrimSpace(reason)
		if report.LastError == "" {
			report.LastError = report.AbnormalReason
		}
	}
	_ = s.reports.Update(report)
	s.activeReportID = ""
	s.activeReport = nil
}

func (s *Service) reportChannelMetadataLocked(ids []string) ([]string, []int) {
	labels := make([]string, 0, len(ids))
	pins := make([]int, 0, len(ids))
	customLabels := s.screenStrikeCustomLabelsLocked()
	strikeIndexes := s.screenStrikeChannelIndexesLocked()
	for _, id := range ids {
		state := s.channels[id]
		if state == nil {
			continue
		}
		strikeIndex, ok := strikeIndexes[id]
		if !ok {
			strikeIndex = -1
		}
		labels = append(labels, reportChannelLabel(state.def, strikeIndex, customLabels))
		pins = append(pins, state.def.Pin)
	}
	return labels, pins
}

func (s *Service) screenStrikeCustomLabelsLocked() []string {
	if s.settings == nil {
		return nil
	}
	settings, ok, err := s.settings.LoadUser()
	if err != nil || !ok {
		return nil
	}
	return append([]string(nil), settings.ScreenStrikeChannelLabels...)
}

func (s *Service) screenStrikeChannelIndexesLocked() map[string]int {
	indexes := make(map[string]int, 3)
	for index, id := range s.screenStrikeChannelIDsLocked() {
		indexes[id] = index
	}
	return indexes
}

func reportChannelLabel(def ChannelDefinition, strikeIndex int, customLabels []string) string {
	if strikeIndex >= 0 && strikeIndex < len(customLabels) {
		if label := strings.TrimSpace(customLabels[strikeIndex]); label != "" {
			return label
		}
	}
	if label := formatStrikeBands(def.Bands); label != "" {
		return label
	}
	if def.Label != "" {
		return def.Label
	}
	return def.ID
}

func formatStrikeBands(bands []string) string {
	parts := make([]string, 0, len(bands))
	for _, band := range bands {
		if label := formatStrikeBand(band); label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, "/")
}

func formatStrikeBand(value string) string {
	band := strings.TrimSpace(value)
	if band == "" {
		return ""
	}
	numeric, err := strconv.ParseFloat(band, 64)
	if err == nil {
		if numeric >= 100 {
			return band + "M"
		}
		return band + "G"
	}
	return band
}

func interferenceReportSummary(labels []string, durationSeconds int) string {
	parts := make([]string, 0, len(labels))
	for _, label := range labels {
		if value := strings.TrimSpace(label); value != "" {
			parts = append(parts, value)
		}
	}
	channelText := strings.Join(parts, ", ")
	if channelText == "" {
		channelText = "unknown"
	}
	if durationSeconds > 0 {
		return fmt.Sprintf("%s / %ds", channelText, durationSeconds)
	}
	return channelText
}

func cloneStrikeRequest(req model.ScreenStrikeRequest) model.ScreenStrikeRequest {
	req.ChannelIDs = append([]string(nil), req.ChannelIDs...)
	return req
}

func cloneStrikeState(state *model.ScreenStrikeState) *model.ScreenStrikeState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.ChannelIDs = append([]string(nil), state.ChannelIDs...)
	cloned.Channels = cloneGPIOChannels(state.Channels)
	if state.StartedAt != nil {
		startedAt := *state.StartedAt
		cloned.StartedAt = &startedAt
	}
	if state.EndsAt != nil {
		endsAt := *state.EndsAt
		cloned.EndsAt = &endsAt
	}
	return &cloned
}

func cloneGPIOChannels(channels []model.GpioChannel) []model.GpioChannel {
	if len(channels) == 0 {
		return nil
	}
	cloned := make([]model.GpioChannel, len(channels))
	for index, channel := range channels {
		cloned[index] = channel
		cloned[index].Bands = append([]string(nil), channel.Bands...)
	}
	return cloned
}

func cloneInterferenceReport(report model.InterferenceReport) model.InterferenceReport {
	report.ChannelIDs = append([]string(nil), report.ChannelIDs...)
	report.ChannelLabels = append([]string(nil), report.ChannelLabels...)
	report.ChannelPins = append([]int(nil), report.ChannelPins...)
	report.Request = cloneStrikeRequest(report.Request)
	report.StartState = cloneStrikeState(report.StartState)
	report.EndState = cloneStrikeState(report.EndState)
	if report.EndedAt != nil {
		endedAt := *report.EndedAt
		report.EndedAt = &endedAt
	}
	return report
}

func ceilSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	return int((duration + time.Second - 1) / time.Second)
}

func (s *Service) localizedError(locale string, code string) error {
	return &codedError{
		code:    code,
		message: s.translator.T(locale, "errors", code),
	}
}

// markError 将通道更新为错误状态，并发布当前状态。
func (s *Service) markError(state *channelState, err error) (model.GpioChannel, error) {
	state.status = "error"
	state.lastError = err.Error()
	channel := state.dto()
	s.store.Publish(model.Event{Type: "gpio.channel.updated", Time: time.Now(), Payload: channel})
	return channel, err
}

// dto 将可变通道状态复制到 API 模型。
func (s *channelState) dto() model.GpioChannel {
	bands := make([]string, len(s.def.Bands))
	copy(bands, s.def.Bands)

	return model.GpioChannel{
		ID:           s.def.ID,
		Label:        s.def.Label,
		Pin:          s.def.Pin,
		Bands:        bands,
		Reserved:     s.def.Reserved,
		Enabled:      s.enabled,
		ActualLevel:  s.actualLevel,
		DesiredLevel: s.desiredLevel,
		Status:       s.status,
		LastError:    s.lastError,
	}
}

func (s *Service) dtoWithActual(state *channelState) model.GpioChannel {
	channel := state.dto()

	pin := state.pin
	if pin == nil {
		pin = s.pinFactory(state.def.Pin)
	}
	value, err := pin.GetValue()
	if err != nil {
		return channel
	}

	switch value {
	case 0:
		channel.Enabled = false
		channel.ActualLevel = "low"
		channel.Status = "idle"
	case 1:
		channel.Enabled = true
		channel.ActualLevel = "high"
		channel.Status = "active"
	default:
		channel.Enabled = value != 0
		channel.ActualLevel = strconv.Itoa(value)
		if channel.Enabled {
			channel.Status = "active"
		} else {
			channel.Status = "idle"
		}
	}
	return channel
}

// initialStatus 返回通道定义对应的启动状态。
func initialStatus(def ChannelDefinition) string {
	return "idle"
}
