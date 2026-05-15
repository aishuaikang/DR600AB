// Package store 保存有上限的运行记录，并管理服务端事件订阅者。
package store

import (
	"sync"

	"dr600ab-api/internal/model"
)

// MemoryStore 在内存中保存有上限的记录，并广播运行时事件。
type MemoryStore struct {
	mu sync.RWMutex

	maxDetections int
	maxParsed     int
	maxFPV        int

	detections []model.DetectionRecord
	parsed     []model.ParsedMessage
	fpv        []model.FpvRecord
	gps        []model.GPSRecord

	subscribers map[chan model.Event]struct{}
}

// NewMemoryStore 创建带历史长度上限的内存存储。
func NewMemoryStore(maxDetections, maxParsed, maxFPV int) *MemoryStore {
	return &MemoryStore{
		maxDetections: max(1, maxDetections),
		maxParsed:     max(1, maxParsed),
		maxFPV:        max(1, maxFPV),
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
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.record", Time: record.ReceivedAt, Payload: record})
}

// AddFPV 追加图传记录，并发布图传事件。
func (s *MemoryStore) AddFPV(record model.FpvRecord) {
	s.mu.Lock()
	s.fpv = appendBounded(s.fpv, record, s.maxFPV)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "fpv.record", Time: record.ReceivedAt, Payload: record})
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

// ListFPV 按时间倒序返回最新图传记录。
func (s *MemoryStore) ListFPV(limit int) []model.FpvRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.fpv, limit)
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
