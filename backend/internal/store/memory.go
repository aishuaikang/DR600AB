// Package store 保存有上限的运行记录，并管理服务端事件订阅者。
package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"dr600ab-api/internal/model"
	protocolmerge "uav-protocol/merge"
	protocolmodel "uav-protocol/model"
)

const (
	screenDetectionTTL       = 60 * time.Second
	screenPositionTTL        = 60 * time.Second
	screenDetectionEventType = "screen.detection.updated"
	screenPositionEventType  = "screen.position.updated"
	uncrackedDJIDroneModel   = "DJI-Drone"
)

// MemoryStore 在内存中保存有上限的记录，并广播运行时事件。
type MemoryStore struct {
	mu sync.RWMutex

	maxDetections          int
	maxParsed              int
	archiver               IntrusionArchiver
	deviceLocationProvider func() *model.ScreenDeviceLocationResponse

	detections []model.DetectionRecord
	screen     []model.ScreenDetectionTarget
	positions  []model.ScreenPositionTarget
	parsed     []model.ParsedMessage
	gps        []model.GPSRecord
	compass    []model.CompassRecord

	expiredDetections []model.ScreenDetectionTarget
	expiredPositions  []model.ScreenPositionTarget
	screenSequence    uint64
	positionSeq       uint64
	subscribers       map[chan model.Event]struct{}
}

// IntrusionArchiver 持久化从大屏当前列表中消失的目标。
type IntrusionArchiver interface {
	ArchiveDetection(model.ScreenDetectionTarget) error
	ArchivePosition(model.ScreenPositionTarget) error
}

// NewMemoryStore 创建带历史长度上限的内存存储。
func NewMemoryStore(maxDetections, maxParsed int) *MemoryStore {
	return &MemoryStore{
		maxDetections: max(1, maxDetections),
		maxParsed:     max(1, maxParsed),
		subscribers:   make(map[chan model.Event]struct{}),
	}
}

// SetIntrusionArchiver 设置目标消失时使用的归档器。
func (s *MemoryStore) SetIntrusionArchiver(archiver IntrusionArchiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.archiver = archiver
}

// SetDeviceLocationProvider sets the source used to calculate live position target relations.
func (s *MemoryStore) SetDeviceLocationProvider(provider func() *model.ScreenDeviceLocationResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceLocationProvider = provider
}

// AddParsed 追加解析消息，并发布解析事件。
func (s *MemoryStore) AddParsed(msg model.ParsedMessage) {
	s.mu.Lock()
	s.parsed = appendBounded(s.parsed, msg, s.maxParsed)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.parsed", Time: msg.Time, Payload: msg})
}

// AddDetection 追加侦测记录，并发布侦测事件。
func (s *MemoryStore) AddDetection(record model.DetectionRecord) {
	record.DisplayModel = model.DisplayModelName(record.Model)
	s.mu.Lock()
	s.detections = appendBounded(s.detections, record, s.maxDetections)
	target, updated := s.addScreenDetectionLocked(record, record.ReceivedAt)
	archiver := s.archiver
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)

	s.Publish(model.Event{Type: "detection.record", Time: record.ReceivedAt, Payload: record})
	if updated {
		s.Publish(model.Event{Type: screenDetectionEventType, Time: target.LastSeen, Payload: target})
	}
}

// AddScreenPosition 追加或合并大屏定位目标，并发布定位事件。
func (s *MemoryStore) AddScreenPosition(target model.ScreenPositionTarget) (model.ScreenPositionTarget, bool) {
	s.mu.Lock()
	merged, updated := s.addScreenPositionLocked(target)
	archiver := s.archiver
	deviceLocation := s.currentDeviceLocationLocked()
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)
	merged = withPositionRelations(merged, deviceLocation)

	if updated {
		s.Publish(model.Event{Type: screenPositionEventType, Time: merged.LastSeen, Payload: merged})
	}
	return merged, updated
}

// AddGPS 追加 GPS 记录，并发布 GPS 事件。
func (s *MemoryStore) AddGPS(record model.GPSRecord) {
	s.mu.Lock()
	s.gps = appendBounded(s.gps, record, s.maxParsed)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "gps.record", Time: record.ReceivedAt, Payload: record})
}

