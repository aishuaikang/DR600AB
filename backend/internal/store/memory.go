// Package store 保存有上限的运行记录，并管理服务端事件订阅者。
package store

import (
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"dr600ab-api/internal/model"
)

const (
	screenDetectionBaseThresholdMHz = 15.0
	screenDetectionTTL              = 60 * time.Second
	screenPositionTTL               = 60 * time.Second
	screenPositionTrajectoryLimit   = 120
	screenDetectionEventType        = "screen.detection.updated"
	screenPositionEventType         = "screen.position.updated"
)

// MemoryStore 在内存中保存有上限的记录，并广播运行时事件。
type MemoryStore struct {
	mu sync.RWMutex

	maxDetections int
	maxParsed     int
	archiver      IntrusionArchiver

	detections []model.DetectionRecord
	screen     []model.ScreenDetectionTarget
	positions  []model.ScreenPositionTarget
	parsed     []model.ParsedMessage
	gps        []model.GPSRecord

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

// AddParsed 追加解析消息，并发布解析事件。
func (s *MemoryStore) AddParsed(msg model.ParsedMessage) {
	s.mu.Lock()
	s.parsed = appendBounded(s.parsed, msg, s.maxParsed)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.parsed", Time: msg.Time, Payload: msg})
}

// AddDetection 追加侦测记录，并发布侦测事件。
func (s *MemoryStore) AddDetection(record model.DetectionRecord) {
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
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)

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
	s.mu.Unlock()

	s.archiveExpiredScreenTargets(archiver)
	return items
}

// ListGPS 按时间倒序返回最新 GPS 记录。
func (s *MemoryStore) ListGPS(limit int) []model.GPSRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.gps, limit)
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
			ID:         fmt.Sprintf("screen-%d-%d", now.UnixNano(), s.screenSequence),
			Model:      record.Model,
			Frequency:  record.Frequency,
			RSSI:       record.RSSI,
			Device:     stringsTrim(record.Device),
			FirstSeen:  now,
			LastSeen:   now,
			HitCount:   1,
			LastRecord: screenDetectionLastRecord(record),
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
		if s.screen[matchIndex].FirstSeen.Before(merged.FirstSeen) {
			merged.FirstSeen = s.screen[matchIndex].FirstSeen
		}
		merged.HitCount += s.screen[matchIndex].HitCount
	}
	merged.Model = record.Model
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
	target.Model = normalizeScreenPositionModel(target.Model)
	if target.Serial == "" || target.Model == "" {
		return model.ScreenPositionTarget{}, false
	}
	target.LastRecord.Model = normalizeScreenPositionModel(target.LastRecord.Model)
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
	result := normalizedScreenPositionTrajectory(trajectory)
	if !validScreenPositionTrackCoordinate(point) || seenAt.IsZero() {
		return trimScreenPositionTrajectory(result)
	}

	next := model.ScreenPositionTrackPoint{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
		Speed:     cloneFloat64Ptr(speed),
		Height:    cloneFloat64Ptr(height),
		Time:      seenAt,
	}
	result = append(result, next)
	return trimScreenPositionTrajectory(deduplicateScreenPositionTrajectory(result))
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
	if len(current) == 0 {
		return trimScreenPositionTrajectory(normalizedScreenPositionTrajectory(incoming))
	}
	if len(incoming) == 0 {
		return trimScreenPositionTrajectory(normalizedScreenPositionTrajectory(current))
	}

	merged := append(normalizedScreenPositionTrajectory(current), normalizedScreenPositionTrajectory(incoming)...)
	return trimScreenPositionTrajectory(deduplicateScreenPositionTrajectory(merged))
}

func normalizedScreenPositionTrajectory(points []model.ScreenPositionTrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]model.ScreenPositionTrackPoint, 0, len(points))
	for _, point := range points {
		if !validTrackPointCoordinate(point.Latitude, point.Longitude) || point.Time.IsZero() {
			continue
		}
		point.Speed = cloneFloat64Ptr(point.Speed)
		point.Height = cloneFloat64Ptr(point.Height)
		out = append(out, point)
	}
	return out
}

