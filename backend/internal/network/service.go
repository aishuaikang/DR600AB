// Package network reads and applies Linux network interface settings.
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
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
	// ErrCellularUnavailable 表示当前系统没有可用移动网络模块。
	ErrCellularUnavailable = errors.New("cellular modem unavailable")
	// ErrInvalidCellularConfig 表示移动网络配置不合法。
	ErrInvalidCellularConfig = errors.New("invalid cellular configuration")
)

// CommandRunner 执行受控系统命令，便于测试替换。
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// SettingsStore 持久化需要在重启后重新应用的网络偏好。
type SettingsStore interface {
	LoadNetwork() (model.NetworkSettings, bool, error)
	SaveNetwork(model.NetworkSettings) error
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
	runner   CommandRunner
	settings SettingsStore
}

// NewService 创建网口配置服务。
func NewService(runner CommandRunner, settingsStore SettingsStore) *Service {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Service{
		runner:   runner,
		settings: settingsStore,
	}
}

// ListInterfaces 返回系统网络接口状态，并合并 NetworkManager、内核网卡和 ModemManager 信息。
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
	if err := applyKernelInterfaces(interfaces); err != nil {
		return nil, err
	}
	modems, _ := s.listCellularModems(ctx)
	applyCellularModems(interfaces, modems)

	result := make([]model.NetworkInterface, 0, len(interfaces))
	for _, item := range interfaces {
		if item.Name == "" || item.Type == "loopback" {
			continue
		}
		if item.ConnectionName != "" && item.ConnectionName != "--" {
			if method, err := s.connectionIPv4Method(ctx, item.ConnectionName); err == nil {
				item.IPv4Method = method
			}
			if routeMetric, err := s.connectionRouteMetric(ctx, item.ConnectionName); err == nil {
				item.RouteMetric = routeMetric
			}
		}
		if item.IPv4Method == "" {
			item.IPv4Method = "unknown"
		}
		item.Managed = item.ConnectionName != "" && item.ConnectionName != "--" && !item.ReadOnly
		item.Capabilities = interfaceCapabilities(item)
		result = append(result, item)
	}
	sortNetworkInterfaces(result)

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
		args := []string{
			"connection", "modify", target.ConnectionName,
			"ipv4.method", "auto",
			"ipv4.addresses", "",
			"ipv4.gateway", "",
			"ipv4.dns", "",
		}
		args = appendRouteMetricArgs(args, req.RouteMetric)
		if _, err := s.nmcli(ctx, args...); err != nil {
			return model.NetworkInterface{}, err
		}
	} else {
		args := []string{
			"connection", "modify", target.ConnectionName,
			"ipv4.method", "manual",
			"ipv4.addresses", strings.Join(updateIPv4Addresses(req), ","),
			"ipv4.gateway", strings.TrimSpace(req.Gateway4),
			"ipv4.dns", strings.Join(trimStrings(req.DNS4), ","),
		}
		args = appendRouteMetricArgs(args, req.RouteMetric)
		if _, err := s.nmcli(ctx, args...); err != nil {
			return model.NetworkInterface{}, err
		}
	}

	if _, err := s.nmcli(ctx, "connection", "up", target.ConnectionName); err != nil {
		return model.NetworkInterface{}, err
	}
	if req.RouteMetric != nil {
		if err := s.saveNetworkPriority(target, *req.RouteMetric); err != nil {
			return model.NetworkInterface{}, err
		}
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

// UpdateInterfacePriorities 批量更新网口连接的 IPv4 路由优先级。
func (s *Service) UpdateInterfacePriorities(
	ctx context.Context,
	req model.NetworkPriorityBatchRequest,
) ([]model.NetworkInterface, error) {
	if err := s.checkBackend(ctx); err != nil {
		return nil, err
	}
	if len(req.Priorities) == 0 {
		return nil, fmt.Errorf("%w: priorities are required", ErrInvalidConfig)
	}

	items, err := s.ListInterfaces(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := resolvePriorityTargets(items, req.Priorities)
	if err != nil {
		return nil, err
	}

	settings := model.NetworkSettings{
		Priorities: make([]model.NetworkPrioritySetting, 0, len(targets)),
	}
	for _, target := range targets {
		args := append([]string{"connection", "modify", target.item.ConnectionName}, persistentRouteMetricArgs(target.routeMetric)...)
		if _, err := s.nmcli(ctx, args...); err != nil {
			return nil, err
		}
		settings.Priorities = append(settings.Priorities, model.NetworkPrioritySetting{
			InterfaceName:  target.item.Name,
			ConnectionName: target.item.ConnectionName,
			RouteMetric:    target.routeMetric,
		})
	}

	if s.settings != nil {
		if err := s.settings.SaveNetwork(settings); err != nil {
			return nil, err
		}
	}
	for _, target := range targets {
		if err := s.reapplyConnectedConnection(ctx, target.item); err != nil {
			return nil, err
		}
	}

	return s.ListInterfaces(ctx)
}

// RestoreSavedSettings 将已保存的网络优先级偏好重新写入 NetworkManager。
func (s *Service) RestoreSavedSettings(ctx context.Context) error {
	if s.settings == nil {
		return nil
	}

	settings, ok, err := s.settings.LoadNetwork()
	if err != nil || !ok {
		return err
	}
	if len(settings.Priorities) == 0 {
		return nil
	}

	items, err := s.ListInterfaces(ctx)
	if err != nil {
		return err
	}
	index := networkPriorityIndex(items)

	for _, priority := range settings.Priorities {
		if err := validateRouteMetric(&priority.RouteMetric, ErrInvalidConfig); err != nil {
			continue
		}
		target, ok := index.lookup(priority)
		if !ok || target.ConnectionName == "" || target.ConnectionName == "--" {
			continue
		}
		args := append([]string{"connection", "modify", target.ConnectionName}, persistentRouteMetricArgs(priority.RouteMetric)...)
		if _, err := s.nmcli(ctx, args...); err != nil {
			return err
		}
		if err := s.reapplyConnectedConnection(ctx, target); err != nil {
			return err
		}
	}
	return nil
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

	if _, err := s.nmcli(ctx, args...); err != nil {
		return err
	}
	return nil
}

// ConnectCellular 创建或更新 4G/移动网络连接并尝试拨号。
func (s *Service) ConnectCellular(ctx context.Context, req model.CellularConnectRequest) ([]model.NetworkInterface, error) {
	if err := s.checkBackend(ctx); err != nil {
		return nil, err
	}
	if err := validateCellularRequest(req); err != nil {
		return nil, err
	}

	modems, err := s.listCellularModems(ctx)
	if err != nil {
		return nil, ErrCellularUnavailable
	}
	modem, ok := selectCellularModem(modems, req)
	if !ok || strings.TrimSpace(modem.PrimaryPort) == "" {
		return nil, ErrCellularUnavailable
	}

	connectionName := strings.TrimSpace(req.ConnectionName)
	if connectionName == "" {
		connectionName = defaultCellularConnectionName(modem)
	}
	if err := s.configureCellularConnection(ctx, modem, req, connectionName); err != nil {
		return nil, err
	}

	if req.RouteMetric != nil {
		if err := s.saveNetworkPriority(model.NetworkInterface{
			Name:           modem.PrimaryPort,
			ConnectionName: connectionName,
		}, *req.RouteMetric); err != nil {
			return nil, err
		}
	}
	if _, err := s.nmcli(ctx, "connection", "up", connectionName); err != nil {
		return nil, err
	}
	return s.ListInterfaces(ctx)
}

func (s *Service) configureCellularConnection(ctx context.Context, modem model.CellularModem, req model.CellularConnectRequest, connectionName string) error {
	if connectionType, ok := s.connectionType(ctx, connectionName); ok {
		if connectionType != "gsm" {
			return ErrInvalidCellularConfig
		}
		args := []string{
			"connection", "modify", connectionName,
			"connection.interface-name", modem.PrimaryPort,
			"gsm.apn", strings.TrimSpace(req.APN),
			"gsm.username", strings.TrimSpace(req.Username),
			"gsm.password", strings.TrimSpace(req.Password),
			"ipv4.method", "auto",
			"ipv6.method", "auto",
		}
		args = appendRouteMetricArgs(args, req.RouteMetric)
		if _, err := s.nmcli(ctx, args...); err != nil {
			return err
		}
	} else {
		args := []string{
			"connection", "add",
			"type", "gsm",
			"ifname", modem.PrimaryPort,
			"con-name", connectionName,
			"apn", strings.TrimSpace(req.APN),
		}
		if _, err := s.nmcli(ctx, args...); err != nil {
			return err
		}
		modArgs := []string{
			"connection", "modify", connectionName,
			"gsm.username", strings.TrimSpace(req.Username),
			"gsm.password", strings.TrimSpace(req.Password),
			"ipv4.method", "auto",
			"ipv6.method", "auto",
		}
		modArgs = appendRouteMetricArgs(modArgs, req.RouteMetric)
		if _, err := s.nmcli(ctx, modArgs...); err != nil {
			return err
		}
	}
	return nil
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

func (s *Service) findManagedInterface(ctx context.Context, name string) (model.NetworkInterface, error) {
	items, err := s.ListInterfaces(ctx)
	if err != nil {
		return model.NetworkInterface{}, err
	}
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if !item.Managed || item.ConnectionName == "" || item.ConnectionName == "--" {
			return model.NetworkInterface{}, ErrInterfaceUnmanaged
		}
		return item, nil
	}
	return model.NetworkInterface{}, ErrInterfaceNotFound
}

func (s *Service) connectionRouteMetric(ctx context.Context, connectionName string) (*int, error) {
	output, err := s.nmcli(ctx, "-g", "ipv4.route-metric", "connection", "show", connectionName)
	if err != nil {
		return nil, err
	}
	value := strings.TrimSpace(output)
	if value == "" || value == "-1" {
		return nil, nil
	}
	metric, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}
	return &metric, nil
}

func (s *Service) connectionType(ctx context.Context, connectionName string) (string, bool) {
	output, err := s.nmcli(ctx, "-g", "connection.type", "connection", "show", connectionName)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(output), true
}

func (s *Service) listCellularModems(ctx context.Context) ([]model.CellularModem, error) {
	if _, err := exec.LookPath("mmcli"); err != nil {
		return []model.CellularModem{}, err
	}
	output, err := s.mmcli(ctx, "-L", "-K")
	if err != nil {
		return []model.CellularModem{}, err
	}
	ids := parseModemList(output)
	modems := make([]model.CellularModem, 0, len(ids))
	for _, id := range ids {
		detail, err := s.mmcli(ctx, "-m", id, "-K")
		if err != nil {
			continue
		}
		modem := parseModemDetail(detail)
		if modem.ID == "" {
			modem.ID = id
		}
		modems = append(modems, modem)
	}
	return modems, nil
}

func (s *Service) mmcli(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	output, err := s.runner.Run(ctx, "mmcli", args...)
	if err != nil {
		return "", commandError("mmcli", args, output, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (s *Service) saveNetworkPriority(target model.NetworkInterface, routeMetric int) error {
	if s.settings == nil {
		return nil
	}

	settings, _, err := s.settings.LoadNetwork()
	if err != nil {
		return err
	}
	settings = upsertNetworkPriority(settings, model.NetworkPrioritySetting{
		InterfaceName:  target.Name,
		ConnectionName: target.ConnectionName,
		RouteMetric:    routeMetric,
	})
	return s.settings.SaveNetwork(settings)
}

func (s *Service) reapplyConnectedConnection(ctx context.Context, target model.NetworkInterface) error {
	if !shouldReactivateConnection(target) {
		return nil
	}
	if _, err := s.nmcli(ctx, "device", "reapply", target.Name); err == nil {
		return nil
	}
	_, err := s.nmcli(ctx, "connection", "up", target.ConnectionName)
	return err
}

func (s *Service) nmcli(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	output, err := s.runner.Run(ctx, "nmcli", args...)
	if err != nil {
		return "", commandError("nmcli", args, output, err)
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
			Kind:           normalizeInterfaceKind(strings.TrimSpace(parts[1]), name),
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

func applyKernelInterfaces(interfaces map[string]model.NetworkInterface) error {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if _, ok := interfaces[name]; ok {
			continue
		}
		if name == "lo" {
			continue
		}

		item := model.NetworkInterface{
			Name:            name,
			Type:            kernelInterfaceType(name),
			Kind:            normalizeInterfaceKind(kernelInterfaceType(name), name),
			State:           kernelOperState(name),
			HardwareAddress: readSysfsNetValue(name, "address"),
			IPv4:            []model.NetworkAddress{},
			IPv6:            []model.NetworkAddress{},
			DNS4:            []string{},
			DNS6:            []string{},
			IPv4Method:      "unknown",
			Managed:         false,
			ReadOnly:        true,
			Source:          "kernel",
		}
		if mtu, err := strconv.Atoi(readSysfsNetValue(name, "mtu")); err == nil {
			item.MTU = mtu
		}
		applyKernelAddresses(&item)
		interfaces[name] = item
	}
	return nil
}

func applyKernelAddresses(item *model.NetworkInterface) {
	iface, err := net.InterfaceByName(item.Name)
	if err != nil {
		return
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return
	}
	for _, address := range addresses {
		ipNet, ok := address.(*net.IPNet)
		if !ok {
			continue
		}
		ones, _ := ipNet.Mask.Size()
		if ipv4 := ipNet.IP.To4(); ipv4 != nil {
			item.IPv4 = append(item.IPv4, model.NetworkAddress{
				Address: ipv4.String(),
				Prefix:  ones,
			})
			continue
		}
		item.IPv6 = append(item.IPv6, model.NetworkAddress{
			Address: ipNet.IP.String(),
			Prefix:  ones,
		})
	}
}

func readSysfsNetValue(name string, property string) string {
	data, err := os.ReadFile("/sys/class/net/" + name + "/" + property)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func kernelOperState(name string) string {
	state := readSysfsNetValue(name, "operstate")
	if state == "" {
		return "unknown"
	}
	return state
}

func kernelInterfaceType(name string) string {
	switch {
	case strings.HasPrefix(name, "wl"):
		return "wifi"
	case strings.HasPrefix(name, "wwan"), strings.HasPrefix(name, "wwp"):
		return "gsm"
	case strings.HasPrefix(name, "usb"):
		return "ethernet"
	case strings.HasPrefix(name, "can"):
		return "can"
	default:
		return "ethernet"
	}
}

func normalizeInterfaceKind(deviceType string, name string) string {
	value := strings.ToLower(strings.TrimSpace(deviceType))
	switch {
	case strings.Contains(value, "wifi"), strings.Contains(value, "wireless"), strings.HasPrefix(name, "wl"):
		return "wifi"
	case strings.Contains(value, "gsm"), strings.Contains(value, "wwan"), strings.HasPrefix(name, "wwan"), strings.HasPrefix(name, "wwp"), strings.HasPrefix(name, "ttyUSB"):
		return "cellular"
	case strings.Contains(value, "ethernet"), strings.HasPrefix(name, "eth"), strings.HasPrefix(name, "en"), strings.HasPrefix(name, "usb"):
		return "ethernet"
	case strings.Contains(value, "can"), strings.HasPrefix(name, "can"):
		return "can"
	default:
		return value
	}
}

func interfaceCapabilities(item model.NetworkInterface) []string {
	capabilities := []string{"status"}
	if item.Managed && !item.ReadOnly {
		capabilities = append(capabilities, "priority")
	}
	switch item.Kind {
	case "wifi":
		capabilities = append(capabilities, "wifi")
		if item.Managed && !item.ReadOnly {
			capabilities = append(capabilities, "ipv4")
		}
	case "ethernet":
		if item.Managed && !item.ReadOnly {
			capabilities = append(capabilities, "ipv4")
		}
	case "cellular":
		capabilities = append(capabilities, "cellular")
		if item.Modem != nil && item.Modem.PrimaryPort != "" {
			capabilities = append(capabilities, "cellular-connect")
		}
		if strings.TrimSpace(item.ConnectionName) != "" && strings.TrimSpace(item.ConnectionName) != "--" {
			capabilities = append(capabilities, "priority")
		}
	}
	return dedupeStrings(capabilities)
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
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

func parseModemList(output string) []string {
	ids := []string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(strings.TrimSpace(key), "modem-list.value[") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || value == "--" {
			continue
		}
		if id := modemIDFromPath(value); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseModemDetail(output string) model.CellularModem {
	fields := map[string]string{}
	ports := []string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "--" {
			value = ""
		}
		fields[key] = value
		if strings.HasPrefix(key, "modem.generic.ports.value[") && value != "" {
			ports = append(ports, value)
		}
	}

	modem := model.CellularModem{
		ID:                 modemIDFromPath(fields["modem.dbus-path"]),
		DBusPath:           fields["modem.dbus-path"],
		Manufacturer:       fields["modem.generic.manufacturer"],
		Model:              fields["modem.generic.model"],
		Revision:           fields["modem.generic.revision"],
		EquipmentID:        fields["modem.generic.equipment-identifier"],
		PrimaryPort:        fields["modem.generic.primary-port"],
		State:              fields["modem.generic.state"],
		FailedReason:       fields["modem.generic.state-failed-reason"],
		PowerState:         fields["modem.generic.power-state"],
		AccessTechnologies: fields["modem.generic.access-technologies"],
		OperatorName:       fields["modem.3gpp.operator-name"],
		OperatorCode:       fields["modem.3gpp.operator-code"],
		RegistrationState:  fields["modem.3gpp.registration-state"],
		PacketServiceState: fields["modem.3gpp.packet-service-state"],
		SIMPath:            fields["modem.generic.sim"],
		Ports:              ports,
	}
	if signal, err := strconv.Atoi(fields["modem.generic.signal-quality.value"]); err == nil {
		modem.SignalQuality = signal
	}
	modem.DataInterface = modemDataInterface(ports)
	return modem
}

func modemIDFromPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	_, id, ok := strings.Cut(value, "/Modem/")
	if ok {
		return strings.TrimSpace(id)
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if _, err := strconv.Atoi(last); err == nil {
		return last
	}
	return ""
}

func modemDataInterface(ports []string) string {
	for _, port := range ports {
		name, portType := splitModemPort(port)
		if portType == "net" {
			return name
		}
	}
	return ""
}

func splitModemPort(value string) (string, string) {
	name := strings.TrimSpace(value)
	portType := ""
	if before, after, ok := strings.Cut(name, "("); ok {
		name = strings.TrimSpace(before)
		portType = strings.TrimSuffix(strings.TrimSpace(after), ")")
	}
	return name, portType
}

func applyCellularModems(interfaces map[string]model.NetworkInterface, modems []model.CellularModem) {
	for _, modem := range modems {
		if modem.PrimaryPort == "" && modem.DataInterface == "" {
			continue
		}
		if modem.DataInterface != "" {
			item := ensureInterface(interfaces, modem.DataInterface, "gsm")
			item.Kind = "cellular"
			item.Type = "gsm"
			item.Source = mergeSource(item.Source, "modemmanager")
			item.ReadOnly = true
			item.Modem = cloneCellularModem(modem)
			if item.State == "" || item.State == "unknown" || item.State == "down" {
				item.State = cellularInterfaceState(item, modem)
			}
			interfaces[item.Name] = item
		}
		if modem.PrimaryPort != "" {
			item := ensureInterface(interfaces, modem.PrimaryPort, "gsm")
			item.Kind = "cellular"
			item.Type = "gsm"
			item.Source = mergeSource(item.Source, "modemmanager")
			item.ReadOnly = false
			item.Modem = cloneCellularModem(modem)
			if item.State == "" || item.State == "unknown" {
				item.State = cellularModemState(modem)
			}
			interfaces[item.Name] = item
		}
	}
}

func ensureInterface(interfaces map[string]model.NetworkInterface, name string, deviceType string) model.NetworkInterface {
	if item, ok := interfaces[name]; ok {
		return item
	}
	return model.NetworkInterface{
		Name:       name,
		Type:       deviceType,
		Kind:       normalizeInterfaceKind(deviceType, name),
		State:      "unknown",
		IPv4:       []model.NetworkAddress{},
		IPv6:       []model.NetworkAddress{},
		DNS4:       []string{},
		DNS6:       []string{},
		IPv4Method: "unknown",
		Source:     "modemmanager",
	}
}

func cloneCellularModem(modem model.CellularModem) *model.CellularModem {
	clone := modem
	clone.Ports = append([]string{}, modem.Ports...)
	return &clone
}

func mergeSource(current string, next string) string {
	if current == "" {
		return next
	}
	for _, part := range strings.Split(current, "+") {
		if part == next {
			return current
		}
	}
	return current + "+" + next
}

func cellularInterfaceState(item model.NetworkInterface, modem model.CellularModem) string {
	if modem.State == "connected" {
		return "connected"
	}
	if modem.FailedReason != "" {
		return "unavailable"
	}
	if item.State != "" && item.State != "unknown" {
		return item.State
	}
	return cellularModemState(modem)
}

func cellularModemState(modem model.CellularModem) string {
	if modem.FailedReason != "" {
		return "unavailable"
	}
	if modem.State != "" {
		return modem.State
	}
	return "unknown"
}

type priorityIndex struct {
	byInterface  map[string]model.NetworkInterface
	byConnection map[string]model.NetworkInterface
}

type priorityTarget struct {
	item        model.NetworkInterface
	routeMetric int
}

func networkPriorityIndex(items []model.NetworkInterface) priorityIndex {
	index := priorityIndex{
		byInterface:  map[string]model.NetworkInterface{},
		byConnection: map[string]model.NetworkInterface{},
	}
	for _, item := range items {
		if item.Name != "" {
			index.byInterface[item.Name] = item
		}
		if item.ConnectionName != "" && item.ConnectionName != "--" {
			index.byConnection[item.ConnectionName] = item
		}
	}
	return index
}

func (i priorityIndex) lookup(priority model.NetworkPrioritySetting) (model.NetworkInterface, bool) {
	if priority.InterfaceName != "" {
		if item, ok := i.byInterface[priority.InterfaceName]; ok {
			return item, true
		}
	}
	if priority.ConnectionName != "" {
		if item, ok := i.byConnection[priority.ConnectionName]; ok {
			return item, true
		}
	}
	return model.NetworkInterface{}, false
}

func resolvePriorityTargets(items []model.NetworkInterface, priorities []model.NetworkPriorityBatchItem) ([]priorityTarget, error) {
	index := networkPriorityIndex(items)
	targets := make([]priorityTarget, 0, len(priorities))
	seen := map[string]struct{}{}

	for _, priority := range priorities {
		name := strings.TrimSpace(priority.InterfaceName)
		if name == "" {
			return nil, fmt.Errorf("%w: interface name is required", ErrInvalidConfig)
		}
		if err := validateInterfaceName(name); err != nil {
			return nil, err
		}
		if err := validateRouteMetric(&priority.RouteMetric, ErrInvalidConfig); err != nil {
			return nil, err
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("%w: duplicate interface name", ErrInvalidConfig)
		}
		seen[name] = struct{}{}

		item, ok := index.byInterface[name]
		if !ok {
			return nil, ErrInterfaceNotFound
		}
		if !isPriorityConfigurable(item) {
			return nil, ErrInterfaceUnmanaged
		}
		targets = append(targets, priorityTarget{
			item:        item,
			routeMetric: priority.RouteMetric,
		})
	}

	return targets, nil
}

func isPriorityConfigurable(item model.NetworkInterface) bool {
	connectionName := strings.TrimSpace(item.ConnectionName)
	if connectionName == "" || connectionName == "--" {
		return false
	}
	if item.Managed && !item.ReadOnly {
		return true
	}
	return item.Kind == "cellular" && hasCapability(item, "cellular-connect")
}

func hasCapability(item model.NetworkInterface, capability string) bool {
	for _, itemCapability := range item.Capabilities {
		if itemCapability == capability {
			return true
		}
	}
	return false
}

func sortNetworkInterfaces(items []model.NetworkInterface) {
	sort.SliceStable(items, func(i, j int) bool {
		left := interfaceKindSortScore(items[i])
		right := interfaceKindSortScore(items[j])
		if left != right {
			return left < right
		}
		if items[i].State == "connected" && items[j].State != "connected" {
			return true
		}
		if items[i].State != "connected" && items[j].State == "connected" {
			return false
		}
		return items[i].Name < items[j].Name
	})
}

func interfaceKindSortScore(item model.NetworkInterface) int {
	switch item.Kind {
	case "ethernet":
		return 0
	case "wifi":
		return 1
	case "cellular":
		return 2
	default:
		return 3
	}
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
		switch normalizeDeviceDetailKey(key) {
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

func normalizeDeviceDetailKey(key string) string {
	if base, _, ok := strings.Cut(key, "["); ok {
		return base
	}
	return key
}

func parseNetworkAddress(value string) model.NetworkAddress {
	address, rawPrefix, ok := strings.Cut(strings.TrimSpace(value), "/")
	if !ok {
		return model.NetworkAddress{Address: strings.TrimSpace(value)}
	}
	prefix, _ := strconv.Atoi(rawPrefix)
	return model.NetworkAddress{Address: address, Prefix: prefix}
}

func updateIPv4Addresses(req model.NetworkInterfaceUpdateRequest) []string {
	addresses := req.IPv4
	if len(addresses) == 0 && strings.TrimSpace(req.IPv4Address) != "" {
		addresses = []model.NetworkAddress{{
			Address: req.IPv4Address,
			Prefix:  req.Prefix,
		}}
	}

	out := make([]string, 0, len(addresses))
	for _, item := range addresses {
		out = append(out, fmt.Sprintf("%s/%d", strings.TrimSpace(item.Address), item.Prefix))
	}
	return out
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
	if err := validateRouteMetric(req.RouteMetric, ErrInvalidConfig); err != nil {
		return err
	}
	switch req.Mode {
	case "dhcp":
		return nil
	case "static":
		if err := validateUpdateIPv4Addresses(req); err != nil {
			return err
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

func validateUpdateIPv4Addresses(req model.NetworkInterfaceUpdateRequest) error {
	addresses := req.IPv4
	if len(addresses) == 0 && strings.TrimSpace(req.IPv4Address) != "" {
		addresses = []model.NetworkAddress{{
			Address: req.IPv4Address,
			Prefix:  req.Prefix,
		}}
	}
	if len(addresses) == 0 {
		return fmt.Errorf("%w: invalid ipv4 address", ErrInvalidConfig)
	}
	seen := map[string]struct{}{}
	for _, item := range addresses {
		address := strings.TrimSpace(item.Address)
		ip := net.ParseIP(address)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("%w: invalid ipv4 address", ErrInvalidConfig)
		}
		if item.Prefix < 1 || item.Prefix > 32 {
			return fmt.Errorf("%w: invalid ipv4 prefix", ErrInvalidConfig)
		}
		key := ip.To4().String()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%w: duplicate ipv4 address", ErrInvalidConfig)
		}
		seen[key] = struct{}{}
	}
	return nil
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

func validateCellularRequest(req model.CellularConnectRequest) error {
	if err := validateRouteMetric(req.RouteMetric, ErrInvalidCellularConfig); err != nil {
		return err
	}
	apn := strings.TrimSpace(req.APN)
	if apn == "" || len(apn) > 100 {
		return fmt.Errorf("%w: invalid apn", ErrInvalidCellularConfig)
	}
	if strings.TrimSpace(req.InterfaceName) != "" {
		if err := validateInterfaceName(strings.TrimSpace(req.InterfaceName)); err != nil {
			return fmt.Errorf("%w: invalid interface name", ErrInvalidCellularConfig)
		}
	}
	if len(strings.TrimSpace(req.Username)) > 128 || len(strings.TrimSpace(req.Password)) > 128 {
		return fmt.Errorf("%w: invalid credentials", ErrInvalidCellularConfig)
	}
	if len(strings.TrimSpace(req.ConnectionName)) > 128 {
		return fmt.Errorf("%w: invalid connection name", ErrInvalidCellularConfig)
	}
	return nil
}

func selectCellularModem(modems []model.CellularModem, req model.CellularConnectRequest) (model.CellularModem, bool) {
	modemID := strings.TrimSpace(req.ModemID)
	interfaceName := strings.TrimSpace(req.InterfaceName)
	for _, modem := range modems {
		if modemID != "" && modem.ID == modemID {
			return modem, true
		}
		if interfaceName != "" && modemMatchesInterface(modem, interfaceName) {
			return modem, true
		}
	}
	if len(modems) == 1 && modemID == "" && interfaceName == "" {
		return modems[0], true
	}
	return model.CellularModem{}, false
}

func modemMatchesInterface(modem model.CellularModem, name string) bool {
	if modem.PrimaryPort == name || modem.DataInterface == name {
		return true
	}
	for _, port := range modem.Ports {
		portName, _ := splitModemPort(port)
		if portName == name {
			return true
		}
	}
	return false
}

func defaultCellularConnectionName(modem model.CellularModem) string {
	if modem.Model != "" {
		return "4g-" + sanitizeConnectionSuffix(modem.Model)
	}
	if modem.PrimaryPort != "" {
		return "4g-" + sanitizeConnectionSuffix(modem.PrimaryPort)
	}
	if modem.ID != "" {
		return "4g-" + sanitizeConnectionSuffix(modem.ID)
	}
	return "4g"
}

func sanitizeConnectionSuffix(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case r == ' ' || r == '/':
			builder.WriteRune('-')
		}
	}
	out := strings.Trim(builder.String(), "-_")
	if out == "" {
		return "modem"
	}
	return out
}

func validateRouteMetric(value *int, wrap error) error {
	if value == nil {
		return nil
	}
	if *value < -1 || *value > 9999 {
		return fmt.Errorf("%w: invalid route metric", wrap)
	}
	return nil
}

func appendRouteMetricArgs(args []string, routeMetric *int) []string {
	if routeMetric == nil {
		return args
	}
	return append(args, persistentRouteMetricArgs(*routeMetric)...)
}

func persistentRouteMetricArgs(routeMetric int) []string {
	return []string{
		"ipv4.route-metric", strconv.Itoa(routeMetric),
		"connection.autoconnect", "yes",
		"connection.autoconnect-priority", strconv.Itoa(autoconnectPriorityForRouteMetric(routeMetric)),
	}
}

func autoconnectPriorityForRouteMetric(routeMetric int) int {
	if routeMetric < 0 {
		return 0
	}

	priority := 999 - routeMetric
	if priority > 999 {
		return 999
	}
	if priority < -999 {
		return -999
	}
	return priority
}

func shouldReactivateConnection(target model.NetworkInterface) bool {
	connectionName := strings.TrimSpace(target.ConnectionName)
	return connectionName != "" &&
		connectionName != "--" &&
		strings.TrimSpace(target.State) == "connected"
}

func commandError(command string, args []string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	commandLine := strings.TrimSpace(command + " " + strings.Join(args, " "))
	if detail == "" || detail == err.Error() {
		return fmt.Errorf("%s: %w", commandLine, err)
	}
	return fmt.Errorf("%s: %s: %w", commandLine, detail, err)
}

func upsertNetworkPriority(settings model.NetworkSettings, priority model.NetworkPrioritySetting) model.NetworkSettings {
	priorities := make([]model.NetworkPrioritySetting, 0, len(settings.Priorities)+1)
	replaced := false
	for _, item := range settings.Priorities {
		if item.InterfaceName == priority.InterfaceName {
			priorities = append(priorities, priority)
			replaced = true
			continue
		}
		priorities = append(priorities, item)
	}
	if !replaced {
		priorities = append(priorities, priority)
	}
	settings.Priorities = priorities
	return settings
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
