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
	IsFPV      bool          `json:"isFpv"`
	FPVBand    string        `json:"fpvBand,omitempty"`
}

// FpvRecord 保存被识别为图传信号的侦测记录。
type FpvRecord struct {
	ID          string    `json:"id"`
	DetectionID string    `json:"detectionId"`
	Band        string    `json:"band"`
	Label       string    `json:"label"`
	PortName    string    `json:"portName"`
	Device      string    `json:"device,omitempty"`
	Model       string    `json:"model,omitempty"`
	Frequency   float64   `json:"frequency"`
	RSSI        float64   `json:"rssi"`
	ReceivedAt  time.Time `json:"receivedAt"`
	SourceKind  string    `json:"sourceKind"`
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
