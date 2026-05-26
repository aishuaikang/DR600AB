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
	DeviceSN                  string               `json:"deviceSn,omitempty"`
	DeviceHardwareID          string               `json:"deviceHardwareId,omitempty"`
	ManualDeviceLocation      *GeoPoint            `json:"manualDeviceLocation,omitempty"`
	ScreenStrikeChannelLabels []string             `json:"screenStrikeChannelLabels,omitempty"`
	IntrusionRetentionDays    *int                 `json:"intrusionRetentionDays,omitempty"`
	Whitelist                 []WhitelistItem      `json:"whitelist,omitempty"`
	ScreenAlarmSettings       *ScreenAlarmSettings `json:"screenAlarmSettings,omitempty"`
}

// WhitelistItem 描述允许在大屏中放行的目标身份。
type WhitelistItem struct {
	Serial    string    `json:"serial"`
	Model     string    `json:"model,omitempty"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

// ScreenAlarmSettings 控制大屏未入白名单目标告警来源。
type ScreenAlarmSettings struct {
	Detection bool `json:"detection"`
	Position  bool `json:"position"`
	FPV       bool `json:"fpv"`
	Sound     bool `json:"sound"`
}

const DefaultIntrusionRetentionDays = 90

// UserSettingsWithDefaults fills optional user settings with public defaults.
func UserSettingsWithDefaults(settings UserSettings) UserSettings {
	if settings.IntrusionRetentionDays == nil {
		days := DefaultIntrusionRetentionDays
		settings.IntrusionRetentionDays = &days
	}
	if settings.ScreenAlarmSettings == nil {
		settings.ScreenAlarmSettings = DefaultScreenAlarmSettings()
	}
	return settings
}

// DefaultScreenAlarmSettings returns the public default alarm switches.
func DefaultScreenAlarmSettings() *ScreenAlarmSettings {
	return &ScreenAlarmSettings{
		Detection: true,
		Position:  true,
		FPV:       true,
		Sound:     true,
	}
}

// UserSettingsIntrusionRetentionDays returns the effective target intrusion retention setting.
func UserSettingsIntrusionRetentionDays(settings UserSettings) int {
	if settings.IntrusionRetentionDays == nil {
		return DefaultIntrusionRetentionDays
	}
	return *settings.IntrusionRetentionDays
}

// IntrusionDeleteRequest deletes selected intrusion records.
type IntrusionDeleteRequest struct {
	IDs []string `json:"ids"`
}

// IntrusionDeleteResponse reports how many intrusion records were deleted.
type IntrusionDeleteResponse struct {
	Deleted int64 `json:"deleted"`
}

// ScreenDeviceLocationResponse 返回大屏地图使用的设备位置。
type ScreenDeviceLocationResponse struct {
	Source    string     `json:"source"`
	Point     *GeoPoint  `json:"point,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Valid     bool       `json:"valid"`
}