// AddCompass 追加三维电子罗盘角度记录，并发布罗盘事件。
func (s *MemoryStore) AddCompass(record model.CompassRecord) {
	s.mu.Lock()
	s.compass = appendBounded(s.compass, record, s.maxParsed)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "compass.record", Time: record.ReceivedAt, Payload: record})
}

// ListParsed 按时间倒序返回最新解析消息。
func (s *MemoryStore) ListParsed(limit int) []model.ParsedMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.parsed, limit)
}

// ListDetections 按时间倒序返回最新侦测记录。
func (s *MemoryStore) ListDetections(limit int) []model.DetectionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.detections, limit)
}

// ListScreenDetections 按首次发现时间倒序返回大屏合并侦测目标，避免实时更新导致列表跳动。
func (s *MemoryStore) ListScreenDetections(limit int) []model.ScreenDetectionTarget {
	s.mu.Lock()
	now := time.Now()
	s.pruneExpiredScreenDetectionsLocked(now)
	items := latestScreenDetections(s.screen, limit)
	archiver := s.archiver
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)
	return items
}

// ListScreenPositions 按首次发现时间倒序返回大屏合并定位目标，避免实时更新导致列表跳动。
func (s *MemoryStore) ListScreenPositions(limit int) []model.ScreenPositionTarget {
	s.mu.Lock()
	now := time.Now()
	s.pruneExpiredScreenPositionsLocked(now)
	items := latestScreenPositions(s.positions, limit)
	archiver := s.archiver
	deviceLocation := s.currentDeviceLocationLocked()
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)
	applyPositionRelations(items, deviceLocation)
	return items
}

// ListScreenPositionsWithDeviceLocation returns screen position targets with relations
// calculated from the supplied device location. Distances are omitted only when the
// device location or target coordinates are unavailable.
func (s *MemoryStore) ListScreenPositionsWithDeviceLocation(limit int, deviceLocation *model.ScreenDeviceLocationResponse) []model.ScreenPositionTarget {
	s.mu.Lock()
	now := time.Now()
	s.pruneExpiredScreenPositionsLocked(now)
	items := latestScreenPositions(s.positions, limit)
	archiver := s.archiver
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)
	applyPositionRelations(items, deviceLocation)
	return items
}

// ListGPS 按时间倒序返回最新 GPS 记录。
func (s *MemoryStore) ListGPS(limit int) []model.GPSRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.gps, limit)
}

// ListCompass 按时间倒序返回最新三维电子罗盘角度记录。
func (s *MemoryStore) ListCompass(limit int) []model.CompassRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.compass, limit)
}

// Subscribe 注册事件通道，并返回取消订阅函数。
func (s *MemoryStore) Subscribe(buffer int) (<-chan model.Event, func()) {
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan model.Event, buffer)

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

// Publish 向订阅者广播事件，且不会阻塞事件生产方。
func (s *MemoryStore) Publish(evt model.Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (s *MemoryStore) archiveExpiredScreenTargets(archiver IntrusionArchiver) {
	detections, positions := s.popExpiredScreenTargets()
	if archiver == nil {
		return
	}
	for _, target := range detections {
		if err := archiver.ArchiveDetection(target); err != nil {
			slog.Warn("归档侦测入侵目标失败", "targetId", target.ID, "error", err)
		}
	}
	for _, target := range positions {
		if err := archiver.ArchivePosition(target); err != nil {
			slog.Warn("归档定位入侵目标失败", "targetId", target.ID, "error", err)
		}
	}
}

func (s *MemoryStore) popExpiredScreenTargets() ([]model.ScreenDetectionTarget, []model.ScreenPositionTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()

	detections := s.expiredDetections
	positions := s.expiredPositions
	s.expiredDetections = nil
	s.expiredPositions = nil
	return detections, positions
}

func (s *MemoryStore) currentDeviceLocationLocked() *model.ScreenDeviceLocationResponse {
	if s.deviceLocationProvider == nil {
		return nil
	}
	return cloneDeviceLocation(s.deviceLocationProvider())
}

// appendBounded 追加数据，并只保留最新的 maxItems 条。
func appendBounded[T any](items []T, item T, maxItems int) []T {
	items = append(items, item)
	if len(items) <= maxItems {
		return items
	}
	return items[len(items)-maxItems:]
}

// latest 以最新优先返回数据，并避免暴露存储底层切片。
func latest[T any](items []T, limit int) []T {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]T, 0, limit)
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, items[i])
	}
	return out
}

