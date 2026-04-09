package gpio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSetupExportsPinAndSetsDirection(t *testing.T) {
	root := configureTestSysfs(t)
	pin := NewPin(23)

	go func() {
		time.Sleep(2 * time.Millisecond)
		pinDir := filepath.Join(root, "gpio23")
		_ = os.MkdirAll(pinDir, 0o755)
		_ = os.WriteFile(filepath.Join(pinDir, "direction"), []byte("in"), 0o644)
		_ = os.WriteFile(filepath.Join(pinDir, "value"), []byte("0"), 0o644)
	}()

	if err := pin.Setup(); err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	exportPath := filepath.Join(root, "export")
	if err := os.Chmod(exportPath, 0o644); err != nil {
		t.Fatalf("chmod export file: %v", err)
	}
	exportData, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("reading export file: %v", err)
	}
	if got := strings.TrimSpace(string(exportData)); got != "23" {
		t.Fatalf("export file = %q, want %q", got, "23")
	}

	directionData, err := os.ReadFile(filepath.Join(root, "gpio23", "direction"))
	if err != nil {
		t.Fatalf("reading direction file: %v", err)
	}
	if got := strings.TrimSpace(string(directionData)); got != directionOut {
		t.Fatalf("direction = %q, want %q", got, directionOut)
	}
}

func TestExportReturnsTimeoutWhenValueNeverAppears(t *testing.T) {
	_ = configureTestSysfs(t)
	exportReadyRetries = 2
	exportReadyDelay = time.Millisecond

	pin := NewPin(24)
	err := pin.Export()
	if err == nil {
		t.Fatal("Export() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "等待 GPIO24/value 就绪超时") {
		t.Fatalf("Export() error = %q, want timeout message", err)
	}
}

func TestExportReturnsBusyErrorWhenPinDirectoryAlreadyExists(t *testing.T) {
	root := configureTestSysfs(t)
	pin := NewPin(25)

	if err := os.MkdirAll(filepath.Join(root, "gpio25"), 0o755); err != nil {
		t.Fatalf("creating gpio25 dir: %v", err)
	}

	oldWriteFile := writeFile
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if filepath.Base(path) == "export" {
			return syscall.EBUSY
		}
		return oldWriteFile(path, data, perm)
	}
	t.Cleanup(func() {
		writeFile = oldWriteFile
	})

	err := pin.Export()
	if err == nil {
		t.Fatal("Export() error = nil, want busy error")
	}
	if !errors.Is(err, syscall.EBUSY) && !strings.Contains(err.Error(), "已被其他进程导出或被内核占用") {
		t.Fatalf("Export() error = %q, want busy message", err)
	}
}

func TestExportReturnsBusyErrorWhenConcurrentExportCreatesPinDirectory(t *testing.T) {
	root := configureTestSysfs(t)
	pin := NewPin(26)

	oldWriteFile := writeFile
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if filepath.Base(path) == "export" {
			if err := os.MkdirAll(filepath.Join(root, "gpio26"), 0o755); err != nil {
				t.Fatalf("creating gpio26 dir: %v", err)
			}
			return syscall.EBUSY
		}
		return oldWriteFile(path, data, perm)
	}
	t.Cleanup(func() {
		writeFile = oldWriteFile
	})

	err := pin.Export()
	if err == nil {
		t.Fatal("Export() error = nil, want busy error")
	}
	if !strings.Contains(err.Error(), "已被其他进程导出或被内核占用") {
		t.Fatalf("Export() error = %q, want busy message", err)
	}
}

func TestListExportedPinsFiltersAndSorts(t *testing.T) {
	root := configureTestSysfs(t)

	for _, name := range []string{"gpio9", "gpio2", "gpiochip0", "gpiox", "export"} {
		path := filepath.Join(root, name)
		if strings.HasPrefix(name, "gpio") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("creating %s: %v", name, err)
			}
			continue
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}

	got := ListExportedPins()
	want := []int{2, 9}
	if len(got) != len(want) {
		t.Fatalf("ListExportedPins() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListExportedPins()[%d] = %d, want %d (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestListGPIOChipsReadsMetadataAndSortsByBase(t *testing.T) {
	root := configureTestSysfs(t)

	createChip := func(name, label, base, ngpio string) {
		chipDir := filepath.Join(root, name)
		if err := os.MkdirAll(chipDir, 0o755); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(chipDir, "label"), []byte(label), 0o644); err != nil {
			t.Fatalf("writing label for %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(chipDir, "base"), []byte(base), 0o644); err != nil {
			t.Fatalf("writing base for %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(chipDir, "ngpio"), []byte(ngpio), 0o644); err != nil {
			t.Fatalf("writing ngpio for %s: %v", name, err)
		}
	}

	createChip("gpiochip32", "bank-b", "32", "16")
	createChip("gpiochip0", "bank-a", "0", "32")
	if err := os.MkdirAll(filepath.Join(root, "gpio17"), 0o755); err != nil {
		t.Fatalf("creating gpio17: %v", err)
	}

	got := ListGPIOChips()
	if len(got) != 2 {
		t.Fatalf("ListGPIOChips() len = %d, want 2 (%v)", len(got), got)
	}

	if got[0].Name != "gpiochip0" || got[0].Label != "bank-a" || got[0].Base != 0 || got[0].Ngpio != 32 {
		t.Fatalf("ListGPIOChips()[0] = %+v, want gpiochip0/bank-a/base0/ngpio32", got[0])
	}
	if got[1].Name != "gpiochip32" || got[1].Label != "bank-b" || got[1].Base != 32 || got[1].Ngpio != 16 {
		t.Fatalf("ListGPIOChips()[1] = %+v, want gpiochip32/bank-b/base32/ngpio16", got[1])
	}
}

func configureTestSysfs(t *testing.T) string {
	t.Helper()

	oldRoot := sysfsRoot
	oldExportRetries := exportReadyRetries
	oldExportDelay := exportReadyDelay
	oldWriteRetries := writeRetryCount
	oldWriteDelay := writeRetryDelay
	oldStatFile := statFile
	oldReadFile := readFile
	oldWriteFile := writeFile
	oldReadDir := readDir
	oldSleep := sleep

	root := t.TempDir()
	sysfsRoot = root
	exportReadyRetries = 5
	exportReadyDelay = time.Millisecond
	writeRetryCount = 3
	writeRetryDelay = time.Millisecond
	statFile = os.Stat
	readFile = os.ReadFile
	writeFile = os.WriteFile
	readDir = os.ReadDir
	sleep = time.Sleep

	t.Cleanup(func() {
		sysfsRoot = oldRoot
		exportReadyRetries = oldExportRetries
		exportReadyDelay = oldExportDelay
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
