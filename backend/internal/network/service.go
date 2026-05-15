// Package network reads and applies Linux network interface settings.
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"dr600ab-api/internal/model"
)

const (
	backendName    = "networkmanager"
	commandTimeout = 12 * time.Second
)

var (
	// ErrBackendUnavailable 表示当前系统没有可用的 NetworkManager 命令。
	ErrBackendUnavailable = errors.New("network backend unavailable")
	// ErrWiFiUnavailable 表示当前系统没有可用无线设备。
	ErrWiFiUnavailable = errors.New("wifi unavailable")
	// ErrInterfaceNotFound 表示指定网口不存在。
	ErrInterfaceNotFound = errors.New("network interface not found")
	// ErrInterfaceUnmanaged 表示指定网口没有可配置连接。
	ErrInterfaceUnmanaged = errors.New("network interface is unmanaged")
	// ErrInvalidConfig 表示请求的网络配置不合法。
	ErrInvalidConfig = errors.New("invalid network configuration")
	// ErrInvalidWiFiConfig 表示无线网络配置不合法。
	ErrInvalidWiFiConfig = errors.New("invalid wifi configuration")
)

// CommandRunner 执行受控系统命令，便于测试替换。
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner 使用 os/exec 运行命令。
type ExecRunner struct{}

// Run 执行命令并返回合并输出。
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Service 管理系统网口配置。
type Service struct {
	runner CommandRunner
}

// NewService 创建网口配置服务。
func NewService(runner CommandRunner) *Service {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Service{runner: runner}
}

// ListInterfaces 返回 NetworkManager 管理到的设备和当前地址信息。
func (s *Service) ListInterfaces(ctx context.Context) ([]model.NetworkInterface, error) {
	if err := s.checkBackend(ctx); err != nil {
		return nil, err
	}

	deviceRows, err := s.nmcli(ctx, "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device", "status")
	if err != nil {
		return nil, err
	}
	detailRows, err := s.nmcli(ctx, "-t", "-f", strings.Join([]string{
		"GENERAL.DEVICE",
		"GENERAL.HWADDR",
		"GENERAL.MTU",
		"IP4.ADDRESS",
		"IP4.GATEWAY",
		"IP4.DNS",
		"IP6.ADDRESS",
		"IP6.GATEWAY",
		"IP6.DNS",
	}, ","), "device", "show")
	if err != nil {
		return nil, err
	}

	interfaces := parseDeviceStatus(deviceRows)
	applyDeviceDetails(interfaces, detailRows)

	result := make([]model.NetworkInterface, 0, len(interfaces))
	for _, item := range interfaces {
		if item.Name == "" || item.Type == "loopback" {
			continue
		}
		if item.ConnectionName != "" && item.ConnectionName != "--" {
			if method, err := s.connectionIPv4Method(ctx, item.ConnectionName); err == nil {
				item.IPv4Method = method
			}
		}
		if item.IPv4Method == "" {
			item.IPv4Method = "unknown"
		}
		item.Managed = item.ConnectionName != "" && item.ConnectionName != "--"
		result = append(result, item)
	}

	return result, nil
}

// UpdateInterface 更新指定网口 IPv4 配置，并重新激活连接。
func (s *Service) UpdateInterface(
	ctx context.Context,
	name string,
	req model.NetworkInterfaceUpdateRequest,
) (model.NetworkInterface, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.NetworkInterface{}, fmt.Errorf("%w: interface name is required", ErrInvalidConfig)
	}
	if err := s.checkBackend(ctx); err != nil {
		return model.NetworkInterface{}, err
	}
	if err := validateInterfaceName(name); err != nil {
		return model.NetworkInterface{}, err
	}
	if err := validateUpdateRequest(req); err != nil {
		return model.NetworkInterface{}, err
	}

	items, err := s.ListInterfaces(ctx)
	if err != nil {
		return model.NetworkInterface{}, err
	}
	var target model.NetworkInterface
	for _, item := range items {
		if item.Name == name {
			target = item
			break
		}
	}
	if target.Name == "" {
		return model.NetworkInterface{}, ErrInterfaceNotFound
	}
	if !target.Managed || target.ConnectionName == "" || target.ConnectionName == "--" {
		return model.NetworkInterface{}, ErrInterfaceUnmanaged
	}

	if req.Mode == "dhcp" {
		if _, err := s.nmcli(ctx, "connection", "modify", target.ConnectionName, "ipv4.method", "auto", "ipv4.addresses", "", "ipv4.gateway", "", "ipv4.dns", ""); err != nil {
			return model.NetworkInterface{}, err
		}
	} else {
		address := fmt.Sprintf("%s/%d", strings.TrimSpace(req.IPv4Address), req.Prefix)
		args := []string{
			"connection", "modify", target.ConnectionName,
			"ipv4.method", "manual",
			"ipv4.addresses", address,
			"ipv4.gateway", strings.TrimSpace(req.Gateway4),
			"ipv4.dns", strings.Join(trimStrings(req.DNS4), ","),
		}
		if _, err := s.nmcli(ctx, args...); err != nil {
			return model.NetworkInterface{}, err
		}
	}

	if _, err := s.nmcli(ctx, "connection", "up", target.ConnectionName); err != nil {
		return model.NetworkInterface{}, err
	}

	updated, err := s.ListInterfaces(ctx)
	if err != nil {
		return model.NetworkInterface{}, err
	}
	for _, item := range updated {
		if item.Name == name {
			return item, nil
		}
	}
	return model.NetworkInterface{}, ErrInterfaceNotFound
}

