package model

import (
	"encoding/json"
	"time"
)

type LocaleMeta struct {
	Default    string   `json:"defaultLocale"`
	Supported  []string `json:"supportedLocales"`
	Namespaces []string `json:"namespaces"`
}

type PortInfo struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

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

type ParsedMessage struct {
	Type string          `json:"type"`
	Time time.Time       `json:"time"`
	Raw  string          `json:"raw"`
	Data json.RawMessage `json:"data"`
}

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

type GpioChannelStateRequest struct {
	Enabled bool `json:"enabled"`
}

type GpioChannelStateResponse struct {
	Channel GpioChannel `json:"channel"`
	Message string      `json:"message"`
}

type Event struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Payload any       `json:"payload,omitempty"`
}

type ListResponse[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

type ApiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}
