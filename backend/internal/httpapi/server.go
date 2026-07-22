// Package httpapi 将后端服务接入 HTTP 和服务端事件 API。
package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"dr600ab-api/internal/compass"
	"dr600ab-api/internal/config"
	"dr600ab-api/internal/deception"
	"dr600ab-api/internal/deceptionreport"
	"dr600ab-api/internal/detection"
	"dr600ab-api/internal/developer"
	"dr600ab-api/internal/fpv"
	"dr600ab-api/internal/fpvrecord"
	"dr600ab-api/internal/gps"
	"dr600ab-api/internal/i18n"
	"dr600ab-api/internal/interference"
	"dr600ab-api/internal/interferencereport"
	"dr600ab-api/internal/intrusion"
	"dr600ab-api/internal/license"
	"dr600ab-api/internal/model"
	"dr600ab-api/internal/network"
	"dr600ab-api/internal/systemtime"
)

// UserSettingsStore 持久化公开用户设置。
type UserSettingsStore interface {
	LoadUser() (model.UserSettings, bool, error)
	SaveUser(model.UserSettings) error
	SaveEditableUser(model.UserSettings) (model.UserSettings, error)
}

// SystemTimeService 提供本机系统时间管理能力。
type SystemTimeService interface {
	GetInfo(context.Context) (systemtime.Info, error)
	ListTimezones(context.Context) ([]string, error)
	SetTimezone(context.Context, string) error
	SetNTPEnabled(context.Context, bool) error
	SetManualTime(context.Context, string) error
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

// InterferenceReportStore 查询已归档的干扰报告。
type InterferenceReportStore interface {
	List(interferencereport.QueryOptions) ([]model.InterferenceReportSummary, error)
	Get(string) (model.InterferenceReport, error)
	DeleteFailed(string) (int64, error)
	CloseRunning(reason string, now time.Time) (int64, error)
	Close() error
}

// FPVVideoRecordStore 查询已归档的 FPV 图传观看记录。
type FPVVideoRecordStore interface {
	Insert(model.FPVVideoRecord) error
	List(fpvrecord.QueryOptions) ([]model.FPVVideoRecord, error)
	Get(string) (model.FPVVideoRecord, bool, error)
	Delete([]string) (int64, error)
	PruneRetention(days int, now time.Time) (int64, error)
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
	app                 *fiber.App
	cfg                 config.Config
	translator          *i18n.Translator
	detection           *detection.Service
	interference        *interference.Service
	developer           *developer.Service
	gps                 *gps.Service
	network             *network.Service
	systemTime          SystemTimeService
	deception           *deception.Service
	compass             *compass.Service
	fpv                 *fpv.Service
	userSettings        UserSettingsStore
	intrusions          IntrusionStore
	reports             DeceptionReportStore
	interferenceReports InterferenceReportStore
	fpvRecords          FPVVideoRecordStore
	license             *license.Service

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
	systemTimeSvc SystemTimeService,
	deceptionSvc *deception.Service,
	compassSvc *compass.Service,
	userSettingsStore UserSettingsStore,
	intrusionStore IntrusionStore,
	reportStore DeceptionReportStore,
	interferenceReportStore InterferenceReportStore,
	fpvRecordStore FPVVideoRecordStore,
	fpvSvc *fpv.Service,
	licenseSvc *license.Service,
) *Server {
	s := &Server{
		cfg:                 cfg,
		translator:          translator,
		detection:           detectionSvc,
		interference:        interferenceSvc,
		developer:           developerSvc,
		gps:                 gpsSvc,
		network:             networkSvc,
		systemTime:          systemTimeSvc,
		deception:           deceptionSvc,
		compass:             compassSvc,
		fpv:                 fpvSvc,
		userSettings:        userSettingsStore,
		intrusions:          intrusionStore,
		reports:             reportStore,
		interferenceReports: interferenceReportStore,
		fpvRecords:          fpvRecordStore,
		license:             licenseSvc,
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
	s.compass.Shutdown()
	if s.intrusions != nil {
		_ = s.intrusions.Close()
	}
	if s.reports != nil {
		_, _ = s.reports.CloseRunning("service_shutdown", time.Now())
		_ = s.reports.Close()
	}
	if s.interferenceReports != nil {
		_, _ = s.interferenceReports.CloseRunning("service_shutdown", time.Now())
		_ = s.interferenceReports.Close()
	}
	if s.fpvRecords != nil {
		_ = s.fpvRecords.Close()
	}
	return s.app.Shutdown()
}
