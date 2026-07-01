package interference

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/store"
	"gpio-controller/board"
)

type fakePin struct {
	setup   int
	high    int
	low     int
	cleanup int
	value   int
	highErr error
	lowErr  error
}

func (f *fakePin) Setup() error {
	f.setup++
	return nil
}

func (f *fakePin) SetHigh() error {
	if f.highErr != nil {
		return f.highErr
	}
	f.high++
	f.value = 1
	return nil
}

func (f *fakePin) SetLow() error {
	if f.lowErr != nil {
		return f.lowErr
	}
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

type directionalFakePin struct {
	fakePin
	direction string
}

func (f *directionalFakePin) Setup() error {
	f.direction = "out"
	return f.fakePin.Setup()
}

func (f *directionalFakePin) SetHigh() error {
	f.direction = "out"
	return f.fakePin.SetHigh()
}

func (f *directionalFakePin) SetLow() error {
	return f.fakePin.SetLow()
}

func (f *directionalFakePin) Cleanup() {
	f.fakePin.Cleanup()
}

func (f *directionalFakePin) GetDirection() (string, error) {
	return f.direction, nil
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

func TestSetStateDoesNotWriteLowForInputHighUnownedPin(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	fake := &directionalFakePin{
		fakePin: fakePin{
			value:  1,
			lowErr: errors.New("invalid argument"),
		},
		direction: "in",
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 1, Bands: []string{"2.4"}},
	}, func(number int) GPIOPin {
		return fake
	})

	channel, err := svc.SetState("io1", false, "zh-CN")
	if err != nil {
		t.Fatalf("SetState(false) error = %v", err)
	}
	if channel.Enabled || channel.Status != "idle" {
		t.Fatalf("channel = %+v, want idle input-high channel", channel)
	}
	if fake.low != 0 || fake.cleanup != 0 {
		t.Fatalf("pin low/cleanup calls = %d/%d, want untouched", fake.low, fake.cleanup)
	}
}

func TestSetStateTurnsOffOutputHighUnownedPin(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	fake := &directionalFakePin{
		fakePin:   fakePin{value: 1},
		direction: "out",
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io1", Label: "IO1", Pin: 1, Bands: []string{"2.4"}},
	}, func(number int) GPIOPin {
		return fake
	})

	channel, err := svc.SetState("io1", false, "zh-CN")
	if err != nil {
		t.Fatalf("SetState(false) error = %v", err)
	}
	if channel.Enabled || channel.Status != "idle" {
		t.Fatalf("channel = %+v, want idle output-low channel", channel)
	}
	if fake.low != 1 || fake.cleanup != 1 {
		t.Fatalf("pin low/cleanup calls = %d/%d, want 1/1", fake.low, fake.cleanup)
	}
}

func TestChannelsFromBoardPinsMapsDiscoveredPins(t *testing.T) {
	channels := channelsFromBoardPins(board.PinsFromNumbers([]int{0, 1, 2, 3, 4, 5}))

	tests := []struct {
		index    int
		id       string
		label    string
		pin      int
		bands    []string
		reserved bool
	}{
		{index: 0, id: "io1", label: "IO2", pin: 2, bands: []string{"433", "800", "900", "1.4"}},
		{index: 1, id: "io2", label: "IO3", pin: 3, bands: []string{"1.2", "1.5"}},
		{index: 2, id: "io3", label: "IO1", pin: 1, bands: []string{"2.4", "5.2", "5.8"}},
		{index: 3, id: "io4", label: "IO4", pin: 4, bands: []string{}, reserved: true},
		{index: 4, id: "io5", label: "IO0", pin: 0, bands: []string{}, reserved: true},
		{index: 5, id: "io6", label: "IO5", pin: 5, bands: []string{}, reserved: true},
	}

	if len(channels) != len(tests) {
		t.Fatalf("channels len = %d, want %d", len(channels), len(tests))
	}
	for _, tt := range tests {
		channel := channels[tt.index]
		if channel.ID != tt.id || channel.Label != tt.label || channel.Pin != tt.pin || channel.Reserved != tt.reserved {
			t.Fatalf("channels[%d] = %+v, want id=%s label=%s pin=%d reserved=%v", tt.index, channel, tt.id, tt.label, tt.pin, tt.reserved)
		}
		if !reflect.DeepEqual(channel.Bands, tt.bands) {
			t.Fatalf("channels[%d].Bands = %+v, want %+v", tt.index, channel.Bands, tt.bands)
		}
	}
}

