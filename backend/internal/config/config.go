package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                  string
	DefaultLocale         string
	SettingsPath          string
	DefaultBaudRate       int
	DefaultDataBits       int
	DefaultStopBits       int
	DefaultParity         string
	DefaultReadTimeout    time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	MaxDetectionRecords   int
	MaxParsedMessages     int
	MaxFPVRecords         int
	EventBufferSize       int
	CORSAllowedOrigins    []string
}

func Load() Config {
	return Config{
		Addr:                  envString("API_ADDR", ":18080"),
		DefaultLocale:         envString("API_DEFAULT_LOCALE", "zh-CN"),
		SettingsPath:          envString("API_SETTINGS_PATH", "./backend/data/detection-settings.json"),
		DefaultBaudRate:       envInt("API_DEFAULT_BAUD", 115200),
		DefaultDataBits:       envInt("API_DEFAULT_DATA_BITS", 8),
		DefaultStopBits:       envInt("API_DEFAULT_STOP_BITS", 1),
		DefaultParity:         envString("API_DEFAULT_PARITY", "none"),
		DefaultReadTimeout:    time.Duration(envInt("API_DEFAULT_READ_TIMEOUT_MS", 1000)) * time.Millisecond,
		ReconnectInitialDelay: time.Duration(envInt("API_RECONNECT_INITIAL_DELAY_MS", 1000)) * time.Millisecond,
		ReconnectMaxDelay:     time.Duration(envInt("API_RECONNECT_MAX_DELAY_MS", 15000)) * time.Millisecond,
		MaxDetectionRecords:   envInt("API_MAX_DETECTION_RECORDS", 500),
		MaxParsedMessages:     envInt("API_MAX_PARSED_MESSAGES", 500),
		MaxFPVRecords:         envInt("API_MAX_FPV_RECORDS", 300),
		EventBufferSize:       envInt("API_EVENT_BUFFER_SIZE", 64),
		CORSAllowedOrigins:    envList("API_CORS_ORIGINS", []string{"http://localhost:5173", "http://127.0.0.1:5173"}),
	}
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

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
