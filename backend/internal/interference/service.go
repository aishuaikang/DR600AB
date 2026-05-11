package interference

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"gpio-controller/gpio"
)

type GPIOPin interface {
	Setup() error
	SetHigh() error
	SetLow() error
	Cleanup()
}

type PinFactory func(number int) GPIOPin

type ChannelDefinition struct {
	ID       string
	Label    string
	Pin      int
	Bands    []string
	Reserved bool
}

type Service struct {
	mu sync.RWMutex

	channels   map[string]*channelState
	order      []string
	pinFactory PinFactory
	store      *store.MemoryStore
	translator *i18n.Translator
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

func DefaultChannels() []ChannelDefinition {
	return []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 96, Bands: []string{"433", "800", "900", "1.4"}},
		{ID: "io2", Label: "IO2", Pin: 107, Bands: []string{"1.2", "1.5"}},
		{ID: "io3", Label: "IO3", Pin: 106, Bands: []string{"2.4", "5.2", "5.8"}},
		{ID: "io4", Label: "IO4", Pin: 62, Bands: []string{}, Reserved: true},
	}
}

func (s *Service) ListChannels() []model.GpioChannel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.GpioChannel, 0, len(s.order))
	for _, id := range s.order {
		result = append(result, s.channels[id].dto())
	}
	return result
}

func (s *Service) SetState(id string, enabled bool, locale string) (model.GpioChannel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.channels[id]
	if !ok {
		return model.GpioChannel{}, fmt.Errorf("%s", s.translator.T(locale, "errors", "channel_not_found"))
	}
	if state.def.Reserved {
		return state.dto(), fmt.Errorf("%s", s.translator.T(locale, "errors", "channel_reserved"))
	}

	if state.pin == nil {
		state.pin = s.pinFactory(state.def.Pin)
	}

	if enabled {
		if !state.initialized {
			if err := state.pin.Setup(); err != nil {
				return s.markError(state, locale, err)
			}
			state.initialized = true
		}
		if err := state.pin.SetHigh(); err != nil {
			return s.markError(state, locale, err)
		}
		state.enabled = true
		state.actualLevel = "high"
		state.desiredLevel = "high"
		state.status = "active"
		state.lastError = ""
	} else {
		if state.pin != nil {
			if err := state.pin.SetLow(); err != nil {
				return s.markError(state, locale, err)
			}
		}
		state.enabled = false
		state.actualLevel = "low"
		state.desiredLevel = "low"
		state.status = "idle"
		state.lastError = ""
	}

	channel := state.dto()
	s.store.Publish(model.Event{Type: "gpio.channel.updated", Time: time.Now(), Payload: channel})
	return channel, nil
}

func (s *Service) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

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
}

func (s *Service) markError(state *channelState, locale string, err error) (model.GpioChannel, error) {
	state.status = "error"
	state.lastError = err.Error()
	channel := state.dto()
	s.store.Publish(model.Event{Type: "gpio.channel.updated", Time: time.Now(), Payload: channel})
	return channel, fmt.Errorf("%s: %w", s.translator.T(locale, "errors", "gpio_update_failed"), err)
}

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

func initialStatus(def ChannelDefinition) string {
	if def.Reserved {
		return "reserved"
	}
	return "idle"
}
