// Package model 定义后端共享的请求、响应、事件和记录 DTO。
package model

import (
	"encoding/json"
	"time"
)

// LocaleMeta 描述前端可用的本地化资源。
type LocaleMeta struct {
	Default    string   `json:"defaultLocale"`
	Supported  []string `json:"supportedLocales"`
	Namespaces []string `json:"namespaces"`
}

// PortInfo 描述一个串口，以及它是否被当前会话占用。
type PortInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// DetectionSessionRequest 配置侦测串口会话。
type DetectionSessionRequest struct {
	PortName      string `json:"portName,omitempty"`
	RxPortName    string `json:"rxPortName,omitempty"`
	TxPortName    string `json:"txPortName,omitempty"`
	BaudRate      int    `json:"baudRate"`
	DataBits      int    `json:"dataBits"`
	StopBits      int    `json:"stopBits"`
	Parity        string `json:"parity"`
	ReadTimeoutMs int    `json:"readTimeoutMs,omitempty"`
	AutoConnect   bool   `json:"autoConnect,omitempty"`
}

// DetectionSessionResponse 返回当前侦测会话状态。
type DetectionSessionResponse struct {
	Active        bool      `json:"active"`
	SessionID     string    `json:"sessionId,omitempty"`
	PortName      string    `json:"portName,omitempty"`
	RxPortName    string    `json:"rxPortName,omitempty"`
	TxPortName    string    `json:"txPortName,omitempty"`
	BaudRate      int       `json:"baudRate,omitempty"`
	DataBits      int       `json:"dataBits,omitempty"`
	StopBits      int       `json:"stopBits,omitempty"`
	Parity        string    `json:"parity,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	State         string    `json:"state,omitempty"`
	AutoReconnect bool      `json:"autoReconnect,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	RetryCount    int       `json:"retryCount,omitempty"`
	Message       string    `json:"message"`
}

// GPSSessionRequest 配置 GPS NMEA 0183 串口会话。
type GPSSessionRequest struct {
	PortName        string `json:"portName,omitempty"`
	DataPortName    string `json:"dataPortName,omitempty"`
	ControlPortName string `json:"controlPortName,omitempty"`
	BaudRate        int    `json:"baudRate"`
	DataBits        int    `json:"dataBits"`
	StopBits        int    `json:"stopBits"`
	Parity          string `json:"parity"`
	ReadTimeoutMs   int    `json:"readTimeoutMs,omitempty"`
	AutoConnect     bool   `json:"autoConnect,omitempty"`
}

// GPSFix 描述从 NMEA 0183 中解析出的定位结果。
type GPSFix struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	AltitudeM    float64 `json:"altitudeM,omitempty"`
	SpeedKnots   float64 `json:"speedKnots,omitempty"`
	CourseDegree float64 `json:"courseDegree,omitempty"`
	FixQuality   int     `json:"fixQuality,omitempty"`
	Satellites   int     `json:"satellites,omitempty"`
	Valid        bool    `json:"valid"`
}

// GPSRecord 保存一条 GPS NMEA 0183 原始数据及其解析结果。
type GPSRecord struct {
	SessionID  string    `json:"sessionId"`
	PortName   string    `json:"portName"`
	ReceivedAt time.Time `json:"receivedAt"`
	Type       string    `json:"type"`
	Raw        string    `json:"raw"`
	Fix        *GPSFix   `json:"fix,omitempty"`
}

// GeoPoint 描述 WGS84 经纬度坐标。
type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// UserSettings 保存公开用户设置。
type UserSettings struct {
	ManualDeviceLocation      *GeoPoint `json:"manualDeviceLocation,omitempty"`
	ScreenStrikeChannelLabels []string  `json:"screenStrikeChannelLabels,omitempty"`
}