// ScreenSerialCapabilityStatus 描述大屏依赖串口能力的配置和运行状态。
type ScreenSerialCapabilityStatus struct {
	Configured       bool       `json:"configured"`
	Active           bool       `json:"active"`
	State            string     `json:"state,omitempty"`
	PortName         string     `json:"portName,omitempty"`
	RxPortName       string     `json:"rxPortName,omitempty"`
	TxPortName       string     `json:"txPortName,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	HeadingDeg       *float64   `json:"headingDeg,omitempty"`
	HeadingUpdatedAt *time.Time `json:"headingUpdatedAt,omitempty"`
}

// ScreenRuntimeStatus 返回大屏各串口能力的公开运行状态。
type ScreenRuntimeStatus struct {
	Detection ScreenSerialCapabilityStatus `json:"detection"`
	Deception ScreenSerialCapabilityStatus `json:"deception"`
	Compass   ScreenSerialCapabilityStatus `json:"compass"`
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

// CompassSessionRequest 配置三维电子罗盘串口会话。
type CompassSessionRequest struct {
	PortName      string `json:"portName,omitempty"`
	BaudRate      int    `json:"baudRate"`
	DataBits      int    `json:"dataBits"`
	StopBits      int    `json:"stopBits"`
	Parity        string `json:"parity"`
	ReadTimeoutMs int    `json:"readTimeoutMs,omitempty"`
	AutoConnect   bool   `json:"autoConnect,omitempty"`
}

// CompassRecord 保存一条三维电子罗盘角度记录。
type CompassRecord struct {
	SessionID  string    `json:"sessionId"`
	PortName   string    `json:"portName"`
	ReceivedAt time.Time `json:"receivedAt"`
	Pitch      float64   `json:"pitch"`
	Roll       float64   `json:"roll"`
	Heading    float64   `json:"heading"`
	RawHex     string    `json:"rawHex,omitempty"`
}

// CompassSessionResponse 返回当前三维电子罗盘串口会话状态。
type CompassSessionResponse struct {
	Active         bool           `json:"active"`
	SessionID      string         `json:"sessionId,omitempty"`
	PortName       string         `json:"portName,omitempty"`
	BaudRate       int            `json:"baudRate,omitempty"`
	DataBits       int            `json:"dataBits,omitempty"`
	StopBits       int            `json:"stopBits,omitempty"`
	Parity         string         `json:"parity,omitempty"`
	StartedAt      time.Time      `json:"startedAt,omitempty"`
	State          string         `json:"state,omitempty"`
	AutoReconnect  bool           `json:"autoReconnect,omitempty"`
	LastError      string         `json:"lastError,omitempty"`
	RetryCount     int            `json:"retryCount,omitempty"`
	LastRecord     *CompassRecord `json:"lastRecord,omitempty"`
	LastPitch      *float64       `json:"lastPitch,omitempty"`
	LastRoll       *float64       `json:"lastRoll,omitempty"`
	LastHeading    *float64       `json:"lastHeading,omitempty"`
	LastRawHex     string         `json:"lastRawHex,omitempty"`
	LastUpdatedAt  *time.Time     `json:"lastUpdatedAt,omitempty"`
	AutoOutput     bool           `json:"autoOutput"`
	AutoOutputRate int            `json:"autoOutputRate,omitempty"`
	Message        string         `json:"message"`
}

// DeceptionSessionRequest 配置 GNSS 诱骗设备串口会话。
type DeceptionSessionRequest struct {
	PortName      string `json:"portName,omitempty"`
	BaudRate      int    `json:"baudRate"`
	DataBits      int    `json:"dataBits"`
	StopBits      int    `json:"stopBits"`
	Parity        string `json:"parity"`
	ReadTimeoutMs int    `json:"readTimeoutMs,omitempty"`
	AutoConnect   bool   `json:"autoConnect,omitempty"`
}

// DeceptionSessionResponse 返回当前 GNSS 诱骗设备串口会话状态。
type DeceptionSessionResponse struct {
	Active        bool      `json:"active"`
	SessionID     string    `json:"sessionId,omitempty"`
	PortName      string    `json:"portName,omitempty"`
	BaudRate      int       `json:"baudRate,omitempty"`
	DataBits      int       `json:"dataBits,omitempty"`
	StopBits      int       `json:"stopBits,omitempty"`
	Parity        string    `json:"parity,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	State         string    `json:"state,omitempty"`
	AutoReconnect bool      `json:"autoReconnect,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	Message       string    `json:"message"`
}

// DeceptionRecord 保存一条 GNSS 诱骗设备协议交互记录。
type DeceptionRecord struct {
	Time        time.Time `json:"time"`
	Direction   string    `json:"direction"`
	Command     string    `json:"command,omitempty"`
	Control     string    `json:"control,omitempty"`
	RawHex      string    `json:"rawHex,omitempty"`
	Description string    `json:"description,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// DeceptionReportStatus 描述诱骗报告生命周期状态。
type DeceptionReportStatus string

const (
	DeceptionReportStatusRunning   DeceptionReportStatus = "running"
	DeceptionReportStatusCompleted DeceptionReportStatus = "completed"
	DeceptionReportStatusFailed    DeceptionReportStatus = "failed"
	DeceptionReportStatusAbnormal  DeceptionReportStatus = "abnormal"
)

// DeceptionReportSummary 是诱骗报告列表使用的摘要记录。
type DeceptionReportSummary struct {
	ID              string                `json:"id"`
	Status          DeceptionReportStatus `json:"status"`
	StartedAt       time.Time             `json:"startedAt"`
	EndedAt         *time.Time            `json:"endedAt,omitempty"`
	DurationSeconds int64                 `json:"durationSeconds"`
	TargetID        string                `json:"targetId,omitempty"`
	Mode            string                `json:"mode,omitempty"`
	Point           *GeoPoint             `json:"point,omitempty"`
	AltitudeM       float64               `json:"altitudeM,omitempty"`
	SignalMask      uint16                `json:"signalMask,omitempty"`
	SignalNames     []string              `json:"signalNames,omitempty"`
	StrengthPreset  string                `json:"strengthPreset,omitempty"`
	AttenuationDB   int                   `json:"attenuationDB,omitempty"`
	DelayMode       string                `json:"delayMode,omitempty"`
	DelayNS         float64               `json:"delayNS,omitempty"`
	PortName        string                `json:"portName,omitempty"`
	Summary         string                `json:"summary,omitempty"`
	LastError       string                `json:"lastError,omitempty"`
	AbnormalReason  string                `json:"abnormalReason,omitempty"`
	CreatedAt       time.Time             `json:"createdAt"`
	UpdatedAt       time.Time             `json:"updatedAt"`
}

// DeceptionReport 保存一次诱骗操作的完整证据快照。
type DeceptionReport struct {
	DeceptionReportSummary
	Request           ScreenDeceptionRequest       `json:"request"`
	Session           DeceptionSessionResponse     `json:"session"`
	StartState        *ScreenDeceptionState        `json:"startState,omitempty"`
	EndState          *ScreenDeceptionState        `json:"endState,omitempty"`
	StartDeviceStatus *ScreenDeceptionDeviceStatus `json:"startDeviceStatus,omitempty"`
	BeforeStopStatus  *ScreenDeceptionDeviceStatus `json:"beforeStopStatus,omitempty"`
	AfterStopStatus   *ScreenDeceptionDeviceStatus `json:"afterStopStatus,omitempty"`
	RawDescriptions   map[string]string            `json:"rawDescriptions,omitempty"`
	QueryErrors       map[string]string            `json:"queryErrors,omitempty"`
	Records           []DeceptionRecord            `json:"records,omitempty"`
	RecordCount       int                          `json:"recordCount"`
}

// DeceptionQueryResponse 返回 GNSS 诱骗设备调试查询结果。
type DeceptionQueryResponse struct {
	Item        string `json:"item"`
	Command     string `json:"command"`
	RawHex      string `json:"rawHex,omitempty"`
	Description string `json:"description,omitempty"`
	Message     string `json:"message"`
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
	ID           string        `json:"id"`
	SessionID    string        `json:"sessionId"`
	PortName     string        `json:"portName"`
	Kind         string        `json:"kind"`
	ReceivedAt   time.Time     `json:"receivedAt"`
	Device       string        `json:"device,omitempty"`
	Model        string        `json:"model,omitempty"`
	DisplayModel string        `json:"displayModel,omitempty"`
	Frequency    float64       `json:"frequency,omitempty"`
	RSSI         float64       `json:"rssi,omitempty"`
	Summary      string        `json:"summary"`
	Parsed       ParsedMessage `json:"parsed"`
}

// ScreenDetectionLastRecord 是大屏公开接口可返回的最近侦测摘要，不包含解析原文。
type ScreenDetectionLastRecord struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ReceivedAt   time.Time `json:"receivedAt"`
	Device       string    `json:"device,omitempty"`
	Model        string    `json:"model,omitempty"`
	DisplayModel string    `json:"displayModel,omitempty"`
	Frequency    float64   `json:"frequency,omitempty"`
	RSSI         float64   `json:"rssi,omitempty"`
	Summary      string    `json:"summary"`
}