func TestChannelsFromBoardPinsAllowsFewerThanThreePins(t *testing.T) {
	channels := channelsFromBoardPins(board.PinsFromNumbers([]int{3, 1}))
	if len(channels) != 2 {
		t.Fatalf("channels len = %d, want 2", len(channels))
	}
	if channels[0].ID != "io2" || channels[0].Pin != 3 || channels[0].Reserved ||
		!reflect.DeepEqual(channels[0].Bands, []string{"1.2", "1.5"}) {
		t.Fatalf("channels[0] = %+v, want active IO3", channels[0])
	}
	if channels[1].ID != "io3" || channels[1].Pin != 1 || channels[1].Reserved ||
		!reflect.DeepEqual(channels[1].Bands, []string{"2.4", "5.2", "5.8"}) {
		t.Fatalf("channels[1] = %+v, want active IO1", channels[1])
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

func TestSetDirectionSwitchControlsFourthGPIO(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}
	fake := &fakePin{}
	var requestedPins []int
	svc := NewService(store.NewMemoryStore(10, 10), tr, []ChannelDefinition{
		{ID: "io4", Label: "IO4", Pin: 4, Reserved: true},
	}, func(number int) GPIOPin {
		requestedPins = append(requestedPins, number)
		return fake
	})

	if err := svc.SetDirectionSwitch(true); err != nil {
		t.Fatalf("SetDirectionSwitch(true) error = %v", err)
	}
	if err := svc.SetDirectionSwitch(false); err != nil {
		t.Fatalf("SetDirectionSwitch(false) error = %v", err)
	}

	for _, pin := range requestedPins {
		if pin != 4 {
			t.Fatalf("requested pins = %v, want only pin 4", requestedPins)
		}
	}
	if fake.setup != 1 || fake.high != 1 || fake.low != 1 || fake.cleanup != 1 {
		t.Fatalf(
			"pin calls = setup:%d high:%d low:%d cleanup:%d, want 1 each",
			fake.setup,
			fake.high,
			fake.low,
			fake.cleanup,
		)
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

func TestScreenStrikeStateLazyDiscoversDefaultChannels(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	restore := stubDiscoverDefaultChannels(t, []ChannelDefinition{
		{ID: "io1", Label: "IO2", Pin: 2, Bands: []string{"433"}},
		{ID: "io2", Label: "IO3", Pin: 3, Bands: []string{"1.2"}},
		{ID: "io3", Label: "IO1", Pin: 1, Bands: []string{"2.4"}},
	})
	defer restore()

	pins := map[int]*fakePin{
		1: {},
		2: {},
		3: {},
	}
	svc := NewService(store.NewMemoryStore(10, 10), tr, nil, func(number int) GPIOPin {
		return pins[number]
	})

	state := svc.ScreenStrikeState()
	if len(state.Channels) != 3 {
		t.Fatalf("screen strike channels len = %d, want 3", len(state.Channels))
	}
	if state.Channels[0].ID != "io1" || state.Channels[0].Label != "IO2" || !reflect.DeepEqual(state.Channels[0].Bands, []string{"433"}) {
		t.Fatalf("first screen strike channel = %+v, want discovered io1", state.Channels[0])
	}
}

func TestListChannelsLazyDiscoversDefaultChannels(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	restore := stubDiscoverDefaultChannels(t, []ChannelDefinition{
		{ID: "io1", Label: "IO2", Pin: 2, Bands: []string{"433"}},
	})
	defer restore()

	svc := NewService(store.NewMemoryStore(10, 10), tr, nil, func(number int) GPIOPin {
		return &fakePin{}
	})

	channels := svc.ListChannels()
	if len(channels) != 1 {
		t.Fatalf("channels len = %d, want 1", len(channels))
	}
	if channels[0].ID != "io1" || channels[0].Pin != 2 {
		t.Fatalf("channel = %+v, want lazily discovered io1 pin 2", channels[0])
	}
}

func TestSetScreenStrikeStartsOnlySelectedChannels(t *testing.T) {
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
	if pins["io2"].low != 0 || pins["io2"].cleanup != 0 {
		t.Fatalf("unselected io2 low/cleanup = %d/%d, want untouched", pins["io2"].low, pins["io2"].cleanup)
	}
}

func TestSetScreenStrikeDoesNotTouchUnselectedChannelWithLowError(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	pins["io3"].lowErr = errors.New("operation not permitted")

	state, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1"},
		DurationSeconds: 60,
	}, "zh-CN")
	if err != nil {
		t.Fatalf("SetScreenStrike() error = %v", err)
	}

	if !state.Active || !reflect.DeepEqual(state.ChannelIDs, []string{"io1"}) {
		t.Fatalf("state = %+v, want active with io1 only", state)
	}
	if pins["io1"].value != 1 {
		t.Fatalf("io1 value = %d, want high", pins["io1"].value)
	}
	if pins["io3"].low != 0 || pins["io3"].cleanup != 0 {
		t.Fatalf("io3 low/cleanup = %d/%d, want untouched", pins["io3"].low, pins["io3"].cleanup)
	}
}

func TestScreenStrikeStateUsesSelectedChannelsWhileStrikeIsActive(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1"},
		DurationSeconds: 60,
	}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike() error = %v", err)
	}

	pins["io3"].value = 1
	state := svc.ScreenStrikeState()
	if !reflect.DeepEqual(state.ChannelIDs, []string{"io1"}) {
		t.Fatalf("channel IDs = %+v, want selected io1 only", state.ChannelIDs)
	}
	if !state.Active || state.RemainingSeconds <= 0 {
		t.Fatalf("state = %+v, want active countdown", state)
	}
	var io3 model.GpioChannel
	for _, channel := range state.Channels {
		if channel.ID == "io3" {
			io3 = channel
			break
		}
	}
	if !io3.Enabled || io3.ActualLevel != "high" {
		t.Fatalf("io3 channel = %+v, want actual high still visible", io3)
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

func TestSetScreenStrikeStopDoesNotTouchUnselectedChannel(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	pins["io3"].lowErr = errors.New("operation not permitted")

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1"},
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
	if pins["io1"].value != 0 || pins["io1"].low != 1 {
		t.Fatalf("io1 value/low = %d/%d, want stopped", pins["io1"].value, pins["io1"].low)
	}
	if pins["io3"].low != 0 || pins["io3"].cleanup != 0 {
		t.Fatalf("unselected io3 low/cleanup = %d/%d, want untouched", pins["io3"].low, pins["io3"].cleanup)
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

func TestScreenStrikeTimeoutDoesNotTouchUnselectedChannel(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	pins["io3"].lowErr = errors.New("operation not permitted")

	if _, err := svc.applyScreenStrike(true, []string{"io1"}, 20*time.Millisecond, 10, "zh-CN"); err != nil {
		t.Fatalf("applyScreenStrike() error = %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		state := svc.ScreenStrikeState()
		if !state.Active && pins["io1"].value == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("strike did not time out, state=%+v io1=%d", state, pins["io1"].value)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pins["io3"].low != 0 || pins["io3"].cleanup != 0 {
		t.Fatalf("unselected io3 low/cleanup = %d/%d, want untouched", pins["io3"].low, pins["io3"].cleanup)
	}
}

func TestSetScreenStrikeCreatesAndCompletesReport(t *testing.T) {
	svc, _ := newStrikeTestService(t)
	defer svc.Shutdown()

	reports := &fakeInterferenceReportStore{}
	svc.SetReportStore(reports)

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1", "io3"},
		DurationSeconds: 60,
	}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike(start) error = %v", err)
	}

	if len(reports.items) != 1 {
		t.Fatalf("reports len = %d, want 1", len(reports.items))
	}
	started := reports.items[0]
	if started.Status != model.InterferenceReportStatusRunning {
		t.Fatalf("started status = %q, want running", started.Status)
	}
	if !reflect.DeepEqual(started.ChannelIDs, []string{"io1", "io3"}) {
		t.Fatalf("started channel IDs = %+v, want io1/io3", started.ChannelIDs)
	}
	if len(started.ChannelLabels) != 2 || started.ChannelLabels[0] != "433M" || started.ChannelLabels[1] != "2.4G" {
		t.Fatalf("started channel labels = %+v, want 433M/2.4G", started.ChannelLabels)
	}

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{Enabled: false}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike(stop) error = %v", err)
	}
	completed := reports.items[0]
	if completed.Status != model.InterferenceReportStatusCompleted {
		t.Fatalf("completed status = %q, want completed", completed.Status)
	}
	if completed.EndedAt == nil || completed.EndState == nil || completed.EndState.Active {
		t.Fatalf("completed report = %+v, want inactive end state with end time", completed)
	}
}

func TestSetScreenStrikeReportUsesConfiguredBandLabels(t *testing.T) {
	svc, _ := newStrikeTestService(t)
	defer svc.Shutdown()

	reports := &fakeInterferenceReportStore{}
	svc.SetReportStore(reports)
	svc.SetUserSettingsStore(fakeUserSettingsStore{
		settings: model.UserSettings{
			ScreenStrikeChannelLabels: []string{"低频段", "中频段", "高频段"},
		},
		ok: true,
	})

	if _, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1", "io3"},
		DurationSeconds: 60,
	}, "zh-CN"); err != nil {
		t.Fatalf("SetScreenStrike(start) error = %v", err)
	}

	if len(reports.items) != 1 {
		t.Fatalf("reports len = %d, want 1", len(reports.items))
	}
	started := reports.items[0]
	if !reflect.DeepEqual(started.ChannelLabels, []string{"低频段", "高频段"}) {
		t.Fatalf("started channel labels = %+v, want configured labels", started.ChannelLabels)
	}
}

