package store

import (
	"sync"

	"dr600ab-api/internal/model"
)

type MemoryStore struct {
	mu sync.RWMutex

	maxDetections int
	maxParsed     int
	maxFPV        int

	detections []model.DetectionRecord
	parsed     []model.ParsedMessage
	fpv        []model.FpvRecord

	subscribers map[chan model.Event]struct{}
}

func NewMemoryStore(maxDetections, maxParsed, maxFPV int) *MemoryStore {
	return &MemoryStore{
		maxDetections: max(1, maxDetections),
		maxParsed:     max(1, maxParsed),
		maxFPV:        max(1, maxFPV),
		subscribers:   make(map[chan model.Event]struct{}),
	}
}

func (s *MemoryStore) AddParsed(msg model.ParsedMessage) {
	s.mu.Lock()
	s.parsed = appendBounded(s.parsed, msg, s.maxParsed)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.parsed", Time: msg.Time, Payload: msg})
}

func (s *MemoryStore) AddDetection(record model.DetectionRecord) {
	s.mu.Lock()
	s.detections = appendBounded(s.detections, record, s.maxDetections)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "detection.record", Time: record.ReceivedAt, Payload: record})
}

func (s *MemoryStore) AddFPV(record model.FpvRecord) {
	s.mu.Lock()
	s.fpv = appendBounded(s.fpv, record, s.maxFPV)
	s.mu.Unlock()

	s.Publish(model.Event{Type: "fpv.record", Time: record.ReceivedAt, Payload: record})
}

func (s *MemoryStore) ListParsed(limit int) []model.ParsedMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.parsed, limit)
}

func (s *MemoryStore) ListDetections(limit int) []model.DetectionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.detections, limit)
}

func (s *MemoryStore) ListFPV(limit int) []model.FpvRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return latest(s.fpv, limit)
}

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

func appendBounded[T any](items []T, item T, maxItems int) []T {
	items = append(items, item)
	if len(items) <= maxItems {
		return items
	}
	return items[len(items)-maxItems:]
}

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