func (s *MemoryStore) addScreenDetectionLocked(
	record model.DetectionRecord,
	now time.Time,
) (model.ScreenDetectionTarget, bool) {
	if record.Kind != "detect" {
		return model.ScreenDetectionTarget{}, false
	}
	record.Model = normalizeScreenTargetModel(record.Model)
	record.DisplayModel = model.DisplayModelName(record.Model)
	if record.Model == "" || record.Frequency == 0 {
		return model.ScreenDetectionTarget{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}

	s.pruneExpiredScreenDetectionsLocked(now)
	matches := make([]int, 0, 1)
	for index := range s.screen {
		if screenDetectionTargetMatches(s.screen[index], record) {
			matches = append(matches, index)
		}
	}

	if len(matches) == 0 {
		s.screenSequence++
		target := model.ScreenDetectionTarget{
			ID:           fmt.Sprintf("screen-%d-%d", now.UnixNano(), s.screenSequence),
			Serial:       newScreenDetectionSerial(),
			Model:        record.Model,
			DisplayModel: record.DisplayModel,
			Frequency:    record.Frequency,
			RSSI:         record.RSSI,
			Device:       stringsTrim(record.Device),
			FirstSeen:    now,
			LastSeen:     now,
			HitCount:     1,
			LastRecord:   screenDetectionLastRecord(record),
		}
		insertScreenDetectionByFirstSeen(&s.screen, target)
		trimScreenDetectionsToLimit(&s.screen, s.maxDetections)
		return cloneScreenDetectionTarget(target), true
	}

	merged := s.screen[matches[0]]
	for _, matchIndex := range matches[1:] {
		if merged.Device == "" {
			merged.Device = s.screen[matchIndex].Device
		}
		if merged.Serial == "" {
			merged.Serial = s.screen[matchIndex].Serial
		}
		if s.screen[matchIndex].FirstSeen.Before(merged.FirstSeen) {
			merged.FirstSeen = s.screen[matchIndex].FirstSeen
		}
		merged.HitCount += s.screen[matchIndex].HitCount
	}
	if merged.Serial == "" {
		merged.Serial = newScreenDetectionSerial()
	}
	merged.Model = record.Model
	merged.DisplayModel = record.DisplayModel
	merged.Frequency = record.Frequency
	merged.RSSI = record.RSSI
	if device := stringsTrim(record.Device); device != "" {
		merged.Device = device
	}
	merged.LastSeen = now
	merged.HitCount++
	merged.LastRecord = screenDetectionLastRecord(record)

	next := make([]model.ScreenDetectionTarget, 0, len(s.screen)-len(matches)+1)
	for index, target := range s.screen {
		if slices.Contains(matches, index) {
			continue
		}
		next = append(next, target)
	}
	insertScreenDetectionByFirstSeen(&next, merged)
	s.screen = next
	return cloneScreenDetectionTarget(merged), true
}

func (s *MemoryStore) pruneExpiredScreenDetectionsLocked(now time.Time) {
	if len(s.screen) == 0 {
		return
	}
	active := s.screen[:0]
	for _, target := range s.screen {
		if now.Sub(target.LastSeen) <= screenDetectionTTL {
			active = append(active, target)
			continue
		}
		s.expiredDetections = append(s.expiredDetections, cloneScreenDetectionTarget(target))
	}
	clear(s.screen[len(active):])
	s.screen = active
}

func (s *MemoryStore) addScreenPositionLocked(target model.ScreenPositionTarget) (model.ScreenPositionTarget, bool) {
	target.CorrelationID = stringsTrim(target.CorrelationID)
	target.Serial = stringsTrim(target.Serial)
	target.Model = normalizeScreenTargetModel(target.Model)
	target.Source = stringsTrim(target.Source)
	target.Sources = appendScreenTargetSources(target.Sources, target.Source)
	if isUncrackedDJIDroneTarget(target) {
		return model.ScreenPositionTarget{}, false
	}
	if target.Serial == "" || target.Model == "" {
		return model.ScreenPositionTarget{}, false
	}
	target.LastRecord.Model = normalizeScreenTargetModel(target.LastRecord.Model)
	if target.LastSeen.IsZero() {
		target.LastSeen = time.Now()
	}
	if target.FirstSeen.IsZero() {
		target.FirstSeen = target.LastSeen
	}
	target.Device = stringsTrim(target.Device)
	target.DroneTrajectory = appendScreenPositionTrajectory(
		target.DroneTrajectory,
		target.Drone,
		target.LastSeen,
		screenPositionTrajectoryValue(target.TrajectorySpeed, target.Speed),
		screenPositionTrajectoryValue(target.TrajectoryHeight, target.Height),
	)
	target.PilotTrajectory = appendScreenPositionTrajectory(
		target.PilotTrajectory,
		target.Pilot,
		target.LastSeen,
		screenPositionTrajectoryValue(target.TrajectorySpeed, target.Speed),
		screenPositionTrajectoryValue(target.TrajectoryHeight, target.Height),
	)

	now := target.LastSeen
	s.pruneExpiredScreenPositionsLocked(now)

	matches := make([]int, 0, 1)
	for index := range s.positions {
		if screenPositionTargetMatches(s.positions[index], target) {
			matches = append(matches, index)
		}
	}

	if len(matches) == 0 {
		s.positionSeq++
		target.ID = fmt.Sprintf("screen-position-%d-%d", target.LastSeen.UnixNano(), s.positionSeq)
		target.HitCount = 1
		s.positions = appendBounded(s.positions, target, s.maxDetections)
		return cloneScreenPositionTarget(target), true
	}

	baseIndex := screenPositionMergeBaseIndex(s.positions, matches, target)
	merged := s.positions[baseIndex]
	for _, matchIndex := range matches {
		if matchIndex == baseIndex {
			continue
		}
		if merged.Device == "" {
			merged.Device = s.positions[matchIndex].Device
		}
		if merged.CorrelationID == "" {
			merged.CorrelationID = s.positions[matchIndex].CorrelationID
		}
		merged.Sources = appendScreenTargetSources(merged.Sources, s.positions[matchIndex].Source)
		merged.Sources = appendScreenTargetSources(merged.Sources, s.positions[matchIndex].Sources...)
		if s.positions[matchIndex].FirstSeen.Before(merged.FirstSeen) {
			merged.FirstSeen = s.positions[matchIndex].FirstSeen
		}
		merged.DroneTrajectory = mergeScreenPositionTrajectories(
			merged.DroneTrajectory,
			s.positions[matchIndex].DroneTrajectory,
		)
		merged.PilotTrajectory = mergeScreenPositionTrajectories(
			merged.PilotTrajectory,
			s.positions[matchIndex].PilotTrajectory,
		)
		merged.HitCount += s.positions[matchIndex].HitCount
	}
	if target.CorrelationID != "" {
		merged.CorrelationID = target.CorrelationID
	}
	merged.Sources = appendScreenTargetSources(merged.Sources, target.Source)
	merged.Sources = appendScreenTargetSources(merged.Sources, target.Sources...)
	keepDecodedFields := shouldKeepDecodedScreenPositionFields(merged, target)
	if !keepDecodedFields {
		merged.Serial = target.Serial
		merged.Model = target.Model
		merged.Source = target.Source
	}
	merged.Frequency = target.Frequency
	merged.RSSI = target.RSSI
	if target.Device != "" {
		merged.Device = target.Device
	}
	merged.DroneTrajectory = mergeScreenPositionTrajectories(merged.DroneTrajectory, target.DroneTrajectory)
	merged.PilotTrajectory = mergeScreenPositionTrajectories(merged.PilotTrajectory, target.PilotTrajectory)
	if !keepDecodedFields {
		merged.Drone = cloneScreenPositionPoint(target.Drone)
		merged.Pilot = cloneScreenPositionPoint(target.Pilot)
		merged.Home = cloneScreenPositionPoint(target.Home)
		merged.Height = cloneFloat64Ptr(target.Height)
		merged.Altitude = cloneFloat64Ptr(target.Altitude)
		merged.Speed = cloneFloat64Ptr(target.Speed)
		merged.Cracked = target.Cracked
		merged.LastRecord = target.LastRecord
	}
	merged.LastSeen = target.LastSeen
	merged.HitCount++
	if target.FirstSeen.Before(merged.FirstSeen) {
		merged.FirstSeen = target.FirstSeen
	}

	next := make([]model.ScreenPositionTarget, 0, len(s.positions)-len(matches)+1)
	for index, existing := range s.positions {
		if slices.Contains(matches, index) {
			continue
		}
		next = append(next, existing)
	}
	next = append(next, merged)
	s.positions = next
	return cloneScreenPositionTarget(merged), true
}

func appendScreenPositionTrajectory(
	trajectory []model.ScreenPositionTrackPoint,
	point *model.ScreenPositionPoint,
	seenAt time.Time,
	speed *float64,
	height *float64,
) []model.ScreenPositionTrackPoint {
	merged := protocolmerge.AppendTrajectory(
		screenTrackPointsToProtocol(trajectory),
		screenPositionPointToProtocol(point),
		seenAt,
		speed,
		height,
		protocolmerge.TrajectoryOptions{},
	)
	return protocolTrackPointsToScreen(merged)
}

func screenPositionTrajectoryValue(primary, fallback *float64) *float64 {
	if primary != nil {
		return primary
	}
	return fallback
}

func mergeScreenPositionTrajectories(
	current []model.ScreenPositionTrackPoint,
	incoming []model.ScreenPositionTrackPoint,
) []model.ScreenPositionTrackPoint {
	merged := protocolmerge.MergeTrajectories(
		screenTrackPointsToProtocol(current),
		screenTrackPointsToProtocol(incoming),
		protocolmerge.TrajectoryOptions{},
	)
	return protocolTrackPointsToScreen(merged)
}

func (s *MemoryStore) pruneExpiredScreenPositionsLocked(now time.Time) {
	if len(s.positions) == 0 {
		return
	}
	active := s.positions[:0]
	for _, target := range s.positions {
		if now.Sub(target.LastSeen) <= screenPositionTTL {
			active = append(active, target)
			continue
		}
		s.expiredPositions = append(s.expiredPositions, cloneScreenPositionTarget(target))
	}
	clear(s.positions[len(active):])
	s.positions = active
}

func screenPositionTargetMatches(existing, incoming model.ScreenPositionTarget) bool {
	return protocolmerge.PositionMatches(
		screenPositionTargetToProtocol(existing),
		screenPositionTargetToProtocol(incoming),
	)
}

func isUncrackedDJIDroneTarget(target model.ScreenPositionTarget) bool {
	return !target.Cracked && target.Model == uncrackedDJIDroneModel
}

func normalizeScreenTargetModel(modelName string) string {
	return protocolmerge.NormalizeDetectionModel(modelName)
}

func newScreenDetectionSerial() string {
	var token [6]byte
	if _, err := rand.Read(token[:]); err == nil {
		return "DET-" + strings.ToUpper(hex.EncodeToString(token[:]))
	}
	return fmt.Sprintf("DET-%d", time.Now().UnixNano())
}

func appendScreenTargetSources(current []string, values ...string) []string {
	for _, value := range values {
		value = stringsTrim(value)
		if value == "" || slices.Contains(current, value) {
			continue
		}
		current = append(current, value)
	}
	return current
}

func screenPositionMergeBaseIndex(
	positions []model.ScreenPositionTarget,
	matches []int,
	incoming model.ScreenPositionTarget,
) int {
	baseIndex := matches[0]
	if incoming.Cracked {
		return baseIndex
	}
	for _, matchIndex := range matches {
		if positions[matchIndex].Cracked {
			return matchIndex
		}
	}
	return baseIndex
}

func shouldKeepDecodedScreenPositionFields(existing, incoming model.ScreenPositionTarget) bool {
	return protocolmerge.ShouldKeepDecodedPositionFields(
		screenPositionTargetToProtocol(existing),
		screenPositionTargetToProtocol(incoming),
	)
}

func screenDetectionTargetMatches(target model.ScreenDetectionTarget, record model.DetectionRecord) bool {
	return protocolmerge.DetectionMatches(
		target.Model,
		target.Frequency,
		record.Model,
		record.Frequency,
		protocolmerge.DetectionOptions{},
	)
}

func latestScreenDetections(items []model.ScreenDetectionTarget, limit int) []model.ScreenDetectionTarget {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]model.ScreenDetectionTarget, len(items))
	for index, item := range items {
		out[index] = cloneScreenDetectionTarget(item)
	}
	slices.SortFunc(out, func(a, b model.ScreenDetectionTarget) int {
		if result := b.FirstSeen.Compare(a.FirstSeen); result != 0 {
			return result
		}
		return cmpStringDescending(a.ID, b.ID)
	})
	return out[:limit]
}

