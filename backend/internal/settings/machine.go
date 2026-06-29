package settings

import (
	"errors"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
)

var errMachineHardwareIDMissing = errors.New("machine hardware ID is missing")

type machineHardwareIDFile struct {
	Name string
	Path string
}

type machineHardwareIDProbe struct {
	HardwareFiles  []machineHardwareIDFile
	MachineIDFiles []machineHardwareIDFile
	ReadFile       func(string) ([]byte, error)
	Interfaces     func() ([]net.Interface, error)
}

// MachineHardwareID 返回用于生成授权 SN 的本机稳定硬件标识。
func MachineHardwareID() (string, error) {
	return machineHardwareID(defaultMachineHardwareIDProbe())
}

func defaultMachineHardwareIDProbe() machineHardwareIDProbe {
	probe := machineHardwareIDProbe{
		ReadFile:   os.ReadFile,
		Interfaces: net.Interfaces,
	}
	if runtime.GOOS == "linux" {
		probe.HardwareFiles = []machineHardwareIDFile{
			{Name: "dmi-product-uuid", Path: "/sys/class/dmi/id/product_uuid"},
			{Name: "dmi-product-serial", Path: "/sys/class/dmi/id/product_serial"},
			{Name: "dmi-board-serial", Path: "/sys/class/dmi/id/board_serial"},
			{Name: "dmi-chassis-serial", Path: "/sys/class/dmi/id/chassis_serial"},
		}
		probe.MachineIDFiles = []machineHardwareIDFile{
			{Name: "machine-id", Path: "/etc/machine-id"},
			{Name: "dbus-machine-id", Path: "/var/lib/dbus/machine-id"},
		}
	}
	return probe
}

func machineHardwareID(probe machineHardwareIDProbe) (string, error) {
	probe = machineHardwareIDProbeWithDefaults(probe)
	if ids := machineHardwareIDFileValues(probe.HardwareFiles, probe.ReadFile); len(ids) > 0 {
		return strings.Join(ids, "/"), nil
	}
	if ids := machineHardwareIDFileValues(probe.MachineIDFiles, probe.ReadFile); len(ids) > 0 {
		return strings.Join(ids, "/"), nil
	}
	if ids := machineNetworkHardwareIDs(probe.Interfaces); len(ids) > 0 {
		return strings.Join(ids, "/"), nil
	}
	return "", errMachineHardwareIDMissing
}

func machineHardwareIDProbeWithDefaults(probe machineHardwareIDProbe) machineHardwareIDProbe {
	if probe.ReadFile == nil {
		probe.ReadFile = os.ReadFile
	}
	if probe.Interfaces == nil {
		probe.Interfaces = net.Interfaces
	}
	return probe
}

func machineHardwareIDFileValues(
	files []machineHardwareIDFile,
	readFile func(string) ([]byte, error),
) []string {
	ids := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		data, err := readFile(file.Path)
		if err != nil {
			continue
		}
		value := normalizeMachineHardwareIDValue(string(data))
		if value == "" {
			continue
		}
		id := file.Name + ":" + value
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func machineNetworkHardwareIDs(interfaces func() ([]net.Interface, error)) []string {
	ifaces, err := interfaces()
	if err != nil {
		return []string{}
	}

	globalIDs := make([]string, 0, len(ifaces))
	localIDs := make([]string, 0, len(ifaces))
	seen := make(map[string]struct{}, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || isVirtualInterfaceName(iface.Name) {
			continue
		}
		mac := normalizeMACAddress(iface.HardwareAddr)
		if mac == "" {
			continue
		}
		id := "mac:" + mac
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if iface.HardwareAddr[0]&0x02 != 0 {
			localIDs = append(localIDs, id)
			continue
		}
		globalIDs = append(globalIDs, id)
	}
	sort.Strings(globalIDs)
	if len(globalIDs) > 0 {
		return globalIDs
	}
	sort.Strings(localIDs)
	return localIDs
}

func normalizeMachineHardwareIDValue(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "\x00"))
	value = strings.Trim(value, `"'`)
	if value == "" {
		return ""
	}

	lower := strings.ToLower(value)
	switch lower {
	case "none", "unknown", "not specified", "to be filled by o.e.m.",
		"default string", "system serial number", "null", "undefined", "n/a", "na":
		return ""
	}
	if isZeroHardwareID(lower) {
		return ""
	}
	return strings.ToUpper(value)
}

func normalizeMACAddress(mac net.HardwareAddr) string {
	if len(mac) < 6 {
		return ""
	}
	value := strings.ToUpper(strings.ReplaceAll(mac.String(), ":", ""))
	if isZeroHardwareID(value) {
		return ""
	}
	return value
}

func isZeroHardwareID(value string) bool {
	hasDigit := false
	for _, r := range value {
		if r == '-' || r == ':' || r == ' ' {
			continue
		}
		if r != '0' {
			return false
		}
		hasDigit = true
	}
	return hasDigit
}

func isVirtualInterfaceName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range []string{
		"awdl",
		"br-",
		"bridge",
		"docker",
		"gif",
		"llw",
		"lo",
		"stf",
		"utun",
		"veth",
		"vmnet",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
