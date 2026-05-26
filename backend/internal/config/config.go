// Package config 从环境变量加载后端运行配置。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 保存后端 API 和服务运行配置。
type Config struct {
	Addr                     string
	DefaultLocale            string
	SettingsPath             string
	IntrusionDBPath          string
	DeceptionReportDBPath    string
	InterferenceReportDBPath string
	OfflineMapPath           string
	DetectionDefaultBaud     int
	DefaultBaudRate          int
	DefaultDataBits          int
	DefaultStopBits          int
	DefaultParity            string
	DefaultReadTimeout       time.Duration
	ReconnectInitialDelay    time.Duration
	ReconnectMaxDelay        time.Duration
	MaxDetectionRecords      int
	MaxParsedMessages        int
	EventBufferSize          int
	DeveloperTOTPSecret      string
	DeveloperSessionTTL      time.Duration
	CORSAllowedOrigins       []string
	O3Decrypt                O3DecryptConfig
}

// O3DecryptConfig 保存 O3+/O4 MQTT 解密配置。
type O3DecryptConfig struct {
	Enabled        bool
	Broker         string
	Port           int
	Username       string
	Password       string
	Timeout        time.Duration
	ConnectTimeout time.Duration
}

// Load 读取环境变量，并返回带默认值的配置。
func Load() Config {
	return Config{
		Addr:                     envString("API_ADDR", ":18080"),
		DefaultLocale:            envString("API_DEFAULT_LOCALE", "zh-CN"),
		SettingsPath:             envString("API_SETTINGS_PATH", "./backend/data/detection-settings.json"),
		IntrusionDBPath:          envString("API_INTRUSION_DB_PATH", "./backend/data/intrusions.db"),
		DeceptionReportDBPath:    envString("API_DECEPTION_REPORT_DB_PATH", "./backend/data/deception-reports.db"),
		InterferenceReportDBPath: envString("API_INTERFERENCE_REPORT_DB_PATH", "./backend/data/interference-reports.db"),
		OfflineMapPath:           envString("API_OFFLINE_MAP_PATH", "./static/map"),
		DetectionDefaultBaud:     envInt("API_DETECTION_DEFAULT_BAUD", 460800),
		DefaultBaudRate:          envInt("API_DEFAULT_BAUD", 115200),
		DefaultDataBits:          envInt("API_DEFAULT_DATA_BITS", 8),
		DefaultStopBits:          envInt("API_DEFAULT_STOP_BITS", 1),
		DefaultParity:            envString("API_DEFAULT_PARITY", "none"),
		DefaultReadTimeout:       time.Duration(envInt("API_DEFAULT_READ_TIMEOUT_MS", 1000)) * time.Millisecond,
		ReconnectInitialDelay:    time.Duration(envInt("API_RECONNECT_INITIAL_DELAY_MS", 1000)) * time.Millisecond,
		ReconnectMaxDelay:        time.Duration(envInt("API_RECONNECT_MAX_DELAY_MS", 15000)) * time.Millisecond,
		MaxDetectionRecords:      envInt("API_MAX_DETECTION_RECORDS", 500),
		MaxParsedMessages:        envInt("API_MAX_PARSED_MESSAGES", 500),
		EventBufferSize:          envInt("API_EVENT_BUFFER_SIZE", 64),
		DeveloperTOTPSecret:      envString("API_DEVELOPER_TOTP_SECRET", "VUPSQXCB6U5WPBKOKTQ6WICVWHAUX2S4"),
		DeveloperSessionTTL:      time.Duration(envInt("API_DEVELOPER_SESSION_TTL_SECONDS", 600)) * time.Second,
		CORSAllowedOrigins:       envList("API_CORS_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173"}),
		O3Decrypt: O3DecryptConfig{
			Enabled:        envBool("API_O3_DECRYPT_ENABLED", true),
			Broker:         envString("API_O3_DECRYPT_BROKER", "101.36.159.2"),
			Port:           envInt("API_O3_DECRYPT_PORT", 1883),
			Username:       envString("API_O3_DECRYPT_USERNAME", "zkzp"),
			Password:       envString("API_O3_DECRYPT_PASSWORD", "Zkzp123456.."),
			Timeout:        time.Duration(envInt("API_O3_DECRYPT_TIMEOUT_MS", 10000)) * time.Millisecond,
			ConnectTimeout: time.Duration(envInt("API_O3_DECRYPT_CONNECT_TIMEOUT_MS", 10000)) * time.Millisecond,
		},
	}
}

// envString 返回去除空白后的环境变量值，空值时返回默认值。
func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// envInt 解析整数环境变量，失败时返回默认值。
func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// envBool 解析布尔环境变量，空值或无法识别时返回默认值。
func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

// envList 解析逗号分隔的环境变量列表，空列表时返回默认值。
func envList(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			result = append(result, v)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}
