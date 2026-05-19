package interference

import (
	"reflect"
	"testing"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
)

type fakePin struct {
	setup   int
	high    int
	low     int
	cleanup int
	value   int
}

func (f *fakePin) Setup() error {
	f.setup++
	return nil
}

func (f *fakePin) SetHigh() error {
	f.high++
	f.value = 1
	return nil
}

func (f *fakePin) SetLow() error {
	f.low++
	f.value = 0
	return nil
}

func (f *fakePin) GetValue() (int, error) {
	return f.value, nil
}

func (f *fakePin) Cleanup() {
	f.cleanup++
}

func TestSetStateControlsPinLifecycle(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	fake := &fakePin{}
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 96, Bands: []string{"2.4"}},
	}, func(number int) GPIOPin {
		return fake
	})

	channel, err := svc.SetState("io1", true, "zh-CN")
	if err != nil {
		t.Fatalf("SetState(true) error = %v", err)
	}
	if !channel.Enabled || channel.Status != "active" {
		t.Fatalf("unexpected enabled channel: %+v", channel)
	}
	if fake.setup != 1 || fake.high != 1 {
		t.Fatalf("pin calls = setup:%d high:%d", fake.setup, fake.high)
	}

	channel, err = svc.SetState("io1", false, "zh-CN")
	if err != nil {
		t.Fatalf("SetState(false) error = %v", err)
	}
	if channel.Enabled || channel.Status != "idle" {
		t.Fatalf("unexpected disabled channel: %+v", channel)
	}
	if fake.low != 1 {
		t.Fatalf("pin low calls = %d, want 1", fake.low)
	}
	if fake.cleanup != 1 {
		t.Fatalf("pin cleanup calls after disable = %d, want 1", fake.cleanup)
	}

	svc.Shutdown()
	if fake.cleanup != 1 {
		t.Fatalf("pin cleanup calls = %d, want 1", fake.cleanup)
	}
}

func TestDefaultChannelsUseBoardMapping(t *testing.T) {
	channels := DefaultChannels()
	if len(channels) != 8 {
		t.Fatalf("DefaultChannels() len = %d, want 8", len(channels))
	}

	tests := []struct {
		index    int
		id       string
		label    string
		pin      int
		bands    []string
		reserved bool
	}{
		{index: 0, id: "io1", label: "IOC4", pin: 20, bands: []string{"433", "800", "900", "1.4"}},
		{index: 1, id: "io2", label: "IOC2", pin: 18, bands: []string{"1.2", "1.5"}},
		{index: 2, id: "io3", label: "IOC3", pin: 19, bands: []string{"2.4", "5.2", "5.8"}},
		{index: 3, id: "io4", label: "IOC5", pin: 21, bands: []string{}, reserved: true},
		{index: 4, id: "io5", label: "I3B4", pin: 108, bands: []string{}, reserved: true},
		{index: 5, id: "io6", label: "I3B5", pin: 109, bands: []string{}, reserved: true},
		{index: 6, id: "io7", label: "I3C0", pin: 112, bands: []string{}, reserved: true},
		{index: 7, id: "io8", label: "I3C1", pin: 113, bands: []string{}, reserved: true},
	}

	for _, tt := range tests {
		channel := channels[tt.index]
		if channel.ID != tt.id || channel.Label != tt.label || channel.Pin != tt.pin || channel.Reserved != tt.reserved {
			t.Fatalf("DefaultChannels()[%d] = %+v, want id=%s label=%s pin=%d reserved=%v", tt.index, channel, tt.id, tt.label, tt.pin, tt.reserved)
		}
		if !reflect.DeepEqual(channel.Bands, tt.bands) {
			t.Fatalf("DefaultChannels()[%d].Bands = %+v, want %+v", tt.index, channel.Bands, tt.bands)
		}
	}
}

func TestListChannelsReflectsActualPinValue(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	fake := &fakePin{value: 1}
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 96, Bands: []string{"2.4"}},
	}, func(number int) GPIOPin {
		return fake
	})

	channel := svc.ListChannels()[0]
	if !channel.Enabled || channel.ActualLevel != "high" || channel.Status != "active" {
		t.Fatalf("channel with high actual value = %+v, want enabled high active", channel)
	}

	fake.value = 0
	channel = svc.ListChannels()[0]
	if channel.Enabled || channel.ActualLevel != "low" || channel.Status != "idle" {
		t.Fatalf("channel with low actual value = %+v, want disabled low idle", channel)
	}
}