// ScreenDeviceLocationResponse 返回大屏地图使用的设备位置。
type ScreenDeviceLocationResponse struct {
	Source    string     `json:"source"`
	Point     *GeoPoint  `json:"point,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Valid     bool       `json:"valid"`
}

// GPSSessionResponse 返回当前 GPS 会话状态。
type GPSSessionResponse struct {
	Active          bool       `json:"active"`
	SessionID       string     `json:"sessionId,omitempty"`
	PortName        string     `json:"portName,omitempty"`
	DataPortName    string     `json:"dataPortName,omitempty"`
	ControlPortName string     `json:"controlPortName,omitempty"`
	BaudRate        int        `json:"baudRate,omitempty"`
	DataBits        int        `json:"dataBits,omitempty"`
	StopBits        int        `json:"stopBits,omitempty"`
	Parity          string     `json:"parity,omitempty"`
	StartedAt       time.Time  `json:"startedAt,omitempty"`
	State           string     `json:"state,omitempty"`
	AutoReconnect   bool       `json:"autoReconnect,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	RetryCount      int        `json:"retryCount,omitempty"`
	LastNMEA        string     `json:"lastNmea,omitempty"`
	LastFix         *GPSFix    `json:"lastFix,omitempty"`
	LastRecord      *GPSRecord `json:"lastRecord,omitempty"`
	Message         string     `json:"message"`
}

// ParsedMessage 保存单行串口数据的解析结果。
type ParsedMessage struct {
	Type string          `json:"type"`
	Time time.Time       `json:"time"`
	Raw  string          `json:"raw"`
	Data json.RawMessage `json:"data"`
}

// DetectionRecord 是侦测视图使用的标准化列表项。
type DetectionRecord struct {
	ID         string        `json:"id"`
	SessionID  string        `json:"sessionId"`
	PortName   string        `json:"portName"`
	Kind       string        `json:"kind"`
	ReceivedAt time.Time     `json:"receivedAt"`
	Device     string        `json:"device,omitempty"`
	Model      string        `json:"model,omitempty"`
	Frequency  float64       `json:"frequency,omitempty"`
	RSSI       float64       `json:"rssi,omitempty"`
	Summary    string        `json:"summary"`
	Parsed     ParsedMessage `json:"parsed"`
}

// ScreenDetectionLastRecord 是大屏公开接口可返回的最近侦测摘要，不包含解析原文。
type ScreenDetectionLastRecord struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	ReceivedAt time.Time `json:"receivedAt"`
	Device     string    `json:"device,omitempty"`
	Model      string    `json:"model,omitempty"`
	Frequency  float64   `json:"frequency,omitempty"`
	RSSI       float64   `json:"rssi,omitempty"`
	Summary    string    `json:"summary"`
}

// ScreenDetectionTarget 是大屏侦测列表使用的合并目标。
type ScreenDetectionTarget struct {
	ID         string                    `json:"id"`
	Model      string                    `json:"model"`
	Frequency  float64                   `json:"frequency"`
	RSSI       float64                   `json:"rssi"`
	Device     string                    `json:"device"`
	FirstSeen  time.Time                 `json:"firstSeen"`
	LastSeen   time.Time                 `json:"lastSeen"`
	HitCount   int                       `json:"hitCount"`
	LastRecord ScreenDetectionLastRecord `json:"lastRecord"`
}

// ScreenPositionPoint 描述大屏定位目标中的一个坐标点。
type ScreenPositionPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// ScreenPositionLastRecord 是大屏公开定位接口可返回的最近解析摘要，不包含原始报文。
type ScreenPositionLastRecord struct {
	Type       string    `json:"type"`
	ReceivedAt time.Time `json:"receivedAt"`
	Device     string    `json:"device,omitempty"`
	Serial     string    `json:"serial,omitempty"`
	Model      string    `json:"model,omitempty"`
	Frequency  float64   `json:"frequency,omitempty"`
	RSSI       float64   `json:"rssi,omitempty"`
	Cracked    bool      `json:"cracked,omitempty"`
}