// ScreenDetectionTarget 是大屏侦测列表使用的合并目标。
type ScreenDetectionTarget struct {
	ID           string                    `json:"id"`
	Serial       string                    `json:"serial,omitempty"`
	Model        string                    `json:"model"`
	DisplayModel string                    `json:"displayModel,omitempty"`
	Frequency    float64                   `json:"frequency"`
	RSSI         float64                   `json:"rssi"`
	Device       string                    `json:"device"`
	FirstSeen    time.Time                 `json:"firstSeen"`
	LastSeen     time.Time                 `json:"lastSeen"`
	HitCount     int                       `json:"hitCount"`
	LastRecord   ScreenDetectionLastRecord `json:"lastRecord"`
}

// ScreenPositionPoint 描述大屏定位目标中的一个坐标点。
type ScreenPositionPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// ScreenPositionTrackPoint 描述定位目标轨迹中的一个历史坐标点。
type ScreenPositionTrackPoint struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Speed     *float64  `json:"speed,omitempty"`
	Height    *float64  `json:"height,omitempty"`
	Time      time.Time `json:"time"`
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
	ID                 string                     `json:"id"`
	CorrelationID      string                     `json:"correlationId,omitempty"`
	Serial             string                     `json:"serial"`
	Model              string                     `json:"model"`
	Source             string                     `json:"source"`
	Sources            []string                   `json:"sources,omitempty"`
	Frequency          float64                    `json:"frequency,omitempty"`
	RSSI               float64                    `json:"rssi,omitempty"`
	Device             string                     `json:"device"`
	Drone              *ScreenPositionPoint       `json:"drone,omitempty"`
	Pilot              *ScreenPositionPoint       `json:"pilot,omitempty"`
	Home               *ScreenPositionPoint       `json:"home,omitempty"`
	DroneTrajectory    []ScreenPositionTrackPoint `json:"droneTrajectory,omitempty"`
	PilotTrajectory    []ScreenPositionTrackPoint `json:"pilotTrajectory,omitempty"`
	TrajectorySpeed    *float64                   `json:"-"`
	TrajectoryHeight   *float64                   `json:"-"`
	Height             *float64                   `json:"height,omitempty"`
	Altitude           *float64                   `json:"altitude,omitempty"`
	Speed              *float64                   `json:"speed,omitempty"`
	Cracked            bool                       `json:"cracked,omitempty"`
	FirstSeen          time.Time                  `json:"firstSeen"`
	LastSeen           time.Time                  `json:"lastSeen"`
	HitCount           int                        `json:"hitCount"`
	LastRecord         ScreenPositionLastRecord   `json:"lastRecord"`
	PilotDistanceM     *float64                   `json:"pilotDistanceM,omitempty"`
	DroneDistanceM     *float64                   `json:"droneDistanceM,omitempty"`
	DroneDirectionDeg  *float64                   `json:"droneDirectionDeg,omitempty"`
	DeviceDirectionDeg *float64                   `json:"deviceDirectionDeg,omitempty"`
}

