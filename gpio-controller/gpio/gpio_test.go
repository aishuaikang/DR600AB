package gpio

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSetupSetsExternalPinToOutput(t *testing.T) {
	root := configureTestExternalGPIO(t)
	createExternalPin(t, root, 0, "0", "0")

	pin := NewPin(0)
	if err := pin.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	directionData, err := os.ReadFile(filepath.Join(root, "jwsioc_inout_gpio0"))
	if err != nil {
		t.Fatalf("reading direction file: %v", err)
	}
	if got := strings.TrimSpace(string(directionData)); got != "1" {
		t.Fatalf("direction = %q, want %q", got, "1")
	}
}

func TestSetValueAndGetValueUseExternalValueFile(t *testing.T) {
	root := configureTestExternalGPIO(t)
	createExternalPin(t, root, 1, "1", "0")

	pin := NewPin(1)
	if err := pin.SetHigh(); err != nil {
		t.Fatalf("SetHigh() error = %v", err)
	}
	value, err := pin.GetValue()
	if err != nil {
		t.Fatalf("GetValue() error = %v", err)
	}
	if value != 1 {
		t.Fatalf("GetValue() = %d, want 1", value)
	}

	valueData, err := os.ReadFile(filepath.Join(root, "jwsioc_gpio1"))
	if err != nil {
		t.Fatalf("reading value file: %v", err)
	}
	if got := strings.TrimSpace(string(valueData)); got != "1" {
		t.Fatalf("value file = %q, want %q", got, "1")
	}
}

func TestCleanupSetsLowAndKeepsOutput(t *testing.T) {
	root := configureTestExternalGPIO(t)
	createExternalPin(t, root, 2, "1", "1")

	pin := NewPin(2)
	pin.Cleanup()

	valueData, err := os.ReadFile(filepath.Join(root, "jwsioc_gpio2"))
	if err != nil {
		t.Fatalf("reading value file: %v", err)
	}
	if got := strings.TrimSpace(string(valueData)); got != "0" {
		t.Fatalf("value file = %q, want low", got)
	}

	directionData, err := os.ReadFile(filepath.Join(root, "jwsioc_inout_gpio2"))
	if err != nil {
		t.Fatalf("reading direction file: %v", err)
	}
	if got := strings.TrimSpace(string(directionData)); got != "1" {
		t.Fatalf("direction file = %q, want output", got)
	}
}

func TestListExternalPinsFiltersPairsAndSorts(t *testing.T) {
	root := configureTestExternalGPIO(t)
	createExternalPin(t, root, 4, "1", "0")
	createExternalPin(t, root, 0, "1", "0")
	createExternalPin(t, root, 2, "1", "0")

	if err := os.WriteFile(filepath.Join(root, "jwsioc_gpio5"), []byte("0"), 0o644); err != nil {
		t.Fatalf("writing orphan value file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "jwsioc_inout_gpio6"), []byte("1"), 0o644); err != nil {
		t.Fatalf("writing orphan direction file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "jwsioc_gpiox"), []byte("0"), 0o644); err != nil {
		t.Fatalf("writing invalid file: %v", err)
	}

	got := ListExternalPins()
	want := []int{0, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListExternalPins() = %v, want %v", got, want)
	}
	if alias := ListExportedPins(); !reflect.DeepEqual(alias, want) {
		t.Fatalf("ListExportedPins() = %v, want %v", alias, want)
	}
}

func TestSetupReturnsErrorWhenExternalFileMissing(t *testing.T) {
	root := configureTestExternalGPIO(t)
	if err := os.WriteFile(filepath.Join(root, "jwsioc_gpio0"), []byte("0"), 0o644); err != nil {
		t.Fatalf("writing value file: %v", err)
	}

	err := NewPin(0).Setup()
	if err == nil {
		t.Fatal("Setup() error = nil, want missing direction file error")
	}
	if !strings.Contains(err.Error(), "方向文件不可用") {
		t.Fatalf("Setup() error = %q, want direction file message", err)
	}
}

func TestDirectionRoundTrip(t *testing.T) {
	root := configureTestExternalGPIO(t)
	createExternalPin(t, root, 3, "0", "0")

	pin := NewPin(3)
	if got, err := pin.GetDirection(); err != nil || got != directionIn {
		t.Fatalf("GetDirection() = %q, %v; want %q, nil", got, err, directionIn)
	}
	if err := pin.SetDirection(directionOut); err != nil {
		t.Fatalf("SetDirection(out) error = %v", err)
	}
	if got, err := pin.GetDirection(); err != nil || got != directionOut {
		t.Fatalf("GetDirection() = %q, %v; want %q, nil", got, err, directionOut)
	}
}

func createExternalPin(t *testing.T, root string, number int, direction string, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "jwsioc_gpio"+strconv.Itoa(number)), []byte(value), 0o644); err != nil {
		t.Fatalf("writing value file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "jwsioc_inout_gpio"+strconv.Itoa(number)), []byte(direction), 0o644); err != nil {
		t.Fatalf("writing direction file: %v", err)
	}
}

func configureTestExternalGPIO(t *testing.T) string {
	t.Helper()

	oldRoot := sysfsRoot
	oldWriteRetries := writeRetryCount
	oldWriteDelay := writeRetryDelay
	oldStatFile := statFile
	oldReadFile := readFile
	oldWriteFile := writeFile
	oldReadDir := readDir
	oldSleep := sleep

	root := t.TempDir()
	sysfsRoot = root
	writeRetryCount = 3
	writeRetryDelay = time.Millisecond
	statFile = os.Stat
	readFile = os.ReadFile
	writeFile = os.WriteFile
	readDir = os.ReadDir
	sleep = time.Sleep

	t.Cleanup(func() {
		sysfsRoot = oldRoot
		writeRetryCount = oldWriteRetries
		writeRetryDelay = oldWriteDelay
		statFile = oldStatFile
		readFile = oldReadFile
		writeFile = oldWriteFile
		readDir = oldReadDir
		sleep = oldSleep
	})

	return root
}