// ScreenPositionTarget 是大屏定位列表使用的合并目标。
type ScreenPositionTarget struct {
	ID            string                   `json:"id"`
	CorrelationID string                   `json:"correlationId,omitempty"`
	Serial        string                   `json:"serial"`
	Model         string                   `json:"model"`
	Source        string                   `json:"source"`
	Frequency     float64                  `json:"frequency,omitempty"`
	RSSI          float64                  `json:"rssi,omitempty"`
	Device        string                   `json:"device"`
	Drone         *ScreenPositionPoint     `json:"drone,omitempty"`
	Pilot         *ScreenPositionPoint     `json:"pilot,omitempty"`
	Home          *ScreenPositionPoint     `json:"home,omitempty"`
	Height        *float64                 `json:"height,omitempty"`
	Altitude      *float64                 `json:"altitude,omitempty"`
	Speed         *float64                 `json:"speed,omitempty"`
	Cracked       bool                     `json:"cracked,omitempty"`
	FirstSeen     time.Time                `json:"firstSeen"`
	LastSeen      time.Time                `json:"lastSeen"`
	HitCount      int                      `json:"hitCount"`
	LastRecord    ScreenPositionLastRecord `json:"lastRecord"`
}

// GpioChannel 描述一个 GPIO 控制通道及其运行状态。
type GpioChannel struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Pin          int      `json:"pin"`
	Bands        []string `json:"bands"`
	Reserved     bool     `json:"reserved"`
	Enabled      bool     `json:"enabled"`
	ActualLevel  string   `json:"actualLevel"`
	DesiredLevel string   `json:"desiredLevel"`
	Status       string   `json:"status"`
	LastError    string   `json:"lastError,omitempty"`
}

// GpioChannelStateRequest 更新 GPIO 通道是否启用。
type GpioChannelStateRequest struct {
	Enabled bool `json:"enabled"`
}

// GpioChannelStateResponse 返回更新后的 GPIO 通道和用户提示。
type GpioChannelStateResponse struct {
	Channel GpioChannel `json:"channel"`
	Message string      `json:"message"`
}

// ScreenStrikeRequest 控制大屏干扰面板绑定的 GPIO 通道。
type ScreenStrikeRequest struct {
	Enabled         bool     `json:"enabled"`
	ChannelIDs      []string `json:"channelIds"`
	DurationSeconds int      `json:"durationSeconds"`
}

// ScreenStrikeState 描述大屏干扰控制当前状态。
type ScreenStrikeState struct {
	Active           bool          `json:"active"`
	ChannelIDs       []string      `json:"channelIds"`
	DurationSeconds  int           `json:"durationSeconds"`
	RemainingSeconds int           `json:"remainingSeconds"`
	StartedAt        *time.Time    `json:"startedAt,omitempty"`
	EndsAt           *time.Time    `json:"endsAt,omitempty"`
	Channels         []GpioChannel `json:"channels"`
}

// ScreenStrikeResponse 返回大屏干扰控制状态和用户提示。
type ScreenStrikeResponse struct {
	State   ScreenStrikeState `json:"state"`
	Message string            `json:"message"`
}

// DeveloperLoginRequest 使用动态码换取短时开发者会话。
type DeveloperLoginRequest struct {
	Code string `json:"code"`
}

// DeveloperSessionResponse 返回开发者短时会话。
type DeveloperSessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
	Message   string `json:"message"`
}

// NetworkAddress 描述一个接口地址和前缀长度。
type NetworkAddress struct {
	Address string `json:"address"`
	Prefix  int    `json:"prefix"`
}

// NetworkInterface 描述一个系统网络接口的可配置状态。
type NetworkInterface struct {
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	State           string           `json:"state"`
	ConnectionName  string           `json:"connectionName,omitempty"`
	HardwareAddress string           `json:"hardwareAddress,omitempty"`
	MTU             int              `json:"mtu,omitempty"`
	IPv4            []NetworkAddress `json:"ipv4"`
	IPv6            []NetworkAddress `json:"ipv6"`
	Gateway4        string           `json:"gateway4,omitempty"`
	Gateway6        string           `json:"gateway6,omitempty"`
	DNS4            []string         `json:"dns4"`
	DNS6            []string         `json:"dns6"`
	IPv4Method      string           `json:"ipv4Method"`
	RouteMetric     *int             `json:"routeMetric,omitempty"`
	Managed         bool             `json:"managed"`
}