func insertScreenDetectionByFirstSeen(items *[]model.ScreenDetectionTarget, target model.ScreenDetectionTarget) {
	index := len(*items)
	for i, item := range *items {
		if target.FirstSeen.After(item.FirstSeen) || (target.FirstSeen.Equal(item.FirstSeen) && target.ID > item.ID) {
			index = i
			break
		}
	}
	*items = append(*items, model.ScreenDetectionTarget{})
	copy((*items)[index+1:], (*items)[index:])
	(*items)[index] = target
}

func trimScreenDetectionsToLimit(items *[]model.ScreenDetectionTarget, limit int) {
	if limit <= 0 || len(*items) <= limit {
		return
	}
	clear((*items)[limit:])
	*items = (*items)[:limit]
}

func cmpStringDescending(a, b string) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	default:
		return 0
	}
}

func cloneScreenDetectionTarget(target model.ScreenDetectionTarget) model.ScreenDetectionTarget {
	return target
}

func latestScreenPositions(items []model.ScreenPositionTarget, limit int) []model.ScreenPositionTarget {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]model.ScreenPositionTarget, len(items))
	for index, item := range items {
		out[index] = cloneScreenPositionTarget(item)
	}
	slices.SortFunc(out, func(a, b model.ScreenPositionTarget) int {
		if result := b.FirstSeen.Compare(a.FirstSeen); result != 0 {
			return result
		}
		return cmpStringDescending(a.ID, b.ID)
	})
	return out[:limit]
}