// IntrusionTargetType 标识归档目标来自侦测列表还是定位列表。
type IntrusionTargetType string

const (
	IntrusionTargetTypeDetection IntrusionTargetType = "detection"
	IntrusionTargetTypePosition  IntrusionTargetType = "position"
)

// IntrusionRecord 保存一个消失后的目标入侵历史。
type IntrusionRecord struct {
	ID                 string                        `json:"id"`
	TargetID           string                        `json:"targetId"`
	TargetType         IntrusionTargetType           `json:"targetType"`
	Model              string                        `json:"model,omitempty"`
	DisplayModel       string                        `json:"displayModel,omitempty"`
	Serial             string                        `json:"serial,omitempty"`
	Device             string                        `json:"device,omitempty"`
	Frequency          float64                       `json:"frequency,omitempty"`
	RSSI               float64                       `json:"rssi,omitempty"`
	FirstSeen          time.Time                     `json:"firstSeen"`
	LastSeen           time.Time                     `json:"lastSeen"`
	DurationSeconds    int64                         `json:"durationSeconds"`
	HitCount           int                           `json:"hitCount"`
	Source             string                        `json:"source,omitempty"`
	Sources            []string                      `json:"sources,omitempty"`
	Cracked            bool                          `json:"cracked,omitempty"`
	DeviceLocation     *ScreenDeviceLocationResponse `json:"deviceLocation,omitempty"`
	Drone              *ScreenPositionPoint          `json:"drone,omitempty"`
	Pilot              *ScreenPositionPoint          `json:"pilot,omitempty"`
	Home               *ScreenPositionPoint          `json:"home,omitempty"`
	DroneTrajectory    []ScreenPositionTrackPoint    `json:"droneTrajectory,omitempty"`
	PilotTrajectory    []ScreenPositionTrackPoint    `json:"pilotTrajectory,omitempty"`
	PilotDistanceM     *float64                      `json:"pilotDistanceM,omitempty"`
	DroneDistanceM     *float64                      `json:"droneDistanceM,omitempty"`
	DroneDirectionDeg  *float64                      `json:"droneDirectionDeg,omitempty"`
	DeviceDirectionDeg *float64                      `json:"deviceDirectionDeg,omitempty"`
	Height             *float64                      `json:"height,omitempty"`
	Altitude           *float64                      `json:"altitude,omitempty"`
	Speed              *float64                      `json:"speed,omitempty"`
	LastRecord         any                           `json:"lastRecord,omitempty"`
	ArchivedAt         time.Time                     `json:"archivedAt"`
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

// InterferenceReportStatus 描述干扰报告生命周期状态。
type InterferenceReportStatus string

const (
	InterferenceReportStatusRunning   InterferenceReportStatus = "running"
	InterferenceReportStatusCompleted InterferenceReportStatus = "completed"
	InterferenceReportStatusFailed    InterferenceReportStatus = "failed"
	InterferenceReportStatusAbnormal  InterferenceReportStatus = "abnormal"
)

// InterferenceReportSummary 是干扰报告列表使用的摘要记录。
type InterferenceReportSummary struct {
	ID                       string                   `json:"id"`
	Status                   InterferenceReportStatus `json:"status"`
	StartedAt                time.Time                `json:"startedAt"`
	EndedAt                  *time.Time               `json:"endedAt,omitempty"`
	DurationSeconds          int64                    `json:"durationSeconds"`
	RequestedDurationSeconds int                      `json:"requestedDurationSeconds,omitempty"`
	ChannelIDs               []string                 `json:"channelIds,omitempty"`
	ChannelLabels            []string                 `json:"channelLabels,omitempty"`
	ChannelPins              []int                    `json:"channelPins,omitempty"`
	Summary                  string                   `json:"summary,omitempty"`
	LastError                string                   `json:"lastError,omitempty"`
	AbnormalReason           string                   `json:"abnormalReason,omitempty"`
	CreatedAt                time.Time                `json:"createdAt"`
	UpdatedAt                time.Time                `json:"updatedAt"`
}

// InterferenceReport 保存一次大屏干扰操作的证据快照。
type InterferenceReport struct {
	InterferenceReportSummary
	Request    ScreenStrikeRequest `json:"request"`
	StartState *ScreenStrikeState  `json:"startState,omitempty"`
	EndState   *ScreenStrikeState  `json:"endState,omitempty"`
}

// ScreenDeceptionRequest 控制大屏诱骗面板绑定的 GNSS 诱骗设备。
type ScreenDeceptionRequest struct {
	Enabled        bool                         `json:"enabled"`
	TargetID       string                       `json:"targetId,omitempty"`
	Mode           string                       `json:"mode,omitempty"`
	Longitude      *float64                     `json:"longitude,omitempty"`
	Latitude       *float64                     `json:"latitude,omitempty"`
	AltitudeM      *float64                     `json:"altitudeM,omitempty"`
	SignalMask     *uint16                      `json:"signalMask,omitempty"`
	StrengthPreset string                       `json:"strengthPreset,omitempty"`
	AttenuationDB  *int                         `json:"attenuationDB,omitempty"`
	DelayMode      string                       `json:"delayMode,omitempty"`
	DelayNS        *float64                     `json:"delayNS,omitempty"`
	Circle         *ScreenDeceptionCircleParams `json:"circle,omitempty"`
	Linear         *ScreenDeceptionLinearParams `json:"linear,omitempty"`
	Random         *ScreenDeceptionRandomParams `json:"random,omitempty"`
}

// ScreenDeceptionCircleParams 描述圆周诱骗参数。
type ScreenDeceptionCircleParams struct {
	RadiusM       float64 `json:"radiusM,omitempty"`
	PeriodSeconds float64 `json:"periodSeconds,omitempty"`
	Direction     string  `json:"direction,omitempty"`
}

// ScreenDeceptionLinearParams 描述线性诱骗参数。
type ScreenDeceptionLinearParams struct {
	SpeedMPS     float64  `json:"speedMps,omitempty"`
	DirectionDeg *float64 `json:"directionDeg,omitempty"`
	MaxSpeedMPS  float64  `json:"maxSpeedMps,omitempty"`
}

// ScreenDeceptionRandomParams 描述固定位置模式下的随机坐标开关。
type ScreenDeceptionRandomParams struct {
	Enabled        bool   `json:"enabled"`
	RadiusM        uint32 `json:"radiusM,omitempty"`
	RefreshSeconds uint32 `json:"refreshSeconds,omitempty"`
}

// ScreenDeceptionState 描述大屏诱骗控制当前状态。
type ScreenDeceptionState struct {
	Active            bool                         `json:"active"`
	TargetID          string                       `json:"targetId,omitempty"`
	Mode              string                       `json:"mode,omitempty"`
	Point             *GeoPoint                    `json:"point,omitempty"`
	AltitudeM         float64                      `json:"altitudeM,omitempty"`
	SignalMask        uint16                       `json:"signalMask,omitempty"`
	StrengthPreset    string                       `json:"strengthPreset,omitempty"`
	AttenuationDB     int                          `json:"attenuationDB,omitempty"`
	DelayMode         string                       `json:"delayMode,omitempty"`
	DelayNS           float64                      `json:"delayNS,omitempty"`
	DistanceM         float64                      `json:"distanceM,omitempty"`
	Summary           string                       `json:"summary,omitempty"`
	UnsupportedReason string                       `json:"unsupportedReason,omitempty"`
	Circle            *ScreenDeceptionCircleParams `json:"circle,omitempty"`
	Linear            *ScreenDeceptionLinearParams `json:"linear,omitempty"`
	Random            *ScreenDeceptionRandomParams `json:"random,omitempty"`
	SerialActive      bool                         `json:"serialActive"`
	LastAck           string                       `json:"lastAck,omitempty"`
	LastError         string                       `json:"lastError,omitempty"`
}

// ScreenDeceptionStatusPoint 描述诱骗设备状态中的经纬高坐标。
type ScreenDeceptionStatusPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AltitudeM float64 `json:"altitudeM"`
}