func deduplicateScreenPositionTrajectory(points []model.ScreenPositionTrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) <= 1 {
		return points
	}
	slices.SortFunc(points, func(a, b model.ScreenPositionTrackPoint) int {
		if result := a.Time.Compare(b.Time); result != 0 {
			return result
		}
		if a.Latitude < b.Latitude {
			return -1
		}
		if a.Latitude > b.Latitude {
			return 1
		}
		if a.Longitude < b.Longitude {
			return -1
		}
		if a.Longitude > b.Longitude {
			return 1
		}
		return 0
	})

	out := points[:0]
	for _, point := range points {
		if len(out) > 0 && sameScreenPositionTrackPoint(out[len(out)-1], point) {
			continue
		}
		out = append(out, point)
	}
	clear(points[len(out):])
	return out
}

func trimScreenPositionTrajectory(points []model.ScreenPositionTrackPoint) []model.ScreenPositionTrackPoint {
	if len(points) <= screenPositionTrajectoryLimit {
		return points
	}
	return points[len(points)-screenPositionTrajectoryLimit:]
}

func sameScreenPositionTrackPoint(a, b model.ScreenPositionTrackPoint) bool {
	return a.Latitude == b.Latitude &&
		a.Longitude == b.Longitude &&
		float64PtrEqual(a.Speed, b.Speed) &&
		float64PtrEqual(a.Height, b.Height) &&
		a.Time.Equal(b.Time)
}

func float64PtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func validScreenPositionTrackCoordinate(point *model.ScreenPositionPoint) bool {
	return point != nil && validTrackPointCoordinate(point.Latitude, point.Longitude)
}

func validTrackPointCoordinate(latitude, longitude float64) bool {
	return finiteFloat64(latitude) &&
		finiteFloat64(longitude) &&
		latitude >= -90 &&
		latitude <= 90 &&
		longitude >= -180 &&
		longitude <= 180 &&
		!(latitude == 0 && longitude == 0)
}

func finiteFloat64(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
	if screenPositionSerialMatches(existing.Serial, incoming.Serial) {
		return true
	}
	return screenPositionPendingEncryptedTargetMatches(existing, incoming)
}

func normalizeScreenPositionModel(modelName string) string {
	modelName = stringsTrim(modelName)
	prefix, suffix, ok := strings.Cut(modelName, "-")
	if !ok || stringsTrim(suffix) == "" || !isDecimalString(stringsTrim(prefix)) {
		return modelName
	}
	return stringsTrim(suffix)
}

func isDecimalString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func screenPositionPendingEncryptedTargetMatches(existing, incoming model.ScreenPositionTarget) bool {
	existingCorrelationID := stringsTrim(existing.CorrelationID)
	incomingCorrelationID := stringsTrim(incoming.CorrelationID)
	return existingCorrelationID != "" &&
		incomingCorrelationID != "" &&
		existingCorrelationID == incomingCorrelationID &&
		(screenPositionIsPendingEncrypted(existing) || screenPositionIsPendingEncrypted(incoming))
}

func screenPositionIsPendingEncrypted(target model.ScreenPositionTarget) bool {
	if target.Cracked || stringsTrim(target.CorrelationID) == "" {
		return false
	}
	return strings.TrimPrefix(stringsTrim(target.CorrelationID), "did_encrypted:") == strings.ToLower(stringsTrim(target.Serial))
}

func screenPositionSerialMatches(existing, incoming string) bool {
	existingRaw := strings.ToUpper(stringsTrim(existing))
	incomingRaw := strings.ToUpper(stringsTrim(incoming))
	existing = screenPositionCanonicalSerial(existingRaw)
	incoming = screenPositionCanonicalSerial(incomingRaw)
	if existing == "" || incoming == "" {
		return false
	}
	if existing == incoming {
		return true
	}

	const ridSerialPrefix = "1581"
	if screenPositionTrimRIDSerialPrefix(existing) == incoming {
		return true
	}
	if screenPositionTrimRIDSerialPrefix(incoming) == existing {
		return true
	}
	if screenPositionSerialSuffixMatches(existing, incoming, existingRaw, incomingRaw) {
		return true
	}
	return false
}