func cloneScreenPositionTarget(target model.ScreenPositionTarget) model.ScreenPositionTarget {
	target.Sources = cloneStrings(target.Sources)
	target.Drone = cloneScreenPositionPoint(target.Drone)
	target.Pilot = cloneScreenPositionPoint(target.Pilot)
	target.Home = cloneScreenPositionPoint(target.Home)
	target.DroneTrajectory = cloneScreenPositionTrajectory(target.DroneTrajectory)
	target.PilotTrajectory = cloneScreenPositionTrajectory(target.PilotTrajectory)
	target.TrajectorySpeed = cloneFloat64Ptr(target.TrajectorySpeed)
	target.TrajectoryHeight = cloneFloat64Ptr(target.TrajectoryHeight)
	target.Height = cloneFloat64Ptr(target.Height)
	target.Altitude = cloneFloat64Ptr(target.Altitude)
	target.Speed = cloneFloat64Ptr(target.Speed)
	target.PilotDistanceM = cloneFloat64Ptr(target.PilotDistanceM)
	target.DroneDistanceM = cloneFloat64Ptr(target.DroneDistanceM)
	target.DroneDirectionDeg = cloneFloat64Ptr(target.DroneDirectionDeg)
	target.DeviceDirectionDeg = cloneFloat64Ptr(target.DeviceDirectionDeg)
	return target
}

