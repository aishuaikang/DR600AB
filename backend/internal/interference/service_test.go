package interference

import (
	"reflect"
	"testing"

	"dr600ab-api/internal/i18n"
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