func TestReservedChannelAllowsStateChanges(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	fake := &fakePin{}
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io4", Label: "IO4", Pin: 62, Reserved: true},
	}, func(number int) GPIOPin {
		return fake
	})

	channel, err := svc.SetState("io4", true, "zh-CN")
	if err != nil {
		t.Fatalf("SetState(true) reserved error = %v", err)
	}
	if !channel.Reserved || !channel.Enabled || channel.Status != "active" {
		t.Fatalf("unexpected reserved channel state: %+v", channel)
	}
	if fake.setup != 1 || fake.high != 1 {
		t.Fatalf("pin calls = setup:%d high:%d, want setup:1 high:1", fake.setup, fake.high)
	}
}

func TestListChannelsReturnsEmptyBandsForReservedChannel(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io4", Label: "IO4", Pin: 62, Reserved: true},
	}, nil)

	channels := svc.ListChannels()
	if len(channels) != 1 {
		t.Fatalf("channel count = %d, want 1", len(channels))
	}
	if channels[0].Bands == nil {
		t.Fatal("bands should be an empty slice, not nil")
	}
	if len(channels[0].Bands) != 0 {
		t.Fatalf("bands count = %d, want 0", len(channels[0].Bands))
	}
}

func TestSetScreenStrikeStartsSelectedChannelsAndStopsUnselected(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	state, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1", "io3"},
		DurationSeconds: 60,
	}, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenStrike() error = %v", err)
	}

	if !state.Active || state.RemainingSeconds <= 0 {
		t.Fatalf("state = %+v, want active countdown", state)
	}
	if !reflect.DeepEqual(state.ChannelIDs, []string{"io1", "io3"}) {
		t.Fatalf("channel IDs = %+v, want io1/io3", state.ChannelIDs)
	}
	if pins["io1"].value != 1 || pins["io3"].value != 1 {
		t.Fatalf("selected pins values = io1:%d io3:%d, want high", pins["io1"].value, pins["io3"].value)
	}
	if pins["io2"].value != 0 {
		t.Fatalf("unselected io2 value = %d, want low", pins["io2"].value)
	}
}

func TestSetScreenStrikeStopTurnsOffAllStrikeChannels(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1", "io2", "io3"},
		DurationSeconds: 60,
	}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike(start) error = %v", err)
	}

	state, err := svc.SetScreenStrike(model.ScreenStrikeRequest{Enabled: false}, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenStrike(stop) error = %v", err)
	}
	if state.Active || state.RemainingSeconds != 0 || len(state.ChannelIDs) != 0 {
		t.Fatalf("state after stop = %+v, want inactive", state)
	}
	for id, pin := range pins {
		if id == "io4" {
			continue
		}
		if pin.value != 0 {
			t.Fatalf("%s value = %d, want low", id, pin.value)
		}
	}
}

func TestSetScreenStrikeRejectsStartWhenAnyStrikeChannelIsHigh(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1"},
		DurationSeconds: 60,
	}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike(first) error = %v", err)
	}
	first := svc.ScreenStrikeState()

	state, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io2"},
		DurationSeconds: 30,
	}, "zh-CN")
	if err == nil {
		t.Fatal("SetScreenStrike(second) error = nil, want strike_already_active")
	}
	if code := ErrorCode(err); code != "strike_already_active" {
		t.Fatalf("ErrorCode() = %q, want strike_already_active (err=%v)", code, err)
	}

	if !state.Active || !reflect.DeepEqual(state.ChannelIDs, []string{"io1"}) {
		t.Fatalf("state after rejected replace = %+v, want original io1 active", state)
	}
	if first.StartedAt == nil || state.StartedAt == nil || !state.StartedAt.Equal(*first.StartedAt) {
		t.Fatalf("startedAt after rejected replace = %v, first = %v", state.StartedAt, first.StartedAt)
	}
	if pins["io1"].value != 1 || pins["io2"].value != 0 || pins["io3"].value != 0 {
		t.Fatalf("pin values = io1:%d io2:%d io3:%d, want original io1 high", pins["io1"].value, pins["io2"].value, pins["io3"].value)
	}
}