// ScanWiFi 扫描附近无线网络。
func (s *Service) ScanWiFi(ctx context.Context) ([]model.WiFiNetwork, error) {
	if err := s.checkBackend(ctx); err != nil {
		return nil, err
	}

	if err := s.ensureWiFiDevice(ctx); err != nil {
		return nil, err
	}

	_, _ = s.nmcli(ctx, "device", "wifi", "rescan")
	output, err := s.nmcli(ctx, "-t", "-f", "IN-USE,BSSID,SSID,MODE,CHAN,RATE,SIGNAL,SECURITY,DEVICE", "device", "wifi", "list")
	if err != nil {
		return nil, err
	}

	return parseWiFiList(output), nil
}

// ConnectWiFi 连接指定无线网络。
func (s *Service) ConnectWiFi(ctx context.Context, req model.WiFiConnectRequest) error {
	if err := s.checkBackend(ctx); err != nil {
		return err
	}
	if err := validateWiFiRequest(req); err != nil {
		return err
	}
	if err := s.ensureWiFiDevice(ctx); err != nil {
		return err
	}

	args := []string{"device", "wifi", "connect", strings.TrimSpace(req.SSID)}
	if strings.TrimSpace(req.Password) != "" {
		args = append(args, "password", strings.TrimSpace(req.Password))
	}
	if strings.TrimSpace(req.Device) != "" {
		if err := validateInterfaceName(strings.TrimSpace(req.Device)); err != nil {
			return err
		}
		args = append(args, "ifname", strings.TrimSpace(req.Device))
	}

	_, err := s.nmcli(ctx, args...)
	return err
}

func (s *Service) checkBackend(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return ErrBackendUnavailable
	}
	if _, err := exec.LookPath("nmcli"); err != nil {
		return ErrBackendUnavailable
	}
	_, err := s.nmcli(ctx, "-v")
	return err
}