func TestSetScreenStrikeCreatesFailedReportOnPinError(t *testing.T) {
	svc, pins := newStrikeTestService(t)
	defer svc.Shutdown()

	reports := &fakeInterferenceReportStore{}
	svc.SetReportStore(reports)
	pins["io2"].highErr = errors.New("gpio high failed")
	pins["io3"].lowErr = errors.New("operation not permitted")

	state, err := svc.SetScreenStrike(model.ScreenStrikeRequest{
		Enabled:         true,
		ChannelIDs:      []string{"io1", "io2"},
		DurationSeconds: 60,
	}, "zh-CN")
	if err == nil {
		t.Fatal("SetScreenStrike() error = nil, want pin error")
	}
	if state.Active {
		t.Fatalf("state active = true after failure: %+v", state)
	}
	if len(reports.items) != 1 {
		t.Fatalf("reports len = %d, want 1", len(reports.items))
	}
	failed := reports.items[0]
	if failed.Status != model.InterferenceReportStatusFailed {
		t.Fatalf("failed status = %q, want failed", failed.Status)
	}
	if failed.LastError == "" || failed.EndedAt == nil {
		t.Fatalf("failed report = %+v, want error and end time", failed)
	}
	if pins["io3"].low != 0 || pins["io3"].cleanup != 0 {
		t.Fatalf("unselected io3 low/cleanup = %d/%d, want untouched", pins["io3"].low, pins["io3"].cleanup)
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

func stubDiscoverDefaultChannels(t *testing.T, definitions []ChannelDefinition) func() {
	t.Helper()

	original := discoverDefaultChannels
	calls := 0
	discoverDefaultChannels = func() []ChannelDefinition {
		calls++
		if calls == 1 {
			return nil
		}
		return cloneChannelDefinitions(definitions)
	}

	return func() {
		discoverDefaultChannels = original
	}
}

func cloneChannelDefinitions(definitions []ChannelDefinition) []ChannelDefinition {
	cloned := make([]ChannelDefinition, len(definitions))
	for index, def := range definitions {
		cloned[index] = def
		cloned[index].Bands = append([]string(nil), def.Bands...)
	}
	return cloned
}

type fakeInterferenceReportStore struct {
	items []model.InterferenceReport
}

type fakeUserSettingsStore struct {
	settings model.UserSettings
	ok       bool
	err      error
}

func (s fakeUserSettingsStore) LoadUser() (model.UserSettings, bool, error) {
	return s.settings, s.ok, s.err
}

func (s *fakeInterferenceReportStore) Create(report model.InterferenceReport) (model.InterferenceReport, error) {
	if report.ID == "" {
		report.ID = "report-" + string(rune('1'+len(s.items)))
	}
	s.items = append(s.items, cloneInterferenceReport(report))
	return report, nil
}

func (s *fakeInterferenceReportStore) CreateRunning(report model.InterferenceReport) (model.InterferenceReport, error) {
	report.Status = model.InterferenceReportStatusRunning
	return s.Create(report)
}

func (s *fakeInterferenceReportStore) Update(report model.InterferenceReport) error {
	for index := range s.items {
		if s.items[index].ID == report.ID {
			s.items[index] = cloneInterferenceReport(report)
			return nil
		}
	}
	return errors.New("report not found")
}
