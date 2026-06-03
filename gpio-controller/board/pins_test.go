package board

import (
	"reflect"
	"testing"
)

func TestPinsFromNumbersMapsConfiguredPinsToInterferenceBands(t *testing.T) {
	got := PinsFromNumbers([]int{4, 0, 2, 1, 3, 5, 0})

	want := []PinDefinition{
		{ID: "io1", Label: "IO2", Number: 2, Bands: []string{"433", "800", "900", "1.4"}},
		{ID: "io2", Label: "IO3", Number: 3, Bands: []string{"1.2", "1.5"}},
		{ID: "io3", Label: "IO1", Number: 1, Bands: []string{"2.4", "5.2", "5.8"}},
		{ID: "io4", Label: "IO4", Number: 4, Bands: []string{}, Reserved: true},
		{ID: "io5", Label: "IO0", Number: 0, Bands: []string{}, Reserved: true},
		{ID: "io6", Label: "IO5", Number: 5, Bands: []string{}, Reserved: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PinsFromNumbers() = %#v, want %#v", got, want)
	}
}

func TestPinsFromNumbersAllowsFewerThanThreePins(t *testing.T) {
	got := PinsFromNumbers([]int{3, 1})

	want := []PinDefinition{
		{ID: "io2", Label: "IO3", Number: 3, Bands: []string{"1.2", "1.5"}},
		{ID: "io3", Label: "IO1", Number: 1, Bands: []string{"2.4", "5.2", "5.8"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PinsFromNumbers() = %#v, want %#v", got, want)
	}
}

func TestPinsFromNumbersAppendsUnknownPinsAsSortedReserved(t *testing.T) {
	got := PinsFromNumbers([]int{7, 2, 6})

	want := []PinDefinition{
		{ID: "io1", Label: "IO2", Number: 2, Bands: []string{"433", "800", "900", "1.4"}},
		{ID: "io7", Label: "IO6", Number: 6, Bands: []string{}, Reserved: true},
		{ID: "io8", Label: "IO7", Number: 7, Bands: []string{}, Reserved: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PinsFromNumbers() = %#v, want %#v", got, want)
	}
}