func screenPositionCanonicalSerial(serial string) string {
	var builder strings.Builder
	builder.Grow(len(serial))
	for _, r := range serial {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func screenPositionTrimRIDSerialPrefix(serial string) string {
	const ridSerialPrefix = "1581"
	if len(serial) <= len(ridSerialPrefix) || !strings.HasPrefix(serial, ridSerialPrefix) {
		return serial
	}
	return strings.TrimPrefix(serial, ridSerialPrefix)
}

func screenPositionSerialSuffixMatches(existing, incoming, existingRaw, incomingRaw string) bool {
	const minSuffixLength = 10
	shorter, longer := existing, incoming
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	commonSuffixLength := screenPositionCommonSuffixLength(existing, incoming)
	if commonSuffixLength < minSuffixLength {
		return false
	}
	if screenPositionSerialHasCorruptedPrefix(existingRaw, existing, commonSuffixLength) ||
		screenPositionSerialHasCorruptedPrefix(incomingRaw, incoming, commonSuffixLength) {
		return true
	}
	return len(shorter) == commonSuffixLength && len(longer)-len(shorter) >= 4
}

func screenPositionSerialHasCorruptedPrefix(raw, canonical string, suffixLength int) bool {
	if suffixLength >= len(canonical) {
		return false
	}
	if len(canonical)-suffixLength > 3 {
		return false
	}
	for _, r := range raw {
		return !screenPositionSerialRuneIsCanonical(r)
	}
	return false
}

func screenPositionSerialRuneIsCanonical(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z')
}

func screenPositionCommonSuffixLength(left, right string) int {
	count := 0
	for count < len(left) && count < len(right) {
		if left[len(left)-1-count] != right[len(right)-1-count] {
			break
		}
		count++
	}
	return count
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
	if !existing.Cracked || incoming.Cracked {
		return false
	}
	existingCorrelationID := stringsTrim(existing.CorrelationID)
	incomingCorrelationID := stringsTrim(incoming.CorrelationID)
	if existingCorrelationID != "" && incomingCorrelationID != "" && existingCorrelationID == incomingCorrelationID {
		return true
	}
	return screenPositionSerialMatches(existing.Serial, incoming.Serial)
}

func screenDetectionTargetMatches(target model.ScreenDetectionTarget, record model.DetectionRecord) bool {
	if target.Model == "" || record.Model == "" {
		return false
	}
	freqDiff := math.Abs(target.Frequency - record.Frequency)
	switch {
	case isAutelType(target.Model) || isAutelType(record.Model):
		return freqDiff <= screenDetectionBaseThresholdMHz+25 && (target.Model == record.Model || (isAutelType(target.Model) && isAutelType(record.Model)))
	case target.Model == "O3+_ofdm_datalink" || record.Model == "O3+_ofdm_datalink":
		return freqDiff <= screenDetectionBaseThresholdMHz+5 && target.Model == record.Model
	default:
		return freqDiff <= screenDetectionBaseThresholdMHz && (target.Model == record.Model || (isDJIType(target.Model) && isDJIType(record.Model)))
	}
}

func isAutelType(model string) bool {
	return model == "Autel_type1" || model == "Autel_type2" || model == "Autel_type3" || model == "Autel_type4" || model == "Autel_type5"
}

func isDJIType(model string) bool {
	return model == "DJI_OC123_10M" || model == "DJI_OC123_20M"
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
	return target
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
		ID:         record.ID,
		Kind:       record.Kind,
		ReceivedAt: record.ReceivedAt,
		Device:     record.Device,
		Model:      record.Model,
		Frequency:  record.Frequency,
		RSSI:       record.RSSI,
		Summary:    record.Summary,
	}
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
