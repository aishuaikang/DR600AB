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
			if routeMetric, err := s.connectionRouteMetric(ctx, item.ConnectionName); err == nil {
				item.RouteMetric = routeMetric
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
		if !item.Managed || item.ConnectionName == "" || item.ConnectionName == "--" {
			return nil, ErrInterfaceUnmanaged
		}
		targets = append(targets, priorityTarget{
			item:        item,
			routeMetric: priority.RouteMetric,
		})
	}

	return targets, nil
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