func applyPositionRelations(items []model.ScreenPositionTarget, deviceLocation *model.ScreenDeviceLocationResponse) {
	for index := range items {
		items[index] = withPositionRelations(items[index], deviceLocation)
	}
}

func withPositionRelations(target model.ScreenPositionTarget, deviceLocation *model.ScreenDeviceLocationResponse) model.ScreenPositionTarget {
	relations := protocolmerge.PositionRelations(
		screenDeviceLocationToProtocolPoint(deviceLocation),
		screenPositionPointToProtocol(target.Drone),
		screenPositionPointToProtocol(target.Pilot),
	)
	target.PilotDistanceM = relations.PilotDistanceM
	target.DroneDistanceM = relations.DroneDistanceM
	target.DroneDirectionDeg = relations.DroneDirectionDeg
	target.DeviceDirectionDeg = relations.DeviceDirectionDeg
	return target
}

func screenPositionTargetToProtocol(target model.ScreenPositionTarget) protocolmodel.PositionTarget {
	return protocolmodel.PositionTarget{
		CorrelationID: target.CorrelationID,
		Serial:        target.Serial,
		Model:         target.Model,
		Source:        protocolmodel.MessageType(target.Source),
		Frequency:     target.Frequency,
		RSSI:          target.RSSI,
		Device:        target.Device,
		Drone:         screenPositionPointToProtocol(target.Drone),
		Pilot:         screenPositionPointToProtocol(target.Pilot),
		Home:          screenPositionPointToProtocol(target.Home),
		Height:        cloneFloat64Ptr(target.Height),
		Altitude:      cloneFloat64Ptr(target.Altitude),
		Speed:         cloneFloat64Ptr(target.Speed),
		Cracked:       target.Cracked,
		FirstSeen:     target.FirstSeen,
		LastSeen:      target.LastSeen,
	}
}

