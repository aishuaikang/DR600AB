package settings

import (
	"errors"
	"net"
	"testing"
)

func TestMachineHardwareIDUsesHardwareFilesFirst(t *testing.T) {
	probe := machineHardwareIDProbe{
		HardwareFiles: []machineHardwareIDFile{
			{Name: "dmi-product-uuid", Path: "/product_uuid"},
			{Name: "dmi-board-serial", Path: "/board_serial"},
		},
		MachineIDFiles: []machineHardwareIDFile{
			{Name: "machine-id", Path: "/etc/machine-id"},
		},
		ReadFile: func(path string) ([]byte, error) {
			switch path {
			case "/product_uuid":
				return []byte("00000000-0000-0000-0000-000000000000\n"), nil
			case "/board_serial":
				return []byte("board-123\n"), nil
			case "/etc/machine-id":
				return []byte("machine-456\n"), nil
			default:
				return nil, errors.New("missing file")
			}
		},
		Interfaces: func() ([]net.Interface, error) {
			t.Fatal("interfaces should not be queried when hardware files are available")
			return nil, nil
		},
	}

	got, err := machineHardwareID(probe)
	if err != nil {
		t.Fatalf("machineHardwareID() error = %v", err)
	}
	if got != "dmi-board-serial:BOARD-123" {
		t.Fatalf("machineHardwareID() = %q, want dmi-board-serial:BOARD-123", got)
	}
}

func TestMachineHardwareIDFallsBackToMachineID(t *testing.T) {
	probe := machineHardwareIDProbe{
		HardwareFiles: []machineHardwareIDFile{
			{Name: "dmi-product-uuid", Path: "/product_uuid"},
		},
		MachineIDFiles: []machineHardwareIDFile{
			{Name: "machine-id", Path: "/etc/machine-id"},
		},
		ReadFile: func(path string) ([]byte, error) {
			switch path {
			case "/product_uuid":
				return []byte("To Be Filled By O.E.M.\n"), nil
			case "/etc/machine-id":
				return []byte("machine-456\n"), nil
			default:
				return nil, errors.New("missing file")
			}
		},
		Interfaces: func() ([]net.Interface, error) {
			t.Fatal("interfaces should not be queried when machine-id is available")
			return nil, nil
		},
	}

	got, err := machineHardwareID(probe)
	if err != nil {
		t.Fatalf("machineHardwareID() error = %v", err)
	}
	if got != "machine-id:MACHINE-456" {
		t.Fatalf("machineHardwareID() = %q, want machine-id:MACHINE-456", got)
	}
}

func TestMachineHardwareIDFallsBackToNetworkInterfaces(t *testing.T) {
	probe := machineHardwareIDProbe{
		ReadFile: func(string) ([]byte, error) {
			return nil, errors.New("missing file")
		},
		Interfaces: func() ([]net.Interface, error) {
			return []net.Interface{
				{
					Name:         "lo0",
					Flags:        net.FlagLoopback,
					HardwareAddr: net.HardwareAddr{0, 0, 0, 0, 0, 0},
				},
				{
					Name:         "en1",
					HardwareAddr: net.HardwareAddr{0xAE, 0xDE, 0x48, 0x00, 0x00, 0x02},
				},
				{
					Name:         "en0",
					HardwareAddr: net.HardwareAddr{0x00, 0x1A, 0x2B, 0x3C, 0x4D, 0x5E},
				},
			}, nil
		},
	}

	got, err := machineHardwareID(probe)
	if err != nil {
		t.Fatalf("machineHardwareID() error = %v", err)
	}
	if got != "mac:001A2B3C4D5E" {
		t.Fatalf("machineHardwareID() = %q, want mac:001A2B3C4D5E", got)
	}
}

func TestMachineHardwareIDReturnsErrorWhenNoIdentityExists(t *testing.T) {
	probe := machineHardwareIDProbe{
		ReadFile: func(string) ([]byte, error) {
			return nil, errors.New("missing file")
		},
		Interfaces: func() ([]net.Interface, error) {
			return []net.Interface{}, nil
		},
	}

	if got, err := machineHardwareID(probe); !errors.Is(err, errMachineHardwareIDMissing) || got != "" {
		t.Fatalf("machineHardwareID() = %q, %v; want missing identity error", got, err)
	}
}