// NetworkInterfacesResponse 返回全部网口配置状态。
type NetworkInterfacesResponse struct {
	Interfaces []NetworkInterface `json:"interfaces"`
	Count      int                `json:"count"`
	Backend    string             `json:"backend"`
	Available  bool               `json:"available"`
	ReadOnly   bool               `json:"readOnly"`
	Message    string             `json:"message,omitempty"`
}

// NetworkInterfaceUpdateRequest 更新单个网口 IPv4 配置。
type NetworkInterfaceUpdateRequest struct {
	Mode        string   `json:"mode"`
	IPv4Address string   `json:"ipv4Address,omitempty"`
	Prefix      int      `json:"prefix,omitempty"`
	Gateway4    string   `json:"gateway4,omitempty"`
	DNS4        []string `json:"dns4,omitempty"`
	RouteMetric *int     `json:"routeMetric,omitempty"`
}

// NetworkInterfaceUpdateResponse 返回更新后的网口状态和提示。
type NetworkInterfaceUpdateResponse struct {
	Interface NetworkInterface `json:"interface"`
	Message   string           `json:"message"`
}

// NetworkPriorityBatchItem 更新单个网口的路由优先级。
type NetworkPriorityBatchItem struct {
	InterfaceName string `json:"interfaceName"`
	RouteMetric   int    `json:"routeMetric"`
}

// NetworkPriorityBatchRequest 批量更新网口路由优先级。
type NetworkPriorityBatchRequest struct {
	Priorities []NetworkPriorityBatchItem `json:"priorities"`
}

// NetworkPriorityBatchResponse 返回批量更新后的网口状态。
type NetworkPriorityBatchResponse struct {
	Interfaces []NetworkInterface `json:"interfaces"`
	Message    string             `json:"message"`
}

// NetworkPrioritySetting 保存单个网口的路由优先级偏好。
type NetworkPrioritySetting struct {
	InterfaceName  string `json:"interfaceName"`
	ConnectionName string `json:"connectionName,omitempty"`
	RouteMetric    int    `json:"routeMetric"`
}

// NetworkSettings 保存需要在后端重启后重新应用的网络偏好。
type NetworkSettings struct {
	Priorities []NetworkPrioritySetting `json:"priorities"`
}

// WiFiNetwork 描述扫描到的无线网络。
type WiFiNetwork struct {
	SSID     string `json:"ssid"`
	BSSID    string `json:"bssid,omitempty"`
	Device   string `json:"device,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Rate     string `json:"rate,omitempty"`
	Signal   int    `json:"signal"`
	Security string `json:"security,omitempty"`
	Active   bool   `json:"active"`
}

// WiFiNetworksResponse 返回无线网络扫描结果。
type WiFiNetworksResponse struct {
	Networks  []WiFiNetwork `json:"networks"`
	Count     int           `json:"count"`
	Available bool          `json:"available"`
	ReadOnly  bool          `json:"readOnly"`
	Message   string        `json:"message,omitempty"`
}

// WiFiConnectRequest 连接指定无线网络。
type WiFiConnectRequest struct {
	SSID     string `json:"ssid"`
	Password string `json:"password,omitempty"`
	Device   string `json:"device,omitempty"`
}

// WiFiConnectResponse 返回无线连接操作结果。
type WiFiConnectResponse struct {
	Message string `json:"message"`
}

// Event 是发送给服务端事件订阅者的运行时事件。
type Event struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Payload any       `json:"payload,omitempty"`
}

// ListResponse 包装列表接口响应，并附带条目数量。
type ListResponse[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

// ApiError 是标准 JSON 错误响应。
type ApiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
