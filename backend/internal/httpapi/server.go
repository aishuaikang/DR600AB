// Package httpapi 将后端服务接入 HTTP 和服务端事件 API。
package httpapi

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/config"
	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/deceptionreport"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/gps"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/network"
)

// UserSettingsStore 持久化公开用户设置。
type UserSettingsStore interface {
	LoadUser() (model.UserSettings, bool, error)
	SaveUser(model.UserSettings) error
	SaveEditableUser(model.UserSettings) (model.UserSettings, error)
}

// IntrusionStore 查询已归档的目标入侵记录。
type IntrusionStore interface {
	List(intrusion.QueryOptions) ([]model.IntrusionRecord, error)
	Delete([]string) (int64, error)
	PruneRetention(days int, now time.Time) (int64, error)
	Close() error
}

// DeceptionReportStore 查询已归档的诱骗报告。
type DeceptionReportStore interface {
	List(deceptionreport.QueryOptions) ([]model.DeceptionReportSummary, error)
	Get(string) (model.DeceptionReport, error)
	DeleteFailed(string) (int64, error)
	CloseRunning(reason string, now time.Time) (int64, error)
	Close() error
}

type intrusionDeviceLocationSetter interface {
	SetDeviceLocationProvider(intrusion.DeviceLocationProvider)
}

type screenPositionRelationSetter interface {
	SetDeviceLocationProvider(func() *model.ScreenDeviceLocationResponse)
}

// Server 持有 Fiber 应用以及对外暴露的后端服务。
type Server struct {
	app          *fiber.App
	cfg          config.Config
	translator   *i18n.Translator
	detection    *detection.Service
	interference *interference.Service
	developer    *developer.Service
	gps          *gps.Service
	network      *network.Service
	deception    *deception.Service
	userSettings UserSettingsStore
	intrusions   IntrusionStore
	reports      DeceptionReportStore

	intrusionPruneMu      sync.Mutex
	lastIntrusionPruneRun time.Time
}

// New 创建 Server，并注册中间件和 API 路由。
func New(
	cfg config.Config,
	translator *i18n.Translator,
	detectionSvc *detection.Service,
	interferenceSvc *interference.Service,
	developerSvc *developer.Service,
	gpsSvc *gps.Service,
	networkSvc *network.Service,
	deceptionSvc *deception.Service,
	userSettingsStore UserSettingsStore,
	intrusionStore IntrusionStore,
	reportStore DeceptionReportStore,
) *Server {
	s := &Server{
		cfg:          cfg,
		translator:   translator,
		detection:    detectionSvc,
		interference: interferenceSvc,
		developer:    developerSvc,
		gps:          gpsSvc,
		network:      networkSvc,
		deception:    deceptionSvc,
		userSettings: userSettingsStore,
		intrusions:   intrusionStore,
		reports:      reportStore,
	}
	s.app = fiber.New(fiber.Config{
		AppName: "dr600ab-api",
	})
	if setter, ok := intrusionStore.(intrusionDeviceLocationSetter); ok {
		setter.SetDeviceLocationProvider(func() *model.ScreenDeviceLocationResponse {
			location, err := s.currentScreenDeviceLocation()
			if err != nil || !location.Valid {
				return nil
			}
			return &location
		})
	}
	var screenStore any
	if detectionSvc != nil {
		screenStore = detectionSvc.Store()
	}
	if setter, ok := screenStore.(screenPositionRelationSetter); ok {
		setter.SetDeviceLocationProvider(func() *model.ScreenDeviceLocationResponse {
			location, err := s.currentScreenDeviceLocation()
			if err != nil || !location.Valid {
				return nil
			}
			return &location
		})
	}
	s.routes()
	return s
}

// Listen 在指定地址启动 HTTP 服务。
func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

// Shutdown 关闭运行中的服务并停止 HTTP 服务。
func (s *Server) Shutdown() error {
	s.detection.Stop("")
	s.gps.Stop("")
	s.interference.Shutdown()
	s.deception.Shutdown()
	if s.intrusions != nil {
		_ = s.intrusions.Close()
	}
	if s.reports != nil {
		_, _ = s.reports.CloseRunning("service_shutdown", time.Now())
		_ = s.reports.Close()
	}
	return s.app.Shutdown()
}
