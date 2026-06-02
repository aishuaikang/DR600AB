// Package app 负责组装后端服务为可运行应用。
package app

import (
	"context"
	"time"

	"dr600ab-api/internal/compass"
	"dr600ab-api/internal/config"
	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/deceptionreport"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/fpv"
	"dr600ab-api/internal/gps"
	"dr600ab-api/internal/httpapi"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/interferencereport"
	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/network"
	"dr600ab-api/internal/settings"
	"dr600ab-api/internal/store"
)

// App 聚合后端 HTTP 服务及其运行依赖。
type App struct {
	server    *httpapi.Server
	fpvCancel context.CancelFunc
	fpvDone   chan struct{}
}

// New 根据配置创建应用，并恢复已保存的串口设置。
func New(cfg config.Config) (*App, error) {
	translator, err := i18n.New(cfg.DefaultLocale)
	if err != nil {
		return nil, err
	}

	state := store.NewMemoryStore(cfg.MaxDetectionRecords, cfg.MaxParsedMessages)
	settingsStore := settings.NewStore(cfg.SettingsPath)
	intrusionStore, err := intrusion.NewStore(cfg.IntrusionDBPath, intrusion.Options{DBKey: cfg.DBKey})
	if err != nil {
		return nil, err
	}
	reportStore, err := deceptionreport.NewStore(cfg.DeceptionReportDBPath, deceptionreport.Options{DBKey: cfg.DBKey})
	if err != nil {
		_ = intrusionStore.Close()
		return nil, err
	}
	interferenceReportStore, err := interferencereport.NewStore(cfg.InterferenceReportDBPath, interferencereport.Options{DBKey: cfg.DBKey})
	if err != nil {
		_ = intrusionStore.Close()
		_ = reportStore.Close()
		return nil, err
	}
	if _, err := reportStore.CloseRunning("abnormal_restart", time.Now()); err != nil {
		_ = intrusionStore.Close()
		_ = reportStore.Close()
		_ = interferenceReportStore.Close()
		return nil, err
	}
	if _, err := interferenceReportStore.CloseRunning("abnormal_restart", time.Now()); err != nil {
		_ = intrusionStore.Close()
		_ = reportStore.Close()
		_ = interferenceReportStore.Close()
		return nil, err
	}
	state.SetIntrusionArchiver(intrusionStore)
	detectionSvc := detection.NewService(state, translator, settingsStore, detection.Options{
		DefaultBaudRate:       cfg.DetectionDefaultBaud,
		DefaultRxBaudRate:     cfg.DetectionDefaultRxBaud,
		DefaultTxBaudRate:     cfg.DetectionDefaultTxBaud,
		DefaultDataBits:       cfg.DefaultDataBits,
		DefaultStopBits:       cfg.DefaultStopBits,
		DefaultParity:         cfg.DefaultParity,
		DefaultReadTimeout:    cfg.DefaultReadTimeout,
		ReconnectInitialDelay: cfg.ReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
		O3Decrypt: detection.O3DecryptOptions{
			Enabled:        cfg.O3Decrypt.Enabled,
			Broker:         cfg.O3Decrypt.Broker,
			Port:           cfg.O3Decrypt.Port,
			Username:       cfg.O3Decrypt.Username,
			Password:       cfg.O3Decrypt.Password,
			Timeout:        cfg.O3Decrypt.Timeout,
			ConnectTimeout: cfg.O3Decrypt.ConnectTimeout,
		},
	})
	interferenceSvc := interference.NewService(state, translator, interference.DefaultChannels(), nil)
	interferenceSvc.SetReportStore(interferenceReportStore)
	interferenceSvc.SetUserSettingsStore(settingsStore)
	developerSvc, err := developer.NewService(cfg.DeveloperTOTPSecret, cfg.DeveloperSessionTTL)
	if err != nil {
		_ = intrusionStore.Close()
		_ = reportStore.Close()
		_ = interferenceReportStore.Close()
		return nil, err
	}
	gpsSvc := gps.NewService(state, translator, settingsStore, gps.Options{
		DefaultBaudRate:       cfg.DefaultBaudRate,
		DefaultDataBits:       cfg.DefaultDataBits,
		DefaultStopBits:       cfg.DefaultStopBits,
		DefaultParity:         cfg.DefaultParity,
		DefaultReadTimeout:    cfg.DefaultReadTimeout,
		ReconnectInitialDelay: cfg.ReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
	})
	networkSvc := network.NewService(nil, settingsStore)
	deceptionSvc := deception.NewService(state, translator, settingsStore, deception.Options{
		DefaultBaudRate:       cfg.DefaultBaudRate,
		DefaultDataBits:       cfg.DefaultDataBits,
		DefaultStopBits:       cfg.DefaultStopBits,
		DefaultParity:         cfg.DefaultParity,
		DefaultReadTimeout:    cfg.DefaultReadTimeout,
		ReconnectInitialDelay: cfg.ReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
	})
	deceptionSvc.SetReportStore(reportStore)
	compassSvc := compass.NewService(state, translator, settingsStore, compass.Options{
		DefaultBaudRate:       compass.DefaultBaudRate,
		DefaultDataBits:       cfg.DefaultDataBits,
		DefaultStopBits:       cfg.DefaultStopBits,
		DefaultParity:         cfg.DefaultParity,
		DefaultReadTimeout:    cfg.DefaultReadTimeout,
		ReconnectInitialDelay: cfg.ReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
	})
	fpvSvc := fpv.NewService(fpv.Options{
		Host:              cfg.FPVTCPHost,
		Port:              cfg.FPVTCPPort,
		BindRetryInterval: cfg.FPVBindRetryInterval,
		MaxFrameBytes:     cfg.FPVMaxFrameBytes,
		FirstFrameTimeout: cfg.FPVFirstFrameTimeout,
		ReadIdleTimeout:   cfg.FPVReadIdleTimeout,
	})
	fpvCtx, fpvCancel := context.WithCancel(context.Background())
	fpvDone := make(chan struct{})
	go func() {
		defer close(fpvDone)
		fpvSvc.Run(fpvCtx)
	}()

	detectionSvc.RestoreSavedSettings(cfg.DefaultLocale)
	gpsSvc.RestoreSavedSettings(cfg.DefaultLocale)
	deceptionSvc.RestoreSavedSettings(cfg.DefaultLocale)
	compassSvc.RestoreSavedSettings(cfg.DefaultLocale)
	_ = networkSvc.RestoreSavedSettings(context.Background())

	return &App{
		server: httpapi.New(
			cfg,
			translator,
			detectionSvc,
			interferenceSvc,
			developerSvc,
			gpsSvc,
			networkSvc,
			deceptionSvc,
			compassSvc,
			settingsStore,
			intrusionStore,
			reportStore,
			interferenceReportStore,
			fpvSvc,
		),
		fpvCancel: fpvCancel,
		fpvDone:   fpvDone,
	}, nil
}

// Listen 启动 HTTP API 服务。
func (a *App) Listen(addr string) error {
	return a.server.Listen(addr)
}

// Shutdown 停止 HTTP API 并释放服务资源。
func (a *App) Shutdown() error {
	if a.fpvCancel != nil {
		a.fpvCancel()
	}
	if a.fpvDone != nil {
		<-a.fpvDone
	}
	return a.server.Shutdown()
}