func screenDeviceLocationToProtocolPoint(location *model.ScreenDeviceLocationResponse) *protocolmodel.Point {
	if location == nil || !location.Valid || location.Point == nil {
		return nil
	}
	return &protocolmodel.Point{
		Latitude:  location.Point.Latitude,
		Longitude: location.Point.Longitude,
	}
}

func screenPositionPointToProtocol(point *model.ScreenPositionPoint) *protocolmodel.Point {
	if point == nil {
		return nil
	}
	return &protocolmodel.Point{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
	}
}

func screenTrackPointsToProtocol(points []model.ScreenPositionTrackPoint) []protocolmodel.TrackPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]protocolmodel.TrackPoint, 0, len(points))
	for _, point := range points {
		out = append(out, protocolmodel.TrackPoint{
			Latitude:  point.Latitude,
			Longitude: point.Longitude,
			Speed:     cloneFloat64Ptr(point.Speed),
			Height:    cloneFloat64Ptr(point.Height),
			Time:      point.Time,
		})
	}
	return out
}

func protocolTrackPointsToScreen(points []protocolmodel.TrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]model.ScreenPositionTrackPoint, 0, len(points))
	for _, point := range points {
		out = append(out, model.ScreenPositionTrackPoint{
			Latitude:  point.Latitude,
			Longitude: point.Longitude,
			Speed:     cloneFloat64Ptr(point.Speed),
			Height:    cloneFloat64Ptr(point.Height),
			Time:      point.Time,
		})
	}
	return out
}

func cloneDeviceLocation(location *model.ScreenDeviceLocationResponse) *model.ScreenDeviceLocationResponse {
	if location == nil || !location.Valid || location.Point == nil {
		return nil
	}
	cloned := *location
	point := *location.Point
	cloned.Point = &point
	if location.UpdatedAt != nil {
		updatedAt := *location.UpdatedAt
		cloned.UpdatedAt = &updatedAt
	}
	return &cloned
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneScreenPositionTrajectory(points []model.ScreenPositionTrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) == 0 {
		return nil
	}
	cloned := make([]model.ScreenPositionTrackPoint, len(points))
	for index, point := range points {
		point.Speed = cloneFloat64Ptr(point.Speed)
		point.Height = cloneFloat64Ptr(point.Height)
		cloned[index] = point
	}
	return cloned
}

func cloneScreenPositionPoint(point *model.ScreenPositionPoint) *model.ScreenPositionPoint {
	if point == nil {
		return nil
	}
	cloned := *point
	return &cloned
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func screenDetectionLastRecord(record model.DetectionRecord) model.ScreenDetectionLastRecord {
	return model.ScreenDetectionLastRecord{
		ID:           record.ID,
		Kind:         record.Kind,
		ReceivedAt:   record.ReceivedAt,
		Device:       record.Device,
		Model:        record.Model,
		DisplayModel: record.DisplayModel,
		Frequency:    record.Frequency,
		RSSI:         record.RSSI,
		Summary:      record.Summary,
	}
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
