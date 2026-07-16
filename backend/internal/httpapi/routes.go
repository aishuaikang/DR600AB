package httpapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// routes 安装全局中间件，并按版本分组注册 API 路由。
func (s *Server) routes() {
	s.app.Use(recover.New())
	s.app.Use(logger.New())
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: strings.Join(s.cfg.CORSAllowedOrigins, ","),
		AllowHeaders: "Origin, Content-Type, Accept, X-Locale, X-Developer-Token",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
	}))

	s.app.Get("/healthz", s.handleHealth)

	api := s.app.Group("/api/v1")
	s.registerMetaRoutes(api)
	s.registerUserRoutes(api)
	s.registerDeveloperRoutes(api)
	s.registerLicenseRoutes(api)
	s.registerDetectionRecoveryRoutes(api)

	protected := api.Group("", s.requireLicense)
	s.registerGPSReadRoutes(protected)
	s.registerDetectionSessionRoutes(protected)
	s.registerDetectionRecordRoutes(protected)
	s.registerIntrusionRoutes(protected)
	s.registerDeceptionReportRoutes(protected)
	s.registerInterferenceReportRoutes(protected)
	s.registerFPVVideoRecordRoutes(protected)
	s.registerScreenRoutes(protected)
	s.registerInterferenceRoutes(protected)
	s.registerDeceptionRoutes(protected)
	s.registerCompassRoutes(protected)
	s.registerNetworkRoutes(protected)

	s.registerOfflineMapRoutes()
	s.registerFrontendRoutes()
}

// handleHealth 返回进程存活状态，供本地检查使用。
func (s *Server) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"ok":   true,
		"time": time.Now(),
	})
}
