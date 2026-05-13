// Package app 负责组装后端服务为可运行应用。
package app

import (
	"dr600ab-api/internal/config"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/httpapi"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/settings"
	"dr600ab-api/internal/store"
)

// App 聚合后端 HTTP 服务及其运行依赖。
type App struct {
	server *httpapi.Server
}

// New 根据配置创建应用，并恢复已保存的侦测设置。
func New(cfg config.Config) (*App, error) {
	translator, err := i18n.New(cfg.DefaultLocale)
	if err != nil {
		return nil, err
	}

	state := store.NewMemoryStore(cfg.MaxDetectionRecords, cfg.MaxParsedMessages, cfg.MaxFPVRecords)
	settingsStore := settings.NewStore(cfg.SettingsPath)
	detectionSvc := detection.NewService(state, translator, settingsStore, detection.Options{
		DefaultBaudRate:       cfg.DefaultBaudRate,
		DefaultDataBits:       cfg.DefaultDataBits,
		DefaultStopBits:       cfg.DefaultStopBits,
		DefaultParity:         cfg.DefaultParity,
		DefaultReadTimeout:    cfg.DefaultReadTimeout,
		ReconnectInitialDelay: cfg.ReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
	})
	interferenceSvc := interference.NewService(state, translator, interference.DefaultChannels(), nil)

	detectionSvc.RestoreSavedSettings(cfg.DefaultLocale)

	return &App{
		server: httpapi.New(cfg, translator, detectionSvc, interferenceSvc),
	}, nil
}

// Listen 启动 HTTP API 服务。
func (a *App) Listen(addr string) error {
	return a.server.Listen(addr)
}

// Shutdown 停止 HTTP API 并释放服务资源。
func (a *App) Shutdown() error {
	return a.server.Shutdown()
}
