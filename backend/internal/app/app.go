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

type App struct {
	server *httpapi.Server
}

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

func (a *App) Listen(addr string) error {
	return a.server.Listen(addr)
}

func (a *App) Shutdown() error {
	return a.server.Shutdown()
}