// ScreenDeceptionVersionStatus 描述诱骗设备版本信息。
type ScreenDeceptionVersionStatus struct {
	Software string `json:"software,omitempty"`
	FPGA     string `json:"fpga,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// ScreenDeceptionTargetStatus 描述目标位置查询状态。
type ScreenDeceptionTargetStatus struct {
	DistanceM    int32   `json:"distanceM"`
	HeightM      int32   `json:"heightM"`
	DirectionDeg float64 `json:"directionDeg"`
	HeadingDeg   float64 `json:"headingDeg"`
}

// ScreenDeceptionSpoofCircleStatus 描述诱骗圆周运动查询状态。
type ScreenDeceptionSpoofCircleStatus struct {
	DistanceM     int32   `json:"distanceM"`
	HeightM       int32   `json:"heightM"`
	DirectionDeg  float64 `json:"directionDeg"`
	HeadingDeg    float64 `json:"headingDeg"`
	RadiusM       float64 `json:"radiusM"`
	PeriodSeconds float64 `json:"periodSeconds"`
	Direction     string  `json:"direction,omitempty"`
}

// ScreenDeceptionSuppressionStatus 描述压制/干扰发射查询状态。
type ScreenDeceptionSuppressionStatus struct {
	WaveformMask int32 `json:"waveformMask"`
	TransmitOn   bool  `json:"transmitOn"`
}

// ScreenDeceptionRandomStatus 描述随机坐标查询状态。
type ScreenDeceptionRandomStatus struct {
	Enabled        bool   `json:"enabled"`
	RadiusM        uint32 `json:"radiusM"`
	RefreshSeconds uint32 `json:"refreshSeconds"`
}

// ScreenDeceptionSyncStatus 描述诱骗设备授时同步状态 bit。
type ScreenDeceptionSyncStatus struct {
	ReceiverWorking    bool `json:"receiverWorking"`
	ReceiverPositioned bool `json:"receiverPositioned"`
	LeapSecondValid    bool `json:"leapSecondValid"`
	TimeSynced         bool `json:"timeSynced"`
	AntennaOK          bool `json:"antennaOk"`
}

// ScreenDeceptionMotionStatus 描述诱骗设备当前运动参数。
type ScreenDeceptionMotionStatus struct {
	MaxSpeedMPS              *float64 `json:"maxSpeedMps,omitempty"`
	InitialSpeedMPS          *float64 `json:"initialSpeedMps,omitempty"`
	InitialDirectionDeg      *float64 `json:"initialDirectionDeg,omitempty"`
	AccelerationMPS2         *float64 `json:"accelerationMps2,omitempty"`
	AccelerationDirectionDeg *float64 `json:"accelerationDirectionDeg,omitempty"`
	CircleRadiusM            *float64 `json:"circleRadiusM,omitempty"`
	CirclePeriodSeconds      *float64 `json:"circlePeriodSeconds,omitempty"`
	CircleDirection          string   `json:"circleDirection,omitempty"`
}

// ScreenDeceptionAttenuationStatus 描述各星座功率衰减。
type ScreenDeceptionAttenuationStatus struct {
	GPS int `json:"gps"`
	BDS int `json:"bds"`
	GLO int `json:"glo"`
	GAL int `json:"gal"`
}

// ScreenDeceptionDelayStatus 描述各星座信号时延。
type ScreenDeceptionDelayStatus struct {
	GPS *float64 `json:"gps,omitempty"`
	BDS *float64 `json:"bds,omitempty"`
	GLO *float64 `json:"glo,omitempty"`
	GAL *float64 `json:"gal,omitempty"`
}

// ScreenDeceptionSignalWorkStatus 描述设备伪卫星信号工作状态 bit。
type ScreenDeceptionSignalWorkStatus struct {
	ClockOK         bool `json:"clockOk"`
	EphemerisValid  bool `json:"ephemerisValid"`
	RFModuleOK      bool `json:"rfModuleOk"`
	SignalTransmit  bool `json:"signalTransmit"`
	TransmitChannel bool `json:"transmitChannel"`
	FPGAOK          bool `json:"fpgaOk"`
	Raw             byte `json:"raw"`
}

// ScreenDeceptionDeviceSignalStatus 描述查询 0x5D 返回的设备伪卫星状态。
type ScreenDeceptionDeviceSignalStatus struct {
	SystemTime             *time.Time                      `json:"systemTime,omitempty"`
	SignalMask             uint16                          `json:"signalMask"`
	SignalNames            []string                        `json:"signalNames,omitempty"`
	DelayNS                float64                         `json:"delayNs"`
	WorkStatus             ScreenDeceptionSignalWorkStatus `json:"workStatus"`
	TransmitSwitch         bool                            `json:"transmitSwitch"`
	AttenuationDB          int                             `json:"attenuationDb"`
	ReceivedSatelliteCount int                             `json:"receivedSatelliteCount"`
	ReceivedPRNs           []int                           `json:"receivedPrns,omitempty"`
	ReceivedCN0            []int                           `json:"receivedCn0,omitempty"`
	TransmittedCount       int                             `json:"transmittedCount"`
	TransmittedPRNs        []int                           `json:"transmittedPrns,omitempty"`
}

// ScreenDeceptionDeviceStatus 返回大屏诱骗设备完整只读状态。
type ScreenDeceptionDeviceStatus struct {
	SerialActive             bool                                `json:"serialActive"`
	UpdatedAt                *time.Time                          `json:"updatedAt,omitempty"`
	SystemTime               *time.Time                          `json:"systemTime,omitempty"`
	ReportedSystemTime       *time.Time                          `json:"reportedSystemTime,omitempty"`
	Version                  *ScreenDeceptionVersionStatus       `json:"version,omitempty"`
	TransmitMask             *uint16                             `json:"transmitMask,omitempty"`
	TransmitSignals          []string                            `json:"transmitSignals,omitempty"`
	AmplifierOn              *bool                               `json:"amplifierOn,omitempty"`
	AutoTransmit             *bool                               `json:"autoTransmit,omitempty"`
	FirstTimeSynced          *bool                               `json:"firstTimeSynced,omitempty"`
	OscillatorState          string                              `json:"oscillatorState,omitempty"`
	SyncStatus               *ScreenDeceptionSyncStatus          `json:"syncStatus,omitempty"`
	CurrentPosition          *ScreenDeceptionStatusPoint         `json:"currentPosition,omitempty"`
	SimulatedPosition        *ScreenDeceptionStatusPoint         `json:"simulatedPosition,omitempty"`
	QueriedDevicePosition    *ScreenDeceptionStatusPoint         `json:"queriedDevicePosition,omitempty"`
	QueriedSimulatedPosition *ScreenDeceptionStatusPoint         `json:"queriedSimulatedPosition,omitempty"`
	TargetPosition           *ScreenDeceptionTargetStatus        `json:"targetPosition,omitempty"`
	TemperatureC             *float64                            `json:"temperatureC,omitempty"`
	TimePrecisionNS          *float64                            `json:"timePrecisionNs,omitempty"`
	UptimeSeconds            *uint32                             `json:"uptimeSeconds,omitempty"`
	Motion                   *ScreenDeceptionMotionStatus        `json:"motion,omitempty"`
	Attenuation              *ScreenDeceptionAttenuationStatus   `json:"attenuation,omitempty"`
	DelayNS                  *float64                            `json:"delayNS,omitempty"`
	DelayBySignalNS          *ScreenDeceptionDelayStatus         `json:"delayBySignalNs,omitempty"`
	SpoofCircle              *ScreenDeceptionSpoofCircleStatus   `json:"spoofCircle,omitempty"`
	Suppression              *ScreenDeceptionSuppressionStatus   `json:"suppression,omitempty"`
	Random                   *ScreenDeceptionRandomStatus        `json:"random,omitempty"`
	TimedSearch              *bool                               `json:"timedSearch,omitempty"`
	DeviceSignal             *ScreenDeceptionDeviceSignalStatus  `json:"deviceSignal,omitempty"`
	DeviceSignals            []ScreenDeceptionDeviceSignalStatus `json:"deviceSignals,omitempty"`
	RawDescriptions          map[string]string                   `json:"rawDescriptions"`
	QueryErrors              map[string]string                   `json:"queryErrors,omitempty"`
	LastError                string                              `json:"lastError,omitempty"`
}

// ScreenDeceptionResponse 返回大屏诱骗控制状态和用户提示。
type ScreenDeceptionResponse struct {
	State   ScreenDeceptionState `json:"state"`
	Message string               `json:"message"`
}

// DeceptionReportDeleteResponse 返回诱骗报告删除数量。
type DeceptionReportDeleteResponse struct {
	Deleted int64 `json:"deleted"`
}

// InterferenceReportDeleteResponse 返回干扰报告删除数量。
type InterferenceReportDeleteResponse struct {
	Deleted int64 `json:"deleted"`
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

// ListResponse 包装列表接口响应，并附带条目数量和后续批次游标。
type ListResponse[T any] struct {
	Items      []T  `json:"items"`
	Count      int  `json:"count"`
	HasMore    bool `json:"hasMore,omitempty"`
	NextOffset int  `json:"nextOffset,omitempty"`
}

// ApiError 是标准 JSON 错误响应。
type ApiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
