// Package store 保存有上限的运行记录，并管理服务端事件订阅者。
package store

import (
	"fmt"
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
	screenDetectionEventType        = "screen.detection.updated"
	screenPositionEventType         = "screen.position.updated"
)

// MemoryStore 在内存中保存有上限的记录，并广播运行时事件。
type MemoryStore struct {
	mu sync.RWMutex

	maxDetections int
	maxParsed     int

	detections []model.DetectionRecord
	screen     []model.ScreenDetectionTarget
	positions  []model.ScreenPositionTarget
	parsed     []model.ParsedMessage
	gps        []model.GPSRecord

	screenSequence uint64
	positionSeq    uint64
	subscribers    map[chan model.Event]struct{}
}

// NewMemoryStore 创建带历史长度上限的内存存储。
func NewMemoryStore(maxDetections, maxParsed int) *MemoryStore {
	return &MemoryStore{
		maxDetections: max(1, maxDetections),
		maxParsed:     max(1, maxParsed),
		subscribers:   make(map[chan model.Event]struct{}),
	}
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
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.record", Time: record.ReceivedAt, Payload: record})
	if updated {
		s.Publish(model.Event{Type: screenDetectionEventType, Time: target.LastSeen, Payload: target})
	}
}

// AddScreenPosition 追加或合并大屏定位目标，并发布定位事件。
func (s *MemoryStore) AddScreenPosition(target model.ScreenPositionTarget) (model.ScreenPositionTarget, bool) {
	s.mu.Lock()
	merged, updated := s.addScreenPositionLocked(target)
	s.mu.Unlock()

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
	defer s.mu.Unlock()
	now := time.Now()
	s.pruneExpiredScreenDetectionsLocked(now)
	return latestScreenDetections(s.screen, limit)
}

// ListScreenPositions 按首次发现时间倒序返回大屏合并定位目标，避免实时更新导致列表跳动。
func (s *MemoryStore) ListScreenPositions(limit int) []model.ScreenPositionTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.pruneExpiredScreenPositionsLocked(now)
	return latestScreenPositions(s.positions, limit)
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
		}
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

func (s *MemoryStore) pruneExpiredScreenPositionsLocked(now time.Time) {
	if len(s.positions) == 0 {
		return
	}
	active := s.positions[:0]
	for _, target := range s.positions {
		if now.Sub(target.LastSeen) <= screenPositionTTL {
			active = append(active, target)
		}
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
	existing = strings.ToUpper(stringsTrim(existing))
	incoming = strings.ToUpper(stringsTrim(incoming))
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
	return false
}

func screenPositionTrimRIDSerialPrefix(serial string) string {
	const ridSerialPrefix = "1581"
	if len(serial) <= len(ridSerialPrefix) || !strings.HasPrefix(serial, ridSerialPrefix) {
		return serial
	}
	return strings.TrimPrefix(serial, ridSerialPrefix)
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
	target.Height = cloneFloat64Ptr(target.Height)
	target.Altitude = cloneFloat64Ptr(target.Altitude)
	target.Speed = cloneFloat64Ptr(target.Speed)
	return target
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