func TestSetScreenStrikeInvalidRequestKeepsActiveTimeout(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	if _, err := svc.applyScreenStrike(true, []string{"io1"}, 40*time.Millisecond, 10, "zh-CN"); err != nil {
		t.Fatalf("applyScreenStrike(start) error = %v", err)
	}

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1"},
		DurationSeconds: 9,
	}, "zh-CN"); err == nil {
		t.Fatal("SetScreenStrike(invalid) error = nil, want error")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		state := svc.ScreenStrikeState()
		if !state.Active && pins["io1"].value == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("strike did not time out after invalid request, state=%+v io1=%d", state, pins["io1"].value)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScreenStrikeTimeoutStopsChannels(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	if _, err := svc.applyScreenStrike(true, []string{"io1", "io3"}, 20*time.Millisecond, 10, "zh-CN"); err != nil {
		t.Fatalf("applyScreenStrike() error = %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		state := svc.ScreenStrikeState()
		if !state.Active && pins["io1"].value == 0 && pins["io3"].value == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("strike did not time out, state=%+v io1=%d io3=%d", state, pins["io1"].value, pins["io3"].value)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScreenStrikeStateUsesActualHighLevel(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	pins["io2"].value = 1

	state := svc.ScreenStrikeState()
	if !state.Active {
		t.Fatalf("state active = false, want true: %+v", state)
	}
	if !reflect.DeepEqual(state.ChannelIDs, []string{"io2"}) {
		t.Fatalf("channel IDs = %+v, want io2", state.ChannelIDs)
	}
	if state.RemainingSeconds != 0 || state.StartedAt != nil || state.EndsAt != nil {
		t.Fatalf("manual high state should not invent countdown fields: %+v", state)
	}
}

func TestSetScreenStrikeRejectsInvalidInput(t *testing.T) {
	svc, _ := newStrikeTestService(t)
	defer svc.Shutdown()

	tests := []struct {
		name string
		req  model.ScreenStrikeRequest
		code string
	}{
		{
			name: "empty channels",
			req: model.ScreenStrikeRequest{
				Enabled:         true,
				ChannelIDs:      nil,
				DurationSeconds: 60,
			},
			code: "strike_channels_required",
		},
		{
			name: "reserved channel",
			req: model.ScreenStrikeRequest{
				Enabled:         true,
				ChannelIDs:      []string{"io4"},
				DurationSeconds: 60,
			},
			code: "strike_invalid_channels",
		},
		{
			name: "zero duration",
			req: model.ScreenStrikeRequest{
				Enabled:         true,
				ChannelIDs:      []string{"io1"},
				DurationSeconds: 0,
			},
			code: "strike_invalid_duration",
		},
		{
			name: "below minimum duration",
			req: model.ScreenStrikeRequest{
				Enabled:         true,
				ChannelIDs:      []string{"io1"},
				DurationSeconds: 9,
			},
			code: "strike_invalid_duration",
		},
		{
			name: "too long duration",
			req: model.ScreenStrikeRequest{
				Enabled:         true,
				ChannelIDs:      []string{"io1"},
				DurationSeconds: 61,
			},
			code: "strike_invalid_duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.SetScreenStrike(tt.req, "zh-CN"); err == nil {
				t.Fatal("SetScreenStrike() error = nil, want error")
			} else if code := ErrorCode(err); code != tt.code {
				t.Fatalf("ErrorCode() = %q, want %q (err=%v)", code, tt.code, err)
			}
		})
	}
}

func newStrikeTestService(t *testing.T) (*Service, map[string]*fakePin) {
	t.Helper()

	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	definitions := []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 1, Bands: []string{"433"}},
		{ID: "io2", Label: "IO2", Pin: 2, Bands: []string{"1.2"}},
		{ID: "io3", Label: "IO3", Pin: 3, Bands: []string{"2.4"}},
		{ID: "io4", Label: "IO4", Pin: 4, Reserved: true},
	}
	pins := map[string]*fakePin{
		"io1": {},
		"io2": {},
		"io3": {},
		"io4": {},
	}
	pinsByNumber := map[int]*fakePin{
		1: pins["io1"],
		2: pins["io2"],
		3: pins["io3"],
		4: pins["io4"],
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, definitions, func(number int) GPIOPin {
		return pinsByNumber[number]
	})
	return svc, pins
}
