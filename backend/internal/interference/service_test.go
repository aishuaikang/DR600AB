package interference

import (
	"testing"

	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/store"
)

type fakePin struct {
	setup   int
	high    int
	low     int
	cleanup int
}

func (f *fakePin) Setup() error {
	f.setup++
	return nil
}

func (f *fakePin) SetHigh() error {
	f.high++
	return nil
}

func (f *fakePin) SetLow() error {
	f.low++
	return nil
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
	svc := NewService(store.NewMemoryStore(10, 10, 10), tr, []ChannelDefinition{
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

	svc.Shutdown()
	if fake.cleanup != 1 {
		t.Fatalf("pin cleanup calls = %d, want 1", fake.cleanup)
	}
}

func TestReservedChannelRejectsStateChanges(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	svc := NewService(store.NewMemoryStore(10, 10, 10), tr, []ChannelDefinition{
		{ID: "io4", Label: "IO4", Pin: 62, Reserved: true},
	}, nil)

	_, err = svc.SetState("io4", true, "zh-CN")
	if err == nil {
		t.Fatal("expected reserved channel error")
	}
}

func TestListChannelsReturnsEmptyBandsForReservedChannel(t *testing.T) {
	tr, err := i18n.New("zh-CN")
	if err != nil {
		t.Fatalf("i18n.New() error = %v", err)
	}

	svc := NewService(store.NewMemoryStore(10, 10, 10), tr, []ChannelDefinition{
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