func (s *Service) ensureWiFiDevice(ctx context.Context) error {
	output, err := s.nmcli(ctx, "-t", "-f", "DEVICE,TYPE", "device", "status")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(output, "\n") {
		parts := splitNMCLIFields(line, 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == "wifi" {
			return nil
		}
	}
	return ErrWiFiUnavailable
}

func (s *Service) connectionIPv4Method(ctx context.Context, connectionName string) (string, error) {
	output, err := s.nmcli(ctx, "-g", "ipv4.method", "connection", "show", connectionName)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (s *Service) nmcli(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	output, err := s.runner.Run(ctx, "nmcli", args...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s: %w", detail, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func parseDeviceStatus(output string) map[string]model.NetworkInterface {
	result := map[string]model.NetworkInterface{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := splitNMCLIFields(line, 4)
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		result[name] = model.NetworkInterface{
			Name:           name,
			Type:           strings.TrimSpace(parts[1]),
			State:          strings.TrimSpace(parts[2]),
			ConnectionName: strings.TrimSpace(parts[3]),
			IPv4:           []model.NetworkAddress{},
			IPv6:           []model.NetworkAddress{},
			DNS4:           []string{},
			DNS6:           []string{},
		}
	}
	return result
}

func parseWiFiList(output string) []model.WiFiNetwork {
	seen := map[string]model.WiFiNetwork{}
	order := []string{}
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := splitNMCLIFields(line, 9)
		ssid := strings.TrimSpace(parts[2])
		if ssid == "" {
			continue
		}
		signal, _ := strconv.Atoi(strings.TrimSpace(parts[6]))
		item := model.WiFiNetwork{
			Active:   strings.TrimSpace(parts[0]) == "*",
			BSSID:    strings.TrimSpace(parts[1]),
			SSID:     ssid,
			Mode:     strings.TrimSpace(parts[3]),
			Channel:  strings.TrimSpace(parts[4]),
			Rate:     strings.TrimSpace(parts[5]),
			Signal:   signal,
			Security: strings.TrimSpace(parts[7]),
			Device:   strings.TrimSpace(parts[8]),
		}
		previous, ok := seen[ssid]
		if !ok {
			seen[ssid] = item
			order = append(order, ssid)
			continue
		}
		if item.Active || (!previous.Active && item.Signal > previous.Signal) {
			seen[ssid] = item
		}
	}

	result := make([]model.WiFiNetwork, 0, len(order))
	for _, ssid := range order {
		result = append(result, seen[ssid])
	}
	return result
}

func applyDeviceDetails(interfaces map[string]model.NetworkInterface, output string) {
	currentName := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = unescapeNMCLI(strings.TrimSpace(value))
		if key == "GENERAL.DEVICE" {
			currentName = value
			continue
		}
		item, ok := interfaces[currentName]
		if !ok {
			continue
		}
		switch key {
		case "GENERAL.HWADDR":
			item.HardwareAddress = value
		case "GENERAL.MTU":
			if mtu, err := strconv.Atoi(value); err == nil {
				item.MTU = mtu
			}
		case "IP4.ADDRESS":
			item.IPv4 = append(item.IPv4, parseNetworkAddress(value))
		case "IP4.GATEWAY":
			item.Gateway4 = value
		case "IP4.DNS":
			item.DNS4 = append(item.DNS4, value)
		case "IP6.ADDRESS":
			item.IPv6 = append(item.IPv6, parseNetworkAddress(value))
		case "IP6.GATEWAY":
			item.Gateway6 = value
		case "IP6.DNS":
			item.DNS6 = append(item.DNS6, value)
		}
		interfaces[currentName] = item
	}
}

func parseNetworkAddress(value string) model.NetworkAddress {
	address, rawPrefix, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok {
		return model.NetworkAddress{Address: strings.TrimSpace(value)}
	}
	prefix, _ := strconv.Atoi(rawPrefix)
	return model.NetworkAddress{Address: address, Prefix: prefix}
}

func validateInterfaceName(name string) error {
	if _, err := net.InterfaceByName(name); err == nil {
		return nil
	}
	for _, r := range name {
		isValid := r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if !isValid {
			return fmt.Errorf("%w: invalid interface name", ErrInvalidConfig)
		}
	}
	return nil
}

func validateUpdateRequest(req model.NetworkInterfaceUpdateRequest) error {
	switch req.Mode {
	case "dhcp":
		return nil
	case "static":
		ip := net.ParseIP(strings.TrimSpace(req.IPv4Address))
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("%w: invalid ipv4 address", ErrInvalidConfig)
		}
		if req.Prefix < 1 || req.Prefix > 32 {
			return fmt.Errorf("%w: invalid ipv4 prefix", ErrInvalidConfig)
		}
		if strings.TrimSpace(req.Gateway4) != "" {
			gateway := net.ParseIP(strings.TrimSpace(req.Gateway4))
			if gateway == nil || gateway.To4() == nil {
				return fmt.Errorf("%w: invalid ipv4 gateway", ErrInvalidConfig)
			}
		}
		for _, dns := range trimStrings(req.DNS4) {
			ip := net.ParseIP(dns)
			if ip == nil || ip.To4() == nil {
				return fmt.Errorf("%w: invalid dns server", ErrInvalidConfig)
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: invalid ipv4 mode", ErrInvalidConfig)
	}
}

func validateWiFiRequest(req model.WiFiConnectRequest) error {
	ssid := strings.TrimSpace(req.SSID)
	if ssid == "" || len(ssid) > 32 {
		return fmt.Errorf("%w: invalid ssid", ErrInvalidWiFiConfig)
	}
	if len(strings.TrimSpace(req.Password)) > 64 {
		return fmt.Errorf("%w: invalid password", ErrInvalidWiFiConfig)
	}
	return nil
}

func unescapeNMCLI(value string) string {
	replacer := strings.NewReplacer(`\:`, ":", `\\`, `\`)
	return replacer.Replace(value)
}

func splitNMCLIFields(line string, minFields int) []string {
	fields := []string{}
	var current strings.Builder
	escaped := false

	for _, r := range line {
		if escaped {
			switch r {
			case ':', '\\':
				current.WriteRune(r)
			default:
				current.WriteRune('\\')
				current.WriteRune(r)
			}
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case ':':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	fields = append(fields, current.String())

	for len(fields) < minFields {
		fields = append(fields, "")
	}
	return fields
}

func trimStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
